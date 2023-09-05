package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

// GetClusterSecurityGroup retrieves the security group created for the EKS
// cluster by AWS during provisioning.
func (c *ResourceClient) GetClusterSecurityGroup(clusterName string) (string, error) {
	svc := ec2.NewFromConfig(*c.AWSConfig)

	filterName := fmt.Sprintf("tag:aws:eks:cluster-name")
	filters := []types.Filter{
		{
			Name:   &filterName,
			Values: []string{clusterName},
		},
	}
	describeSecurityGroupsInput := ec2.DescribeSecurityGroupsInput{
		Filters: filters,
	}
	resp, err := svc.DescribeSecurityGroups(c.Context, &describeSecurityGroupsInput)
	if err != nil {
		return "", fmt.Errorf("failed to describe security groups filtered by cluster name %s", clusterName)
	}

	if len(resp.SecurityGroups) == 0 {
		return "", errors.New(fmt.Sprintf("found zero security groups filtered by cluster name %s", clusterName))
	}
	if len(resp.SecurityGroups) > 1 {
		return "", errors.New(fmt.Sprintf("found multiple security groups filtered by cluster name %s", clusterName))
	}

	return *resp.SecurityGroups[0].GroupId, nil
}
