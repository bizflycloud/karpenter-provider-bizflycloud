package capacity

import (
    "strings"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
)

// GetCapacityFromFlavor parses flavor name to extract CPU and memory capacity
// Supports new format: p4a.2c_2g, b2.4c_8g (tier + cpu vendor . cpu_ram)
func GetCapacityFromFlavor(flavorName string) corev1.ResourceList {
    // Default capacity
    capacity := corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse("2"),
        corev1.ResourceMemory: resource.MustParse("4Gi"),
        corev1.ResourcePods:   resource.MustParse("110"),
    }

    // New format: p4a.2c_2g or b2.4c_8g
    if strings.Contains(flavorName, ".") {
        parts := strings.Split(flavorName, ".")
        if len(parts) == 2 {
            // Parse the part after the dot: "2c_2g"
            cpuRamParts := strings.Split(parts[1], "_")
            if len(cpuRamParts) >= 2 {
                // Extract CPU
                if strings.HasSuffix(cpuRamParts[0], "c") {
                    cpuStr := strings.TrimSuffix(cpuRamParts[0], "c")
                    if cpu, err := resource.ParseQuantity(cpuStr); err == nil {
                        capacity[corev1.ResourceCPU] = cpu
                    }
                }

                // Extract Memory
                if strings.HasSuffix(cpuRamParts[1], "g") {
                    memStr := strings.TrimSuffix(cpuRamParts[1], "g") + "Gi"
                    if mem, err := resource.ParseQuantity(memStr); err == nil {
                        capacity[corev1.ResourceMemory] = mem
                    }
                }
            }
        }
    }

    return capacity
}
