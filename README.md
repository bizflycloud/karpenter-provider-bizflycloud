# Karpenter Provider for BizFly Cloud

This project implements a Karpenter provider for BizFly Cloud, enabling Kubernetes auto-scaling capabilities on BizFly Cloud infrastructure.

## Overview

Karpenter is a Kubernetes node auto-scaling project that helps improve application availability and cluster efficiency. This provider extends Karpenter's capabilities to BizFly Cloud, allowing dynamic provisioning and termination of compute resources based on application demands.

## Prerequisites

- Kubernetes cluster running on BizFly Cloud
- Karpenter core installed in your cluster
- BizFly Cloud API credentials

## Installation

### Using Helm

```bash
# Add the helm repository
helm repo add karpenter-bizflycloud https://bizflycloud.github.io/karpenter-provider-bizflycloud

# Install the chart
helm install karpenter-provider-bizflycloud karpenter-bizflycloud/karpenter-provider-bizflycloud \
  --namespace karpenter \
  --create-namespace
```

### Using kubectl

```bash
kubectl apply -f https://raw.githubusercontent.com/bizflycloud/karpenter-provider-bizflycloud/main/config/install.yaml
```

## Configuration

1. Create a secret with your BizFly Cloud credentials:

```yaml
apiVersion: v1
kind: Secret
metadata:
  name: bizflycloud-credentials
  namespace: karpenter
type: Opaque
stringData:
  email: your-email@example.com
  password: your-password
```

2. Create a BizflyCloudProviderConfig:

```yaml
apiVersion: karpenter.bizflycloud.vn/v1alpha1
kind: ProviderConfig
metadata:
  name: default
spec:
  region: HN1
  secretRef:
    name: bizflycloud-credentials
    namespace: karpenter
```

3. Create a Karpenter Provisioner with BizFly Cloud settings:

```yaml
apiVersion: karpenter.sh/v1alpha5
kind: Provisioner
metadata:
  name: default
spec:
  providerRef:
    name: default
    apiVersion: karpenter.bizflycloud.vn/v1alpha1
    kind: ProviderConfig
  requirements:
    - key: "node.kubernetes.io/instance-type"
      operator: In
      values: ["4c_8g", "4c_8g_enterprise"]
    - key: "topology.kubernetes.io/zone"
      operator: In
      values: ["HN1", "HN2"]
  limits:
    resources:
      cpu: 100
      memory: 100Gi
  ttlSecondsAfterEmpty: 30
```

## Development

### Building from Source

```bash
# Build binary
make build

# Build and push Docker image
make docker-build docker-push IMG=your-registry/karpenter-provider-bizflycloud:tag
```

## License

Apache 2.0 License