package metrics

import (
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

var (
	// Instance lifecycle metrics
	InstanceCreationDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karpenter_bizflycloud_instance_creation_duration_seconds",
			Help:    "Time taken to create instances",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
	)

	InstanceDeletionDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karpenter_bizflycloud_instance_deletion_duration_seconds",
			Help:    "Time taken to delete instances",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
	)

	InstancesCreated = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_instances_created_total",
			Help: "Total number of instances created",
		},
		[]string{"instance_type", "zone", "capacity_type"},
	)

	InstancesDeleted = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_instances_deleted_total",
			Help: "Total number of instances deleted",
		},
		[]string{"instance_type", "zone", "reason"},
	)

	// Drift detection metrics
	DriftCheckDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karpenter_bizflycloud_drift_check_duration_seconds",
			Help:    "Time taken to check for node drift",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
	)

	DriftDetected = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_drift_detected_total",
			Help: "Total number of drift events detected",
		},
		[]string{"node_name", "drift_reason"},
	)

	// API call metrics
	APICallDuration = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "karpenter_bizflycloud_api_call_duration_seconds",
			Help:    "Duration of API calls to BizflyCloud",
			Buckets: prometheus.ExponentialBuckets(0.1, 2, 10),
		},
		[]string{"operation", "service"},
	)

	APIErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_api_errors_total",
			Help: "Total number of API errors",
		},
		[]string{"operation", "service", "error_type"},
	)

	// Consolidation metrics
	NodeConsolidationEvents = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_node_consolidation_events_total",
			Help: "Total number of node consolidation events",
		},
	)

	NodeTerminationDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karpenter_bizflycloud_node_termination_duration_seconds",
			Help:    "Time taken to terminate nodes during scale-down",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
	)

	UnderutilizedNodesDetected = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "karpenter_bizflycloud_underutilized_nodes",
			Help: "Number of underutilized nodes detected",
		},
	)

	// Resource metrics
	NodeCapacityByType = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "karpenter_bizflycloud_node_capacity",
			Help: "Node capacity by resource type",
		},
		[]string{"node_name", "resource_type"},
	)

	NodeUtilizationByType = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "karpenter_bizflycloud_node_utilization",
			Help: "Node utilization by resource type",
		},
		[]string{"node_name", "resource_type"},
	)
)

// Initialize registers all metrics with the controller-runtime metrics registry
func init() {
	metrics.Registry.MustRegister(
		InstanceCreationDuration,
		InstanceDeletionDuration,
		InstancesCreated,
		InstancesDeleted,
		DriftCheckDuration,
		DriftDetected,
		APICallDuration,
		APIErrors,
		NodeConsolidationEvents,
		NodeTerminationDuration,
		UnderutilizedNodesDetected,
		NodeCapacityByType,
		NodeUtilizationByType,
	)
}

// RecordInstanceCreation records instance creation metrics
func RecordInstanceCreation(instanceType, zone, capacityType string, duration time.Duration) {
	InstanceCreationDuration.Observe(duration.Seconds())
	InstancesCreated.WithLabelValues(instanceType, zone, capacityType).Inc()
}

// RecordInstanceDeletion records instance deletion metrics
func RecordInstanceDeletion(instanceType, zone, reason string, duration time.Duration) {
	InstanceDeletionDuration.Observe(duration.Seconds())
	InstancesDeleted.WithLabelValues(instanceType, zone, reason).Inc()
}

// RecordDriftCheck records drift detection metrics
func RecordDriftCheck(duration time.Duration) {
	DriftCheckDuration.Observe(duration.Seconds())
}

// RecordDriftDetected records when drift is detected
func RecordDriftDetected(nodeName, reason string) {
	DriftDetected.WithLabelValues(nodeName, reason).Inc()
}

// RecordAPICall records API call metrics
func RecordAPICall(operation, service string, duration time.Duration, err error) {
	APICallDuration.WithLabelValues(operation, service).Observe(duration.Seconds())
	if err != nil {
		errorType := "unknown"
		if err.Error() != "" {
			errorType = "api_error"
		}
		APIErrors.WithLabelValues(operation, service, errorType).Inc()
	}
}

// RecordConsolidationEvent records consolidation events
func RecordConsolidationEvent() {
	NodeConsolidationEvents.Inc()
}

// RecordNodeTermination records node termination duration
func RecordNodeTermination(duration time.Duration) {
	NodeTerminationDuration.Observe(duration.Seconds())
}

// SetUnderutilizedNodes sets the number of underutilized nodes
func SetUnderutilizedNodes(count float64) {
	UnderutilizedNodesDetected.Set(count)
}

// SetNodeCapacity sets node capacity metrics
func SetNodeCapacity(nodeName, resourceType string, capacity float64) {
	NodeCapacityByType.WithLabelValues(nodeName, resourceType).Set(capacity)
}

// SetNodeUtilization sets node utilization metrics
func SetNodeUtilization(nodeName, resourceType string, utilization float64) {
	NodeUtilizationByType.WithLabelValues(nodeName, resourceType).Set(utilization)
}
