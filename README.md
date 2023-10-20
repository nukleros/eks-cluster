# eks-cluster

A CLI and Go library to create and delete EKS clusters in AWS.

## Prerequisites

[Local configuration for
AWS](https://docs.aws.amazon.com/cli/latest/userguide/cli-configure-quickstart.html).

This eks-cluster CLI doesn't require the AWS CLI, however it does need the config
and credentials - that are usually set up with the AWS CLI.

## Build

Clone this repo, then:

```bash
go build
```

## Quickstart

Create a cluster:

```bash
./eks-cluster create -c sample/eks-cluster-config.yaml
```

Note: if creating and deleting clusters one at a time, it is safe to use the
default inventory filename `eks-cluster-inventory.json`.  However, if you create
more than one before deleting any, be sure to pass in a distinct inventory file
name for each cluster so that you can delete the resources later.

Delete the cluster:

```bash
./eks-cluster delete
```

TODO: EC2 instance remote access

