/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"github.com/spf13/cobra"

	"github.com/nukleros/eks-cluster/pkg/api"
)

var (
	configFile string
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Provision an EKS cluster in AWS",
	Long:  `Provision an EKS cluster in AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		err := api.Create(awsConfigEnv, awsConfigProfile, inventoryFile)
		if err != nil {
			return err
		}
		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(&configFile, "config-file", "c", "",
		"File to read EKS cluster config from")
	createCmd.Flags().StringVarP(&inventoryFile, "inventory-file", "i",
		"eks-cluster-inventory.json", "File to write resource inventory to")
}
