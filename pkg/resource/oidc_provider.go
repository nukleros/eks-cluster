package resource

import (
	"bytes"
	"crypto/sha1"
	"crypto/tls"
	"errors"
	"fmt"
	"net/url"
	"strings"

	"github.com/aws/aws-sdk-go-v2/service/iam"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// CreateOIDCProvider creates a new identity provider in IAM for the EKS cluster.
// This enables IAM roles for Kubernetes service accounts (IRSA).
func (c *ResourceClient) CreateOIDCProvider(
	tags *[]types.Tag,
	providerURL string,
) (string, error) {
	svc := iam.NewFromConfig(*c.AWSConfig)

	var oidcProviderARN string
	// get the OIDC provider server certificate thumbprint
	u, err := url.Parse(providerURL)
	if err != nil {
		return oidcProviderARN, fmt.Errorf("failed to parse OIDC provider URL: %w", err)
	}
	conn, err := tls.Dial("tcp", fmt.Sprintf("%s:%d", u.Hostname(), 443), &tls.Config{})
	if err != nil {
		return oidcProviderARN, fmt.Errorf("failed to connect to OIDC provider: %w", err)
	}
	cert := conn.ConnectionState().PeerCertificates[len(conn.ConnectionState().PeerCertificates)-1]
	thumbprint := sha1.Sum(cert.Raw)
	var thumbprintString string
	for _, t := range thumbprint {
		var buf bytes.Buffer
		fmt.Fprintf(&buf, "%02X", t)
		thumbprintString = thumbprintString + strings.ToLower(buf.String())
	}

	createOIDCProviderInput := iam.CreateOpenIDConnectProviderInput{
		ClientIDList:   []string{"sts.amazonaws.com"},
		ThumbprintList: []string{thumbprintString},
		Url:            &providerURL,
	}
	resp, err := svc.CreateOpenIDConnectProvider(c.Context, &createOIDCProviderInput)
	if err != nil {
		return oidcProviderARN, fmt.Errorf("failed to create IAM identity provider: %w", err)
	}
	oidcProviderARN = *resp.OpenIDConnectProviderArn

	return oidcProviderARN, nil
}

// DeleteOIDCProvider deletes an OIDC identity cluster in IAM.  If  an empty ARN
// is provided or if not found it returns without error.
func (c *ResourceClient) DeleteOIDCProvider(oidcProviderARN string) error {
	// if clusterName is empty, there's nothing to delete
	if oidcProviderARN == "" {
		return nil
	}

	svc := iam.NewFromConfig(*c.AWSConfig)

	deleteOIDCProviderInput := iam.DeleteOpenIDConnectProviderInput{
		OpenIDConnectProviderArn: &oidcProviderARN,
	}
	_, err := svc.DeleteOpenIDConnectProvider(c.Context, &deleteOIDCProviderInput)
	if err != nil {
		var noSuchEntityErr *types.NoSuchEntityException
		if errors.As(err, &noSuchEntityErr) {
			return nil
		} else {
			return fmt.Errorf("failed to delete IAM identity provider %s: %w", oidcProviderARN, err)
		}
	}

	return nil
}
