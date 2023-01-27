package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// CreateRouteTables creates the route tables for the subnets used by the EKS
// cluster.  A single route table is shared by all the public subnets, however a
// separate route table is needed for each private subnet because they each get
// a route to a different NAT gateway.
func (c *ResourceClient) CreateRouteTables(
	tags *[]types.Tag,
	vpcID string,
	internetGatewayID string,
	availabilityZones *[]AvailabilityZone,
) (*[]types.RouteTable, *types.RouteTable, error) {
	svc := ec2.NewFromConfig(c.AWSConfig)

	destinationCIDR := "0.0.0.0/0"

	var privateRouteTables []types.RouteTable
	var publicRouteTable types.RouteTable

	// create a single route table for public subnets
	createPublicRouteTableInput := ec2.CreateRouteTableInput{
		VpcId: &vpcID,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeRouteTable,
				Tags:         *tags,
			},
		},
	}
	publicResp, err := svc.CreateRouteTable(c.Context, &createPublicRouteTableInput)
	if err != nil {
		return &privateRouteTables, &publicRouteTable, fmt.Errorf("failed to create public route table for VPC ID %s: %w", vpcID, err)
	}
	publicRouteTable = *publicResp.RouteTable

	// add route to internet gateway for public subnets' route table
	createRouteInput := ec2.CreateRouteInput{
		RouteTableId:         publicRouteTable.RouteTableId,
		GatewayId:            &internetGatewayID,
		DestinationCidrBlock: &destinationCIDR,
	}
	_, err = svc.CreateRoute(c.Context, &createRouteInput)
	if err != nil {
		return &privateRouteTables, &publicRouteTable, fmt.Errorf(
			"failed to create route to internet gateway with ID %s for route table with ID %s: %w",
			internetGatewayID, *publicRouteTable.RouteTableId, err)
	}

	// create a route table for each private subnet
	for _, az := range *availabilityZones {
		createPrivateRouteTableInput := ec2.CreateRouteTableInput{
			VpcId: &vpcID,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeRouteTable,
					Tags:         *tags,
				},
			},
		}
		privateResp, err := svc.CreateRouteTable(c.Context, &createPrivateRouteTableInput)
		if err != nil {
			return &privateRouteTables, &publicRouteTable, fmt.Errorf("failed to create private route table for VPC ID %s: %w", vpcID, err)
		}
		privateRouteTables = append(privateRouteTables, *privateResp.RouteTable)

		// associate the private route table with the private subnet for this
		// availability zone
		associatePrivateRouteTableInput := ec2.AssociateRouteTableInput{
			RouteTableId: privateResp.RouteTable.RouteTableId,
			SubnetId:     &az.PrivateSubnetID,
		}
		_, err = svc.AssociateRouteTable(c.Context, &associatePrivateRouteTableInput)
		if err != nil {
			return &privateRouteTables, &publicRouteTable, fmt.Errorf(
				"failed to associate private route table with ID %s to subnet with ID %s: %w",
				privateResp.RouteTable.RouteTableId, az.PrivateSubnetID, err)
		}

		// add a route to the NAT gateway for the private subnet
		createRouteInput := ec2.CreateRouteInput{
			RouteTableId:         privateResp.RouteTable.RouteTableId,
			NatGatewayId:         &az.NATGatewayID,
			DestinationCidrBlock: &destinationCIDR,
		}
		_, err = svc.CreateRoute(c.Context, &createRouteInput)
		if err != nil {
			return &privateRouteTables, &publicRouteTable, fmt.Errorf(
				"failed to create route to NAT gateway with ID %s for route table with ID %s: %w",
				az.NATGatewayID, *privateResp.RouteTable.RouteTableId, err)
		}

		// associate the public route table with the public subnet for this
		// availability zone
		associatePublicRouteTableInput := ec2.AssociateRouteTableInput{
			RouteTableId: publicRouteTable.RouteTableId,
			SubnetId:     &az.PublicSubnetID,
		}
		_, err = svc.AssociateRouteTable(c.Context, &associatePublicRouteTableInput)
		if err != nil {
			return &privateRouteTables, &publicRouteTable, fmt.Errorf(
				"failed to associate public route table with ID %s to subnet with ID %s: %w",
				publicRouteTable.RouteTableId, az.PublicSubnetID, err)
		}
	}

	return &privateRouteTables, &publicRouteTable, nil
}

// DeleteRouteTables deletes the route tables for the public and private subnets
// that are used by EKS.
func (c *ResourceClient) DeleteRouteTables(privateRouteTableIDs []string, publicRouteTable string) error {
	svc := ec2.NewFromConfig(c.AWSConfig)

	var allRouteTableIDs []string
	switch {
	case len(privateRouteTableIDs) == 0 && publicRouteTable == "":
		// if route table IDs are empty, there's nothing to delete
		return nil
	case publicRouteTable == "":
		// don't want to add an empty string to slice of route table IDs
		allRouteTableIDs = privateRouteTableIDs
	default:
		// there are private and public route table IDs to delete
		allRouteTableIDs = append(privateRouteTableIDs, publicRouteTable)
	}

	for _, routeTableID := range allRouteTableIDs {
		deleteRouteTableInput := ec2.DeleteRouteTableInput{RouteTableId: &routeTableID}
		_, err := svc.DeleteRouteTable(c.Context, &deleteRouteTableInput)
		if err != nil {
			var ae smithy.APIError
			if errors.As(err, &ae) {
				if ae.ErrorCode() == "InvalidRouteTableID.NotFound" {
					// attempting to delete a route table that doesn't exist so return
					// without error
					return nil
				} else {
					return fmt.Errorf("failed to delete route table with ID %s: %w", routeTableID, err)
				}
			} else {
				return fmt.Errorf("failed to delete route table with ID %s: %w", routeTableID, err)
			}
		}
	}

	return nil
}
