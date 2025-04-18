package bizflycloud

import (
	"context"
	"sync"
	"time"

	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/selection"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// Define metrics for drift detection
var (
	driftedNodesTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_drifted_nodes_total",
			Help: "Total number of drifted nodes detected",
		},
	)

	driftRemediationTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_drift_remediations_total",
			Help: "Total number of drift remediations performed",
		},
	)

	driftRemediationErrors = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_drift_remediation_errors_total",
			Help: "Total number of drift remediation errors",
		},
	)

	driftRemediationDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "karpenter_bizflycloud_drift_remediation_duration_seconds",
			Help:    "Histogram of drift remediation durations in seconds",
			Buckets: prometheus.ExponentialBuckets(1, 2, 10),
		},
	)
)

func init() {
	// Register metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		driftedNodesTotal,
		driftRemediationTotal,
		driftRemediationErrors,
		driftRemediationDuration,
	)
}

// DriftDetectionInterface defines methods needed for drift detection
type DriftDetectionInterface interface {
	DetectNodeDrift(ctx context.Context, node *corev1.Node) (bool, error)
	ReconcileNodeDrift(ctx context.Context, node *corev1.Node) error
}

// DriftDetector implements drift detection for BizFly Cloud instances
type DriftDetector struct {
	client          client.Client
	log             logr.Logger
	provider        DriftDetectionInterface
	reconcileInterval time.Duration
	autoRemediate   bool
	mutex           sync.Mutex
	isRunning       bool
}

// NewDriftDetector creates a new drift detector
func NewDriftDetector(
	client client.Client, 
	log logr.Logger, 
	provider DriftDetectionInterface,
	reconcileInterval time.Duration,
	autoRemediate bool,
) *DriftDetector {
	return &DriftDetector{
		client:          client,
		log:             log,
		provider:        provider,
		reconcileInterval: reconcileInterval,
		autoRemediate:   autoRemediate,
	}
}

// Start begins the drift detection background process
func (d *DriftDetector) Start(ctx context.Context) {
	d.mutex.Lock()
	if d.isRunning {
		d.mutex.Unlock()
		return
	}
	d.isRunning = true
	d.mutex.Unlock()

	d.log.Info("Starting drift detector", 
		"reconcileInterval", d.reconcileInterval,
		"autoRemediate", d.autoRemediate,
	)

	ticker := time.NewTicker(d.reconcileInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			d.log.Info("Stopping drift detector")
			return
		case <-ticker.C:
			d.log.V(1).Info("Running drift detection check")
			if err := d.CheckAllNodes(ctx); err != nil {
				d.log.Error(err, "Error during drift detection")
			}
		}
	}
}

// CheckAllNodes checks all Karpenter-managed nodes for drift
func (d *DriftDetector) CheckAllNodes(ctx context.Context) error {
	// Create a requirement to select nodes managed by our provider
	req, err := labels.NewRequirement(
		"karpenter.sh/provisioner-name",
		selection.Exists, 
		nil,
	)
	if err != nil {
		return err
	}

	// Create a selector with the requirement
	selector := labels.NewSelector().Add(*req)
	
	// List all nodes matching the selector
	nodeList := &corev1.NodeList{}
	if err := d.client.List(ctx, nodeList, &client.ListOptions{
		LabelSelector: selector,
	}); err != nil {
		return err
	}

	d.log.V(1).Info("Checking nodes for drift", "count", len(nodeList.Items))
	
	// For each node, check for drift
	for i := range nodeList.Items {
		node := &nodeList.Items[i]
		
		// Check if the provider ID is in the right format
		if node.Spec.ProviderID == "" || !strings.HasPrefix(node.Spec.ProviderID, ProviderIDPrefix) {
			continue
		}
		
		drifted, err := d.provider.DetectNodeDrift(ctx, node)
		if err != nil {
			d.log.Error(err, "Error detecting node drift", "name", node.Name)
			continue
		}
		
		if drifted {
			d.log.Info("Detected drifted node", "name", node.Name)
			driftedNodesTotal.Inc()
			
			if d.autoRemediate {
				d.log.Info("Auto-remediating drifted node", "name", node.Name)
				start := time.Now()
				
				if err := d.provider.ReconcileNodeDrift(ctx, node); err != nil {
					d.log.Error(err, "Failed to remediate drifted node", "name", node.Name)
					driftRemediationErrors.Inc()
				} else {
					d.log.Info("Successfully remediated drifted node", 
						"name", node.Name,
						"duration", time.Since(start),
					)
					driftRemediationTotal.Inc()
					driftRemediationDuration.Observe(time.Since(start).Seconds())
				}
			}
		}
	}
	
	return nil
}