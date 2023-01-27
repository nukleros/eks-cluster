package resource

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

type NATGatewayCondition string

const (
	NATGatewayConditionCreated = "NATGatewayCreated"
	NATGatewayConditionDeleted = "NATGatewayDeleted"
	NATGatewayCheckInterval    = 15 //check cluster status every 15 seconds
	NATGatewayCheckMaxCount    = 20 // check 20 times before giving up (5 minutes)
)

// CreateNATGateways creates a NAT gateway for each private subnet so that it
// may reach the public internet.
func (c *ResourceClient) CreateNATGateways(
	tags *[]types.Tag,
	availabilityZones []AvailabilityZone,
	elasticIPIDs []string,
) error {
	svc := ec2.NewFromConfig(c.AWSConfig)

	for i, az := range availabilityZones {
		eip := elasticIPIDs[i]
		createNATGatewayInput := ec2.CreateNatGatewayInput{
			SubnetId:     &az.PublicSubnetID,
			AllocationId: &eip,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeNatgateway,
					Tags:         *tags,
				},
			},
		}
		_, err := svc.CreateNatGateway(c.Context, &createNATGatewayInput)
		if err != nil {
			return fmt.Errorf("failed to create NAT gateway in subnet with ID %s: %w", az.PublicSubnetID, err)
		}

	}

	return nil
}

// DeleteNATGateways deletes all the NAT gateways for the VPC in which the EKS
// cluster was provisioned.  If an empty VPC ID is supplied, or if there are no
// NAT gateways found in the VPC it returns without error.
func (c *ResourceClient) DeleteNATGateways(vpcID string) error {
	svc := ec2.NewFromConfig(c.AWSConfig)

	// if no VPC ID, there's nothing to delete
	if vpcID == "" {
		return nil
	}

	_, natGatewayIDs, err := c.getNATGatewayStatuses(vpcID, nil)
	if err != nil {
		return fmt.Errorf("failed to get NAT gateway statuses for VPC with ID %s during deletion: %w", vpcID, err)
	}

	for _, natGatewayID := range natGatewayIDs {
		deleteNATGatewayInput := ec2.DeleteNatGatewayInput{NatGatewayId: &natGatewayID}
		_, err := svc.DeleteNatGateway(c.Context, &deleteNATGatewayInput)
		if err != nil {
			var ae smithy.APIError
			if errors.As(err, &ae) {
				if ae.ErrorCode() == "InvalidNATGatewayID.NotFound" {
					// attempting to delete a NAT gateway that doesn't exist so return
					// without error
					return nil
				} else {
					return fmt.Errorf("failed to delete NAT gateway with ID %s: %w", natGatewayID, err)
				}
			} else {
				return fmt.Errorf("failed to delete NAT gateway with ID %s: %w", natGatewayID, err)
			}
		}
	}

	return nil
}

// WaitForNATGateways waits for a NAT gateway to reach a given condition.  One of:
// * NATGatewayConditionCreated
// * NATGatewayConditionDeleted
func (c *ResourceClient) WaitForNATGateways(
	vpcID string,
	availabilityZones *[]AvailabilityZone,
	natGatewayCondition NATGatewayCondition,
) error {
	natGatewayCheckCount := 0

	for {
		natGatewayCheckCount += 1
		if natGatewayCheckCount > NATGatewayCheckMaxCount {
			return errors.New("NAT gateway condition check timed out")
		}

		natGatewayStates, _, err := c.getNATGatewayStatuses(vpcID, availabilityZones)
		if err != nil {
			return fmt.Errorf("failed to get NAT gateway statuses for VPC with ID %s: %w", vpcID, err)
		}

		if len(*natGatewayStates) == 0 && natGatewayCondition == NATGatewayConditionDeleted {
			// no NAT gateway resources found for this VPC while waiting for
			// deletion so condition is met
			break
		}

		allConditionsMet := true
		for _, state := range *natGatewayStates {
			if state != types.NatGatewayStateAvailable && natGatewayCondition == NATGatewayConditionCreated {
				// resource is not available but we're waiting for it to be
				// created so condition is not met
				allConditionsMet = false
				break
			} else if state != types.NatGatewayStateDeleted && natGatewayCondition == NATGatewayConditionDeleted {
				// resource is not in deleted state but we're waiting for it to
				// be deleted so condition is not met
				allConditionsMet = false
				break
			}
		}

		if allConditionsMet {
			break
		}

		time.Sleep(time.Second * 15)
	}

	return nil
}

// getNATGatewayStatuses returns the state of each NAT gateway for a given VPC
// as well as the public subnet with which the NAT gateway is associated.  This
// mapping is stored in the AvailabilityZone field of the ResourceConfig (during
// resource creation) or in a map[string]string (for deletion).
func (c *ResourceClient) getNATGatewayStatuses(
	vpcID string,
	availabilityZones *[]AvailabilityZone,
) (*[]types.NatGatewayState, map[string]string, error) {
	svc := ec2.NewFromConfig(c.AWSConfig)

	var natGatewayStates []types.NatGatewayState
	subnetMap := make(map[string]string)

	filterName := "vpc-id"
	describeNATGatewaysInput := ec2.DescribeNatGatewaysInput{
		Filter: []types.Filter{
			{
				Name:   &filterName,
				Values: []string{vpcID},
			},
		},
	}
	resp, err := svc.DescribeNatGateways(c.Context, &describeNATGatewaysInput)
	if err != nil {
		return nil, subnetMap, fmt.Errorf("failed to describe NAT gateways for VPC with ID %s: %w", vpcID, err)
	}

	// map the NAT gateways to the public subnets in which they reside and put
	// it in a map[availabiltiy- zone]public-subnet-id (used for deletions) and
	// in the []AvailabilityZone slice (for creations) where applicable
	for _, natGateway := range resp.NatGateways {
		natGatewayStates = append(natGatewayStates, natGateway.State)
		if natGateway.SubnetId != nil && natGateway.NatGatewayId != nil {
			subnetMap[*natGateway.SubnetId] = *natGateway.NatGatewayId
			if availabilityZones != nil {
				azs := *availabilityZones
				subnetMapped := false
				for i, az := range azs {
					if *natGateway.SubnetId == az.PublicSubnetID {
						azs[i].NATGatewayID = *natGateway.NatGatewayId
						availabilityZones = &azs
						subnetMapped = true
						break
					}
				}
				if !subnetMapped {
					return nil, subnetMap, errors.New(fmt.Sprintf(
						"failed to map NAT gateway with ID %s in subnet with ID %s to an availability zone",
						*natGateway.NatGatewayId, *natGateway.SubnetId))
				}
			}
		}
	}

	return &natGatewayStates, subnetMap, nil
}
