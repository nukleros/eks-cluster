package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/smithy-go"
)

// CreateElasticIPs allocates elastic IP addresses for use by NAT gateways.
func (c *ResourceClient) CreateElasticIPs(
	tags *[]types.Tag,
	publicSubnetIDs []string,
) ([]string, error) {
	svc := ec2.NewFromConfig(c.AWSConfig)

	var elasticIPIDs []string

	for _, _ = range publicSubnetIDs {
		allocateAddressInput := ec2.AllocateAddressInput{
			Domain: types.DomainTypeVpc,
			TagSpecifications: []types.TagSpecification{
				{
					ResourceType: types.ResourceTypeElasticIp,
					Tags:         *tags,
				},
			},
		}
		resp, err := svc.AllocateAddress(c.Context, &allocateAddressInput)
		if err != nil {
			return elasticIPIDs, fmt.Errorf("failed to create elastic IP: %w", err)
		}
		elasticIPIDs = append(elasticIPIDs, *resp.AllocationId)
	}

	return elasticIPIDs, nil
}

// DeleteElasticIPs releases elastic IP addresses.  If no IDs are supplied, or
// if the address IDs are not found it exits without error.
func (c *ResourceClient) DeleteElasticIPs(elasticIPIDs []string) error {
	svc := ec2.NewFromConfig(c.AWSConfig)

	// if elasticIPIDs are empty, there's nothing to delete
	if len(elasticIPIDs) == 0 {
		return nil
	}

	for _, elasticIPID := range elasticIPIDs {
		deleteElasticIPInput := ec2.ReleaseAddressInput{AllocationId: &elasticIPID}
		_, err := svc.ReleaseAddress(c.Context, &deleteElasticIPInput)
		if err != nil {
			var ae smithy.APIError
			if errors.As(err, &ae) {
				if ae.ErrorCode() == "InvalidAllocationID.NotFound" {
					// attempting to delete a elastic IP that doesn't exist so return
					// without error
					return nil
				} else {
					return fmt.Errorf("failed to delete elastic IP with ID %s: %w", elasticIPID, err)
				}
			} else {
				return fmt.Errorf("failed to delete elastic IP with ID %s: %w", elasticIPID, err)
			}
		}
	}

	return nil
}
