package capacity

import (
	"strings"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// GetCapacityFromFlavor parses flavor name to extract CPU and memory capacity
func GetCapacityFromFlavor(flavorName string) corev1.ResourceList {
	// Default capacity
	capacity := corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse("2"),
		corev1.ResourceMemory: resource.MustParse("4Gi"),
		corev1.ResourcePods:   resource.MustParse("110"),
	}

	// Parse flavor name (e.g., "12c_12g_basic" -> 12 CPU, 12GB RAM)
	parts := strings.Split(flavorName, "_")
	if len(parts) >= 2 {
		// Extract CPU
		if strings.HasSuffix(parts[0], "c") {
			cpuStr := strings.TrimSuffix(parts[0], "c")
			if cpu, err := resource.ParseQuantity(cpuStr); err == nil {
				capacity[corev1.ResourceCPU] = cpu
			}
		}

		// Extract Memory
		if strings.HasSuffix(parts[1], "g") {
			memStr := strings.TrimSuffix(parts[1], "g") + "Gi"
			if mem, err := resource.ParseQuantity(memStr); err == nil {
				capacity[corev1.ResourceMemory] = mem
			}
		}
	}

	return capacity
}