package api

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/nukleros/eks-cluster/pkg/resource"
	"gopkg.in/yaml.v2"
)

var (
	configFile string
)

func Create(awsConfigEnv bool, awsConfigProfile, inventoryFile string) error {
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

	// create resource client - region is not passed since it can be set in
	// the config file if needed
	awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, "")
	if err != nil {
		return err
	}
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()
	resourceClient := resource.ResourceClient{&msgChan, ctx, awsConfig}

	// Create a channel to receive OS signals
	sigs := make(chan os.Signal, 1)

	// Register the channel to receive SIGINT signals
	signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)

	// Run a goroutine to handle the signal. It will block until it receives a signal
	go func() {
		<-sigs
		fmt.Println("\nReceived Ctrl+C, cleaning up resources...")
		if err = resourceClient.DeleteResourceStack(inventoryFile); err != nil {
			fmt.Errorf("\nError deleting resources: %s", err)
			os.Exit(1)
		}
		os.Exit(0)
	}()

	fmt.Println("Running... Press Ctrl+C to exit")

	// create resources
	fmt.Println("Creating resources for EKS cluster...")
	err = resourceClient.CreateResourceStack(inventoryFile, resourceConfig)
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

func Delete(awsConfigEnv bool, awsConfigProfile, inventoryFile string) error {
	// load inventory
	var resourceInventory resource.ResourceInventory
	if inventoryFile != "" {
		inventoryJSON, err := ioutil.ReadFile(inventoryFile)
		if err != nil {
			return err
		}
		json.Unmarshal(inventoryJSON, &resourceInventory)
	}

	// create resource client
	awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, "")
	if err != nil {
		return err
	}
	msgChan := make(chan string)
	go outputMessages(&msgChan)
	ctx := context.Background()
	resourceClient := resource.ResourceClient{&msgChan, ctx, awsConfig}

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

// outputMessages prints the output messages from the resource client to the
// terminal for the CLI user.
func outputMessages(msgChan *chan string) {
	for {
		msg := <-*msgChan
		fmt.Println(msg)
	}
}
