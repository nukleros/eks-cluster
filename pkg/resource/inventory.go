package resource

import (
	"encoding/json"
	"io/ioutil"
	"os"
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
	DNS01ChallengeRole     RoleInventory    `json:"dns01ChallengeRole"`
	StorageManagementRole  RoleInventory    `json:"storageManagementRole"`
	ClusterAutoscalingRole RoleInventory    `json:"clusterAutoscalingRole"`
	PolicyARNs             []string         `json:"policyARNs"`
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

// WriteInventory writes the inventory to a file.
func WriteInventory(inventoryFile string, inventory *ResourceInventory) error {
	inventoryJSON, err := MarshalInventory(inventory)
	if err != nil {
		return err
	}

	if err := os.WriteFile(inventoryFile, inventoryJSON, 0644); err != nil {
		return err
	}

	return nil
}

// ReadInventory reads the inventory from the inventory file.
func ReadInventory(inventoryFile string) (*ResourceInventory, error) {
	// read inventory file
	inventoryBytes, err := ioutil.ReadFile(inventoryFile)
	if err != nil {
		return nil, err
	}

	// unmarshal JSON data
	var inventory ResourceInventory
	if err := UnmarshalInventory(inventoryBytes, &inventory); err != nil {
		return nil, err
	}

	return &inventory, nil
}

// MarshalInventory returns a json representation of inventory from a
// ResourceInventory object.
func MarshalInventory(inventory *ResourceInventory) ([]byte, error) {
	var inventoryJSON []byte
	inventoryJSON, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return inventoryJSON, err
	}

	return inventoryJSON, nil
}

// UnmarshalInventory unmarshalls an inventory as a JSON byte array into a
// ResourceInventory object.
func UnmarshalInventory(inventoryBytes []byte, inventory *ResourceInventory) error {
	if err := json.Unmarshal(inventoryBytes, &inventory); err != nil {
		return err
	}

	return nil
}
