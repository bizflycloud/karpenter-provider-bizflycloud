# Karpenter Provider for BizFly Cloud

This provider enables [Karpenter](https://karpenter.sh) to provision nodes in BizFly Cloud.

## Prerequisites

- Kubernetes cluster running on BizFly Cloud
- Karpenter v0.27.0 or later installed in your cluster
- `kubectl` configured to access your cluster
- BizFly Cloud account credentials

## Installation

1. First, create the namespace if it doesn't exist:

```bash
kubectl create namespace karpenter
```

2. Create the BizFly Cloud credentials secret:

```bash
# Edit config/samples/secret.yaml to add your credentials first
kubectl apply -f config/samples/secret.yaml
```

3. Install the CRDs:

```bash
kubectl apply -f config/crd/
```

4. Deploy the provider:

```bash
kubectl apply -f config/deployment.yaml
```

5. Create the provider configuration:

```bash
# Edit config/samples/minimal-providerconfig.yaml to match your environment
kubectl apply -f config/samples/minimal-providerconfig.yaml
```

6. Create a provisioner:

```bash
# Edit config/samples/minimal-provisioner.yaml to match your requirements
kubectl apply -f config/samples/minimal-provisioner.yaml
```

## Configuration

### Provider Configuration

The minimal provider configuration requires:

```yaml
apiVersion: bizflycloud.karpenter.sh/v1
kind: ProviderConfig
metadata:
  name: default
spec:
  region: HN1  # Your BizFly Cloud region
  secretRef:
    name: bizflycloud-credentials
    namespace: karpenter
  cloudConfig:
    apiEndpoint: "https://manage.bizflycloud.vn/api"
  imageConfig:
    imageID: "ubuntu-20.04"
    rootDiskSize: 20
```

### Provisioner Configuration

A basic provisioner configuration looks like:

```yaml
apiVersion: karpenter.sh/v1
kind: Provisioner
metadata:
  name: default
spec:
  providerRef:
    name: default
    apiVersion: bizflycloud.karpenter.sh/v1
    kind: ProviderConfig
  
  requirements:
    - key: "node.kubernetes.io/instance-type"
      operator: In
      values: ["4c_8g", "8c_16g"]
    
    - key: "topology.kubernetes.io/zone"
      operator: In
      values: ["HN1-a", "HN1-b"]
  
  limits:
    resources:
      cpu: 20
      memory: 100Gi
  
  ttlSecondsAfterEmpty: 30
```

## Verification

To verify the installation:

1. Check the provider deployment:
```bash
kubectl get pods -n karpenter
```

2. Check the provider configuration:
```bash
kubectl get providerconfig
```

3. Check the provisioner:
```bash
kubectl get provisioner
```

## Troubleshooting

Check the provider logs:
```bash
kubectl logs -n karpenter -l app.kubernetes.io/name=karpenter-provider-bizflycloud
```