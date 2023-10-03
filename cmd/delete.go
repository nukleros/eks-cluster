/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/nukleros/eks-cluster/pkg/resource"
)

var deleteInventoryFile string

// deleteCmd represents the delete command.
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove an EKS cluster from AWS",
	Long:  `Remove an EKS cluster from AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// load inventory
		inventory, err := resource.ReadInventory(deleteInventoryFile)
		if err != nil {
			return fmt.Errorf("failed to read eks cluster inventory: %s\n", err)
		}

		// load AWS config
		awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, inventory.Region, awsRoleArn, awsExternalId, awsSerialNumber)
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

		// capture inventory and write to file as resources are deleted
		go func() {
			for inventory := range *resourceClient.InventoryChan {
				if err := resource.WriteInventory(deleteInventoryFile, &inventory); err != nil {
					fmt.Printf("failed to write inventory file: %s", err)
				}
			}
		}()

		// delete eks cluster resources
		err = resourceClient.DeleteResourceStack(inventory)
		if err != nil {
			return fmt.Errorf("failed to delete eks cluster resource stack: %w", err)
		}

		// remove inventory file from filesystem
		if err := os.Remove(deleteInventoryFile); err != nil {
			return fmt.Errorf("failed to remove eks cluster inventory file: %w", err)
		}

		fmt.Printf("Inventory file '%s' deleted\n", deleteInventoryFile)

		fmt.Println("EKS cluster deleted")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().StringVarP(
		&deleteInventoryFile, "inventory-file", "i", "eks-cluster-inventory.json",
		"File to read resource inventory from",
	)
}
