package bizflycloud

import (
    "sync"
    "time"
    "github.com/prometheus/client_golang/prometheus"
    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/client"
    v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/providers/instancetype"
)

// Constants - ONLY declare these ONCE in this file
const (
    ProviderName         = "karpenter.bizflycloud.com"
    ProviderConfigName   = "default"
    ProviderIDPrefix     = "bizflycloud://"
    NodeLabelInstanceType = "node.kubernetes.io/instance-type"
    NodeLabelRegion      = "topology.kubernetes.io/region"
    NodeLabelZone        = "topology.kubernetes.io/zone"
    NodeAnnotationIsSpot = "karpenter.bizflycloud.sh/instance-spot"
    NodeLabelImageID     = "karpenter.bizflycloud.sh/image-id"
    NodeCategoryLabel    = "karpenter.bizflycloud.com/node-category"
    
    DriftReasonNodeNotFound      = "NodeNotFound"
    DriftReasonNodeClassDrifted  = "NodeClassDrifted"
    DriftReasonInstanceNotFound  = "InstanceNotFound"
)

// Variables - ONLY declare these ONCE in this file
var (
    nodeCreationMutex sync.Mutex
    lastNodeCreation  time.Time
    minCreationInterval = 180 * time.Second
)

// Types - ONLY declare these ONCE in this file
type CloudProvider struct {
    kubeClient           client.Client
    instanceTypeProvider instancetype.Provider
    bizflyClient         *gobizfly.Client
    log                  logr.Logger
    region               string
    zone                 string
    config               *v1bizfly.ProviderConfig
}

// Metrics - ONLY declare these ONCE in this file
var (
    instanceCreationDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "karpenter_bizflycloud_instance_creation_duration_seconds",
            Help:    "Histogram of instance creation durations in seconds",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),
        },
    )

    instanceDeletionDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "karpenter_bizflycloud_instance_deletion_duration_seconds",
            Help:    "Histogram of instance deletion durations in seconds",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),
        },
    )

    driftCheckDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "karpenter_bizflycloud_drift_check_duration_seconds",
            Help:    "Histogram of drift check durations in seconds",
            Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
        },
    )

    apiErrors = prometheus.NewCounterVec(
        prometheus.CounterOpts{
            Name: "karpenter_bizflycloud_api_errors_total",
            Help: "Total number of API errors encountered",
        },
        []string{"operation"},
    )

    nodeConsolidationEvents = prometheus.NewCounter(
        prometheus.CounterOpts{
            Name: "karpenter_bizflycloud_node_consolidation_events_total",
            Help: "Total number of node consolidation events",
        },
    )

    nodeTerminationDuration = prometheus.NewHistogram(
        prometheus.HistogramOpts{
            Name:    "karpenter_bizflycloud_node_termination_duration_seconds",
            Help:    "Time taken to terminate nodes during scale-down",
            Buckets: prometheus.ExponentialBuckets(1, 2, 10),
        },
    )

    underutilizedNodesDetected = prometheus.NewGauge(
        prometheus.GaugeOpts{
            Name: "karpenter_bizflycloud_underutilized_nodes",
            Help: "Number of underutilized nodes detected",
        },
    )
)

// Constructor function - ONLY declare this ONCE in this file
func New(
    instanceTypeProvider instancetype.Provider,
    kubeClient client.Client,
    bizflyClient *gobizfly.Client,
    log logr.Logger,
    config *v1bizfly.ProviderConfig,
) *CloudProvider {
    return &CloudProvider{
        instanceTypeProvider: instanceTypeProvider,
        kubeClient:           kubeClient,
        bizflyClient:         bizflyClient,
        log:                  log,
        region:               config.Spec.Region,
        zone:                 config.Spec.Zone,
        config:               config,
    }
}
