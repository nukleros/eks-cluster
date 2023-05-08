package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/iam/types"
)

// ResourceClient contains the elements needed to manage resources with this
// package.
type ResourceClient struct {
	MessageChan *chan string
	Context     context.Context
	AWSConfig   *aws.Config
}

var ErrResourceNotFound = errors.New("resource not found")

// CreateResourceStack creates all the resources for an EKS cluster.
func (c *ResourceClient) CreateResourceStack(resourceConfig *ResourceConfig) (*ResourceInventory, error) {
	inventory := ResourceInventory{Region: resourceConfig.Region}

	// Tags
	ec2Tags := CreateEC2Tags(resourceConfig.Name, resourceConfig.Tags)
	iamTags := CreateIAMTags(resourceConfig.Name, resourceConfig.Tags)
	mapTags := CreateMapTags(resourceConfig.Name, resourceConfig.Tags)

	// VPC
	vpc, err := c.CreateVPC(ec2Tags, resourceConfig.ClusterCIDR, resourceConfig.Name)
	if vpc != nil {
		inventory.VPCID = *vpc.VpcId
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("VPC created: %s\n", *vpc.VpcId)
	}

	// Internet Gateway
	igw, err := c.CreateInternetGateway(ec2Tags, *vpc.VpcId, resourceConfig.Name)
	if igw != nil {
		inventory.InternetGatewayID = *igw.InternetGatewayId
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Internet gateway created: %s\n", *igw.InternetGatewayId)
	}

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
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Subnets created: %s\n", allSubnetIDs)
	}

	// Elastic IPs
	elasticIPIDs, err := c.CreateElasticIPs(ec2Tags, publicSubnetIDs)
	inventory.ElasticIPIDs = elasticIPIDs
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Elastic IPs created: %s\n", elasticIPIDs)
	}

	// NAT Gateways
	// Note: unlike all other resources, the resource ID is not returned on
	// creation.  The IDs are not added to the inventory.  Instead, the NAT
	// gateways are cleaned up by filtering by VPC ID.
	if err := c.CreateNATGateways(ec2Tags, resourceConfig.AvailabilityZones, elasticIPIDs); err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways created for subnets: %s\n", privateSubnetIDs)
		*c.MessageChan <- fmt.Sprintf("Waiting for NAT gateways to become active for subnets: %s\n", privateSubnetIDs)
	}
	if err := c.WaitForNATGateways(*vpc.VpcId, &resourceConfig.AvailabilityZones, NATGatewayConditionCreated); err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways ready for subnets: %s\n", privateSubnetIDs)
	}

	// Route Tables
	var privateRouteTableIDs []string
	privateRouteTables, publicRouteTable, err := c.CreateRouteTables(ec2Tags,
		*vpc.VpcId, *igw.InternetGatewayId, &resourceConfig.AvailabilityZones)
	if privateRouteTables != nil {
		for _, rt := range *privateRouteTables {
			privateRouteTableIDs = append(privateRouteTableIDs, *rt.RouteTableId)
		}
	}
	inventory.PrivateRouteTableIDs = privateRouteTableIDs
	if publicRouteTable != nil {
		inventory.PublicRouteTableID = *publicRouteTable.RouteTableId
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf(
			"Route tables created: [%s %s]\n",
			privateRouteTableIDs, *publicRouteTable.RouteTableId)
	}

	// IAM Policy for DNS Management
	var createdDNSPolicy types.Policy
	if resourceConfig.DNSManagement {
		dnsPolicy, err := c.CreateDNSManagementPolicy(iamTags)
		if dnsPolicy != nil {
			inventory.PolicyARNs = append(inventory.PolicyARNs, *dnsPolicy.Arn)
		}
		if err != nil {
			return &inventory, err
		}
		if c.MessageChan != nil {
			*c.MessageChan <- fmt.Sprintf("IAM policy created: %s\n", *dnsPolicy.PolicyName)
		}
		createdDNSPolicy = *dnsPolicy
	}

	// IAM Policy for Cluster Autoscaling
	var createdClusterAutoscalingPolicy types.Policy
	if resourceConfig.ClusterAutoscaling {
		clusterAutoscalingPolicy, err := c.CreateClusterAutoscalingPolicy(iamTags, resourceConfig.Name)
		if clusterAutoscalingPolicy != nil {
			inventory.PolicyARNs = append(inventory.PolicyARNs, *clusterAutoscalingPolicy.Arn)
		}
		if err != nil {
			return &inventory, err
		}
		if c.MessageChan != nil {
			*c.MessageChan <- fmt.Sprintf("IAM policy created: %s\n", *clusterAutoscalingPolicy.PolicyName)
		}
		createdClusterAutoscalingPolicy = *clusterAutoscalingPolicy
	}

	// IAM Roles
	clusterRole, workerRole, err := c.CreateRoles(iamTags)
	if clusterRole != nil {
		inventory.ClusterRole = RoleInventory{
			RoleName:       *clusterRole.RoleName,
			RoleARN:        *clusterRole.Arn,
			RolePolicyARNs: []string{ClusterPolicyARN},
		}
	}
	if workerRole != nil {
		inventory.WorkerRole = RoleInventory{
			RoleName:       *workerRole.RoleName,
			RoleARN:        *workerRole.Arn,
			RolePolicyARNs: getWorkerPolicyARNs(),
		}
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("IAM roles created: [%s %s]\n", *clusterRole.RoleName, *workerRole.RoleName)
	}

	// EKS Cluster
	cluster, err := c.CreateCluster(&mapTags, resourceConfig.Name, resourceConfig.KubernetesVersion,
		*clusterRole.Arn, privateSubnetIDs)
	if cluster != nil {
		inventory.Cluster.ClusterName = *cluster.Name
		inventory.Cluster.ClusterARN = *cluster.Arn
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster created: %s\n", *cluster.Name)
		*c.MessageChan <- fmt.Sprintf("Waiting for EKS cluster to become active: %s\n", *cluster.Name)
	}
	oidcIssuer, err := c.WaitForCluster(*cluster.Name, ClusterConditionCreated)
	if oidcIssuer != "" {
		inventory.Cluster.OIDCProviderURL = oidcIssuer
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster ready: %s\n", *cluster.Name)
	}

	// Node Groups
	var nodeGroupNames []string
	nodeGroups, err := c.CreateNodeGroups(&mapTags, *cluster.Name, resourceConfig.KubernetesVersion,
		*workerRole.Arn, privateSubnetIDs, resourceConfig.InstanceTypes,
		resourceConfig.InitialNodes, resourceConfig.MinNodes, resourceConfig.MaxNodes, resourceConfig.KeyPair)
	if nodeGroups != nil {
		for _, nodeGroup := range *nodeGroups {
			nodeGroupNames = append(nodeGroupNames, *nodeGroup.NodegroupName)
		}
		inventory.NodeGroupNames = nodeGroupNames
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS node group created: %s\n", nodeGroupNames)
		*c.MessageChan <- fmt.Sprintf("Waiting for EKS node group to become active: %s\n", nodeGroupNames)
	}
	if err := c.WaitForNodeGroups(*cluster.Name, nodeGroupNames, NodeGroupConditionCreated); err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS node group ready: %s\n", nodeGroupNames)
	}

	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster creation complete: %s\n", *cluster.Name)
	}

	// OIDC Provider
	oidcProviderARN, err := c.CreateOIDCProvider(iamTags, oidcIssuer)
	if oidcProviderARN != "" {
		inventory.OIDCProviderARN = oidcProviderARN
	}
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("OIDC provider created: %s\n", oidcProviderARN)
	}

	// IAM Role for DNS Management
	if resourceConfig.DNSManagement {
		if createdDNSPolicy.Arn == nil {
			return &inventory, errors.New("no DNS policy ARN to attach to DNS management role")
		}
		dnsManagementRole, err := c.CreateDNSManagementRole(iamTags, *createdDNSPolicy.Arn,
			resourceConfig.AWSAccountID, oidcIssuer, &resourceConfig.DNSManagementServiceAccount)
		if dnsManagementRole != nil {
			inventory.DNSManagementRole = RoleInventory{
				RoleName:       *dnsManagementRole.RoleName,
				RoleARN:        *dnsManagementRole.Arn,
				RolePolicyARNs: []string{*createdDNSPolicy.Arn},
			}
		}
		if err != nil {
			return &inventory, err
		}
	}

	// IAM Role for Cluster Autoscaling
	if resourceConfig.ClusterAutoscaling {
		if createdClusterAutoscalingPolicy.Arn == nil {
			return &inventory, errors.New("no cluster autoscaling policy ARN to attach to cluster autoscaling role")
		}
		clusterAutoscalingRole, err := c.CreateClusterAutoscalingRole(iamTags, *createdClusterAutoscalingPolicy.Arn,
			resourceConfig.AWSAccountID, oidcIssuer, &resourceConfig.ClusterAutoscalingServiceAccount)
		if clusterAutoscalingRole != nil {
			inventory.ClusterAutoscalingRole = RoleInventory{
				RoleName:       *clusterAutoscalingRole.RoleName,
				RoleARN:        *clusterAutoscalingRole.Arn,
				RolePolicyARNs: []string{*clusterAutoscalingRole.PermissionsBoundary.PermissionsBoundaryArn},
			}
		}
		if err != nil {
			return &inventory, err
		}
	}

	// IAM Role for Storage Management
	storageManagementRole, err := c.CreateStorageManagementRole(iamTags, resourceConfig.AWSAccountID,
		oidcIssuer, &resourceConfig.StorageManagementServiceAccount)
	if storageManagementRole != nil {
		inventory.StorageManagementRole = RoleInventory{
			RoleName:       *storageManagementRole.RoleName,
			RoleARN:        *storageManagementRole.Arn,
			RolePolicyARNs: []string{*storageManagementRole.PermissionsBoundary.PermissionsBoundaryArn},
		}
	}
	if err != nil {
		return &inventory, err
	}

	// EBS CSI Addon
	ebsStorageAddon, err := c.CreateEBSStorageAddon(&mapTags, *cluster.Name, *storageManagementRole.Arn)
	if err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EBS storage addon created: %s\n", *ebsStorageAddon.AddonName)
	}

	return &inventory, nil
}

// DeleteResourceStack deletes all the resources in the resource inventory.
func (c *ResourceClient) DeleteResourceStack(resourceInv *ResourceInventory) error {
	// OIDC Provider
	if err := c.DeleteOIDCProvider(resourceInv.OIDCProviderARN); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("OIDC provider deleted: %s\n", resourceInv.OIDCProviderARN)
	}

	// Node Groups
	if err := c.DeleteNodeGroups(resourceInv.Cluster.ClusterName, resourceInv.NodeGroupNames); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Node groups deletion initiated: %s\n", resourceInv.NodeGroupNames)
		*c.MessageChan <- fmt.Sprintf("Waiting for node groups to be deleted: %s\n", resourceInv.NodeGroupNames)
	}
	if err := c.WaitForNodeGroups(resourceInv.Cluster.ClusterName, resourceInv.NodeGroupNames, NodeGroupConditionDeleted); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Node groups deletion complete: %s\n", resourceInv.NodeGroupNames)
	}

	// EKS Cluster
	if err := c.DeleteCluster(resourceInv.Cluster.ClusterName); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster deletion initiated: %s\n", resourceInv.Cluster.ClusterName)
		*c.MessageChan <- fmt.Sprintf("Waiting for EKS cluster to be deleted: %s\n", resourceInv.Cluster.ClusterName)
	}
	if _, err := c.WaitForCluster(resourceInv.Cluster.ClusterName, ClusterConditionDeleted); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster deletion complete: %s\n", resourceInv.Cluster.ClusterName)
	}

	// IAM Roles
	iamRoles := []RoleInventory{
		resourceInv.ClusterRole,
		resourceInv.WorkerRole,
		resourceInv.DNSManagementRole,
		resourceInv.ClusterAutoscalingRole,
		resourceInv.StorageManagementRole,
	}
	if err := c.DeleteRoles(&iamRoles); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("IAM roles deleted: %s\n", iamRoles)
	}

	// IAM Policies
	if err := c.DeletePolicies(resourceInv.PolicyARNs); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("IAM policies deleted: %s\n", resourceInv.PolicyARNs)
	}

	// NAT Gateways
	if err := c.DeleteNATGateways(resourceInv.VPCID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways deleted for VPC with ID: %s\n", resourceInv.VPCID)
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways deletion initiated for VPC with ID: %s\n", resourceInv.VPCID)
		*c.MessageChan <- fmt.Sprintf("Waiting for NAT gateways to be deleted for VPC with ID: %s\n", resourceInv.VPCID)
	}
	if err := c.WaitForNATGateways(resourceInv.VPCID, nil, NATGatewayConditionDeleted); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateway deletion complete for VPC with ID: %s\n", resourceInv.VPCID)
	}

	// Internet Gateway
	if err := c.DeleteInternetGateway(resourceInv.InternetGatewayID, resourceInv.VPCID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Internet gateway deleted: %s\n", resourceInv.InternetGatewayID)
	}

	// Elastic IPs
	if err := c.DeleteElasticIPs(resourceInv.ElasticIPIDs); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Elastic IPs deleted: %s\n", resourceInv.ElasticIPIDs)
	}

	// Subnets
	if err := c.DeleteSubnets(resourceInv.SubnetIDs); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Subnets deleted: %s\n", resourceInv.SubnetIDs)
	}

	// Route Tables
	if err := c.DeleteRouteTables(resourceInv.PrivateRouteTableIDs, resourceInv.PublicRouteTableID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Route tables deleted: [%s %s]\n",
			resourceInv.PrivateRouteTableIDs, resourceInv.PublicRouteTableID)
	}

	// VPC
	if err := c.DeleteVPC(resourceInv.VPCID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("VPC deleted: %s\n", resourceInv.VPCID)
	}

	return nil
}
