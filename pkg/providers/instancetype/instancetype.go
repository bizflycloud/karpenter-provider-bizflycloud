package instancetype

import (
    "context"
    "fmt"
    "strings"

    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
    "sigs.k8s.io/karpenter/pkg/scheduling"
)

// NewDefaultProvider creates a new default instance type provider
func NewDefaultProvider(client *gobizfly.Client, log logr.Logger) Provider {
    return &defaultProvider{
        client: client,
        log:    log,
    }
}

// List implements Provider interface
func (d *defaultProvider) List(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {
    d.log.Info("=== STARTING INSTANCE TYPE LISTING ===", 
        "nodeClass", nodeClass.Name,
        "nodeCategory", nodeClass.Spec.NodeCategory)
    
    flavors, err := d.client.CloudServer.Flavors().List(ctx)
    if err != nil {
        d.log.Error(err, "Failed to list flavors", "nodeClass", nodeClass.Name)
        return nil, fmt.Errorf("failed to list flavors: %w", err)
    }

    d.log.V(1).Info("Retrieved flavors", "count", len(flavors))

    var instanceTypes []*cloudprovider.InstanceType
    
    for _, gobizflyFlavor := range flavors {
        flavor := &FlavorResponse{
            ID:       gobizflyFlavor.ID,
            Name:     gobizflyFlavor.Name,
            VCPUs:    gobizflyFlavor.VCPUs,
            RAM:      gobizflyFlavor.RAM,
            Disk:     gobizflyFlavor.Disk,
            Category: gobizflyFlavor.Category,
        }
        
        if !d.isInstanceTypeUsable(flavor, nodeClass) {
            continue
        }
        
        instanceType := d.createInstanceType(flavor, nodeClass)
        instanceTypes = append(instanceTypes, instanceType)
    }

    // Sort instance types from smallest to largest
    d.sortInstanceTypesBySize(instanceTypes)
    
    // Return only the smallest instance type if available
    if len(instanceTypes) > 0 {
        d.log.Info("=== INSTANCE TYPE LISTING COMPLETE ===",
            "totalTypes", len(instanceTypes),
            "smallestSelected", instanceTypes[0].Name)
        
        return instanceTypes[:1], nil // Return only the smallest instance type
    }
    
    d.log.Info("=== INSTANCE TYPE LISTING COMPLETE ===",
        "totalTypes", 0,
        "smallestSelected", "none")
    
    return instanceTypes, nil

}

// createInstanceType creates a cloudprovider.InstanceType from a flavor
func (d *defaultProvider) createInstanceType(flavor *FlavorResponse, nodeClass *v1.BizflyCloudNodeClass) *cloudprovider.InstanceType {
    maxPods := d.calculatePodCapacity(flavor.VCPUs, flavor.RAM)
    
    requirements := scheduling.NewRequirements(
        scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
        scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
        scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, d.categorizeFlavor(flavor.Name)),
    )

    capacity := corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", flavor.VCPUs)),
        corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", flavor.RAM)),
        corev1.ResourcePods:   resource.MustParse(fmt.Sprintf("%d", maxPods)),
    }

    offerings := d.createOfferings(flavor)
    overhead := d.calculateOverhead(flavor.VCPUs, flavor.RAM)

    return &cloudprovider.InstanceType{
        Name:         flavor.Name,
        Requirements: requirements,
        Capacity:     capacity,
        Offerings:    offerings,
        Overhead:     &overhead,
    }
}

// isInstanceTypeUsable checks if an instance type is suitable
func (d *defaultProvider) isInstanceTypeUsable(flavor *FlavorResponse, nodeClass *v1.BizflyCloudNodeClass) bool {
    // Exclude VPS flavors
    if strings.Contains(flavor.Name, "_vps") {
        d.log.V(1).Info("Excluding VPS flavor", 
            "name", flavor.Name,
            "reason", "VPS flavors not allowed")
        return false
    }
    
    // Filter out very small instances
    if flavor.VCPUs < 1 || flavor.RAM < 1024 {
        d.log.V(1).Info("Excluding instance type - too small", 
            "name", flavor.Name,
            "vcpus", flavor.VCPUs,
            "ram", flavor.RAM)
        return false
    }
    
    // Filter by node category if specified
    if nodeClass != nil && nodeClass.Spec.NodeCategory != "" {
        flavorCategory := d.categorizeFlavor(flavor.Name)
        if flavorCategory != nodeClass.Spec.NodeCategory {
            d.log.Info("EXCLUDING instance type - category mismatch", 
                "name", flavor.Name,
                "flavorCategory", flavorCategory,
                "requiredCategory", nodeClass.Spec.NodeCategory)
            return false
        }
    }
    
    d.log.Info("Instance type is USABLE", 
        "name", flavor.Name,
        "category", d.categorizeFlavor(flavor.Name),
        "vcpus", flavor.VCPUs,
        "ram", flavor.RAM)
    
    return true
}

// categorizeFlavor categorizes a flavor based on its name
func (d *defaultProvider) categorizeFlavor(flavorName string) string {
    // Exclude VPS flavors entirely
    if strings.Contains(flavorName, "_vps") {
        return "vps" // Special category to exclude
    }
    
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
        return "premium" // Default category
    }
}
