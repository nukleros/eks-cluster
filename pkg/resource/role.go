package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const (
	ClusterRoleName            = "eks-cluster-role"
	WorkerRoleName             = "eks-cluster-workler-role"
	ClusterPolicyARN           = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
	WorkerNodePolicyARN        = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
	ContainerRegistryPolicyARN = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
	CNIPolicyARN               = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
)

// CreateRoles creates the IAM roles needed for EKS clusters and node groups.
func (c *ResourceClient) CreateRoles(tags *[]types.Tag) (*types.Role, *types.Role, error) {
	svc := iam.NewFromConfig(c.AWSConfig)

	clusterRoleName := ClusterRoleName
	clusterPolicyARN := ClusterPolicyARN
	clusterRolePolicyDocument := `{
  "Version": "2012-10-17",
  "Statement": [
	  {
		  "Effect": "Allow",
		  "Principal": {
			  "Service": [
				  "eks.amazonaws.com"
			  ]
		  },
		  "Action": "sts:AssumeRole"
	  }
  ]
}`
	createClusterRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &clusterRolePolicyDocument,
		RoleName:                 &clusterRoleName,
		PermissionsBoundary:      &clusterPolicyARN,
		Tags:                     *tags,
	}
	clusterRoleResp, err := svc.CreateRole(c.Context, &createClusterRoleInput)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to create role %s: %w", clusterRoleName, err)
	}

	attachClusterRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: &clusterPolicyARN,
		RoleName:  clusterRoleResp.Role.RoleName,
	}
	_, err = svc.AttachRolePolicy(c.Context, &attachClusterRolePolicyInput)
	if err != nil {
		return clusterRoleResp.Role, nil, fmt.Errorf("failed to attach role policy %s to %s: %w", clusterPolicyARN, clusterRoleName, err)
	}

	workerRoleName := WorkerRoleName
	workerRolePolicyDocument := `{
  "Version": "2012-10-17",
  "Statement": [
  	{
  		"Effect": "Allow",
  		"Principal": {
  			"Service": [
  				"ec2.amazonaws.com"
  			]
  		},
  		"Action": [
  			"sts:AssumeRole"
  		]
  	}
  ]
}`
	createWorkerRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &workerRolePolicyDocument,
		RoleName:                 &workerRoleName,
	}
	workerRoleResp, err := svc.CreateRole(c.Context, &createWorkerRoleInput)
	if err != nil {
		return clusterRoleResp.Role, nil, fmt.Errorf("failed to create role %s: %w", workerRoleName, err)
	}

	for _, policyARN := range getWorkerPolicyARNs() {
		attachRolePolicyInput := iam.AttachRolePolicyInput{
			PolicyArn: &policyARN,
			RoleName:  workerRoleResp.Role.RoleName,
		}
		_, err = svc.AttachRolePolicy(c.Context, &attachRolePolicyInput)
		if err != nil {
			return clusterRoleResp.Role, workerRoleResp.Role, fmt.Errorf("failed to attach role policy %s to %s: %w", policyARN, workerRoleName, err)
		}
	}

	return clusterRoleResp.Role, workerRoleResp.Role, nil
}

// DeleteRoles deletes the IAM roles used by EKS.  If empty role names are
// provided, or if the roles are not found it returns without error.
func (c *ResourceClient) DeleteRoles(clusterRole, workerRole *RoleInventory) error {
	svc := iam.NewFromConfig(c.AWSConfig)

	if clusterRole.RoleName != "" {
		for _, policyARN := range clusterRole.RolePolicyARNs {
			clusterDetachRolePolicyInput := iam.DetachRolePolicyInput{
				PolicyArn: &policyARN,
				RoleName:  &clusterRole.RoleName,
			}
			_, err := svc.DetachRolePolicy(c.Context, &clusterDetachRolePolicyInput)
			if err != nil {
				var noSuchEntityErr *types.NoSuchEntityException
				if errors.As(err, &noSuchEntityErr) {
					return nil
				} else {
					return fmt.Errorf("failed to detach policy %s from role %s: %w", policyARN, clusterRole.RoleName, err)
				}
			}
		}
		deleteClusterRoleInput := iam.DeleteRoleInput{RoleName: &clusterRole.RoleName}
		_, err := svc.DeleteRole(c.Context, &deleteClusterRoleInput)
		if err != nil {
			var noSuchEntityErr *types.NoSuchEntityException
			if errors.As(err, &noSuchEntityErr) {
				return nil
			} else {
				return fmt.Errorf("failed to delete role %s: %w", clusterRole.RoleName, err)
			}
		}
	}

	if workerRole.RoleName != "" {
		for _, policyARN := range workerRole.RolePolicyARNs {
			workerDetachRolePolicyInput := iam.DetachRolePolicyInput{
				PolicyArn: &policyARN,
				RoleName:  &workerRole.RoleName,
			}
			_, err := svc.DetachRolePolicy(c.Context, &workerDetachRolePolicyInput)
			if err != nil {
				var noSuchEntityErr *types.NoSuchEntityException
				if errors.As(err, &noSuchEntityErr) {
					return nil
				} else {
					return fmt.Errorf("failed to detach policy %s from role %s: %w", policyARN, workerRole.RoleName, err)
				}
			}
		}
		deleteWorkerRoleInput := iam.DeleteRoleInput{RoleName: &workerRole.RoleName}
		_, err := svc.DeleteRole(c.Context, &deleteWorkerRoleInput)
		if err != nil {
			var noSuchEntityErr *types.NoSuchEntityException
			if errors.As(err, &noSuchEntityErr) {
				return nil
			} else {
				return fmt.Errorf("failed to delete role %s: %w", workerRole.RoleName, err)
			}
		}
	}

	return nil
}

// getWorkerPolicyARNs returns the IAM policy ARNs needed for clusters and node
// groups.
func getWorkerPolicyARNs() []string {
	return []string{
		WorkerNodePolicyARN,
		ContainerRegistryPolicyARN,
		CNIPolicyARN,
	}
}
