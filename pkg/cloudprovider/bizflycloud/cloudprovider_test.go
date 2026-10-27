package bizflycloud

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"

	"github.com/bizflycloud/karpenter-provider-bizflycloud/internal/logging"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
)

func newTestProvider() *cloudProviderImpl {
	return &cloudProviderImpl{log: logging.NewLogger("test"), region: "HN", zone: "HN1"}
}

// nodeClaimFromDefaultNodePool carries the requirements of the add-on's
// default NodePool, which allows several categories and any CPU vendor.
func nodeClaimFromDefaultNodePool() *v1.NodeClaim {
	req := func(key string, op corev1.NodeSelectorOperator, values ...string) v1.NodeSelectorRequirementWithMinValues {
		return v1.NodeSelectorRequirementWithMinValues{NodeSelectorRequirement: corev1.NodeSelectorRequirement{Key: key, Operator: op, Values: values}}
	}
	return &v1.NodeClaim{Spec: v1.NodeClaimSpec{Requirements: []v1.NodeSelectorRequirementWithMinValues{
		req(v1.CapacityTypeLabelKey, corev1.NodeSelectorOpIn, v1.CapacityTypeOnDemand),
		req(instancetype.NodeCategoryLabel, corev1.NodeSelectorOpIn, "basic", "enterprise", "premium"),
		req(instancetype.DiskTypeLabel, corev1.NodeSelectorOpIn, "SSD"),
		req(instancetype.CPUVendorLabel, corev1.NodeSelectorOpExists),
		req(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "HN1"),
	}}}
}

// offeringFor mirrors the offering instancetype.createOfferings builds.
func offeringFor(category, cpuVendor string) *cloudprovider.Offering {
	reqs := []*scheduling.Requirement{
		scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, "HN1"),
		scheduling.NewRequirement(instancetype.NodeCategoryLabel, corev1.NodeSelectorOpIn, category),
		scheduling.NewRequirement(instancetype.DiskTypeLabel, corev1.NodeSelectorOpIn, "SSD"),
	}
	if cpuVendor != "" {
		reqs = append(reqs, scheduling.NewRequirement(instancetype.CPUVendorLabel, corev1.NodeSelectorOpIn, cpuVendor))
	}
	return &cloudprovider.Offering{Requirements: scheduling.NewRequirements(reqs...), Available: true}
}

func TestSetRequiredLabelsFollowTheFlavor(t *testing.T) {
	cases := []struct {
		flavor, category, cpuVendor string
	}{
		{"p4a.16c_32g", "premium", "AMD4"},
		{"p2.4c_8g", "premium", "Intel2"},
		{"e2.8c_16g", "enterprise", "Intel2"},
		{"b1.2c_4g", "basic", ""},
	}
	for _, tc := range cases {
		nodeClaim := nodeClaimFromDefaultNodePool()
		newTestProvider().setRequiredLabels(nodeClaim, tc.flavor)

		labels := nodeClaim.Labels
		if got := labels[instancetype.NodeCategoryLabel]; got != tc.category {
			t.Errorf("%s: node-category = %q, want %q", tc.flavor, got, tc.category)
		}
		if got, ok := labels[instancetype.CPUVendorLabel]; got != tc.cpuVendor || ok != (tc.cpuVendor != "") {
			t.Errorf("%s: cpu-vendor = %q (set %t), want %q", tc.flavor, got, ok, tc.cpuVendor)
		}
		if got := labels[v1.CapacityTypeLabelKey]; got != v1.CapacityTypeOnDemand {
			t.Errorf("%s: capacity-type = %q, want %q", tc.flavor, got, v1.CapacityTypeOnDemand)
		}
		for key, value := range labels {
			if value == "" {
				t.Errorf("%s: label %s is set to an empty value", tc.flavor, key)
			}
		}

		// The drift check (InstanceTypeNotFound) and consolidation pricing
		// both look for an offering compatible with the NodeClaim labels.
		offerings := cloudprovider.Offerings{offeringFor(tc.category, tc.cpuVendor)}
		if !offerings.HasCompatible(scheduling.NewLabelRequirements(labels)) {
			t.Errorf("%s: labels %v match none of the flavor's offerings", tc.flavor, labels)
		}
	}
}

// Consolidation only replaces a node with instance types whose
// WorstLaunchPrice is below the node's price. That price is looked up with
// requirements that carry nothing but the capacity type, so offering labels
// Karpenter does not know as well-known make every price infinite.
func TestOfferingsHaveALaunchPrice(t *testing.T) {
	offering := offeringFor("basic", "Intel2")
	offering.Price = 0.1
	reqs := scheduling.NewNodeSelectorRequirementsWithMinValues(nodeClaimFromDefaultNodePool().Spec.Requirements...)
	if got := (cloudprovider.Offerings{offering}).WorstLaunchPrice(reqs); got != offering.Price {
		t.Fatalf("WorstLaunchPrice() = %v, want %v", got, offering.Price)
	}
}

func TestDeleteWithoutInstanceReportsNotFound(t *testing.T) {
	err := newTestProvider().Delete(context.Background(), &v1.NodeClaim{})
	if !cloudprovider.IsNodeClaimNotFoundError(err) {
		t.Fatalf("Delete() = %v, want a NodeClaimNotFoundError", err)
	}
}
