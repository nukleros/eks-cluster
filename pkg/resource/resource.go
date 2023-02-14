package resource

import (
	"context"
	"errors"
	"fmt"

	"github.com/aws/aws-sdk-go-v2/aws"
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
func (c *ResourceClient) CreateResourceStack(r *ResourceConfig) (*ResourceInventory, error) {
	inventory := ResourceInventory{Region: r.Region}

	// Tags
	ec2Tags := CreateEC2Tags(r.Name, r.Tags)
	iamTags := CreateIAMTags(r.Name, r.Tags)
	mapTags := CreateMapTags(r.Name, r.Tags)

	// VPC
	vpc, err := c.CreateVPC(ec2Tags, r.ClusterCIDR, r.Name)
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
	igw, err := c.CreateInternetGateway(ec2Tags, *vpc.VpcId, r.Name)
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
	privateSubnets, publicSubnets, err := c.CreateSubnets(ec2Tags, *vpc.VpcId, r.Name, &r.AvailabilityZones)
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
	if err := c.CreateNATGateways(ec2Tags, r.AvailabilityZones, elasticIPIDs); err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways created for subnets: %s\n", privateSubnetIDs)
		*c.MessageChan <- fmt.Sprintf("Waiting for NAT gateways to become active for subnets: %s\n", privateSubnetIDs)
	}
	if err := c.WaitForNATGateways(*vpc.VpcId, &r.AvailabilityZones, NATGatewayConditionCreated); err != nil {
		return &inventory, err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways ready for subnets: %s\n", privateSubnetIDs)
	}

	// Route Tables
	var privateRouteTableIDs []string
	privateRouteTables, publicRouteTable, err := c.CreateRouteTables(ec2Tags,
		*vpc.VpcId, *igw.InternetGatewayId, &r.AvailabilityZones)
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

	// IAM Policy for DNS management
	if r.DNSManagement {
		dnsPolicy, err := c.CreatePolicy(iamTags)
		if dnsPolicy != nil {
			inventory.DNSManagementPolicyARN = *dnsPolicy.Arn
		}
		if err != nil {
			return &inventory, err
		}
		if c.MessageChan != nil {
			*c.MessageChan <- fmt.Sprintf("IAM policy created: %s\n", *dnsPolicy.PolicyName)
		}
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
	cluster, err := c.CreateCluster(&mapTags, r.Name, r.KubernetesVersion,
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
	nodeGroups, err := c.CreateNodeGroups(&mapTags, *cluster.Name, r.KubernetesVersion,
		*workerRole.Arn, privateSubnetIDs, r.InstanceTypes, r.MinNodes, r.MaxNodes, r.KeyPair)
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
	if r.DNSManagement {
		if inventory.DNSManagementPolicyARN == "" {
			return &inventory, errors.New("no DNS policy ARN to attach to DNS management role")
		}
		dnsManagementRole, err := c.CreateDNSManagementRole(iamTags, inventory.DNSManagementPolicyARN,
			r.AWSAccountID, oidcIssuer, &r.DNSManagementServiceAccount)
		if dnsManagementRole != nil {
			inventory.DNSManagementRole = RoleInventory{
				RoleName:       *dnsManagementRole.RoleName,
				RoleARN:        *dnsManagementRole.Arn,
				RolePolicyARNs: []string{inventory.DNSManagementPolicyARN},
			}
		}
		if err != nil {
			return &inventory, err
		}
	}

	return &inventory, nil
}

// DeleteResourceStack deletes all the resources in the resource inventory.
func (c *ResourceClient) DeleteResourceStack(r *ResourceInventory) error {
	// OIDC Provider
	if err := c.DeleteOIDCProvider(r.OIDCProviderARN); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("OIDC provider deleted: %s\n", r.OIDCProviderARN)
	}

	// Node Groups
	if err := c.DeleteNodeGroups(r.Cluster.ClusterName, r.NodeGroupNames); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Node groups deletion initiated: %s\n", r.NodeGroupNames)
		*c.MessageChan <- fmt.Sprintf("Waiting for node groups to be deleted: %s\n", r.NodeGroupNames)
	}
	if err := c.WaitForNodeGroups(r.Cluster.ClusterName, r.NodeGroupNames, NodeGroupConditionDeleted); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Node groups deletion complete: %s\n", r.NodeGroupNames)
	}

	// EKS Cluster
	if err := c.DeleteCluster(r.Cluster.ClusterName); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster deletion initiated: %s\n", r.Cluster.ClusterName)
		*c.MessageChan <- fmt.Sprintf("Waiting for EKS cluster to be deleted: %s\n", r.Cluster.ClusterName)
	}
	if _, err := c.WaitForCluster(r.Cluster.ClusterName, ClusterConditionDeleted); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("EKS cluster deletion complete: %s\n", r.Cluster.ClusterName)
	}

	// IAM Roles
	iamRoles := []RoleInventory{r.ClusterRole, r.WorkerRole, r.DNSManagementRole}
	//if err := c.DeleteRoles(&r.ClusterRole, &r.WorkerRole); err != nil {
	if err := c.DeleteRoles(&iamRoles); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("IAM roles deleted: [%s %s]\n", &r.ClusterRole, &r.WorkerRole)
	}

	// IAM Policy
	if err := c.DeletePolicy(r.DNSManagementPolicyARN); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("IAM policy deleted: %s\n", r.DNSManagementPolicyARN)
	}

	// NAT Gateways
	if err := c.DeleteNATGateways(r.VPCID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways deleted for VPC with ID: %s\n", r.VPCID)
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateways deletion initiated for VPC with ID: %s\n", r.VPCID)
		*c.MessageChan <- fmt.Sprintf("Waiting for NAT gateways to be deleted for VPC with ID: %s\n", r.VPCID)
	}
	if err := c.WaitForNATGateways(r.VPCID, nil, NATGatewayConditionDeleted); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("NAT gateway deletion complete for VPC with ID: %s\n", r.VPCID)
	}

	// Internet Gateway
	if err := c.DeleteInternetGateway(r.InternetGatewayID, r.VPCID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Internet gateway deleted: %s\n", r.InternetGatewayID)
	}

	// Elastic IPs
	if err := c.DeleteElasticIPs(r.ElasticIPIDs); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Elastic IPs deleted: %s\n", r.ElasticIPIDs)
	}

	// Subnets
	if err := c.DeleteSubnets(r.SubnetIDs); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Subnets deleted: %s\n", r.SubnetIDs)
	}

	// Route Tables
	if err := c.DeleteRouteTables(r.PrivateRouteTableIDs, r.PublicRouteTableID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("Route tables deleted: [%s %s]\n",
			r.PrivateRouteTableIDs, r.PublicRouteTableID)
	}

	// VPC
	if err := c.DeleteVPC(r.VPCID); err != nil {
		return err
	}
	if c.MessageChan != nil {
		*c.MessageChan <- fmt.Sprintf("VPC deleted: %s\n", r.VPCID)
	}

	return nil
}
