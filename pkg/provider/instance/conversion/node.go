package conversion

import (
    "fmt"
    "strings"

    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance/capacity"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
    ProviderIDPrefix = "bizflycloud://"
    
    // Standard Kubernetes Labels
    LabelOS             = "kubernetes.io/os"
    LabelArch           = "kubernetes.io/arch"
    LabelInstanceType   = "node.kubernetes.io/instance-type"
    LabelTopologyRegion = "topology.kubernetes.io/region"
    LabelTopologyZone   = "topology.kubernetes.io/zone"

    // Karpenter Labels
    LabelNodeInitialized = "karpenter.sh/initialized"
    
    // Bizfly Custom Labels
    LabelCapacityType    = "karpenter.sh/capacity-type"
    NodeAnnotationIsSpot = "karpenter.bizflycloud.sh/instance-spot"
    NodeCategoryLabel    = "karpenter.bizflycloud.com/node-category"
    CPUVendorLabel       = "karpenter.bizflycloud.com/cpu-vendor"
)

// ParseCPUVendor extracts CPU vendor with generation from instance type prefix
// Supports:
//   - "p2.1c_1g" or "b2.1c_1g" -> "Intel2"
//   - "p4a.1c_1g" or "b4a.1c_1g" -> "AMD4"
func ParseCPUVendor(instanceType string) string {
    if !strings.Contains(instanceType, ".") {
        return ""
    }

    parts := strings.Split(instanceType, ".")
    if len(parts) < 1 {
        return ""
    }

    prefix := parts[0] // e.g., "p4a", "b2", "e4a"
    
    // Check for AMD (contains 'a' suffix)
    if strings.HasSuffix(prefix, "a") {
        // Extract generation number (second character)
        if len(prefix) >= 2 {
            genChar := string(prefix[1])
            return "AMD" + genChar
        }
    }
    
    // Default to Intel (no 'a' suffix)
    if len(prefix) >= 2 {
        genChar := string(prefix[1])
        return "Intel" + genChar
    }
    
    return ""
}

// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func ConvertToNode(instance *Instance, isSpot bool) *corev1.Node {
    // 1. Determine Capacity from flavor (e.g., p4a.2c_2g)
    resourceCapacity := capacity.GetCapacityFromFlavor(instance.Flavor)
    
    // 2. Determine Capacity Type (Spot vs On-Demand)
    capacityType := "on-demand"
    if isSpot || instance.IsSpot {
        capacityType = "spot"
    }

    // 3. Determine Category from flavor name (first character)
    category := CategorizeFlavor(instance.Flavor)

    // 4. Parse CPU Vendor from flavor name (e.g., p4a -> AMD4, p2 -> Intel2)
    cpuVendor := ParseCPUVendor(instance.Flavor)

    // 5. Build Labels
    labels := map[string]string{
        corev1.LabelInstanceTypeStable: instance.Flavor,
        corev1.LabelArchStable:         "amd64",
        LabelOS:                        "linux",
        LabelCapacityType:              capacityType,
        LabelTopologyRegion:            instance.Region,
        LabelTopologyZone:              instance.Zone,
        NodeCategoryLabel:              category,
        corev1.LabelHostname:           instance.Name,
    }

    // CRITICAL: Add the CPU Vendor label
    if cpuVendor != "" {
        labels[CPUVendorLabel] = cpuVendor
    }

    // NOTE: ImageID will be set by caller after node creation
    // because it's not available in Instance struct

    // 6. Build Annotations
    annotations := map[string]string{
        "karpenter.bizflycloud.sh/instance-id": instance.ID,
    }
    if capacityType == "spot" {
        annotations[NodeAnnotationIsSpot] = "true"
    }

    // 7. Construct Node Object
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name:        instance.Name,
            Labels:      labels,
            Annotations: annotations,
        },
        Spec: corev1.NodeSpec{
            ProviderID: fmt.Sprintf("%s%s", ProviderIDPrefix, instance.ID),
            Taints: []corev1.Taint{
                {
                    Key:    "karpenter.sh/unregistered",
                    Value:  "true",
                    Effect: corev1.TaintEffectNoSchedule,
                },
            },
        },
        Status: corev1.NodeStatus{
            Capacity:    resourceCapacity,
            Allocatable: resourceCapacity,
            Phase:       corev1.NodePending,
            Conditions: []corev1.NodeCondition{
                {
                    Type:    corev1.NodeReady,
                    Status:  corev1.ConditionFalse,
                    Reason:  "KubeletNotReady",
                    Message: "kubelet is starting",
                },
            },
        },
    }

    return node
}
