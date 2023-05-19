/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/nukleros/eks-cluster/pkg/resource"
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove an EKS cluster from AWS",
	Long:  `Remove an EKS cluster from AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().StringVarP(&inventoryFile, "inventory-file", "i", "eks-cluster-inventory.json", "File to read resource inventory from")
}
