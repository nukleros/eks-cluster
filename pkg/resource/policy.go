package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

const (
	DNSPolicyName            = "DNSUpdates"
	DNS01ChallengePolicyName = "DNS01Challenge"
	AutoscalingPolicyName    = "ClusterAutoscaler"
)

// CreateDNSManagementPolicy creates the IAM policy to be used for managing
// Route53 DNS records.
func (c *ResourceClient) CreateDNSManagementPolicy(tags *[]types.Tag, clusterName string) (*types.Policy, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	dnsPolicyName := fmt.Sprintf("%s-%s", DNSPolicyName, clusterName)
	dnsPolicyDescription := "Allow cluster services to update Route53 records"
	dnsPolicyDocument := `{
"Version": "2012-10-17",
"Statement": [
{
  "Effect": "Allow",
  "Action": [
	"route53:ChangeResourceRecordSets"
  ],
  "Resource": [
	"arn:aws:route53:::hostedzone/*"
  ]
},
{
  "Effect": "Allow",
  "Action": [
	"route53:ListHostedZones",
	"route53:ListResourceRecordSets"
  ],
  "Resource": [
	"*"
  ]
}
]
}`
	createR53PolicyInput := iam.CreatePolicyInput{
		PolicyName:     &dnsPolicyName,
		Description:    &dnsPolicyDescription,
		PolicyDocument: &dnsPolicyDocument,
	}
	r53PolicyResp, err := svc.CreatePolicy(c.Context, &createR53PolicyInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS management policy %s: %w", dnsPolicyName, err)
	}

	return r53PolicyResp.Policy, nil
}

// CreateDNS01ChallengePolicy creates the IAM policy to be used for completing
// DNS01 challenges.
func (c *ResourceClient) CreateDNS01ChallengePolicy(tags *[]types.Tag, clusterName string) (*types.Policy, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	dnsPolicyName := fmt.Sprintf("%s-%s", DNS01ChallengePolicyName, clusterName)
	dnsPolicyDescription := "Allow cluster services to complete DNS01 challenges"
	dnsPolicyDocument := `{
"Version": "2012-10-17",
"Statement": [
{
  "Effect": "Allow",
  "Action": [
	"route53:ChangeResourceRecordSets",
  ],
  "Resource": [
	"arn:aws:route53:::hostedzone/*"
  ]
},
{
  "Effect": "Allow",
  "Action": [
	"route53:GetChange",
	"route53:ListHostedZones",
	"route53:ListResourceRecordSets",
	"route53:ListHostedZonesByName"
],
  "Resource": [
	"*"
  ]
}
]
}`
	createR53PolicyInput := iam.CreatePolicyInput{
		PolicyName:     &dnsPolicyName,
		Description:    &dnsPolicyDescription,
		PolicyDocument: &dnsPolicyDocument,
	}
	r53PolicyResp, err := svc.CreatePolicy(c.Context, &createR53PolicyInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create DNS01 challenge policy %s: %w", dnsPolicyName, err)
	}

	return r53PolicyResp.Policy, nil
}

// CreateClusterAutoscalingPolicy creates the IAM policy to be used for cluster
// autoscaling to manage node pool sizes.
func (c *ResourceClient) CreateClusterAutoscalingPolicy(
	tags *[]types.Tag,
	clusterName string,
) (*types.Policy, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	autoscalingPolicyName := fmt.Sprintf("%s-%s", AutoscalingPolicyName, clusterName)
	autoscalingPolicyDescription := "Allow cluster autoscaler to manage node pool sizes"
	autoscalingPolicyDocument := fmt.Sprintf(`{
    "Version": "2012-10-17",
    "Statement": [
        {
            "Effect": "Allow",
            "Action": [
                "autoscaling:SetDesiredCapacity",
                "autoscaling:TerminateInstanceInAutoScalingGroup"
            ],
            "Resource": "*",
            "Condition": {
                "StringEquals": {
                    "aws:ResourceTag/k8s.io/cluster-autoscaler/%s": "owned"
                }
            }
        },
        {
            "Effect": "Allow",
            "Action": [
                "autoscaling:DescribeAutoScalingInstances",
                "autoscaling:DescribeAutoScalingGroups",
                "ec2:DescribeLaunchTemplateVersions",
                "autoscaling:DescribeTags",
                "autoscaling:DescribeLaunchConfigurations",
                "ec2:DescribeInstanceTypes"
            ],
            "Resource": "*"
        }
    ]
}`, clusterName)
	createAutoscalingPolicyInput := iam.CreatePolicyInput{
		PolicyName:     &autoscalingPolicyName,
		Description:    &autoscalingPolicyDescription,
		PolicyDocument: &autoscalingPolicyDocument,
	}
	autoscalingPolicyResp, err := svc.CreatePolicy(c.Context, &createAutoscalingPolicyInput)
	if err != nil {
		return nil, fmt.Errorf("failed to create cluster autoscaler management policy %s: %w", autoscalingPolicyName, err)
	}

	return autoscalingPolicyResp.Policy, nil
}

// DeletePolicies deletes the IAM policies.  If the policyARNs slice is empty it
// returns without error.
func (c *ResourceClient) DeletePolicies(policyARNs []string) error {
	// if roleARN is empty, there's nothing to delete
	if len(policyARNs) == 0 {
		return nil
	}

	for _, policyARN := range policyARNs {
		svc := iam.NewFromConfig(*c.AWSConfig)

		deletePolicyInput := iam.DeletePolicyInput{
			PolicyArn: &policyARN,
		}
		_, err := svc.DeletePolicy(c.Context, &deletePolicyInput)
		if err != nil {
			var noSuchEntityErr *types.NoSuchEntityException
			if errors.As(err, &noSuchEntityErr) {
				continue
			} else {
				return fmt.Errorf("failed to delete policy %s: %w", policyARN, err)
			}
		}
	}

	return nil
}
