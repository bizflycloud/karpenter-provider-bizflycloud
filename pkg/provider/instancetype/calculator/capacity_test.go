package calculator

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// Allocatable reported by real BKE workers (kubelet with the reservations of
// bke-api cluster_join_controller).
func TestCreateAllocatableMatchesRealNodes(t *testing.T) {
	cases := []struct {
		flavor     string
		vcpus      int
		ramMB      int
		realCPU    string
		realMemory string
	}{
		{"p4a.2c_2g", 2, 2048, "1930m", "1408788Ki"},
		{"p4a.16c_32g", 16, 32768, "15890m", "29069500Ki"},
	}
	maxGap := resource.MustParse("300Mi")
	for _, tc := range cases {
		allocatable := NewCapacityCalculator().CreateAllocatable(tc.vcpus, tc.ramMB)

		cpu := allocatable[corev1.ResourceCPU]
		if want := resource.MustParse(tc.realCPU); cpu.Cmp(want) != 0 {
			t.Errorf("%s: cpu = %s, want %s", tc.flavor, cpu.String(), want.String())
		}

		// Never promise more memory than the kubelet allows, or pods packed
		// onto an in-flight NodeClaim do not fit once the Node registers.
		memory := allocatable[corev1.ResourceMemory]
		real := resource.MustParse(tc.realMemory)
		if memory.Cmp(real) > 0 {
			t.Errorf("%s: memory = %s, above the real %s", tc.flavor, memory.String(), real.String())
		}
		gap := real.DeepCopy()
		gap.Sub(memory)
		if gap.Cmp(maxGap) > 0 {
			t.Errorf("%s: memory = %s, %s below the real %s", tc.flavor, memory.String(), gap.String(), real.String())
		}
	}
}

func TestReservedMemoryTiers(t *testing.T) {
	cases := map[int64]int64{
		0:          0,
		4 * 1024:   1024, // 25% of 4Gi
		8 * 1024:   1843, // + 20% of 4Gi
		16 * 1024:  2662, // + 10% of 8Gi
		112 * 1024: 8561, // + 6% of 96Gi
		128 * 1024: 8888, // + 2% of 16Gi
	}
	for capacityMi, want := range cases {
		if got := reservedMemoryMi(capacityMi); got != want {
			t.Errorf("reservedMemoryMi(%d) = %d, want %d", capacityMi, got, want)
		}
	}
}
