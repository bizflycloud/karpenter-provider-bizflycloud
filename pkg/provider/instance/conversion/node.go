package conversion

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance/capacity"
)

const (
	ProviderIDPrefix      = "bizflycloud://"
	NodeLabelInstanceType = "node.kubernetes.io/instance-type"
	NodeLabelRegion       = "topology.kubernetes.io/region"
	NodeLabelZone         = "topology.kubernetes.io/zone"
	NodeAnnotationIsSpot  = "karpenter.bizflycloud.sh/instance-spot"
	NodeCategoryLabel     = "karpenter.bizflycloud.com/node-category"
)

// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func ConvertToNode(instance *Instance, isSpot bool) *corev1.Node {
	resourceCapacity := capacity.GetCapacityFromFlavor(instance.Flavor)
	category := CategorizeFlavor(instance.Flavor)
	
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name,
			Labels: map[string]string{
				corev1.LabelInstanceTypeStable: instance.Flavor,
				corev1.LabelArchStable:         "amd64",
				"karpenter.sh/capacity-type":   "on-demand",
				NodeLabelRegion:                instance.Region,
				NodeLabelZone:                  instance.Zone,
				NodeCategoryLabel:              category,
			},
			Annotations: map[string]string{
				"karpenter.bizflycloud.sh/instance-id": instance.ID,
			},
		},
		Spec: corev1.NodeSpec{
			ProviderID: fmt.Sprintf("%s%s", ProviderIDPrefix, instance.ID),
			Taints: []corev1.Taint{
				{
					Key:    "node.cloudprovider.kubernetes.io/uninitialized",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				},
				{
					Key:    "karpenter.sh/unregistered",
					Value:  "true",
					Effect: corev1.TaintEffectNoSchedule,
				},
			},
		},
		Status: corev1.NodeStatus{
			Capacity:    resourceCapacity,
			Allocatable: resourceCapacity,
			Phase:       corev1.NodePending,
			Conditions: []corev1.NodeCondition{
				{
					Type:    corev1.NodeReady,
					Status:  corev1.ConditionFalse,
					Reason:  "KubeletNotReady",
					Message: "kubelet is starting",
				},
			},
		},
	}

	if isSpot || instance.IsSpot {
		node.Annotations[NodeAnnotationIsSpot] = "true"
	}

	return node
}