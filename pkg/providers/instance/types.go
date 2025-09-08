package instance

import (
    "time"
    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "sigs.k8s.io/controller-runtime/pkg/client"
)

const (
    ProviderIDPrefix = "bizflycloud://"
    NodeLabelInstanceType = "node.kubernetes.io/instance-type"
    NodeLabelRegion = "topology.kubernetes.io/region"
    NodeLabelZone = "topology.kubernetes.io/zone"
    NodeAnnotationIsSpot = "karpenter.bizflycloud.sh/instance-spot"
    NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
    
    maxServerWaitTime = 30 * time.Minute
    pollInterval = 15 * time.Second
)

type Provider struct {
    Client       client.Client
    Log          logr.Logger
    BizflyClient *gobizfly.Client
    Region       string
    Config       *v1bizfly.ProviderConfig
}

type Instance struct {
    ID        string
    Name      string
    Status    string
    Region    string
    Zone      string
    Flavor    string
    IPAddress string
    IsSpot    bool
    Tags      []string
    CreatedAt time.Time
}
