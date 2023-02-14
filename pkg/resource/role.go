package resource

import (
	"errors"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const (
	ClusterRoleName            = "eks-cluster-role"
	WorkerRoleName             = "eks-cluster-workler-role"
	DNSManagementRoleName      = "eks-cluster-dns-management-role"
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

// CreateDNSManagementRole creates the IAM role needed for DNS management by
// the Kubernetes service account of an in-cluster supporting service such as
// external-dns using IRSA (IAM role for service accounts).
func (c *ResourceClient) CreateDNSManagementRole(
	tags *[]types.Tag,
	dnsPolicyARN string,
	awsAccountID string,
	oidcProvider string,
	serviceAccount *DNSManagementServiceAccount,
) (*types.Role, error) {
	svc := iam.NewFromConfig(c.AWSConfig)

	oidcProviderBare := strings.Trim(oidcProvider, "https://")
	dnsManagementRoleName := DNSManagementRoleName
	dnsManagementRolePolicyDocument := fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Principal": {
                "Federated": "arn:aws:iam::%[1]s:oidc-provider/%[2]s"
            },
            "Action": "sts:AssumeRoleWithWebIdentity",
            "Condition": {
                "StringEquals": {
                    "%[2]s:sub": "system:serviceaccount:%[3]s:%[4]s",
                    "%[2]s:aud": "sts.amazonaws.com"
                }
            }
        }
    ]
}`, awsAccountID, oidcProviderBare, serviceAccount.Namespace, serviceAccount.Name)
	createDNSManagementRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &dnsManagementRolePolicyDocument,
		RoleName:                 &dnsManagementRoleName,
		PermissionsBoundary:      &dnsPolicyARN,
		Tags:                     *tags,
	}
	dnsManagementRoleResp, err := svc.CreateRole(c.Context, &createDNSManagementRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create role %s: %w", dnsManagementRoleName, err)
	}

	attachDNSManagementRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: &dnsPolicyARN,
		RoleName:  dnsManagementRoleResp.Role.RoleName,
	}
	_, err = svc.AttachRolePolicy(c.Context, &attachDNSManagementRolePolicyInput)
	if err != nil {
		return dnsManagementRoleResp.Role, fmt.Errorf("failed to attach role policy %s to %s: %w", dnsPolicyARN, dnsManagementRoleName, err)
	}

	return dnsManagementRoleResp.Role, nil
}

// DeleteRoles deletes the IAM roles used by EKS.  If empty role names are
// provided, or if the roles are not found it returns without error.
// func (c *ResourceClient) DeleteRoles(clusterRole, workerRole *RoleInventory) error {
func (c *ResourceClient) DeleteRoles(roles *[]RoleInventory) error {
	svc := iam.NewFromConfig(c.AWSConfig)

	// if roles are empty, there's nothing to delete
	if len(*roles) == 0 {
		return nil
	}

	for _, role := range *roles {

		for _, policyARN := range role.RolePolicyARNs {
			detachRolePolicyInput := iam.DetachRolePolicyInput{
				PolicyArn: &policyARN,
				RoleName:  &role.RoleName,
			}
			_, err := svc.DetachRolePolicy(c.Context, &detachRolePolicyInput)
			if err != nil {
				var noSuchEntityErr *types.NoSuchEntityException
				if errors.As(err, &noSuchEntityErr) {
					return nil
				} else {
					return fmt.Errorf("failed to detach policy %s from role %s: %w", policyARN, role.RoleName, err)
				}
			}
		}
		deleteRoleInput := iam.DeleteRoleInput{RoleName: &role.RoleName}
		_, err := svc.DeleteRole(c.Context, &deleteRoleInput)
		if err != nil {
			var noSuchEntityErr *types.NoSuchEntityException
			if errors.As(err, &noSuchEntityErr) {
				return nil
			} else {
				return fmt.Errorf("failed to delete role %s: %w", role.RoleName, err)
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
