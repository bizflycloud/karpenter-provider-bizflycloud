package bizflycloud

import (
	"context"

	corev1 "k8s.io/api/core/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
)

// CloudProvider defines the interface for BizflyCloud operations
type CloudProvider interface {
	cloudprovider.CloudProvider

	// BizflyCloud-specific methods
	CreateNode(ctx context.Context, nodeSpec *corev1.Node) (*corev1.Node, error)
	DeleteNode(ctx context.Context, node *corev1.Node) error
	DetectNodeDrift(ctx context.Context, node *corev1.Node) (bool, error)
	ReconcileNodeDrift(ctx context.Context, node *corev1.Node) error
	GetAvailableZones(ctx context.Context) ([]string, error)
	GetSpotInstances(ctx context.Context) (bool, error)
}

// InstanceProvider defines the interface for instance operations
type InstanceProvider interface {
	CreateInstance(ctx context.Context, nodeClaim *v1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (InstanceInfo, error)
	GetInstance(ctx context.Context, instanceID string) (InstanceInfo, error)
	DeleteInstance(ctx context.Context, instanceID string) error
	ListInstances(ctx context.Context) ([]InstanceInfo, error)
}

// InstanceTypeProvider defines the interface for instance type operations
type InstanceTypeProvider interface {
	List(ctx context.Context, nodeClass *v1bizfly.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
	GetSupportedInstanceTypes(ctx context.Context) ([]string, error)
}

// DriftDetector defines the interface for drift detection
type DriftDetector interface {
	IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error)
	GetDriftReasons() []string
}

// InstanceInfo represents instance information
type InstanceInfo interface {
	GetID() string
	GetName() string
	GetStatus() string
	GetRegion() string
	GetZone() string
	GetInstanceType() string
	GetIPAddress() string
	IsSpot() bool
	GetTags() map[string]string
	GetCreatedAt() string
}
