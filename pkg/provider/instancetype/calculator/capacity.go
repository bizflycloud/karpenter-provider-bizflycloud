package calculator

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// CapacityCalculator handles resource capacity calculations
type CapacityCalculator struct{}

// NewCapacityCalculator creates a new capacity calculator
func NewCapacityCalculator() *CapacityCalculator {
	return &CapacityCalculator{}
}

// CalculatePodCapacity calculates realistic pod capacity based on instance size
func (c *CapacityCalculator) CalculatePodCapacity(vcpus int, ramMB int) int64 {
	// Base pod capacity calculation
	var maxPods int64

	// Calculate based on CPU and memory
	cpuBasedPods := int64(vcpus * 8)      // 8 pods per CPU core
	memoryBasedPods := int64(ramMB / 256) // 256MB per pod minimum

	// Take the minimum of the two
	maxPods = cpuBasedPods
	if memoryBasedPods < cpuBasedPods {
		maxPods = memoryBasedPods
	}

	// Apply reasonable limits based on instance size
	switch {
	case vcpus <= 2:
		if maxPods > 20 {
			maxPods = 20
		}
	case vcpus <= 4:
		if maxPods > 40 {
			maxPods = 40
		}
	case vcpus <= 8:
		if maxPods > 80 {
			maxPods = 80
		}
	default:
		if maxPods > 110 {
			maxPods = 110
		}
	}

	// Minimum of 5 pods for any instance
	if maxPods < 5 {
		maxPods = 5
	}

	return maxPods
}

// CalculateOverhead calculates resource overhead for system components
func (c *CapacityCalculator) CalculateOverhead(vcpus int, ramMB int) cloudprovider.InstanceTypeOverhead {
	// Much more conservative overhead for small instances
	var cpuOverhead, memoryOverhead, podOverhead string

	cpuOverhead = "500m"      // Increased from 300m
	memoryOverhead = "2Gi"    // Increased from 1Gi  
	podOverhead = "10"        // Increased from 10
	
	return cloudprovider.InstanceTypeOverhead{
		KubeReserved: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(cpuOverhead),
			corev1.ResourceMemory: resource.MustParse(memoryOverhead),
			corev1.ResourcePods:   resource.MustParse(podOverhead),
		},
		SystemReserved: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),  // Increased from 25m
			corev1.ResourceMemory: resource.MustParse("256Mi"), // Increased from 64Mi
			corev1.ResourcePods:   resource.MustParse("2"),     // Increased from 1
		},
		EvictionThreshold: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("100m"),  // Increased from 25m
			corev1.ResourceMemory: resource.MustParse("256Mi"), // Increased from 64Mi
			corev1.ResourcePods:   resource.MustParse("2"),     // Increased from 1
		},
	}
	
}

// CreateCapacity creates a ResourceList for an instance type
func (c *CapacityCalculator) CreateCapacity(vcpus int, ramMB int) corev1.ResourceList {
	maxPods := c.CalculatePodCapacity(vcpus, ramMB)

	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", vcpus)),
		corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", ramMB)),
		corev1.ResourcePods:   resource.MustParse(fmt.Sprintf("%d", maxPods)),
	}
}