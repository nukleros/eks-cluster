/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"context"
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
	configFile string
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
			fmt.Println("Problem encountered creating resources - deleting resources that were created")
			if deleteErr := resourceClient.DeleteResourceStack(inventoryFile); deleteErr != nil {
				return fmt.Errorf("\nError creating resources: %w\nError deleting resources: %s", err, deleteErr)
			}
			return err
		}

		fmt.Printf("Inventory file '%s' written\n", inventoryFile)

		fmt.Println("EKS cluster created")
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
