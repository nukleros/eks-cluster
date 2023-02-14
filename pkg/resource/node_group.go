package resource

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

type NodeGroupCondition string

const (
	NodeGroupConditionCreated       = "NodeGroupCreated"
	NodeGroupConditionDeleted       = "NodeGroupDeleted"
	NodeGroupCheckInterval          = 15 //check cluster status every 15 seconds
	NodeGroupCheckMaxCount          = 60 // check 60 times before giving up (15 minutes)
	NodeGroupInitialSize      int32 = 2
)

// CreateNodeGroups creates a private node group for an EKS cluster.
func (c *ResourceClient) CreateNodeGroups(
	tags *map[string]string,
	clusterName string,
	kubernetesVersion string,
	nodeRoleARN string,
	privateSubnetIDs []string,
	instanceTypes []string,
	minNodes int32,
	maxNodes int32,
	keyPair string,
) (*[]types.Nodegroup, error) {
	svc := eks.NewFromConfig(*c.AWSConfig)

	var nodeGroups []types.Nodegroup

	privateNodeGroupName := fmt.Sprintf("%s-private-node-group", clusterName)
	nodeGroupInitialSize := NodeGroupInitialSize
	var createPrivateNodeGroupInput eks.CreateNodegroupInput
	if keyPair != "" {
		remoteAccessConfig := types.RemoteAccessConfig{
			Ec2SshKey: &keyPair,
		}
		createPrivateNodeGroupInput = eks.CreateNodegroupInput{
			ClusterName:   &clusterName,
			NodeRole:      &nodeRoleARN,
			NodegroupName: &privateNodeGroupName,
			Subnets:       privateSubnetIDs,
			InstanceTypes: instanceTypes,
			Version:       &kubernetesVersion,
			RemoteAccess:  &remoteAccessConfig,
			Tags:          *tags,
			ScalingConfig: &types.NodegroupScalingConfig{
				DesiredSize: &nodeGroupInitialSize,
				MaxSize:     &maxNodes,
				MinSize:     &minNodes,
			},
		}
	} else {
		createPrivateNodeGroupInput = eks.CreateNodegroupInput{
			ClusterName:   &clusterName,
			NodeRole:      &nodeRoleARN,
			NodegroupName: &privateNodeGroupName,
			Subnets:       privateSubnetIDs,
			InstanceTypes: instanceTypes,
			Version:       &kubernetesVersion,
			Tags:          *tags,
		}
	}
	privateNodeGroupResp, err := svc.CreateNodegroup(c.Context, &createPrivateNodeGroupInput)
	if err != nil {
		return &nodeGroups, fmt.Errorf("failed to create node group %s: %w", privateNodeGroupName, err)
	}
	nodeGroups = append(nodeGroups, *privateNodeGroupResp.Nodegroup)

	return &nodeGroups, nil
}

// DeleteNodeGroups deletes the EKS cluster node groups.  If an empty cluster
// name or node group name is supplied, or if it does not find a node group
// matching the given name it returns without error.
func (c *ResourceClient) DeleteNodeGroups(clusterName string, nodeGroupNames []string) error {
	// if clusterName or nodeGroupName are empty, there's nothing to delete
	if clusterName == "" || len(nodeGroupNames) == 0 {
		return nil
	}

	svc := eks.NewFromConfig(*c.AWSConfig)

	for _, nodeGroupName := range nodeGroupNames {
		deleteNodeGroupInput := eks.DeleteNodegroupInput{
			ClusterName:   &clusterName,
			NodegroupName: &nodeGroupName,
		}
		_, err := svc.DeleteNodegroup(c.Context, &deleteNodeGroupInput)
		if err != nil {
			var notFoundErr *types.ResourceNotFoundException
			if errors.As(err, &notFoundErr) {
				return nil
			} else {
				return fmt.Errorf("failed to delete node group %s: %w", nodeGroupName, err)
			}
		}
	}

	return nil
}

// WaitForNodeGroups waits for the provided node groups to reach a given
// condigion.  One of:
// * NodeGroupConditionCreated
// * NodeGroupConditionDeleted
func (c *ResourceClient) WaitForNodeGroups(
	clusterName string,
	nodeGroupNames []string,
	nodeGroupCondition NodeGroupCondition,
) error {
	// if no nodeGroups, there's nothing to check
	if len(nodeGroupNames) == 0 {
		return nil
	}

	nodeGroupCheckCount := 0
	for {
		nodeGroupCheckCount += 1
		if nodeGroupCheckCount > NodeGroupCheckMaxCount {
			return errors.New("node group condition check timed out")
		}

		allConditionsMet := true
		for _, nodeGroupName := range nodeGroupNames {
			nodeGroupStatus, err := c.getNodeGroupStatus(clusterName, nodeGroupName)
			if err != nil {
				if errors.Is(err, ErrResourceNotFound) && nodeGroupCondition == NodeGroupConditionDeleted {
					// resource was not found and we're waiting for it to be
					// deleted so condition is met
					continue
				} else {
					return fmt.Errorf("failed to get node group status while waiting for %s: %w", nodeGroupName, err)
				}
			}

			if *nodeGroupStatus == types.NodegroupStatusActive && nodeGroupCondition == NodeGroupConditionCreated {
				// resource is available and we're waiting for it to be created
				// so condition is met
				continue
			}
			allConditionsMet = false
			break
		}

		if allConditionsMet {
			break
		}
		time.Sleep(time.Second * 15)
	}

	return nil
}

// getNodeGroupStatus retrieves the status of a node group.
func (c *ResourceClient) getNodeGroupStatus(clusterName, nodeGroupName string) (*types.NodegroupStatus, error) {
	svc := eks.NewFromConfig(*c.AWSConfig)

	describeNodeGroupInput := eks.DescribeNodegroupInput{
		ClusterName:   &clusterName,
		NodegroupName: &nodeGroupName,
	}
	resp, err := svc.DescribeNodegroup(c.Context, &describeNodeGroupInput)
	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return nil, ErrResourceNotFound
		} else {
			return nil, fmt.Errorf("failed to describe node group %s: %w", nodeGroupName, err)
		}
	}

	return &resp.Nodegroup.Status, nil
}
