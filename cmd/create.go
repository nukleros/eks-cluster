/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"io/ioutil"
	"os"
	"os/signal"
	"syscall"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nukleros/eks-cluster/pkg/resource"
)

var (
	configFile          string
	createInventoryFile string
)

// createCmd represents the create command.
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Provision an EKS cluster in AWS",
	Long:  `Provision an EKS cluster in AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// load AWS config
		awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, "", awsRoleArn)
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}

		// create resource client
		resourceClient := resource.CreateResourceClient(awsConfig)

		// load config resource config
		resourceConfig := resource.NewResourceConfig()
		if configFile != "" {
			configYAML, err := ioutil.ReadFile(configFile)
			if err != nil {
				return fmt.Errorf("failed to load resource config: %w", err)
			}
			if err := yaml.Unmarshal(configYAML, &resourceConfig); err != nil {
				return fmt.Errorf("failed unmarshal yaml from resource config: %w", err)
			}
		}

		// load AWS config
		awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, resourceConfig.Region, awsRoleArn, awsSerialNumber)
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}

		// create resource client
		resourceClient := resource.CreateResourceClient(awsConfig)

		// capture messages as resources are created and return to user
		go func() {
			for msg := range *resourceClient.MessageChan {
				fmt.Println(msg)
			}
		}()

		// capture inventory and write to file as it is created
		go func() {
			for inventory := range *resourceClient.InventoryChan {
				if err := resource.WriteInventory(createInventoryFile, &inventory); err != nil {
					fmt.Printf("failed to write inventory file: %s", err)
				}
			}
		}()

		// delete inventory if interrupted
		sigs := make(chan os.Signal, 1)
		signal.Notify(sigs, syscall.SIGINT, syscall.SIGTERM)
		go func() {
			<-sigs
			fmt.Println("\nReceived Ctrl+C, cleaning up resources...")
			inventory, err := resource.ReadInventory(createInventoryFile)
			if err != nil {
				fmt.Printf("failed to read eks cluster inventory: %s\n", err)
			}
			if err = resourceClient.DeleteResourceStack(inventory); err != nil {
				fmt.Printf("failed to delete eks cluster resources: %s\n", err)
			}
		}()

		fmt.Println("Running... Press Ctrl+C to exit")

		// create resources
		fmt.Println("Creating resources for EKS cluster...")
		if err := resourceClient.CreateResourceStack(resourceConfig); err != nil {
			fmt.Printf("Problem encountered creating resources - deleting resources that were created: %s\n", err)
			// get inventory
			inventory, invErr := resource.ReadInventory(createInventoryFile)
			if invErr != nil {
				return fmt.Errorf("Error creating resources: %w\nError reading eks cluster inventory: %w", err, invErr)
			}
			// delete resource stack
			if deleteErr := resourceClient.DeleteResourceStack(inventory); deleteErr != nil {
				return fmt.Errorf("Error creating resources: %w\nError deleting resources: %s", err, deleteErr)
			}
			// remove inventory file from filesystem
			if err := os.Remove(deleteInventoryFile); err != nil {
				return err
			}

			return fmt.Errorf("failed to create resource stack for eks cluster: %w", err)
		}

		fmt.Printf("Inventory file '%s' written\n", createInventoryFile)

		fmt.Println("EKS cluster created")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(
		&configFile, "config-file", "c", "",
		"File to read EKS cluster config from",
	)
	createCmd.MarkFlagRequired("config-file")
	createCmd.Flags().StringVarP(
		&createInventoryFile, "inventory-file", "i", "eks-cluster-inventory.json",
		"File to write resource inventory to",
	)
}
