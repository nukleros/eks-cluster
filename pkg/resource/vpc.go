package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// CreateVPC creates a VPC for an EKS cluster.  It adds the necessary tags and
// enables the DNS attributes.
func (c *ResourceClient) CreateVPC(
	tags *[]types.Tag,
	cidrBlock string,
	clusterName string,
) (*types.Vpc, error) {
	svc := ec2.NewFromConfig(*c.AWSConfig)

	clusterNameTagKey := "kubernetes.io/cluster/cluster-name"
	clusterNameTagValue := clusterName
	clusterNameTag := types.Tag{
		Key:   &clusterNameTagKey,
		Value: &clusterNameTagValue,
	}
	*tags = append(*tags, clusterNameTag)

	clusterNameSharedTagKey := fmt.Sprintf("kubernetes.io/cluster/%s", clusterName)
	clusterNameSharedTagValue := "shared"
	clusterNameSharedTag := types.Tag{
		Key:   &clusterNameSharedTagKey,
		Value: &clusterNameSharedTagValue,
	}
	*tags = append(*tags, clusterNameSharedTag)

	createVPCInput := ec2.CreateVpcInput{
		CidrBlock: &cidrBlock,
		TagSpecifications: []types.TagSpecification{
			{
				ResourceType: types.ResourceTypeVpc,
				Tags:         *tags,
			},
		},
	}
	resp, err := svc.CreateVpc(c.Context, &createVPCInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create VPC for cluster %s: %w", clusterName, err)
	}

	valueTrue := true
	attributeTrue := types.AttributeBooleanValue{Value: &valueTrue}
	modifyVPCAttributeDNSHostnamesInput := ec2.ModifyVpcAttributeInput{
		VpcId:              resp.Vpc.VpcId,
		EnableDnsHostnames: &attributeTrue,
	}
	_, err = svc.ModifyVpcAttribute(c.Context, &modifyVPCAttributeDNSHostnamesInput)
	if err != nil {
		return resp.Vpc, fmt.Errorf("failed to modify VPC attribute to enable DNS hostnames for VPC with ID %d: %w",
			resp.Vpc.VpcId, err)
	}
	modifyVPCAttributeDNSSupportInput := ec2.ModifyVpcAttributeInput{
		VpcId:            resp.Vpc.VpcId,
		EnableDnsSupport: &attributeTrue,
	}
	_, err = svc.ModifyVpcAttribute(c.Context, &modifyVPCAttributeDNSSupportInput)
	if err != nil {
		return resp.Vpc, fmt.Errorf("failed to modify VPC attribute to enable DNS support for VPC with ID %d: %w",
			resp.Vpc.VpcId, err)
	}

	return resp.Vpc, nil
}

// DeleteVPC deletes the VPC used by an EKS cluster.  If the VPC ID is empty, or
// if the VPC is not found it returns without error.
func (c *ResourceClient) DeleteVPC(vpcID string) error {
	// if both vpcID is empty, there's nothing to delete
	if vpcID == "" {
		return nil
	}

	svc := ec2.NewFromConfig(*c.AWSConfig)

	deleteVPCInput := ec2.DeleteVpcInput{VpcId: &vpcID}
	_, err := svc.DeleteVpc(c.Context, &deleteVPCInput)
	if err != nil {
		var ae smithy.APIError
		if errors.As(err, &ae) {
			if ae.ErrorCode() == "InvalidVpcID.NotFound" {
				// attempting to delete a VPC that doesn't exist so return
				// without error
				return nil
			} else {
				return fmt.Errorf("failed to delete VPC with ID %s: %w", vpcID, err)
			}
		} else {
			return fmt.Errorf("failed to delete VPC with ID %s: %w", vpcID, err)
		}
	}

	return nil
}
