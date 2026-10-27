package calculator

import (
	"fmt"
	"math"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/utils/resources"
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

const (
	// systemReservedCPUMilli and systemReservedMemoryMi are the system-reserved
	// values the BKE join flow writes to the kubelet config; the rest of the
	// reservation goes to kube-reserved.
	systemReservedCPUMilli = 20
	systemReservedMemoryMi = 100
	// evictionMemoryMi is the kubelet memory.available hard eviction threshold.
	evictionMemoryMi = 100
	// kernelReservedBaseMi and kernelReservedRatio model the RAM the guest
	// kernel keeps out of MemTotal. Measured 3.9% on 2G and 2.1% on 32G
	// flavors, so this errs on the low side of the real allocatable.
	kernelReservedBaseMi = 64
	kernelReservedRatio  = 0.025
)

// memoryCapacityMi estimates the node's MemTotal, the memory capacity the
// kubelet reports, for a flavor with ramMB of RAM.
func memoryCapacityMi(ramMB int) int64 {
	return int64(float64(ramMB) - kernelReservedBaseMi - kernelReservedRatio*float64(ramMB))
}

// reservedCPUMilli mirrors the CPU reservation of the BKE join flow:
// 6% of the first core, 1% of the second, 0.5% of cores 3-4, 0.25% above.
func reservedCPUMilli(vcpus int) int64 {
	cores := float64(vcpus)
	reserved := 0.06*math.Min(math.Max(0, cores), 1) +
		0.01*math.Min(math.Max(0, cores-1), 1) +
		0.005*math.Min(math.Max(0, cores-2), 2) +
		0.0025*math.Max(0, cores-4)
	return int64(math.Round(reserved * 1000))
}

// reservedMemoryMi mirrors the memory reservation of the BKE join flow:
// 25% of the first 4Gi, 20% of the next 4Gi, 10% of the next 8Gi, 6% up to
// 112Gi and 2% above, computed on the node's memory capacity.
func reservedMemoryMi(capacityMi int64) int64 {
	mem := float64(capacityMi)
	reserved := 0.25*math.Min(math.Max(0, mem), 4*1024) +
		0.20*math.Min(math.Max(0, mem-4*1024), 4*1024) +
		0.10*math.Min(math.Max(0, mem-8*1024), 8*1024) +
		0.06*math.Min(math.Max(0, mem-16*1024), 96*1024) +
		0.02*math.Max(0, mem-112*1024)
	return int64(math.Round(reserved))
}

// CalculateOverhead returns the kubelet reservations a BKE worker of this size
// gets, so that capacity minus overhead matches the node's real allocatable.
func (c *CapacityCalculator) CalculateOverhead(vcpus int, ramMB int) cloudprovider.InstanceTypeOverhead {
	cpuMilli := reservedCPUMilli(vcpus)
	memoryMi := reservedMemoryMi(memoryCapacityMi(ramMB))

	return cloudprovider.InstanceTypeOverhead{
		KubeReserved: corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewMilliQuantity(max(cpuMilli-systemReservedCPUMilli, 0), resource.DecimalSI),
			corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", max(memoryMi-systemReservedMemoryMi, 0))),
		},
		SystemReserved: corev1.ResourceList{
			corev1.ResourceCPU:    *resource.NewMilliQuantity(systemReservedCPUMilli, resource.DecimalSI),
			corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", systemReservedMemoryMi)),
		},
		EvictionThreshold: corev1.ResourceList{
			corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", evictionMemoryMi)),
		},
	}
}

// CreateCapacity creates a ResourceList for an instance type
func (c *CapacityCalculator) CreateCapacity(vcpus int, ramMB int) corev1.ResourceList {
	maxPods := c.CalculatePodCapacity(vcpus, ramMB)

	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", vcpus)),
		corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memoryCapacityMi(ramMB))),
		corev1.ResourcePods:   resource.MustParse(fmt.Sprintf("%d", maxPods)),
	}
}

// CreateAllocatable returns what the kubelet of a fresh BKE worker of this
// size reports as allocatable: capacity minus the CalculateOverhead total.
func (c *CapacityCalculator) CreateAllocatable(vcpus int, ramMB int) corev1.ResourceList {
	return resources.Subtract(c.CreateCapacity(vcpus, ramMB), c.CalculateOverhead(vcpus, ramMB).Total())
}
