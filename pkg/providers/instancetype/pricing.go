package instancetype

import (
    "sort"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
    "sigs.k8s.io/karpenter/pkg/scheduling"
    corev1 "k8s.io/api/core/v1"
)

// createOfferings creates offerings for different zones and capacity types
func (d *defaultProvider) createOfferings(flavor *FlavorResponse) cloudprovider.Offerings {
    availableZones := []string{"HN1", "HN2"}
    capacityTypes := []string{"on-demand", "spot", "saving_plan"}
    
    var offerings cloudprovider.Offerings
    
    for _, zone := range availableZones {
        for _, capacityType := range capacityTypes {
            requirements := scheduling.NewRequirements(
                scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
                scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
                scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
                scheduling.NewRequirement("karpenter.sh/capacity-type", corev1.NodeSelectorOpIn, capacityType),
                scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, d.categorizeFlavor(flavor.Name)),
            )
            
            cpuCost := float64(flavor.VCPUs) * 0.01
            memoryCost := float64(flavor.RAM) / 1024 * 0.001
            basePrice := cpuCost + memoryCost
            price := d.calculateCategoryPrice(basePrice, d.categorizeFlavor(flavor.Name), capacityType)
            
            offering := cloudprovider.Offering{
                Requirements:        requirements,
                Price:               price,
                Available:           true,
                ReservationCapacity: 1,
            }
            offerings = append(offerings, &offering)
        }
    }
    
    // Sort offerings by price
    sort.Slice(offerings, func(i, j int) bool {
        return offerings[i].Price < offerings[j].Price
    })
    
    return offerings
}

// calculateCategoryPrice calculates pricing for an instance type
func (d *defaultProvider) calculateCategoryPrice(basePrice float64, category, capacityType string) float64 {
    // Adjust price based on category
    switch category {
    case "basic":
        basePrice *= 1.0  // Standard pricing
    case "premium":
        basePrice *= 1.5  // 50% more expensive
    case "enterprise":
        basePrice *= 2.0  // 100% more expensive
    case "dedicated":
        basePrice *= 2.5  // 150% more expensive
    }
    
    // Adjust for capacity type
    if capacityType == "spot" {
        basePrice *= 0.3 // Spot is 70% cheaper
    }
    
    return basePrice
}
