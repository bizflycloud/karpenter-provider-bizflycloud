package instance

import (
    "fmt"
    "strings"
    "time"

    "github.com/bizflycloud/gobizfly"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/resource"
)

// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func (p *Provider) ConvertToNode(instance *Instance, isSpot bool) *corev1.Node {
    capacity := p.getCapacityFromFlavor(instance.Flavor)
    category := p.categorizeFlavor(instance.Flavor)
    
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: instance.Name,
            Labels: map[string]string{
                corev1.LabelInstanceTypeStable:  instance.Flavor,
                corev1.LabelArchStable:          "amd64",
                "karpenter.sh/capacity-type":    "on-demand",
                NodeLabelRegion:                 instance.Region,
                NodeLabelZone:                   instance.Zone,
                NodeCategoryLabel:               category,
            },
            Annotations: map[string]string{
                "karpenter.bizflycloud.sh/instance-id": instance.ID,
            },
        },
        Spec: corev1.NodeSpec{
            ProviderID: fmt.Sprintf("%s%s", ProviderIDPrefix, instance.ID),
            Taints: []corev1.Taint{
                {
                    Key:    "node.cloudprovider.kubernetes.io/uninitialized",
                    Value:  "true",
                    Effect: corev1.TaintEffectNoSchedule,
                },
                {
                    Key:    "karpenter.sh/unregistered",
                    Value:  "true",
                    Effect: corev1.TaintEffectNoSchedule,
                },
            },
        },
        Status: corev1.NodeStatus{
            Capacity:    capacity,
            Allocatable: capacity,
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

    if isSpot || instance.IsSpot {
        node.Annotations[NodeAnnotationIsSpot] = "true"
    }

    return node
}

// getCapacityFromFlavor parses flavor name to extract CPU and memory capacity
func (p *Provider) getCapacityFromFlavor(flavorName string) corev1.ResourceList {
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

// ConvertGobizflyServerToInstance converts gobizfly.Server to Instance
func (p *Provider) ConvertGobizflyServerToInstance(server *gobizfly.Server) *Instance {
    if server == nil {
        return nil
    }

    zone := server.AvailabilityZone
    if zone == "" {
        zone = fmt.Sprintf("%s-a", p.Region)
    }

    ipAddress := ""
    if len(server.IPAddresses.WanV4Addresses) > 0 {
        ipAddress = server.IPAddresses.WanV4Addresses[0].Address
    } else if len(server.IPAddresses.LanAddresses) > 0 {
        ipAddress = server.IPAddresses.LanAddresses[0].Address
    }

    var createdAt time.Time
    if server.CreatedAt != "" {
        formats := []string{
            "2006-01-02T15:04:05Z",
            "2006-01-02T15:04:05",
            "2006-01-02 15:04:05",
            time.RFC3339,
        }
        
        for _, format := range formats {
            if parsed, err := time.Parse(format, server.CreatedAt); err == nil {
                createdAt = parsed
                break
            }
        }
        
        if createdAt.IsZero() {
            createdAt = time.Now()
        }
    } else {
        createdAt = time.Now()
    }

    var tags []string
    for key, value := range server.Metadata {
        tags = append(tags, fmt.Sprintf("%s=%s", key, value))
    }

    return &Instance{
        ID:        server.ID,
        Name:      server.Name,
        Status:    server.Status,
        Region:    p.Region,
        Zone:      zone,
        Flavor:    server.Flavor.Name,
        IPAddress: ipAddress,
        IsSpot:    false,
        Tags:      tags,
        CreatedAt: createdAt,
    }
}

// categorizeFlavor categorizes a flavor based on its name
func (p *Provider) categorizeFlavor(flavorName string) string {
    switch {
    case strings.HasPrefix(flavorName, "nix."):
        return "premium"
    case strings.HasSuffix(flavorName, "_basic"):
        return "basic"
    case strings.HasSuffix(flavorName, "_enterprise"):
        return "enterprise"
    case strings.HasSuffix(flavorName, "_dedicated"):
        return "dedicated"
    default:
        return "premium"
    }
}
