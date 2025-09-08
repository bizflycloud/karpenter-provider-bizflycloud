package cloudcapacity

import (
    "context"
    "fmt"

    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// NewProvider creates a new cloud capacity provider
func NewProvider(client client.Client, bizflyClient *gobizfly.Client, log logr.Logger, region string) Provider {
    return &defaultProvider{
        client:       client,
        bizflyClient: bizflyClient,
        log:          log,
        region:       region,
    }
}

// GetAvailableCapacity returns the available capacity in a region/zone
func (p *defaultProvider) GetAvailableCapacity(ctx context.Context, region string, zone string) (*Capacity, error) {
    p.log.V(1).Info("Getting available capacity", 
        "region", region, 
        "zone", zone)

    // In a real implementation, this would call BizFly APIs to get actual capacity
    // For now, return mock data
    capacity := &Capacity{
        Region:           region,
        Zone:             zone,
        AvailableVCPUs:   1000,
        AvailableMemory:  2048000, // 2TB in MB
        AvailableStorage: 10000000, // 10TB in GB
        InstanceTypes: map[string]int{
            "nix.2c_4g":    50,
            "nix.4c_8g":    30,
            "nix.8c_16g":   20,
            "nix.12c_24g":  10,
            "nix.16c_32g":  5,
        },
    }

    p.log.Info("Retrieved capacity information",
        "region", region,
        "zone", zone,
        "availableVCPUs", capacity.AvailableVCPUs,
        "availableMemoryMB", capacity.AvailableMemory)

    return capacity, nil
}

// GetQuotas returns the current quotas and usage
func (p *defaultProvider) GetQuotas(ctx context.Context) (*Quotas, error) {
    p.log.V(1).Info("Getting quotas", "region", p.region)

    // In a real implementation, this would call BizFly APIs to get actual quotas
    quotas := &Quotas{
        MaxVCPUs:         1000,
        MaxInstances:     100,
        MaxVolumes:       200,
        MaxVolumeStorage: 50000, // 50TB
        UsedVCPUs:        200,
        UsedInstances:    25,
        UsedVolumes:      50,
        UsedVolumeStorage: 10000, // 10TB
    }

    p.log.Info("Retrieved quota information",
        "maxVCPUs", quotas.MaxVCPUs,
        "usedVCPUs", quotas.UsedVCPUs,
        "maxInstances", quotas.MaxInstances,
        "usedInstances", quotas.UsedInstances)

    return quotas, nil
}

// CheckCapacityAvailability checks if a specific instance type is available in a zone
func (p *defaultProvider) CheckCapacityAvailability(ctx context.Context, instanceType string, zone string) (bool, error) {
    p.log.V(1).Info("Checking capacity availability",
        "instanceType", instanceType,
        "zone", zone)

    capacity, err := p.GetAvailableCapacity(ctx, p.region, zone)
    if err != nil {
        return false, fmt.Errorf("failed to get capacity: %w", err)
    }

    available, exists := capacity.InstanceTypes[instanceType]
    if !exists {
        p.log.V(1).Info("Instance type not found in capacity data",
            "instanceType", instanceType)
        return false, nil
    }

    isAvailable := available > 0
    p.log.V(1).Info("Capacity availability check result",
        "instanceType", instanceType,
        "zone", zone,
        "available", available,
        "isAvailable", isAvailable)

    return isAvailable, nil
}
