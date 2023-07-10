package resource

import (
	"context"

	"github.com/aws/aws-sdk-go-v2/aws"
)

// ResourceClient contains the elements needed to manage resources with this
// package.
type ResourceClient struct {
	// A channel for messages to be passed to client as resources are created
	// and deleted.
	MessageChan *chan string

	// A channel for latest version of resource inventory to be passed to client
	// as resources are created and deleted.
	InventoryChan *chan ResourceInventory

	// A context object available for passing data across operations.
	Context context.Context

	// The AWS configuration for default settings and credentials.
	AWSConfig *aws.Config
}

// CreateResourceClient configures a resource client and returns it.
func CreateResourceClient(awsConfig *aws.Config) *ResourceClient {
	msgChan := make(chan string)
	invChan := make(chan ResourceInventory)
	ctx := context.Background()
	resourceClient := ResourceClient{&msgChan, &invChan, ctx, awsConfig}

	return &resourceClient
}
