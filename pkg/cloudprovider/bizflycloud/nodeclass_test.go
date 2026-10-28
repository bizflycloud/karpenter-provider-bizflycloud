package bizflycloud

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
)

// A NodePool that references another NodeClass must launch with that one,
// not with "default".
func TestResolveNodeClassFollowsTheNodeClaimLabel(t *testing.T) {
	// pkg/apis/v1 registers its types in the client-go scheme.
	kube := fake.NewClientBuilder().WithScheme(scheme.Scheme).WithObjects(
		&v1bizfly.BizflyCloudNodeClass{ObjectMeta: metav1.ObjectMeta{Name: "default"}},
		&v1bizfly.BizflyCloudNodeClass{ObjectMeta: metav1.ObjectMeta{Name: "gpu"}},
	).Build()
	p := newTestProvider()
	p.kubeClient = kube

	if nodeClassLabel != "karpenter.bizflycloud.com/bizflycloudnodeclass" {
		t.Fatalf("nodeClassLabel = %q", nodeClassLabel)
	}
	for labels, want := range map[string]string{"gpu": "gpu", "": "default"} {
		node := &corev1.Node{ObjectMeta: metav1.ObjectMeta{Labels: map[string]string{}}}
		if labels != "" {
			node.Labels[nodeClassLabel] = labels
		}
		nodeClass, err := p.resolveNodeClassFromNodeSpec(context.Background(), node)
		if err != nil {
			t.Fatalf("label %q: %v", labels, err)
		}
		if nodeClass.Name != want {
			t.Errorf("label %q: NodeClass = %q, want %q", labels, nodeClass.Name, want)
		}
	}
}
