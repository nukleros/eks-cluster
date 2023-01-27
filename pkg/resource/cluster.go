package resource

import (
	"errors"
	"fmt"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/eks/types"
)

type ClusterCondition string

const (
	ClusterConditionCreated = "ClusterCreated"
	ClusterConditionDeleted = "ClusterDeleted"
	ClusterCheckInterval    = 15 //check cluster status every 15 seconds
	ClusterCheckMaxCount    = 60 // check 60 times before giving up (15 minutes)
)

// CreateCluster creates a new EKS Cluster.
func (c *ResourceClient) CreateCluster(
	tags *map[string]string,
	clusterName string,
	kubernetesVersion string,
	roleARN string,
	subnetIDs []string,
) (*types.Cluster, error) {
	svc := eks.NewFromConfig(c.AWSConfig)

	privateAccess := true
	publicAccess := true
	vpcConfig := types.VpcConfigRequest{
		EndpointPrivateAccess: &privateAccess,
		EndpointPublicAccess:  &publicAccess,
		SubnetIds:             subnetIDs,
	}

	createClusterInput := eks.CreateClusterInput{
		Name:               &clusterName,
		ResourcesVpcConfig: &vpcConfig,
		RoleArn:            &roleARN,
		Version:            &kubernetesVersion,
		Tags:               *tags,
	}
	resp, err := svc.CreateCluster(c.Context, &createClusterInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster: %w", err)
	}

	return resp.Cluster, nil
}

// DeleteCluster deletes an EKS cluster.  If  an empty cluster name is supplied,
// or if the cluster is not found it returns without error.
func (c *ResourceClient) DeleteCluster(clusterName string) error {
	svc := eks.NewFromConfig(c.AWSConfig)

	// if clusterName is empty, there's nothing to delete
	if clusterName == "" {
		return nil
	}

	deleteClusterInput := eks.DeleteClusterInput{Name: &clusterName}
	_, err := svc.DeleteCluster(c.Context, &deleteClusterInput)
	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return nil
		} else {
			return fmt.Errorf("failed to delete cluster: %w", err)
		}
	}

	return nil
}

// WaitForCluster waits until a cluster reaches a certain condition.  One of:
// * ClusterConditionCreated
// * ClusterConditionDeleted
func (c *ResourceClient) WaitForCluster(clusterName string, clusterCondition ClusterCondition) error {
	// if no cluster, there's nothing to check
	if clusterName == "" {
		return nil
	}

	clusterCheckCount := 0
	for {
		clusterCheckCount += 1
		if clusterCheckCount > ClusterCheckMaxCount {
			return errors.New("cluster condition check timed out")
		}

		clusterState, err := c.getClusterStatus(clusterName)
		if err != nil {
			if errors.Is(err, ErrResourceNotFound) && clusterCondition == ClusterConditionDeleted {
				// resource was not found and we're waiting for it to be
				// deleted so condition is met
				break
			} else {
				return fmt.Errorf("failed to get cluster status while waiting for %s: %w", clusterName, err)
			}
		}
		if *clusterState == types.ClusterStatusActive && clusterCondition == ClusterConditionCreated {
			// resource is available and we're waiting for it to be created
			// so condition is met
			break
		}
		time.Sleep(time.Second * 15)
	}

	return nil
}

// getClusterStatus retrieves the cluster status for a given cluster name.
func (c *ResourceClient) getClusterStatus(clusterName string) (*types.ClusterStatus, error) {
	svc := eks.NewFromConfig(c.AWSConfig)

	describeClusterInput := eks.DescribeClusterInput{
		Name: &clusterName,
	}
	resp, err := svc.DescribeCluster(c.Context, &describeClusterInput)
	if err != nil {
		var notFoundErr *types.ResourceNotFoundException
		if errors.As(err, &notFoundErr) {
			return nil, ErrResourceNotFound
		} else {
			return nil, fmt.Errorf("failed to describe cluster %s: %w", clusterName, err)
		}
	}

	return &resp.Cluster.Status, nil
}
