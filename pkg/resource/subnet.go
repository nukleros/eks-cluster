package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// CreateSubnets creates the subnets used by an EKS clusters.  It creates a
// public and private subnet for each availability zone being used by the
// cluster.  It also tags each subnet so the load balancers may be correctly
// applied to them.
func (c *ResourceClient) CreateSubnets(
	tags *[]types.Tag,
	vpcID string,
	clusterName string,
	availabilityZones *[]AvailabilityZone,
) (*[]types.Subnet, *[]types.Subnet, error) {
	svc := ec2.NewFromConfig(c.AWSConfig)

	var privateSubnets []types.Subnet
	var publicSubnets []types.Subnet

	internalELBTagKey := "kubernetes.io/role/internal-elb"
	internalELBTagValue := "1"
	internalELBTag := types.Tag{
		Key:   &internalELBTagKey,
		Value: &internalELBTagValue,
	}
	privateTags := append(*tags, internalELBTag)

	elbTagKey := "kubernetes.io/role/elb"
	elbTagValue := "1"
	elbTag := types.Tag{
		Key:   &elbTagKey,
		Value: &elbTagValue,
	}
	publicTags := append(*tags, elbTag)

	azs := *availabilityZones
	for i, az := range azs {
		privateCreateSubnetInput := ec2.CreateSubnetInput{
			VpcId:            &vpcID,
			AvailabilityZone: &az.Zone,
			CidrBlock:        &az.PrivateSubnetCIDR,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSubnet,
					Tags:         privateTags,
				},
			},
		}
		privateResp, err := svc.CreateSubnet(c.Context, &privateCreateSubnetInput)
		if err != nil {
			return nil, nil, fmt.Errorf("failed to create private subnet for VPC with ID %s: %w", vpcID, err)
		}
		azs[i].PrivateSubnetID = *privateResp.Subnet.SubnetId
		privateSubnets = append(privateSubnets, *privateResp.Subnet)

		publicCreateSubnetInput := ec2.CreateSubnetInput{
			VpcId:            &vpcID,
			AvailabilityZone: &az.Zone,
			CidrBlock:        &az.PublicSubnetCIDR,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeSubnet,
					Tags:         publicTags,
				},
			},
		}
		publicResp, err := svc.CreateSubnet(c.Context, &publicCreateSubnetInput)
		if err != nil {
			return &privateSubnets, nil, fmt.Errorf("failed to create public subnet for VPC with ID %s: %w", vpcID, err)
		}
		azs[i].PublicSubnetID = *publicResp.Subnet.SubnetId
		publicSubnets = append(publicSubnets, *publicResp.Subnet)

		mapPublicIP := true
		modifySubnetAttributeInput := ec2.ModifySubnetAttributeInput{
			SubnetId:            publicResp.Subnet.SubnetId,
			MapPublicIpOnLaunch: &types.AttributeBooleanValue{Value: &mapPublicIP},
		}
		_, err = svc.ModifySubnetAttribute(c.Context, &modifySubnetAttributeInput)
		if err != nil {
			return &privateSubnets, &publicSubnets, fmt.Errorf("failed to modify subnet attribute for subnet with ID %s: %w",
				*publicResp.Subnet.SubnetId, err)
		}
	}
	availabilityZones = &azs

	return &privateSubnets, &publicSubnets, nil
}

// DeleteSubnets deletes the subnets used by the EKS cluster.  If no subnet IDs
// are supplied, or if the subnets are not found it returns without error.
func (c *ResourceClient) DeleteSubnets(subnetIDs []string) error {
	svc := ec2.NewFromConfig(c.AWSConfig)

	// if there are no subnet IDs there is nothing to do
	if len(subnetIDs) == 0 {
		return nil
	}

	for _, id := range subnetIDs {
		deleteSubnetInput := ec2.DeleteSubnetInput{SubnetId: &id}
		_, err := svc.DeleteSubnet(c.Context, &deleteSubnetInput)
		if err != nil {
			var ae smithy.APIError
			if errors.As(err, &ae) {
				if ae.ErrorCode() == "InvalidSubnetID.NotFound" {
					// attempting to delete a subnet that doesn't exist so return
					// without error
					return nil
				} else {
					return fmt.Errorf("failed to delete subnet with ID %s: %w", id, err)
				}
			} else {
				return fmt.Errorf("failed to delete subnet with ID %s: %w", id, err)
			}
		}
	}

	return nil
}
