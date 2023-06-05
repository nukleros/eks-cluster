/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/nukleros/eks-cluster/pkg/api"
	"github.com/spf13/cobra"
)

// deleteCmd represents the delete command
var deleteCmd = &cobra.Command{
	Use:   "delete",
	Short: "Remove an EKS cluster from AWS",
	Long:  `Remove an EKS cluster from AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {

		// Create resource client
		resourceClient, err := api.CreateResourceClient(awsConfigEnv, awsConfigProfile)
		if err != nil {
			return err
		}

		err = api.Delete(resourceClient, inventoryFile)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(deleteCmd)

	deleteCmd.Flags().StringVarP(&inventoryFile, "inventory-file", "i", "eks-cluster-inventory.json", "File to read resource inventory from")
}
