# Karpenter Provider for BizflyCloud

This repository contains the BizflyCloud provider implementation for [Karpenter](https://karpenter.sh), a Kubernetes-native autoscaling solution. The provider enables Karpenter to provision and manage nodes on BizflyCloud infrastructure.

## Overview

Karpenter is a Kubernetes autoscaler that automatically provisions new nodes in response to unschedulable pods. This provider allows Karpenter to work with BizflyCloud's infrastructure, enabling automatic node provisioning and management.

### Key Features

- Automatic node provisioning based on pod requirements
- Integration with BizflyCloud's API
- Customizable node configurations through NodeClasses
- Drift detection and remediation
- Support for multiple regions and zones
- Resource tagging and management

## Prerequisites

- Kubernetes cluster running on BizflyCloud
- kubectl configured to communicate with your cluster
- BizflyCloud API credentials
- Karpenter v0.32.0 or later

## Installation

### 1. Create the Karpenter namespace

```bash
kubectl create namespace karpenter
```

### 2. Create BizflyCloud credentials secret

Create a secret containing your BizflyCloud credentials:

```bash
kubectl create secret generic bizflycloud-credentials \
  --namespace karpenter \
  --from-literal=api-url=https://manage.bizflycloud.vn/api \
  --from-literal=auth-method=application_credential \
  --from-literal=app-cred-id=YOUR_APP_CREDENTIAL_ID \
  --from-literal=app-cred-secret=YOUR_APP_CREDENTIAL_SECRET \
  --from-literal=region=YOUR_REGION \
  --from-literal=project-id=YOUR_PROJECT_ID
```

### 3. Install the CRDs

```bash
kubectl apply -f https://raw.githubusercontent.com/bizflycloud/karpenter-provider-bizflycloud/refs/heads/main/config/crd/karpenter.bizflycloud.com_bizflycloudnodeclasses.yaml

kubectl apply -f https://raw.githubusercontent.com/aws/karpenter/main/pkg/apis/crds/karpenter.sh_nodeclaims.yaml

kubectl apply -f https://raw.githubusercontent.com/aws/karpenter/main/pkg/apis/crds/karpenter.sh_nodepools.yaml
```

### 4. Install Karpenter with BizflyCloud provider

```bash
kubectl apply -f https://raw.githubusercontent.com/bizflycloud/karpenter-provider-bizflycloud/refs/heads/main/config/crd/karpenter-deployment.yaml
```

### 5. Create a default NodeClass

```bash
kubectl apply -f https://raw.githubusercontent.com/bizflycloud/karpenter-provider-bizflycloud/refs/heads/main/config/crd/node-demo.yaml
```

## Configuration

### NodeClass Configuration

NodeClasses define the configuration for nodes that Karpenter will provision. Here's an example:

```yaml
apiVersion: karpenter.bizflycloud.com/v1
kind: BizflyCloudNodeClass
metadata:
  name: default
spec:
  diskTypes:
  - HDD
  - SSD
  metadataOptions:
    type: template
  networkPlans:
  - free_datatransfer
  - free_bandwidth
  nodeCategories:
  - basic
  - premium
  - enterprise
  - dedicated
  region: HN
  rootDiskSize: 40
  sshKeyName: quanlm
  tags:
  - karpenter-managed=true
  template: Ubuntu 24.04 LTS
  vpcNetworkIds:
  - <your own VPC network ID>
  zones:
  - HN2
  - HN1
```

## Usage

### Creating a NodePool

```yaml
apiVersion: karpenter.sh/v1
kind: NodePool
metadata:
  labels:
    karpenter.bizflycloud.com/disk-type: HDD
    topology.kubernetes.io/zone: HN1
  name: default-2
spec:
  disruption:
    budgets:
    - nodes: "1"
      reasons:
      - Empty
      - Underutilized
    consolidateAfter: 30s
    consolidationPolicy: WhenEmptyOrUnderutilized
  template:
    spec:
      expireAfter: 720h
      nodeClassRef:
        group: karpenter.bizflycloud.com
        kind: BizflyCloudNodeClass
        name: default
      requirements:
      - key: kubernetes.io/arch
        operator: In
        values:
        - amd64
      - key: karpenter.sh/capacity-type
        operator: In
        values:
        - saving-plane
      - key: karpenter.bizflycloud.com/node-category
        operator: In
        values:
        - enterprise
      - key: karpenter.bizflycloud.com/disk-type
        operator: In
        values:
        - HDD
      - key: bizflycloud.com/kubernetes-version
        operator: In
        values:
        - v1.31.5
      - key: topology.kubernetes.io/zone
        operator: In
        values:
        - HN1
      startupTaints:
      - effect: NoExecute
        key: karpenter.sh/unregistered
        value: "true"
      - effect: NoSchedule
        key: karpenter.sh/disrupted
      - effect: NoSchedule
        key: node.cloudprovider.kubernetes.io/uninitialized
        value: "true"
      - effect: NoSchedule
        key: node.kubernetes.io/not-ready
      - effect: NoExecute
        key: node.kubernetes.io/not-ready
      - effect: NoSchedule
        key: node.cilium.io/agent-not-ready
      terminationGracePeriod: 30s
```

### Testing the Installation

Deploy a test workload to verify the installation:

```bash
kubectl apply -f https://raw.githubusercontent.com/bizflycloud/karpenter-provider-bizflycloud/refs/heads/main/config/crd/test-deployment.yaml
```

## Architecture

The BizflyCloud provider for Karpenter consists of several key components:

1. **Cloud Provider**: Implements the Karpenter cloud provider interface for BizflyCloud
2. **Instance Provider**: Handles instance lifecycle operations
3. **Instance Type Provider**: Manages available instance types and their configurations
4. **Drift Detection**: Monitors and remediates configuration drift
5. **Node Conversion**: Converts between BizflyCloud instances and Kubernetes nodes

## Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

## License

This project is licensed under the Apache License 2.0 - see the [LICENSE](LICENSE) file for details.
