package lifecycle

import (
	"testing"

	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func TestManagerLaunchesCheapestCandidate(t *testing.T) {
	m := &Manager{Log: logr.Discard()}
	nodeClaim := &karpenterv1.NodeClaim{Spec: karpenterv1.NodeClaimSpec{Requirements: []karpenterv1.NodeSelectorRequirementWithMinValues{{
		NodeSelectorRequirement: corev1.NodeSelectorRequirement{
			Key: corev1.LabelInstanceTypeStable, Operator: corev1.NodeSelectorOpIn,
			Values: []string{"e2.8c_12g", "p2.8c_12g", "b2.8c_12g"},
		},
	}}}}
	got, err := m.getSmallestInstanceType(nodeClaim)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "b2.8c_12g" {
		t.Fatalf("launched %s, want b2.8c_12g", got)
	}
}
