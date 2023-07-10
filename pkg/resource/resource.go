package resource

import (
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

var ErrResourceNotFound = errors.New("resource not found")

// CreateResourceStack creates all the resources for an EKS cluster.
func (c *ResourceClient) CreateResourceStack(resourceConfig *ResourceConfig) error {
	var inventory ResourceInventory
	if resourceConfig.Region != "" {
		inventory.Region = resourceConfig.Region
		c.AWSConfig.Region = resourceConfig.Region
	} else {
		inventory.Region = c.AWSConfig.Region
		resourceConfig.Region = c.AWSConfig.Region
	}

	// Tags
	ec2Tags := CreateEC2Tags(resourceConfig.Name, resourceConfig.Tags)
	iamTags := CreateIAMTags(resourceConfig.Name, resourceConfig.Tags)
	mapTags := CreateMapTags(resourceConfig.Name, resourceConfig.Tags)

	// set availability zones as needed
	if err := resourceConfig.SetAvailabilityZones(c); err != nil {
		return err
	}

	// VPC
	vpc, err := c.CreateVPC(ec2Tags, resourceConfig.ClusterCIDR, resourceConfig.Name)
	if vpc != nil {
		inventory.VPCID = *vpc.VpcId
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("VPC created: %s\n", *vpc.VpcId))

	// Internet Gateway
	igw, err := c.CreateInternetGateway(ec2Tags, *vpc.VpcId, resourceConfig.Name)
	if igw != nil {
		inventory.InternetGatewayID = *igw.InternetGatewayId
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Internet gateway created: %s\n", *igw.InternetGatewayId))

	// Subnets
	var privateSubnetIDs []string
	var publicSubnetIDs []string
	var allSubnetIDs []string
	privateSubnets, publicSubnets, err := c.CreateSubnets(ec2Tags, *vpc.VpcId, resourceConfig.Name, &resourceConfig.AvailabilityZones)
	if privateSubnets != nil {
		for _, subnet := range *privateSubnets {
			privateSubnetIDs = append(privateSubnetIDs, *subnet.SubnetId)
			allSubnetIDs = append(allSubnetIDs, *subnet.SubnetId)
		}
	}
	if publicSubnets != nil {
		for _, subnet := range *publicSubnets {
			publicSubnetIDs = append(publicSubnetIDs, *subnet.SubnetId)
			allSubnetIDs = append(allSubnetIDs, *subnet.SubnetId)
		}
	}
	inventory.SubnetIDs = allSubnetIDs
	c.sendInventory(&inventory)
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Subnets created: %s\n", allSubnetIDs))

	// Elastic IPs
	elasticIPIDs, err := c.CreateElasticIPs(ec2Tags, publicSubnetIDs)
	inventory.ElasticIPIDs = elasticIPIDs
	c.sendInventory(&inventory)
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Elastic IPs created: %s\n", elasticIPIDs))

	// NAT Gateways
	// Note: unlike all other resources, the resource ID is not returned on
	// creation.  The IDs are not added to the inventory.  Instead, the NAT
	// gateways are cleaned up by filtering by VPC ID.
	if err := c.CreateNATGateways(ec2Tags, resourceConfig.AvailabilityZones, elasticIPIDs); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("NAT gateways created for subnets: %s\n", privateSubnetIDs))
	c.sendMessage(fmt.Sprintf("Waiting for NAT gateways to become active for subnets: %s\n", privateSubnetIDs))
	if err := c.WaitForNATGateways(*vpc.VpcId, &resourceConfig.AvailabilityZones, NATGatewayConditionCreated); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("NAT gateways ready for subnets: %s\n", privateSubnetIDs))

	// Route Tables
	var privateRouteTableIDs []string
	privateRouteTables, publicRouteTable, err := c.CreateRouteTables(ec2Tags,
		*vpc.VpcId, *igw.InternetGatewayId, &resourceConfig.AvailabilityZones,
	)
	if privateRouteTables != nil {
		for _, rt := range *privateRouteTables {
			privateRouteTableIDs = append(privateRouteTableIDs, *rt.RouteTableId)
		}
	}
	inventory.PrivateRouteTableIDs = privateRouteTableIDs
	c.sendInventory(&inventory)
	if publicRouteTable != nil {
		inventory.PublicRouteTableID = *publicRouteTable.RouteTableId
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(
		fmt.Sprintf(
			"Route tables created: [%s %s]\n",
			privateRouteTableIDs, *publicRouteTable.RouteTableId,
		),
	)

	// IAM Policy for DNS Management
	var createdDNSPolicy types.Policy
	if resourceConfig.DNSManagement {
		dnsPolicy, err := c.CreateDNSManagementPolicy(iamTags, resourceConfig.Name)
		if dnsPolicy != nil {
			inventory.PolicyARNs = append(inventory.PolicyARNs, *dnsPolicy.Arn)
			c.sendInventory(&inventory)
		}
		if err != nil {
			return err
		}
		c.sendMessage(fmt.Sprintf("IAM policy created: %s\n", *dnsPolicy.PolicyName))
		createdDNSPolicy = *dnsPolicy
	}

	// IAM Policy for Cluster Autoscaling
	var createdClusterAutoscalingPolicy types.Policy
	if resourceConfig.ClusterAutoscaling {
		clusterAutoscalingPolicy, err := c.CreateClusterAutoscalingPolicy(iamTags, resourceConfig.Name)
		if clusterAutoscalingPolicy != nil {
			inventory.PolicyARNs = append(inventory.PolicyARNs, *clusterAutoscalingPolicy.Arn)
			c.sendInventory(&inventory)
		}
		if err != nil {
			return err
		}
		c.sendMessage(fmt.Sprintf("IAM policy created: %s\n", *clusterAutoscalingPolicy.PolicyName))
		createdClusterAutoscalingPolicy = *clusterAutoscalingPolicy
	}

	// IAM Roles
	clusterRole, workerRole, err := c.CreateRoles(iamTags, resourceConfig.Name)
	if clusterRole != nil {
		inventory.ClusterRole = RoleInventory{
			RoleName:       *clusterRole.RoleName,
			RoleARN:        *clusterRole.Arn,
			RolePolicyARNs: []string{ClusterPolicyARN},
		}
		c.sendInventory(&inventory)
	}
	if workerRole != nil {
		inventory.WorkerRole = RoleInventory{
			RoleName:       *workerRole.RoleName,
			RoleARN:        *workerRole.Arn,
			RolePolicyARNs: getWorkerPolicyARNs(),
		}
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("IAM roles created: [%s %s]\n", *clusterRole.RoleName, *workerRole.RoleName))

	// EKS Cluster
	cluster, err := c.CreateCluster(&mapTags, resourceConfig.Name, resourceConfig.KubernetesVersion,
		*clusterRole.Arn, privateSubnetIDs)
	if cluster != nil {
		inventory.Cluster.ClusterName = *cluster.Name
		inventory.Cluster.ClusterARN = *cluster.Arn
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EKS cluster created: %s\n", *cluster.Name))
	c.sendMessage(fmt.Sprintf("Waiting for EKS cluster to become active: %s\n", *cluster.Name))
	oidcIssuer, err := c.WaitForCluster(*cluster.Name, ClusterConditionCreated)
	if oidcIssuer != "" {
		inventory.Cluster.OIDCProviderURL = oidcIssuer
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EKS cluster ready: %s\n", *cluster.Name))

	// Node Groups
	var nodeGroupNames []string
	nodeGroups, err := c.CreateNodeGroups(&mapTags, *cluster.Name, resourceConfig.KubernetesVersion,
		*workerRole.Arn, privateSubnetIDs, resourceConfig.InstanceTypes, resourceConfig.InitialNodes,
		resourceConfig.MinNodes, resourceConfig.MaxNodes, resourceConfig.KeyPair)
	if nodeGroups != nil {
		for _, nodeGroup := range *nodeGroups {
			nodeGroupNames = append(nodeGroupNames, *nodeGroup.NodegroupName)
		}
		inventory.NodeGroupNames = nodeGroupNames
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EKS node group created: %s\n", nodeGroupNames))
	c.sendMessage(fmt.Sprintf("Waiting for EKS node group to become active: %s\n", nodeGroupNames))
	if err := c.WaitForNodeGroups(*cluster.Name, nodeGroupNames, NodeGroupConditionCreated); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EKS node group ready: %s\n", nodeGroupNames))

	// OIDC Provider
	oidcProviderARN, err := c.CreateOIDCProvider(iamTags, oidcIssuer)
	if oidcProviderARN != "" {
		inventory.OIDCProviderARN = oidcProviderARN
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("OIDC provider created: %s\n", oidcProviderARN))

	// IAM Role for DNS Management
	if resourceConfig.DNSManagement {
		if createdDNSPolicy.Arn == nil {
			return errors.New("no DNS policy ARN to attach to DNS management role")
		}
		dnsManagementRole, err := c.CreateDNSManagementRole(iamTags, *createdDNSPolicy.Arn,
			resourceConfig.AWSAccountID, oidcIssuer, &resourceConfig.DNSManagementServiceAccount,
			resourceConfig.Name)
		if dnsManagementRole != nil {
			inventory.DNSManagementRole = RoleInventory{
				RoleName:       *dnsManagementRole.RoleName,
				RoleARN:        *dnsManagementRole.Arn,
				RolePolicyARNs: []string{*createdDNSPolicy.Arn},
			}
			c.sendInventory(&inventory)
		}
		if err != nil {
			return err
		}
		c.sendMessage(fmt.Sprintf("IAM role for DNS management created: %s\n", *dnsManagementRole.RoleName))
	}

	// IAM Role for Cluster Autoscaling
	if resourceConfig.ClusterAutoscaling {
		if createdClusterAutoscalingPolicy.Arn == nil {
			return errors.New("no cluster autoscaling policy ARN to attach to cluster autoscaling role")
		}
		clusterAutoscalingRole, err := c.CreateClusterAutoscalingRole(iamTags, *createdClusterAutoscalingPolicy.Arn,
			resourceConfig.AWSAccountID, oidcIssuer, &resourceConfig.ClusterAutoscalingServiceAccount,
			resourceConfig.Name)
		if clusterAutoscalingRole != nil {
			inventory.ClusterAutoscalingRole = RoleInventory{
				RoleName:       *clusterAutoscalingRole.RoleName,
				RoleARN:        *clusterAutoscalingRole.Arn,
				RolePolicyARNs: []string{*clusterAutoscalingRole.PermissionsBoundary.PermissionsBoundaryArn},
			}
			c.sendInventory(&inventory)
		}
		if err != nil {
			return err
		}
		c.sendMessage(fmt.Sprintf("IAM role for cluster autoscaling created: %s\n", *clusterAutoscalingRole.RoleName))
	}

	// IAM Role for Storage Management
	storageManagementRole, err := c.CreateStorageManagementRole(iamTags, resourceConfig.AWSAccountID,
		oidcIssuer, &resourceConfig.StorageManagementServiceAccount, resourceConfig.Name)
	if storageManagementRole != nil {
		inventory.StorageManagementRole = RoleInventory{
			RoleName:       *storageManagementRole.RoleName,
			RoleARN:        *storageManagementRole.Arn,
			RolePolicyARNs: []string{*storageManagementRole.PermissionsBoundary.PermissionsBoundaryArn},
		}
		c.sendInventory(&inventory)
	}
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("IAM role for storage management created: %s\n", *storageManagementRole.RoleName))

	// EBS CSI Addon
	ebsStorageAddon, err := c.CreateEBSStorageAddon(&mapTags, *cluster.Name, *storageManagementRole.Arn)
	if err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EBS storage addon created: %s\n", *ebsStorageAddon.AddonName))

	c.sendMessage(fmt.Sprintf("EKS cluster creation complete: %s\n", *cluster.Name))

	return nil
}

// DeleteResourceStack deletes all the resources in the resource inventory.
func (c *ResourceClient) DeleteResourceStack(inventory *ResourceInventory) error {
	c.AWSConfig.Region = inventory.Region

	// OIDC Provider
	if err := c.DeleteOIDCProvider(inventory.OIDCProviderARN); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("OIDC provider deleted: %s\n", inventory.OIDCProviderARN))
	inventory.OIDCProviderARN = ""
	c.sendInventory(inventory)

	// Node Groups
	if err := c.DeleteNodeGroups(inventory.Cluster.ClusterName, inventory.NodeGroupNames); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Node groups deletion initiated: %s\n", inventory.NodeGroupNames))
	c.sendMessage(fmt.Sprintf("Waiting for node groups to be deleted: %s\n", inventory.NodeGroupNames))
	if err := c.WaitForNodeGroups(inventory.Cluster.ClusterName, inventory.NodeGroupNames, NodeGroupConditionDeleted); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Node groups deletion complete: %s\n", inventory.NodeGroupNames))
	inventory.NodeGroupNames = []string{}
	c.sendInventory(inventory)

	// EKS Cluster
	if err := c.DeleteCluster(inventory.Cluster.ClusterName); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EKS cluster deletion initiated: %s\n", inventory.Cluster.ClusterName))
	c.sendMessage(fmt.Sprintf("Waiting for EKS cluster to be deleted: %s\n", inventory.Cluster.ClusterName))
	if _, err := c.WaitForCluster(inventory.Cluster.ClusterName, ClusterConditionDeleted); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("EKS cluster deletion complete: %s\n", inventory.Cluster.ClusterName))
	inventory.Cluster = ClusterInventory{}
	c.sendInventory(inventory)

	// IAM Roles
	iamRoles := []RoleInventory{
		inventory.ClusterRole,
		inventory.WorkerRole,
		inventory.DNSManagementRole,
		inventory.ClusterAutoscalingRole,
		inventory.StorageManagementRole,
	}
	if err := c.DeleteRoles(&iamRoles); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("IAM roles deleted: %s\n", iamRoles))
	inventory.ClusterRole = RoleInventory{}
	inventory.WorkerRole = RoleInventory{}
	inventory.DNSManagementRole = RoleInventory{}
	inventory.ClusterAutoscalingRole = RoleInventory{}
	inventory.StorageManagementRole = RoleInventory{}
	c.sendInventory(inventory)

	// IAM Policies
	if err := c.DeletePolicies(inventory.PolicyARNs); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("IAM policies deleted: %s\n", inventory.PolicyARNs))
	inventory.PolicyARNs = []string{}
	c.sendInventory(inventory)

	// NAT Gateways
	if err := c.DeleteNATGateways(inventory.VPCID); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("NAT gateways deletion initiated for VPC with ID: %s\n", inventory.VPCID))
	c.sendMessage(fmt.Sprintf("Waiting for NAT gateways to be deleted for VPC with ID: %s\n", inventory.VPCID))
	if err := c.WaitForNATGateways(inventory.VPCID, nil, NATGatewayConditionDeleted); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("NAT gateway deletion complete for VPC with ID: %s\n", inventory.VPCID))

	// Internet Gateway
	if err := c.DeleteInternetGateway(inventory.InternetGatewayID, inventory.VPCID); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Internet gateway deleted: %s\n", inventory.InternetGatewayID))
	inventory.InternetGatewayID = ""
	c.sendInventory(inventory)

	// Elastic IPs
	if err := c.DeleteElasticIPs(inventory.ElasticIPIDs); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Elastic IPs deleted: %s\n", inventory.ElasticIPIDs))
	inventory.ElasticIPIDs = []string{}
	c.sendInventory(inventory)

	// Subnets
	if err := c.DeleteSubnets(inventory.SubnetIDs); err != nil {
		return err
	}
	c.sendMessage(fmt.Sprintf("Subnets deleted: %s\n", inventory.SubnetIDs))
	inventory.SubnetIDs = []string{}
	c.sendInventory(inventory)

	// Route Tables
	if err := c.DeleteRouteTables(inventory.PrivateRouteTableIDs, inventory.PublicRouteTableID); err != nil {
		return err
	}
	c.sendMessage(
		fmt.Sprintf("Route tables deleted: [%s %s]\n",
			inventory.PrivateRouteTableIDs, inventory.PublicRouteTableID,
		),
	)
	inventory.PrivateRouteTableIDs = []string{}
	inventory.PublicRouteTableID = ""
	c.sendInventory(inventory)

	// VPC
	c.sendMessage(fmt.Sprintf("VPC deleted: %s\n", inventory.VPCID))
	if err := c.DeleteVPC(inventory.VPCID); err != nil {
		return err
	}
	inventory.VPCID = ""
	c.sendInventory(inventory)

	return nil
}

// sendMessage sends human-readable messages back to the client with updates on
// resource creation or deletion as they occur.
func (c *ResourceClient) sendMessage(message string) {
	if c.MessageChan != nil {
		*c.MessageChan <- message
	}
}

// sendInventory sends a complete version of the latest inventory back to the
// client as it is created or deleted.
func (c *ResourceClient) sendInventory(inventory *ResourceInventory) {
	if c.InventoryChan != nil {
		*c.InventoryChan <- *inventory
	}
}
