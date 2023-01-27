package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// CreateInternetGateway creates an internet gateway for the VPC in which an EKS
// cluster is provisioned.
func (c *ResourceClient) CreateInternetGateway(
	tags *[]types.Tag,
	vpcID string,
	clusterName string,
) (*types.InternetGateway, error) {
	svc := ec2.NewFromConfig(c.AWSConfig)

	createIGWInput := ec2.CreateInternetGatewayInput{
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeInternetGateway,
				Tags:         *tags,
			},
		},
	}
	resp, err := svc.CreateInternetGateway(c.Context, &createIGWInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create internet gateway: %w", err)
	}

	attachIGWInput := ec2.AttachInternetGatewayInput{
		InternetGatewayId: resp.InternetGateway.InternetGatewayId,
		VpcId:             &vpcID,
	}
	_, err = svc.AttachInternetGateway(c.Context, &attachIGWInput)
	if err != nil {
		return resp.InternetGateway, fmt.Errorf(
			"failed to attach internet gateway with ID %s to VPC with ID %s: %w",
			*resp.InternetGateway.InternetGatewayId, vpcID, err)
	}

	return resp.InternetGateway, nil
}

// DeleteInternetGateway deletes an internet gateway.  If an empty ID is
// supplied, or if the internet gateway is not found, it returns without error.
func (c *ResourceClient) DeleteInternetGateway(internetGatewayID, vpcID string) error {
	svc := ec2.NewFromConfig(c.AWSConfig)

	// if internetGatewayID is empty, there's nothing to delete
	if internetGatewayID == "" {
		return nil
	}

	detachInternetGatewayInput := ec2.DetachInternetGatewayInput{
		InternetGatewayId: &internetGatewayID,
		VpcId:             &vpcID,
	}
	_, err := svc.DetachInternetGateway(c.Context, &detachInternetGatewayInput)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			if ae.ErrorCode() == "InvalidInternetGatewayID.NotFound" {
				// attempting to detach a internet gateway that doesn't exist so return
				// without error
				return nil
			} else {
				return fmt.Errorf("failed to detach internet gateway with ID %s: %w", internetGatewayID, err)
			}
		} else {
			return fmt.Errorf("failed to detach internet gateway with ID %s: %w", internetGatewayID, err)
		}
	}

	deleteInternetGatewayInput := ec2.DeleteInternetGatewayInput{InternetGatewayId: &internetGatewayID}
	_, err = svc.DeleteInternetGateway(c.Context, &deleteInternetGatewayInput)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			if ae.ErrorCode() == "InvalidInternetGatewayID.NotFound" {
				// attempting to delete a internet gateway that doesn't exist so return
				// without error
				return nil
			} else {
				return fmt.Errorf("failed to delete internet gateway with ID %s: %w", internetGatewayID, err)
			}
		} else {
			return fmt.Errorf("failed to delete internet gateway with ID %s: %w", internetGatewayID, err)
		}
	}

	return nil
}
