package instancetype

import (
    "sort"
    "strconv"
    "strings"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// sortInstanceTypesBySize sorts instance types from smallest to largest
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
