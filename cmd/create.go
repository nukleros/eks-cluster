/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/nukleros/eks-cluster/pkg/resource"
)

var (
	inventoryFileOut string
	configFile       string
)

// createCmd represents the create command
var createCmd = &cobra.Command{
	Use:   "create",
	Short: "Provision an EKS cluster in AWS",
	Long:  `Provision an EKS cluster in AWS.`,
	RunE: func(cmd *cobra.Command, args []string) error {
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

		// create resource client
		awsConfig, err := resource.LoadAWSConfig(
			awsConfigEnv, awsConfigProfile, awsRegion)
		if err != nil {
			return err
		}
		msgChan := make(chan string)
		go outputMessages(&msgChan)
		ctx := context.Background()
		resourceClient := resource.ResourceClient{&msgChan, ctx, awsConfig}

		// create resources
		fmt.Println("Creating resources for EKS cluster...")
		inventory, err := resourceClient.CreateResourceStack(resourceConfig)
		if err != nil {
			fmt.Println("Problem encountered creating resources - deleting resources that were created")
			if deleteErr := resourceClient.DeleteResourceStack(inventory); deleteErr != nil {
				return fmt.Errorf("\nError creating resources: %w\nError deleting resources: %s", err, deleteErr)
			}
			return err
		}

		// write inventory file
		inventoryJSON, err := json.MarshalIndent(inventory, "", "  ")
		if err != nil {
			return err
		}
		ioutil.WriteFile(inventoryFileOut, inventoryJSON, 0644)
		fmt.Printf("Inventory file '%s' written\n", inventoryFileOut)

		fmt.Println("EKS cluster created")
		return nil
	},
}

func init() {
	rootCmd.AddCommand(createCmd)

	createCmd.Flags().StringVarP(&configFile, "config-file", "c", "",
		"File to read EKS cluster config from")
	createCmd.Flags().StringVarP(&inventoryFileOut, "inventory-file", "i",
		"eks-cluster-inventory.json", "File to write resource inventory to")
}
