/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands
var rootCmd = &cobra.Command{
	Use:   "eks-cluster",
	Short: "Manage EKS clusters in AWS",
	Long:  `Manage EKS clusters in AWS.`,
}

// Execute adds all child commands to the root command and sets flags appropriately.
// This is called by main.main(). It only needs to happen once to the rootCmd.
func Execute() {
	err := rootCmd.Execute()
	if err != nil {
		os.Exit(1)
	}
}

func init() {
	rootCmd.Flags().BoolP("toggle", "t", false, "Help message for toggle")
}

// outputMessages prints the output messages from the resource client to the
// terminal for the CLI user.
func outputMessages(msgChan *chan string) {
	for {
		msg := <-*msgChan
		fmt.Println(msg)
	}
}
