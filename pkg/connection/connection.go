package connection

import (
	"context"
	"encoding/base64"
	"fmt"
	"strings"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/eks"
	"github.com/aws/aws-sdk-go-v2/service/sts"
	aws_v1 "github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/credentials"
	"github.com/aws/aws-sdk-go/aws/session"
	"sigs.k8s.io/aws-iam-authenticator/pkg/token"
)

// EKSClusterConnectionInfo contains the information needed to connect to an EKS
// cluster.
type EKSClusterConnectionInfo struct {
	ClusterName     string
	APIEndpoint     string
	CACertificate   string
	Token           string
	TokenExpiration time.Time
}

// Get retrieves connection info for a given EKS cluster by name.
func (c *EKSClusterConnectionInfo) Get(awsConfig *aws.Config) error {
	svc := eks.NewFromConfig(*awsConfig)

	// get EKS cluster info
	describeClusterinput := &eks.DescribeClusterInput{
		Name: aws.String(c.ClusterName),
	}
	eksCluster, err := svc.DescribeCluster(context.Background(), describeClusterinput)
	if err != nil {
		return fmt.Errorf("failed to describe EKS cluster: %w", err)
	}

	// construct a config object for the earlier v1 version of AWS SDK
	creds, err := awsConfig.Credentials.Retrieve(context.Background())
	if err != nil {
		return fmt.Errorf("failed to retrieve credentials from AWS config: %w", err)
	}
	v1Config := aws_v1.Config{
		Region: aws_v1.String(awsConfig.Region),
		Credentials: credentials.NewStaticCredentials(
			creds.AccessKeyID,
			creds.SecretAccessKey,
			creds.SessionToken,
		),
	}

	// create a new session using the v1 SDK which is used by
	// sigs.k8s.io/aws-iam-authenticator/pkg/token to get a token
	sessionOpts := session.Options{
		Config:            v1Config,
		SharedConfigState: session.SharedConfigEnable,
	}
	awsSession, err := session.NewSessionWithOptions(sessionOpts)
	if err != nil {
		return fmt.Errorf("failed to create new AWS session for generating EKS cluster token: %w", err)
	}

	// get EKS cluster token
	gen, err := token.NewGenerator(true, false)
	if err != nil {
		return fmt.Errorf("failed to generate new token: %w", err)
	}
	opts := &token.GetTokenOptions{
		ClusterID: c.ClusterName,
		Session:   awsSession,
	}

	tkn, err := gen.GetWithOptions(opts)
	if err != nil {
		return fmt.Errorf("failed to get token with options: %w", err)
	}
	ca, err := base64.StdEncoding.DecodeString(*eksCluster.Cluster.CertificateAuthority.Data)
	if err != nil {
		return fmt.Errorf("failed to decode CA data: %w", err)
	}

	// update EKSClusterConnectionInfo object
	c.APIEndpoint = *eksCluster.Cluster.Endpoint
	c.CACertificate = string(ca)
	c.Token = tkn.Token
	c.TokenExpiration = tkn.Expiration.UTC()

	return nil
}

// extractRoleARN extracts the role ARN from an IAM role if it is found.
func extractRoleARN(awsConfig *aws.Config) (string, error) {

	svcSts := sts.NewFromConfig(*awsConfig)

	callerIdentity, err := svcSts.GetCallerIdentity(
		context.Background(),
		&sts.GetCallerIdentityInput{},
	)
	if err != nil {
		return "", fmt.Errorf("failed to get caller identity: %w", err)
	}

	// split the input string by '/'
	parts := strings.Split(*callerIdentity.Arn, "/")

	// ensure there are at least two parts (assumed-role and the role name)
	if len(parts) >= 2 {
		// Join the parts up to the second-to-last one using '/' as the separator
		return strings.Join(parts[:len(parts)-1], "/"), nil
	}

	// return empty string if no ARN is found
	return "", nil
}
