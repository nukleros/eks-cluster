package resource

import (
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/ec2/types"
)

const maxAZCount = int32(3)

// defaultCIDRs returns a set of CIDR blocks for subnets.
func defaultCIDRs() []string {
	return []string{
		"10.0.0.0/22",
		"10.0.4.0/22",
		"10.0.8.0/22",
		"10.0.12.0/22",
		"10.0.16.0/22",
		"10.0.20.0/22",
	}
}

// GetAvailabilityZonesForRegion gets the availability zones for a given region.
func (c *ResourceClient) GetAvailabilityZonesForRegion(region string, desiredAZs int32) (*[]AvailabilityZone, error) {
	svc := ec2.NewFromConfig(*c.AWSConfig)
	var availabilityZones []AvailabilityZone
	defaultCIDRs := defaultCIDRs()

	filterName := "region-name"
	describeAZInput := ec2.DescribeAvailabilityZonesInput{
		Filters: []types.Filter{
			{
				Name:   &filterName,
				Values: []string{region},
			},
		},
	}
	resp, err := svc.DescribeAvailabilityZones(c.Context, &describeAZInput)
	if err != nil {
		return &availabilityZones, fmt.Errorf("failed to describe availability zones for region %s: %w", region, err)
	}

	azsSet := int32(0)
	var azCount int32
	if desiredAZs > maxAZCount {
		azCount = maxAZCount
	} else {
		azCount = desiredAZs
	}
	cidrIndex := 0
	for _, az := range resp.AvailabilityZones {
		if azsSet < azCount {
			newAZ := AvailabilityZone{
				Zone:              *az.ZoneName,
				PrivateSubnetCIDR: defaultCIDRs[cidrIndex],
				PublicSubnetCIDR:  defaultCIDRs[cidrIndex+1],
			}
			availabilityZones = append(availabilityZones, newAZ)
			cidrIndex = cidrIndex + 2
			azsSet += 1
		} else {
			break
		}
	}

	return &availabilityZones, nil
}
