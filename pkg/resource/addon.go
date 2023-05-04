package resource

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

const EBSStorageAddonName = "aws-ebs-csi-driver"

// CreateEBSStorageAddon creates installs the EBS CSI driver addon on the EKS
// cluster.
func (c *ResourceClient) CreateEBSStorageAddon(
	tags *map[string]string,
	clusterName string,
	storageManagementRoleARN string,
) (*types.Addon, error) {
	svc := eks.NewFromConfig(*c.AWSConfig)

	ebsAddonName := EBSStorageAddonName

	createEBSAddonInput := eks.CreateAddonInput{
		AddonName:             &ebsAddonName,
		ClusterName:           &clusterName,
		ServiceAccountRoleArn: &storageManagementRoleARN,
		Tags:                  *tags,
	}
	resp, err := svc.CreateAddon(c.Context, &createEBSAddonInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	return resp.Addon, nil
}
