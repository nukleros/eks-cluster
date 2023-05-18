package resource

import (
	"encoding/json"
	"io/ioutil"
)

// ResourceInventory contains a record of all resources created so they can be
// referenced and cleaned up.
type ResourceInventory struct {
	Region                 string           `json:"region"`
	VPCID                  string           `json:"vpcID"`
	SubnetIDs              []string         `json:"subnetIDs"`
	InternetGatewayID      string           `json:"internetGatewayID"`
	ElasticIPIDs           []string         `json:"elasticIPIDs"`
	PrivateRouteTableIDs   []string         `json:"privateRouteTableIDs"`
	PublicRouteTableID     string           `json:"publicRouteTableID"`
	ClusterRole            RoleInventory    `json:"clusterRole"`
	WorkerRole             RoleInventory    `json:"workerRole"`
	DNSManagementRole      RoleInventory    `json:"dnsManagementRole"`
	PolicyARNs             []string         `json:"policyARNs"`
	StorageManagementRole  RoleInventory    `json:"storageManagementRole"`
	ClusterAutoscalingRole RoleInventory    `json:"clusterAutoscalingRole"`
	Cluster                ClusterInventory `json:"cluster"`
	NodeGroupNames         []string         `json:"nodeGroupNames"`
	OIDCProviderARN        string           `json:"oidcProviderARN"`
}

// RoleInventory contains the details for each role created.
type RoleInventory struct {
	RoleName       string   `json:"roleName"`
	RoleARN        string   `json:"roleARN"`
	RolePolicyARNs []string `json:"rolePolicyARNs"`
}

// ClusterInventory contains the details for the EKS cluster.
type ClusterInventory struct {
	ClusterName     string `json:"clusterName"`
	ClusterARN      string `json:"clusterARN"`
	OIDCProviderURL string `json:"oidcProviderURL"`
}

func WriteInventory(inventoryFile string, inventory *ResourceInventory) error {
	// write inventory file
	inventoryJSON, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return err
	}
	ioutil.WriteFile(inventoryFile, inventoryJSON, 0644)

	return nil
}

func ReadInventory(inventoryFile string) (*ResourceInventory, error) {
	// read inventory file
	inventoryBytes, err := ioutil.ReadFile(inventoryFile)
	if err != nil {
		return nil, err
	}

	// unmarshal JSON data
	var inventory ResourceInventory
	err = json.Unmarshal(inventoryBytes, &inventory)
	if err != nil {
		return nil, err
	}

	return &inventory, nil
}
