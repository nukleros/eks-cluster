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

	"github.com/nukleros/eks-cluster/pkg/api"
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

		// Create resource client
		resourceClient, err := api.CreateResourceClient(awsConfigEnv, awsConfigProfile)
		if err != nil {
			return err
		}

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

		err = api.Create(resourceClient, resourceConfig, inventoryFile)
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
