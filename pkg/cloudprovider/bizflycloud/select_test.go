package bizflycloud

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

func instanceTypeRequirement(names ...string) []v1.NodeSelectorRequirementWithMinValues {
	return []v1.NodeSelectorRequirementWithMinValues{{NodeSelectorRequirement: corev1.NodeSelectorRequirement{
		Key: corev1.LabelInstanceTypeStable, Operator: corev1.NodeSelectorOpIn, Values: names,
	}}}
}

// Seen on staging: launching e2.8c_12g (the "smallest" by CPU/RAM) made
// consolidation replace it 17s after it initialized, with p2.8c_12g.
func TestLaunchesCheapestCandidate(t *testing.T) {
	cases := []struct {
		candidates []string
		want       string
	}{
		{[]string{"e2.8c_12g", "p2.8c_12g", "b2.8c_12g", "b2.12c_12g"}, "b2.8c_12g"},
		{[]string{"e2.8c_12g", "p2.8c_12g", "p2.8c_16g"}, "p2.8c_12g"},
		// a bigger basic flavor is still cheaper than a premium one
		{[]string{"p2.8c_12g", "b2.12c_12g"}, "b2.12c_12g"},
		{[]string{"b4a.8c_12g", "b2.8c_12g"}, "b2.8c_12g"},
	}
	for _, c := range cases {
		got, err := newTestProvider().getSmallestInstanceTypeFromRequirements(instanceTypeRequirement(c.candidates...))
		if err != nil {
			t.Fatalf("%v: unexpected error: %v", c.candidates, err)
		}
		if got.Name != c.want {
			t.Errorf("%v: launched %s, want %s", c.candidates, got.Name, c.want)
		}
	}
}
