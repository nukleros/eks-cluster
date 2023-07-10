package cmd

import (
	"encoding/json"
	"errors"
	"io/ioutil"
	"os"

	"github.com/nukleros/eks-cluster/pkg/resource"
)

// writeInventory writes the inventory to a file.
func writeInventory(inventoryFile string, inventory *resource.ResourceInventory) error {
	// create inventory file if not present
	_, err := os.Stat(inventoryFile)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			_, err := os.Create(inventoryFile)
			if err != nil {
				return err
			}
		} else {
			return err
		}
	}

	// write inventory file
	inventoryJSON, err := json.MarshalIndent(inventory, "", "  ")
	if err != nil {
		return err
	}
	ioutil.WriteFile(inventoryFile, inventoryJSON, 0644)

	return nil
}

// readInventory reads the inventory from the inventory file.
func readInventory(inventoryFile string) (*resource.ResourceInventory, error) {
	// read inventory file
	inventoryBytes, err := ioutil.ReadFile(inventoryFile)
	if err != nil {
		return nil, err
	}

	// unmarshal JSON data
	var inventory resource.ResourceInventory
	err = json.Unmarshal(inventoryBytes, &inventory)
	if err != nil {
		return nil, err
	}

	return &inventory, nil
}
