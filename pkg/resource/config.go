package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/credentials/stscreds"
	"github.com/aws/aws-sdk-go-v2/service/sts"
)

const DefaultKubernetesVersion = "1.26"

// ResourceConfig contains the configuration options for an EKS cluster.
type ResourceConfig struct {
	Name                             string                           `yaml:"name"`
	Region                           string                           `yaml:"region"`
	AWSAccountID                     string                           `yaml:"awsAccountID"`
	KubernetesVersion                string                           `yaml:"kubernetesVersion"`
	ClusterCIDR                      string                           `yaml:"clusterCIDR"`
	DesiredAZCount                   int32                            `yaml:"desiredAZCount"`
	AvailabilityZones                []AvailabilityZone               `yaml:"availabilityZones"`
	InstanceTypes                    []string                         `yaml:"instanceTypes"`
	InitialNodes                     int32                            `yaml:"initialNodes"`
	MinNodes                         int32                            `yaml:"minNodes"`
	MaxNodes                         int32                            `yaml:"maxNodes"`
	DNSManagement                    bool                             `yaml:"dnsManagement"`
	DNS01Challenge                   bool                             `yaml:"dns01Challenge"`
	DNSManagementServiceAccount      DNSManagementServiceAccount      `yaml:"dnsManagementServiceAccount"`
	DNS01ChallengeServiceAccount     DNS01ChallengeServiceAccount     `yaml:"dns01ChallengeServiceAccount"`
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

// DNS01ChallengeServiceAccount contains the name and namespace for the
// Kubernetes service account that needs access to perform Route53 DNS01
// challenges.
type DNS01ChallengeServiceAccount struct {
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
func LoadAWSConfig(
	configEnv bool,
	configProfile,
	region,
	roleArn,
	externalId,
	serialNumber string,
) (*aws.Config, error) {
	configOptions := []func(*config.LoadOptions) error{
		config.WithSharedConfigProfile(configProfile),
		config.WithRegion(region),
		config.WithAssumeRoleCredentialOptions(
			func(o *stscreds.AssumeRoleOptions) {
				o.TokenProvider = stscreds.StdinTokenProvider
				o.ExternalID = aws.String(externalId)
			}),
	}

	// load config from filesystem
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		configOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config: %w", err)
	}

	// if serialNumber is provided, request MFA token and get temporary credentials
	if serialNumber != "" {
		stsClient := sts.NewFromConfig(awsConfig)

		// get MFA token from user
		tokenCode, err := stscreds.StdinTokenProvider()
		if err != nil {
			return nil, fmt.Errorf("failed to get token code: %w", err)
		}

		// generate temporary credentials
		sessionToken, err := stsClient.GetSessionToken(
			context.Background(),
			&sts.GetSessionTokenInput{
				SerialNumber: &serialNumber,
				TokenCode:    &tokenCode,
			},
		)
		if err != nil {
			return nil, fmt.Errorf("failed to get session token: %w", err)
		}

		// update configOptions with session token
		configOptions = append(
			configOptions,
			config.WithCredentialsProvider(
				credentials.NewStaticCredentialsProvider(
					*sessionToken.Credentials.AccessKeyId,
					*sessionToken.Credentials.SecretAccessKey,
					*sessionToken.Credentials.SessionToken,
				),
			))

		// update aws config with session token
		awsConfig, err = config.LoadDefaultConfig(
			context.Background(),
			configOptions...,
		)
		if err != nil {
			return nil, fmt.Errorf("failed to load AWS config: %w", err)
		}
	}

	// assume role if roleArn is provided
	if roleArn != "" {
		awsConfig, err = assumeRole(roleArn, externalId, awsConfig, configOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to assume role: %w", err)
		}
		return &awsConfig, err
	}

	return &awsConfig, err
}

// LoadAWSConfigFromAPIKeys returns an AWS config from static API keys and
// overrides the default region if provided.  The token parameter can be an
// empty string.
func LoadAWSConfigFromAPIKeys(
	accessKeyID,
	secretAccessKey,
	token,
	region,
	roleArn,
	externalId string,
) (*aws.Config, error) {
	configOptions := []func(*config.LoadOptions) error{
		config.WithRegion(region),
		config.WithCredentialsProvider(
			credentials.NewStaticCredentialsProvider(
				accessKeyID,
				secretAccessKey,
				token,
			),
		),
	}

	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		configOptions...,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config from static API keys: %w", err)
	}

	// assume role if roleArn is provided
	if roleArn != "" {
		awsConfig, err = assumeRole(roleArn, externalId, awsConfig, configOptions)
		if err != nil {
			return nil, fmt.Errorf("failed to assume role: %w", err)
		}
		return &awsConfig, err
	}

	return &awsConfig, err
}

// assumeRole returns an AWS config with temporary credentials
// from an assumed role.
func assumeRole(
	roleArn,
	externalId string,
	awsConfig aws.Config,
	configOptions []func(*config.LoadOptions) error,
) (aws.Config, error) {
	// create assume role provider
	assumeRoleProvider := stscreds.NewAssumeRoleProvider(
		sts.NewFromConfig(awsConfig),
		roleArn,
		func(o *stscreds.AssumeRoleOptions) {
			o.ExternalID = aws.String(externalId)
		})

	// update configOptions with assume role provider
	configOptions = append(
		configOptions,
		config.WithCredentialsProvider(assumeRoleProvider),
	)

	// load config with assume role provider
	awsConfig, err := config.LoadDefaultConfig(
		context.Background(),
		configOptions...,
	)
	if err != nil {
		return aws.Config{}, fmt.Errorf("failed to load AWS config: %w", err)
	}

	return awsConfig, err
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

	// set no. availability zones - default to 1 if not specified
	var desiredAZs int32
	if r.DesiredAZCount == 0 {
		desiredAZs = 2
	} else {
		desiredAZs = r.DesiredAZCount
	}

	// otherwise set based on number of desired availability zones
	availabilityZones, err := resourceClient.GetAvailabilityZonesForRegion(r.Region, desiredAZs)
	if err != nil {
		return fmt.Errorf(
			fmt.Sprintf("failed to get availability zones for region %s", r.Region),
			err,
		)
	}
	r.AvailabilityZones = *availabilityZones

	return nil
}
