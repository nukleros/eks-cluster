package resource

import (
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const (
	ClusterRoleName            = "cluster-role"
	WorkerRoleName             = "workler-role"
	DNSManagementRoleName      = "dns-mgmt-role"
	DNS01ChallengeRoleName     = "dns-chlg-role"
	ClusterAutoscalingRoleName = "ca-role"
	StorageManagementRoleName  = "csi-role"
	ClusterPolicyARN           = "arn:aws:iam::aws:policy/AmazonEKSClusterPolicy"
	WorkerNodePolicyARN        = "arn:aws:iam::aws:policy/AmazonEKSWorkerNodePolicy"
	ContainerRegistryPolicyARN = "arn:aws:iam::aws:policy/AmazonEC2ContainerRegistryReadOnly"
	CNIPolicyARN               = "arn:aws:iam::aws:policy/AmazonEKS_CNI_Policy"
	CSIDriverPolicyARN         = "arn:aws:iam::aws:policy/service-role/AmazonEBSCSIDriverPolicy"
)

// CreateRoles creates the IAM roles needed for EKS clusters and node groups.
func (c *ResourceClient) CreateRoles(tags *[]types.Tag, clusterName string) (*types.Role, *types.Role, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	clusterRoleName := fmt.Sprintf("%s-%s", ClusterRoleName, clusterName)
	if err := checkRoleName(clusterRoleName); err != nil {
		return nil, nil, err
	}
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

	workerRoleName := fmt.Sprintf("%s-%s", WorkerRoleName, clusterName)
	if err := checkRoleName(workerRoleName); err != nil {
		return nil, nil, err
	}
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
	clusterName string,
) (*types.Role, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	oidcProviderBare := strings.Trim(oidcProvider, "https://")
	dnsManagementRoleName := fmt.Sprintf("%s-%s", DNSManagementRoleName, clusterName)
	if err := checkRoleName(dnsManagementRoleName); err != nil {
		return nil, err
	}
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

// CreateDNS01ChallengeRole creates the IAM role needed for DNS01 challenges by
// the Kubernetes service account of an in-cluster supporting service such as
// cert-manager using IRSA (IAM role for service accounts).
func (c *ResourceClient) CreateDNS01ChallengeRole(
	tags *[]types.Tag,
	dnsPolicyARN string,
	awsAccountID string,
	oidcProvider string,
	serviceAccount *DNS01ChallengeServiceAccount,
	clusterName string,
) (*types.Role, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	oidcProviderBare := strings.Trim(oidcProvider, "https://")
	dns01ChallengeRoleName := fmt.Sprintf("%s-%s", DNS01ChallengeRoleName, clusterName)
	if err := checkRoleName(dns01ChallengeRoleName); err != nil {
		return nil, err
	}
	dns01ChallengeRolePolicyDocument := fmt.Sprintf(`{
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
	createdDNS01ChallengeRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &dns01ChallengeRolePolicyDocument,
		RoleName:                 &dns01ChallengeRoleName,
		PermissionsBoundary:      &dnsPolicyARN,
		Tags:                     *tags,
	}
	dns01ChallengeRoleResp, err := svc.CreateRole(c.Context, &createdDNS01ChallengeRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create role %s: %w", dns01ChallengeRoleName, err)
	}

	attachDNSManagementRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: &dnsPolicyARN,
		RoleName:  dns01ChallengeRoleResp.Role.RoleName,
	}
	_, err = svc.AttachRolePolicy(c.Context, &attachDNSManagementRolePolicyInput)
	if err != nil {
		return dns01ChallengeRoleResp.Role, fmt.Errorf("failed to attach role policy %s to %s: %w", dnsPolicyARN, dns01ChallengeRoleName, err)
	}

	return dns01ChallengeRoleResp.Role, nil
}

// CreateClusterAutoscalingRole creates the IAM role needed for cluster
// autoscaler to manage node pool sizes using IRSA (IAM role for service
// accounts).
func (c *ResourceClient) CreateClusterAutoscalingRole(
	tags *[]types.Tag,
	autoscalingPolicyARN string,
	awsAccountID string,
	oidcProvider string,
	serviceAccount *ClusterAutoscalingServiceAccount,
	clusterName string,
) (*types.Role, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	oidcProviderBare := strings.Trim(oidcProvider, "https://")
	clusterAutoscalingRoleName := fmt.Sprintf("%s-%s", ClusterAutoscalingRoleName, clusterName)
	if err := checkRoleName(clusterAutoscalingRoleName); err != nil {
		return nil, err
	}
	clusterAutoscalingRolePolicyDocument := fmt.Sprintf(`{
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
	createClusterAutoscalingRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &clusterAutoscalingRolePolicyDocument,
		RoleName:                 &clusterAutoscalingRoleName,
		PermissionsBoundary:      &autoscalingPolicyARN,
		Tags:                     *tags,
	}
	clusterAutoscalingRoleResp, err := svc.CreateRole(c.Context, &createClusterAutoscalingRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create role %s: %w", clusterAutoscalingRoleName, err)
	}

	attachClusterAutoscalingRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: &autoscalingPolicyARN,
		RoleName:  clusterAutoscalingRoleResp.Role.RoleName,
	}
	_, err = svc.AttachRolePolicy(c.Context, &attachClusterAutoscalingRolePolicyInput)
	if err != nil {
		return clusterAutoscalingRoleResp.Role, fmt.Errorf("failed to attach role policy %s to %s: %w", autoscalingPolicyARN, clusterAutoscalingRoleName, err)
	}

	return clusterAutoscalingRoleResp.Role, nil
}

// CreateStorageManagementRole creates the IAM role needed for storage
// management by the CSI driver's service account using IRSA (IAM role for
// service accounts).
func (c *ResourceClient) CreateStorageManagementRole(
	tags *[]types.Tag,
	awsAccountID string,
	oidcProvider string,
	serviceAccount *StorageManagementServiceAccount,
	clusterName string,
) (*types.Role, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	oidcProviderBare := strings.Trim(oidcProvider, "https://")
	storageManagementRoleName := fmt.Sprintf("%s-%s", StorageManagementRoleName, clusterName)
	if err := checkRoleName(storageManagementRoleName); err != nil {
		return nil, err
	}
	storagePolicyARN := CSIDriverPolicyARN
	storageManagementRolePolicyDocument := fmt.Sprintf(`{
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
	createStorageManagementRoleInput := iam.CreateRoleInput{
		AssumeRolePolicyDocument: &storageManagementRolePolicyDocument,
		RoleName:                 &storageManagementRoleName,
		PermissionsBoundary:      &storagePolicyARN,
		Tags:                     *tags,
	}
	storageManagementRoleResp, err := svc.CreateRole(c.Context, &createStorageManagementRoleInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create role %s: %w", storageManagementRoleName, err)
	}

	attachStorageManagementRolePolicyInput := iam.AttachRolePolicyInput{
		PolicyArn: &storagePolicyARN,
		RoleName:  storageManagementRoleResp.Role.RoleName,
	}
	_, err = svc.AttachRolePolicy(c.Context, &attachStorageManagementRolePolicyInput)
	if err != nil {
		return storageManagementRoleResp.Role, fmt.Errorf("failed to attach role policy %s to %s: %w", storagePolicyARN, storageManagementRoleName, err)
	}

	return storageManagementRoleResp.Role, nil
}

// DeleteRoles deletes the IAM roles used by EKS.  If empty role names are
// provided, or if the roles are not found it returns without error.
func (c *ResourceClient) DeleteRoles(roles *[]RoleInventory) error {
	// if roles are empty, there's nothing to delete
	if len(*roles) == 0 {
		return nil
	}

	svc := iam.NewFromConfig(*c.AWSConfig)

	for _, role := range *roles {
		if role.RoleName == "" {
			// role is empty - skip
			continue
		}
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

// checkRoleName ensures role names do not exceed the AWS limit for role name
// lengths (64 characters).
func checkRoleName(name string) error {
	if utf8.RuneCountInString(name) > 64 {
		return errors.New(fmt.Sprintf(
			"role name %s too long, must be 64 characters or less", name,
		))
	}

	return nil
}
