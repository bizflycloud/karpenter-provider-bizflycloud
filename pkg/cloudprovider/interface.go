package cloudprovider

import (
    "context"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
    karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// CloudProvider interface defines the contract for cloud provider implementations
type CloudProvider interface {
    cloudprovider.CloudProvider
    Create(context.Context, *karpenterv1.NodeClaim) (*karpenterv1.NodeClaim, error)
    Delete(context.Context, *karpenterv1.NodeClaim) error
    Get(context.Context, string) (*karpenterv1.NodeClaim, error)
    List(context.Context) ([]*karpenterv1.NodeClaim, error)
    GetInstanceTypes(context.Context, *karpenterv1.NodePool) ([]*cloudprovider.InstanceType, error)
    IsDrifted(context.Context, *karpenterv1.NodeClaim) (cloudprovider.DriftReason, error)
}
