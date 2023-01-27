/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/nukleros/eks-cluster/pkg/resource"
	"github.com/spf13/cobra"
)

var inventoryFileIn string

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove an EKS cluster from AWS",
	Long:  `Remove an EKS cluster from AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// load inventory
		var resourceInventory resource.ResourceInventory
		if inventoryFileIn != "" {
			inventoryJSON, err := ioutil.ReadFile(inventoryFileIn)
			if err != nil {
				return err
			}
			json.Unmarshal(inventoryJSON, &resourceInventory)
		}

		// create resource client
		msgChan := make(chan string)
		go outputMessages(&msgChan)
		ctx := context.Background()
		cfg, err := config.LoadDefaultConfig(
			ctx,
			//config.WithRegion(r.Region),
		)
		resourceClient := resource.ResourceClient{
			&msgChan,
			ctx,
			cfg,
		}

		// delete resources
		fmt.Println("Deleting resources for EKS cluster...")
		if err := resourceClient.DeleteResourceStack(&resourceInventory); err != nil {
			return err
		}

		// update inventory file
		var emptyResourceInventory resource.ResourceInventory
		emptyInventoryJSON, err := json.MarshalIndent(emptyResourceInventory, "", "  ")
		if err != nil {
			return err
		}
		ioutil.WriteFile(inventoryFileIn, emptyInventoryJSON, 0644)

		fmt.Println("EKS cluster deleted")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().StringVarP(&inventoryFileIn, "inventory-file", "i", "eks-cluster-inventory.json", "File to read resource inventory from")
}
