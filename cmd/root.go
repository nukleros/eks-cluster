/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"os"

	"github.com/spf13/cobra"
)

// rootCmd represents the base command when called without any subcommands.
var rootCmd = &cobra.Command{
	Use:   "eks-cluster",
	Short: "Manage EKS clusters in AWS",
	Long: `Manage EKS clusters in AWS.

Credentials and default region are set as follows:
If you want to provide these settings as environment variables, pass
'--aws-config-env=true'.  The credentials and default region will be loaded from
the following environment variables:
AWS_ACCESS_KEY_ID
AWS_SECRET_ACCESS_KEY
AWS_DEFAULT_REGION

If you want to use a config profile, pass '--aws-config-profile=[config profile]'.
Instructions on setting profiles for AWS credentials can be found here:
https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-files.html

If neither flag is provided, the 'default' config profile will be used.

In any case, the default region can be overridden with the eks-cluster
config file.  If you set the region there, that value will take
precedence.
`,
}

var (
	awsConfigEnv     bool
	awsConfigProfile string
	inventoryFile    string
)

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
	rootCmd.PersistentFlags().StringVarP(&awsConfigProfile, "aws-config-profile", "a", "default",
		"The AWS config profile to draw credentials from when provisioning resources")
	rootCmd.PersistentFlags().BoolVarP(&awsConfigEnv, "aws-config-env", "e", false,
		"Retrieve credentials from environment variables")
}
