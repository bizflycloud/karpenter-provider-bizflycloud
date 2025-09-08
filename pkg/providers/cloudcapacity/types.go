package cloudcapacity

import (
    "context"
    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

// Provider interface for cloud capacity operations
type Provider interface {
    GetAvailableCapacity(ctx context.Context, region string, zone string) (*Capacity, error)
    GetQuotas(ctx context.Context) (*Quotas, error)
    CheckCapacityAvailability(ctx context.Context, instanceType string, zone string) (bool, error)
}

// Capacity represents available cloud capacity
type Capacity struct {
    Region           string
    Zone             string
    AvailableVCPUs   int64
    AvailableMemory  int64
    AvailableStorage int64
    InstanceTypes    map[string]int // instance type -> available count
}

// Quotas represents cloud resource quotas
type Quotas struct {
    MaxVCPUs         int64
    MaxInstances     int64
    MaxVolumes       int64
    MaxVolumeStorage int64
    UsedVCPUs        int64
    UsedInstances    int64
    UsedVolumes      int64
    UsedVolumeStorage int64
}

// defaultProvider implements the Provider interface
type defaultProvider struct {
    client       client.Client
    bizflyClient *gobizfly.Client
    log          logr.Logger
    region       string
}
