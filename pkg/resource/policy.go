package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// CreatePolicy creates the IAM policy to be used for managing Route53 DNS
// records.
func (c *ResourceClient) CreatePolicy(tags *[]types.Tag) (*types.Policy, error) {
	svc := iam.NewFromConfig(c.AWSConfig)

	dnsPolicyName := "DNSUpdates"
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

// DeletePolicy deletes the DNS management IAM policy.  If the policy is not
// found it returns without error.
func (c *ResourceClient) DeletePolicy(policyARN string) error {
	svc := iam.NewFromConfig(c.AWSConfig)

	// if roleARN is empty, there's nothing to delete
	if policyARN == "" {
		return nil
	}

	deletePolicyInput := iam.DeletePolicyInput{
		PolicyArn: &policyARN,
	}
	_, err := svc.DeletePolicy(c.Context, &deletePolicyInput)
	if err != nil {
		var noSuchEntityErr *types.NoSuchEntityException
		if errors.As(err, &noSuchEntityErr) {
			return nil
		} else {
			return fmt.Errorf("failed to delete policy %s: %w", policyARN, err)
		}
	}

	return nil
}
