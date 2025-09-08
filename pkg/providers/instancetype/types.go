package instancetype

import (
    "context"
    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
)

const (
    NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
)

// Provider interface for instance type operations
type Provider interface {
    List(context.Context, *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
}

// FlavorResponse represents the instance flavor structure
type FlavorResponse struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    VCPUs    int    `json:"vcpus"`
    RAM      int    `json:"ram"`
    Disk     int    `json:"disk"`
    Category string `json:"category"`
}

// defaultProvider implements the Provider interface
type defaultProvider struct {
    client *gobizfly.Client
    log    logr.Logger
}

// InstanceTypeSize represents the size metrics of an instance type
type InstanceTypeSize struct {
    Name     string
    VCPUs    int
    RAM      int
    Score    float64  // Combined size score for sorting
}
