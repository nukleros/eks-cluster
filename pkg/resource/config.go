package resource

import (
	"context"
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
		Region:            "us-east-2",
		ClusterCIDR:       "10.0.0.0/16",
		AvailabilityZones: []AvailabilityZone{
			{
				Zone:              "us-east-2a",
				PrivateSubnetCIDR: "10.0.0.0/22",
				PublicSubnetCIDR:  "10.0.4.0/22",
			}, {
				Zone:              "us-east-2b",
				PrivateSubnetCIDR: "10.0.8.0/22",
				PublicSubnetCIDR:  "10.0.12.0/22",
			}, {
				Zone:              "us-east-2b",
				PrivateSubnetCIDR: "10.0.16.0/22",
				PublicSubnetCIDR:  "10.0.20.0/22",
			},
		},
		InstanceTypes: []string{"t2.micro"},
		MinNodes:      int32(2),
		MaxNodes:      int32(4),
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
