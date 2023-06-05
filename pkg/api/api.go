package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/nukleros/eks-cluster/pkg/resource"
	"gopkg.in/yaml.v2"
)

var (
	configFile string
)

// Create creates an EKS cluster
func Create(resourceClient *resource.ResourceClient, inventoryFile string) error {
	// load config
	resourceConfig := resource.NewResourceConfig()
	if configFile != "" {
		configYAML, err := ioutil.ReadFile(configFile)
		if err != nil {
			return err
		}
		if err := yaml.Unmarshal(configYAML, &resourceConfig); err != nil {
			return err
		}
	}

	// create resources
	fmt.Println("Creating resources for EKS cluster...")
	err := resourceClient.CreateResourceStack(inventoryFile, resourceConfig)
	if err != nil {
		fmt.Println("Problem encountered creating resources - deleting resources that were created: %w", err)
		if deleteErr := resourceClient.DeleteResourceStack(inventoryFile); deleteErr != nil {
			return fmt.Errorf("\nError creating resources: %w\nError deleting resources: %s", err, deleteErr)
		}
		return err
	}

	fmt.Printf("Inventory file '%s' written\n", inventoryFile)

	fmt.Println("EKS cluster created")
	return nil
}

// Delete deletes an EKS cluster
func Delete(resourceClient *resource.ResourceClient, inventoryFile string) error {
	// load inventory
	var resourceInventory resource.ResourceInventory
	if inventoryFile != "" {
		inventoryJSON, err := ioutil.ReadFile(inventoryFile)
		if err != nil {
			return err
		}
		json.Unmarshal(inventoryJSON, &resourceInventory)
	}

	// delete resources
	fmt.Println("Deleting resources for EKS cluster...")
	if err := resourceClient.DeleteResourceStack(inventoryFile); err != nil {
		return err
	}

	// update inventory file
	var emptyResourceInventory resource.ResourceInventory
	emptyInventoryJSON, err := json.MarshalIndent(emptyResourceInventory, "", "  ")
	if err != nil {
		return err
	}
	ioutil.WriteFile(inventoryFile, emptyInventoryJSON, 0644)

	fmt.Println("EKS cluster deleted")
	return nil
}

// CreateResourceClient configures a resource client and returns it
func CreateResourceClient(awsConfigEnv bool, awsConfigProfile string) (*resource.ResourceClient, error) {
	// create resource client - region is not passed since it can be set in
	// the config file if needed
	awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, "")
	if err != nil {
		return nil, err
	}
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()
	resourceClient := resource.ResourceClient{&msgChan, ctx, awsConfig}
	return &resourceClient, nil
}

// outputMessages prints the output messages from the resource client to the
// terminal for the CLI user.
func outputMessages(msgChan *chan string) {
	for {
		msg := <-*msgChan
		fmt.Println(msg)
	}
}
