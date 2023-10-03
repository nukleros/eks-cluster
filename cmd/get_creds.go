/*
Copyright © 2023 NAME HERE <EMAIL ADDRESS>
*/
package cmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/nukleros/eks-cluster/pkg/connection"
	"github.com/nukleros/eks-cluster/pkg/resource"
)

var (
	clusterName string
)

// getCredsCmd represents the get-creds command.
var getCredsCmd = &cobra.Command{
	Use:   "get-creds",
	Short: "Get credentials to connect to EKS cluster",
	Long:  `Get credentials to connect to EKS cluster.`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// load AWS config
		awsConfig, err := resource.LoadAWSConfig(awsConfigEnv, awsConfigProfile, "", awsRoleArn, awsExternalId, awsSerialNumber)
		if err != nil {
			return fmt.Errorf("failed to load AWS config: %w", err)
		}

		// get credentials
		connInfo := connection.EKSClusterConnectionInfo{ClusterName: clusterName}
		if err := connInfo.Get(awsConfig); err != nil {
			return fmt.Errorf("failed to get EKS cluster credentials: %w", err)
		}

		fmt.Println("Token:")
		fmt.Println(connInfo.Token)
		fmt.Println("Token Expiration:")
		fmt.Println(connInfo.TokenExpiration)
		fmt.Println("CA Certificate:")
		fmt.Println(connInfo.CACertificate)

		fmt.Println("EKS cluster credentials retrieved")

		return nil
	},
}

func init() {
	rootCmd.AddCommand(getCredsCmd)

	getCredsCmd.Flags().StringVarP(
		&clusterName, "cluster-name", "c", "",
		"The EKS cluster name to retrieve credentials for",
	)
	getCredsCmd.MarkFlagRequired("cluster-name")
}
