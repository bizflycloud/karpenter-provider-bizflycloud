package instancetype

import (
    "context"
    "fmt"
    "sort"     // ADD this import
    "strconv"  // ADD this import
    "strings"


    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
    "sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
    NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
)

// InstanceTypeSize represents the size metrics of an instance type
type InstanceTypeSize struct {
    Name     string
    VCPUs    int
    RAM      int
    Score    float64  // Combined size score for sorting
}

type Provider interface {
    List(context.Context, *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
}

func NewDefaultProvider(client *gobizfly.Client, log logr.Logger) Provider {
    return &defaultProvider{
        client: client,
        log:    log,
    }
}

type defaultProvider struct {
    client *gobizfly.Client
    log    logr.Logger
}


// calculateInstanceScore calculates a size score for sorting
// Lower score = smaller instance = higher priority
func (d *defaultProvider) calculateInstanceScore(vcpus int, ramMB int) float64 {
    // Normalize CPU and RAM to comparable scales
    // Weight CPU more heavily as it's usually the primary constraint
    cpuScore := float64(vcpus) * 100.0           // CPU weight: 100
    ramScore := float64(ramMB) / 1024.0 * 100.0   // RAM weight: 50 (per GB)
    
    return cpuScore + ramScore
}

// Enhanced sorting algorithm that handles all nix.* instance patterns
func (d *defaultProvider) sortInstanceTypesBySize(instanceTypes []*cloudprovider.InstanceType) {
    sort.Slice(instanceTypes, func(i, j int) bool {
        nameI := instanceTypes[i].Name
        nameJ := instanceTypes[j].Name
        
        // Parse both instance types
        cpuI, ramI := d.parseInstanceTypeName(nameI)
        cpuJ, ramJ := d.parseInstanceTypeName(nameJ)
        
        // Calculate total resource score (CPU weighted more heavily)
        scoreI := float64(cpuI)*1000 + float64(ramI)/1024  // CPU * 1000 + RAM in GB
        scoreJ := float64(cpuJ)*1000 + float64(ramJ)/1024
        
        // Sort by total score (smallest first)
        if scoreI != scoreJ {
            return scoreI < scoreJ
        }
        
        // If scores are equal, prefer fewer CPUs
        if cpuI != cpuJ {
            return cpuI < cpuJ
        }
        
        // If CPU is equal, prefer less RAM
        return ramI < ramJ
    })
    
    // Log the sorted order for debugging
    d.log.Info("Instance types sorted by size")
    for i, instanceType := range instanceTypes[:min(10, len(instanceTypes))] {
        cpu, ram := d.parseInstanceTypeName(instanceType.Name)
        d.log.Info("Sorted instance",
            "index", i,
            "name", instanceType.Name,
            "cpu", cpu,
            "ramGB", ram/1024,
            "score", float64(cpu)*1000 + float64(ram)/1024)
    }
}

// parseInstanceTypeName extracts CPU and RAM from instance type names
// Handles formats like: nix.3c_4g, 4c_8g_basic, 12c_16g_enterprise
func (d *defaultProvider) parseInstanceTypeName(instanceType string) (int, int) {
    // Remove "nix." prefix
    name := strings.TrimPrefix(instanceType, "nix.")
    
    // Split by underscore: ["2c", "3g"] or ["12c", "16g", "basic"]
    parts := strings.Split(name, "_")
    if len(parts) < 2 {
        d.log.V(1).Info("Invalid instance type format", "name", instanceType)
        return 999, 999999 // Sort to end
    }
    
    // Parse CPU (remove 'c' suffix)
    cpuStr := strings.TrimSuffix(parts[0], "c")
    cpu, err := strconv.Atoi(cpuStr)
    if err != nil {
        d.log.V(1).Info("Could not parse CPU", "name", instanceType, "cpuStr", cpuStr)
        return 999, 999999
    }
    
    // Parse RAM (remove 'g' suffix)
    ramStr := strings.TrimSuffix(parts[1], "g")
    ramGB, err := strconv.Atoi(ramStr)
    if err != nil {
        d.log.V(1).Info("Could not parse RAM", "name", instanceType, "ramStr", ramStr)
        return cpu, 999999
    }
    
    ramMB := ramGB * 1024
    return cpu, ramMB
}

// Helper function for min
func min(a, b int) int {
    if a < b {
        return a
    }
    return b
}

func (d *defaultProvider) categorizeFlavor(flavorName string) string {
    d.log.V(2).Info("Categorizing flavor", "flavorName", flavorName)
    
    // CRITICAL: Exclude VPS flavors entirely
    if strings.Contains(flavorName, "_vps") {
        d.log.V(2).Info("Flavor is VPS - categorizing as excluded", "flavorName", flavorName)
        return "vps" // Special category to exclude
    }
    
    switch {
    case strings.HasPrefix(flavorName, "nix."):
        d.log.V(2).Info("Flavor categorized as premium", "flavorName", flavorName)
        return "premium"
    case strings.HasSuffix(flavorName, "_basic"):
        d.log.V(2).Info("Flavor categorized as basic", "flavorName", flavorName)
        return "basic"
    case strings.HasSuffix(flavorName, "_enterprise"):
        d.log.V(2).Info("Flavor categorized as enterprise", "flavorName", flavorName)
        return "enterprise"
    case strings.HasSuffix(flavorName, "_dedicated"):
        d.log.V(2).Info("Flavor categorized as dedicated", "flavorName", flavorName)
        return "dedicated"
    default:
        d.log.V(2).Info("Flavor categorized as basic (default)", "flavorName", flavorName)
        return "premium" // Default category
    }
}




// calculatePodCapacity calculates realistic pod capacity based on instance size
func (d *defaultProvider) calculatePodCapacity(vcpus int, ramMB int) int64 {
    // Base pod capacity calculation
    var maxPods int64
    
    // Calculate based on CPU and memory
    cpuBasedPods := int64(vcpus * 8)  // 8 pods per CPU core
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

func (d *defaultProvider) calculateOverhead(vcpus int, ramMB int) cloudprovider.InstanceTypeOverhead {
    // Much more conservative overhead for small instances
    var cpuOverhead, memoryOverhead, podOverhead string
    
    switch {
    case vcpus <= 4:
        cpuOverhead = "50m"    // Very low overhead for small instances
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
            corev1.ResourceCPU:    resource.MustParse("25m"), // Very low
            corev1.ResourceMemory: resource.MustParse("64Mi"), // Very low
            corev1.ResourcePods:   resource.MustParse("1"),
        },
        EvictionThreshold: corev1.ResourceList{
            corev1.ResourceCPU:    resource.MustParse("25m"), // Very low
            corev1.ResourceMemory: resource.MustParse("64Mi"), // Very low
            corev1.ResourcePods:   resource.MustParse("1"),
        },
    }
}


// Define a local type to match the actual gobizfly structure
type FlavorResponse struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    VCPUs    int    `json:"vcpus"`
    RAM      int    `json:"ram"`
    Disk     int    `json:"disk"`
    Category string `json:"category"`
}

func (d *defaultProvider) isInstanceTypeUsable(flavor *FlavorResponse, nodeClass *v1.BizflyCloudNodeClass) bool {
    // CRITICAL: Exclude VPS flavors entirely
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
        requiredCategory := nodeClass.Spec.NodeCategory
        
        d.log.Info("Checking category filter", 
            "flavorName", flavor.Name,
            "flavorCategory", flavorCategory,
            "requiredCategory", requiredCategory,
            "nodeClass", nodeClass.Name)
        
        if flavorCategory != requiredCategory {
            d.log.Info("EXCLUDING instance type - category mismatch", 
                "name", flavor.Name,
                "flavorCategory", flavorCategory,
                "requiredCategory", requiredCategory)
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




// convertGobizflyFlavor converts the unexported gobizfly type to our local type
func convertGobizflyFlavor(gobizflyFlavor interface{}) *FlavorResponse {
    // Use type assertion to extract fields from the unexported type
    // This is a workaround since we can't directly access the unexported type
    
    // Use reflection or type assertion to extract fields
    // For now, let's use a simple approach with interface{}
    if flavorMap, ok := gobizflyFlavor.(map[string]interface{}); ok {
        return &FlavorResponse{
            ID:       getString(flavorMap, "id"),
            Name:     getString(flavorMap, "name"),
            VCPUs:    getInt(flavorMap, "vcpus"),
            RAM:      getInt(flavorMap, "ram"),
            Disk:     getInt(flavorMap, "disk"),
            Category: getString(flavorMap, "category"),
        }
    }
    
    return nil
}

// Helper functions for type conversion
func getString(m map[string]interface{}, key string) string {
    if val, ok := m[key].(string); ok {
        return val
    }
    return ""
}

func getInt(m map[string]interface{}, key string) int {
    if val, ok := m[key].(float64); ok {
        return int(val)
    }
    if val, ok := m[key].(int); ok {
        return val
    }
    return 0
}

// Update your List function to pass nodeClass to isInstanceTypeUsable
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
        
        // Skip unusable instance types (now includes category filtering)
        if !d.isInstanceTypeUsable(flavor, nodeClass) {
            continue
        }
        
        d.log.V(1).Info("Processing flavor",
            "name", flavor.Name,
            "category", d.categorizeFlavor(flavor.Name),
            "vcpus", flavor.VCPUs,
            "ram", flavor.RAM)

        // Calculate realistic pod capacity
        maxPods := d.calculatePodCapacity(flavor.VCPUs, flavor.RAM)
        
        // Create requirements with proper Kubernetes labels including category
        requirements := scheduling.NewRequirements(
            scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
            scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
            scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, d.categorizeFlavor(flavor.Name)),
        )

        d.log.V(1).Info("Created requirements", 
            "instanceType", flavor.Name,
            "arch", "amd64",
            "capacityType", "on-demand", "saving_plan")

        // Create capacity for CPU, memory, and pods
        cpuQuantity := resource.MustParse(fmt.Sprintf("%d", flavor.VCPUs))
        memoryQuantity := resource.MustParse(fmt.Sprintf("%dMi", flavor.RAM))
        podQuantity := resource.MustParse(fmt.Sprintf("%d", maxPods))
        
        capacity := corev1.ResourceList{
            corev1.ResourceCPU:    cpuQuantity,
            corev1.ResourceMemory: memoryQuantity,
            corev1.ResourcePods:   podQuantity,
        }
        
        d.log.V(1).Info("Created capacity",
            "cpu", cpuQuantity.String(),
            "memory", memoryQuantity.String(),
            "pods", podQuantity.String(),
            "maxPodsCalculated", maxPods)

        // Create offerings
        offerings := cloudprovider.Offerings{}
        availableZones := []string{"HN1", "HN2"}
        capacityTypes := []string{"on-demand", "spot", "saving_plan"}
        
        for _, zone := range availableZones {
            for _, capacityType := range capacityTypes {
                offeringRequirements := scheduling.NewRequirements(
                    scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
                    scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
                    scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
                    scheduling.NewRequirement("karpenter.sh/capacity-type", corev1.NodeSelectorOpIn, capacityType),
                    scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, d.categorizeFlavor(flavor.Name)),
                )
                
                cpuCost := float64(flavor.VCPUs) * 0.01  // $0.01 per vCPU per hour
                memoryCost := float64(flavor.RAM) / 1024 * 0.001  // $0.001 per GB per hour
                basePrice := cpuCost + memoryCost
                price := d.calculateCategoryPrice(basePrice, d.categorizeFlavor(flavor.Name), capacityType)
                
                offering := cloudprovider.Offering{
                    Requirements:        offeringRequirements,
                    Price:               price,
                    Available:           true,
                    ReservationCapacity: 1,
                }
                offerings = append(offerings, &offering)
            }
        }

        sort.Slice(offerings, func(i, j int) bool {
            return offerings[i].Price < offerings[j].Price
        })  
        d.log.V(1).Info("Created offerings", 
            "instanceType", flavor.Name,
            "totalOfferings", len(offerings))

        // Calculate realistic overhead - FIXED: Use correct field names
        overhead := d.calculateOverhead(flavor.VCPUs, flavor.RAM)

        // Use pointer access for String() method
        kubeReservedCPU := overhead.KubeReserved[corev1.ResourceCPU]
        kubeReservedMemory := overhead.KubeReserved[corev1.ResourceMemory]
        kubeReservedPods := overhead.KubeReserved[corev1.ResourcePods]
        
        d.log.V(1).Info("Created overhead",
            "kubeReservedCPU", (&kubeReservedCPU).String(),
            "kubeReservedMemory", (&kubeReservedMemory).String(),
            "kubeReservedPods", (&kubeReservedPods).String())

        // Create the instance type
        instanceType := &cloudprovider.InstanceType{
            Name:         flavor.Name,
            Requirements: requirements,
            Capacity:     capacity,
            Offerings:    offerings,
            Overhead:     &overhead,
        }

        
        // Calculate available resources after overhead
        availableCPU := capacity[corev1.ResourceCPU].DeepCopy()
        availableCPU.Sub(overhead.KubeReserved[corev1.ResourceCPU])
        availableCPU.Sub(overhead.SystemReserved[corev1.ResourceCPU])
        availableCPU.Sub(overhead.EvictionThreshold[corev1.ResourceCPU])
        
        availableMemory := capacity[corev1.ResourceMemory].DeepCopy()
        availableMemory.Sub(overhead.KubeReserved[corev1.ResourceMemory])
        availableMemory.Sub(overhead.SystemReserved[corev1.ResourceMemory])
        availableMemory.Sub(overhead.EvictionThreshold[corev1.ResourceMemory])
        
        d.log.V(1).Info("Created instance type", 
            "name", instanceType.Name,
            "offeringsCount", len(instanceType.Offerings),
            "availableCPU", (&availableCPU).String(),
            "availableMemory", (&availableMemory).String())

        instanceTypes = append(instanceTypes, instanceType)
    }
    
    d.sortInstanceTypesBySize(instanceTypes)

    // Log the sorted order for debugging
    d.log.Info("=== FINAL SORTED INSTANCE TYPES ===")
    for i, instanceType := range instanceTypes {
        cpu, ram := d.parseInstanceTypeName(instanceType.Name)
        score := d.calculateInstanceScore(cpu, ram)
        d.log.V(1).Info("Sorted instance type",
            "index", i,
            "name", instanceType.Name,
            "cpu", cpu,
            "ramMB", ram,
            "score", score)
    }

    d.log.Info("=== INSTANCE TYPE LISTING COMPLETE ===",
        "totalTypes", len(instanceTypes),
        "smallestFirst", instanceTypes[0].Name,
        "largestLast", instanceTypes[len(instanceTypes)-1].Name)

    return instanceTypes, nil
}


// Add category-based pricing
func (d *defaultProvider) calculateCategoryPrice(basePrice float64, category, capacityType string) float64 {
    // Adjust price based on category
    switch category {
    case "basic":
        basePrice *= 1.0  // 20% cheaper
    case "premium":
        basePrice *= 1.5  // Standard pricing
    case "enterprise":
        basePrice *= 2.0  // 50% more expensive
    case "dedicated":
        basePrice *= 2.5  // 30% more expensive
    }
    
    // Adjust for capacity type
    if capacityType == "spot" {
        basePrice *= 0.3 // Spot is 70% cheaper
    }
    
    return basePrice
}

