package instancetype

import (
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// calculateOverhead calculates overhead for an instance type
func (d *defaultProvider) calculateOverhead(vcpus int, ramMB int) cloudprovider.InstanceTypeOverhead {
    var cpuOverhead, memoryOverhead, podOverhead string
    
    switch {
    case vcpus <= 4:
        cpuOverhead = "50m"
        memoryOverhead = "128Mi"
        podOverhead = "3"
    case vcpus <= 8:
        cpuOverhead = "100m"
        memoryOverhead = "256Mi"
        podOverhead = "5"
    case vcpus <= 16:
        cpuOverhead = "200m"
        memoryOverhead = "512Mi"
        podOverhead = "8"
    default:
        cpuOverhead = "300m"
        memoryOverhead = "1Gi"
        podOverhead = "10"
    }
    
    return cloudprovider.InstanceTypeOverhead{
        KubeReserved: corev1.ResourceList{
            corev1.ResourceCPU:    resource.MustParse(cpuOverhead),
            corev1.ResourceMemory: resource.MustParse(memoryOverhead),
            corev1.ResourcePods:   resource.MustParse(podOverhead),
        },
        SystemReserved: corev1.ResourceList{
            corev1.ResourceCPU:    resource.MustParse("25m"),
            corev1.ResourceMemory: resource.MustParse("64Mi"),
            corev1.ResourcePods:   resource.MustParse("2"),
        },
        EvictionThreshold: corev1.ResourceList{
            corev1.ResourceCPU:    resource.MustParse("25m"),
            corev1.ResourceMemory: resource.MustParse("64Mi"),
            corev1.ResourcePods:   resource.MustParse("1"),
        },
    }
}

// calculatePodCapacity calculates realistic pod capacity
func (d *defaultProvider) calculatePodCapacity(vcpus int, ramMB int) int64 {
    cpuBasedPods := int64(vcpus * 8)
    memoryBasedPods := int64(ramMB / 256)
    
    maxPods := cpuBasedPods
    if memoryBasedPods < cpuBasedPods {
        maxPods = memoryBasedPods
    }
    
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
    
    if maxPods < 5 {
        maxPods = 5
    }
    
    return maxPods
}
