package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
)

const DefaultKubernetesVersion = "1.26"

// ResourceConfig contains the configuration options for an EKS cluster.
type ResourceConfig struct {
	Name                             string                           `yaml:"name"`
	Region                           string                           `yaml:"region"`
	AWSAccountID                     string                           `yaml:"awsAccountID"`
	KubernetesVersion                string                           `yaml:"kubernetesVersion"`
	ClusterCIDR                      string                           `yaml:"clusterCIDR"`
	AvailabilityZones                []AvailabilityZone               `yaml:"availabilityZones"`
	InstanceTypes                    []string                         `yaml:"instanceTypes"`
	InitialNodes                     int32                            `yaml:"initialNodes"`
	MinNodes                         int32                            `yaml:"minNodes"`
	MaxNodes                         int32                            `yaml:"maxNodes"`
	DNSManagement                    bool                             `yaml:"dnsManagement"`
	DNSManagementServiceAccount      DNSManagementServiceAccount      `yaml:"dnsManagementServiceAccount"`
	StorageManagementServiceAccount  StorageManagementServiceAccount  `yaml:"storageManagementServiceAccount"`
	ClusterAutoscaling               bool                             `yaml:"clusterAutoscaling"`
	ClusterAutoscalingServiceAccount ClusterAutoscalingServiceAccount `yaml:"clusterAutoscalingServiceAccount"`
	KeyPair                          string                           `yaml:"keyPair"`
	Tags                             map[string]string                `yaml:"tags"`
}

// AvailabilityZone contains configuration options for an EKS cluster
// networking.  It also contains resource ID fields used internally during
// creation.
type AvailabilityZone struct {
	Zone              string `yaml:"zone"`
	PrivateSubnetCIDR string `yaml:"privateSubnetCIDR"`
	PrivateSubnetID   string
	PublicSubnetCIDR  string `yaml:"publicSubnetCIDR"`
	PublicSubnetID    string
	NATGatewayID      string
}

// DNSManagementServiceAccount contains the name and namespace for the
// Kubernetes service account that needs access to manage Route53 DNS records.
type DNSManagementServiceAccount struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// ClusterAutoscalingServiceAccount contains the name and namespace for the
// Kubernetes service account that needs access to manage autoscaling groups.
type ClusterAutoscalingServiceAccount struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// StorageManagementServiceAccount contains the name and namespace for the
// Kubernetes service account that needs access to manage storage provisioning.
type StorageManagementServiceAccount struct {
	Name      string `yaml:"name"`
	Namespace string `yaml:"namespace"`
}

// NewResourceConfig returns a ResourceConfig with default values set.
func NewResourceConfig() *ResourceConfig {
	return &ResourceConfig{
		Name:              "eks-cluster",
		KubernetesVersion: DefaultKubernetesVersion,
		ClusterCIDR:       "10.0.0.0/16",
		InstanceTypes:     []string{"t2.micro"},
		MinNodes:          int32(2),
		MaxNodes:          int32(4),
	}
}

// LoadAWSConfig loads the AWS config from environment or shared config profile
// and overrides the default region if provided.
func LoadAWSConfig(configEnv bool, configProfile, region string) (*aws.Config, error) {
	awsConfig, err := config.LoadDefaultConfig(context.Background(),
		config.WithRegion(region),
		config.WithSharedConfigProfile(configProfile),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}
	return &awsConfig, err
}

func (r *ResourceConfig) SetAvailabilityZones(resourceClient *ResourceClient) error {
	// ensure region is in resource config
	if r.Region == "" {
		return errors.New("region is not set in resource config")
	}

	// if availability zones provided, nothing to do
	if len(r.AvailabilityZones) > 0 {
		return nil
	}

	// otherwise set based on number of desired availability zones
	availabilityZones, err := resourceClient.GetAvailabilityZonesForRegion(r.Region)
	if err != nil {
		return fmt.Errorf(
			fmt.Sprintf("failed to get availability zones for region %s", r.Region),
			err,
		)
	}
	r.AvailabilityZones = *availabilityZones

	return nil
}
