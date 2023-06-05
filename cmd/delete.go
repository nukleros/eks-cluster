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
		err := api.Delete(awsConfigEnv, awsConfigProfile, inventoryFile)
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
