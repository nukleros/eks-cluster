package resource

import (
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	iamtypes "github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// CreateEC2Tags creates tags for EC2 resources.
func CreateEC2Tags(name string, tags map[string]string) *[]ec2types.Tag {
	nameKey := "Name"
	ec2Tags := []ec2types.Tag{
		{
			Key:   &nameKey,
			Value: &name,
		},
	}
	for k, v := range tags {
		t := ec2types.Tag{
			Key:   &k,
			Value: &v,
		}
		ec2Tags = append(ec2Tags, t)
	}

	return &ec2Tags
}

// CreateIAMTags creates tags for IAM resources.
func CreateIAMTags(name string, tags map[string]string) *[]iamtypes.Tag {
	nameKey := "Name"
	ec2Tags := []iamtypes.Tag{
		{
			Key:   &nameKey,
			Value: &name,
		},
	}
	for k, v := range tags {
		t := iamtypes.Tag{
			Key:   &k,
			Value: &v,
		}
		ec2Tags = append(ec2Tags, t)
	}

	return &ec2Tags
}

// CreateMapTags creates tags in map[string]string format for AWS services that
// use that format.
func CreateMapTags(name string, tags map[string]string) map[string]string {
	tags["Name"] = name
	return tags
}
