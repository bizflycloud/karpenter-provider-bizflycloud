package bizflycloud

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
	v1alpha1 "github.com/quanlm/karpenter-provider-bizflycloud/pkg/apis/bizflycloud/v1alpha1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

const (
	// ProviderName is the name of the provider
	ProviderName = "bizflycloud.karpenter.sh"
	
	// ProviderConfigName is the default name for the ProviderConfig
	ProviderConfigName = "default"

	// ProviderID prefix for BizFly Cloud instances
	ProviderIDPrefix = "bizflycloud://"

	// NodeLabelInstanceType is the label for instance type
	NodeLabelInstanceType = "node.kubernetes.io/instance-type"

	// NodeLabelRegion is the label for region
	NodeLabelRegion = "topology.kubernetes.io/region"

	// NodeLabelZone is the label for zone
	NodeLabelZone = "topology.kubernetes.io/zone"

	// NodeAnnotationSpot is the annotation for spot instances
	NodeAnnotationIsSpot = "karpenter.bizflycloud.sh/instance-spot"

	// NodeLabelImageID is the label for image ID
	NodeLabelImageID = "karpenter.bizflycloud.sh/image-id"
)

var (
	// Metrics
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

	apiErrors = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_api_errors_total",
			Help: "Total number of API errors encountered",
		},
		[]string{"operation"},
	)
)

func init() {
	// Register metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		instanceCreationDuration,
		instanceDeletionDuration,
		apiErrors,
	)
}

// Provider implements the Karpenter provider interface for BizFly Cloud
type Provider struct {
	client        client.Client
	log           logr.Logger
	bizflyClient  *gobizfly.Client
	region        string
	config        *v1alpha1.ProviderConfig
	providerName  string
	driftDetector *DriftDetector
}

// NewProvider creates a new BizFly Cloud provider
func NewProvider(client client.Client, log logr.Logger) (*Provider, error) {
	// Initialize with empty BizFly client, will be configured when first used
	provider := &Provider{
		client:       client,
		log:          log,
		providerName: ProviderName,
	}
	
	return provider, nil
}

// Start initializes the provider
func (p *Provider) Start(ctx context.Context) error {
	p.log.Info("Starting BizFly Cloud provider")

	// Setup retry logic for initializing the BizFly client
	err := wait.PollImmediate(5*time.Second, 1*time.Minute, func() (bool, error) {
		// Get provider config first
		config, err := p.getProviderConfig(ctx)
		if err != nil {
			p.log.Error(err, "Failed to get provider config, retrying")
			return false, nil
		}
		p.config = config
		p.region = config.Spec.Region

		// Initialize BizFly client with this config
		client, err := p.getBizFlyClient(ctx)
		if err != nil {
			p.log.Error(err, "Failed to initialize BizFly Cloud client, retrying")
			return false, nil
		}
		p.bizflyClient = client
		
		// Update provider config status
		if err := p.updateProviderConfigStatus(ctx); err != nil {
			p.log.Error(err, "Failed to update provider config status")
			// Continue anyway, this is not critical
		}
		
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("failed to initialize BizFly Cloud provider: %w", err)
	}
	
	// Initialize drift detector if enabled
	if p.config.Spec.DriftDetection != nil && p.config.Spec.DriftDetection.Enabled {
		p.log.Info("Initializing drift detector")
		
		reconcileInterval, err := time.ParseDuration(p.config.Spec.DriftDetection.ReconcileInterval)
		if err != nil {
			reconcileInterval = 5 * time.Minute
			p.log.Error(err, "Invalid reconcile interval, using default", "default", reconcileInterval)
		}
		
		p.driftDetector = NewDriftDetector(
			p.client, 
			p.log.WithName("drift-detector"), 
			p,
			reconcileInterval,
			p.config.Spec.DriftDetection.AutoRemediate,
		)
		
		go p.driftDetector.Start(ctx)
	}

	return nil
}

// getProviderConfig retrieves the ProviderConfig CRD
func (p *Provider) getProviderConfig(ctx context.Context) (*v1alpha1.ProviderConfig, error) {
	// Get the provider config
	config := &v1alpha1.ProviderConfig{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: ProviderConfigName}, config); err != nil {
		return nil, fmt.Errorf("failed to get ProviderConfig %q: %w", ProviderConfigName, err)
	}
	
	// Ensure default values are set if not specified
	if config.Spec.Tagging == nil {
		config.Spec.Tagging = &v1alpha1.TaggingSpec{
			EnableResourceTags: true,
			TagPrefix:          "karpenter.bizflycloud.sh/",
		}
	}
	
	if config.Spec.CloudConfig == nil {
		config.Spec.CloudConfig = &v1alpha1.CloudConfigSpec{
			APIEndpoint:      "https://manage.bizflycloud.vn/api",
			RetryMaxAttempts: 5,
			RetryInitialDelay: "1s",
			Timeout:          "30s",
		}
	}
	
	if config.Spec.ImageConfig == nil {
		config.Spec.ImageConfig = &v1alpha1.ImageConfigSpec{
			ImageID:      "ubuntu-20.04",
			RootDiskSize: 20,
		}
	}
	
	if config.Spec.DriftDetection == nil {
		config.Spec.DriftDetection = &v1alpha1.DriftDetectionSpec{
			Enabled:           true,
			ReconcileInterval: "5m",
			AutoRemediate:     true,
		}
	}
	
	return config, nil
}

// updateProviderConfigStatus updates the status of the ProviderConfig
func (p *Provider) updateProviderConfigStatus(ctx context.Context) error {
	// Get the current provider config
	config := &v1alpha1.ProviderConfig{}
	if err := p.client.Get(ctx, types.NamespacedName{Name: ProviderConfigName}, config); err != nil {
		return fmt.Errorf("failed to get ProviderConfig %q: %w", ProviderConfigName, err)
	}
	
	// Get instance types and regions
	offerings, err := p.getAllInstanceOfferings(ctx)
	if err != nil {
		apiErrors.WithLabelValues("get_offerings").Inc()
		return fmt.Errorf("failed to get instance offerings: %w", err)
	}
	
	// Update the status with instance types and regions
	config.Status.LastUpdated = metav1.Now()
	config.Status.InstanceTypes = lo.Map(offerings, func(o InstanceOffering, _ int) string {
		return o.Name
	})
	config.Status.Regions = lo.Uniq(lo.Map(offerings, func(o InstanceOffering, _ int) string {
		return o.Region
	}))
	
	// Add conditions
	readyCondition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionTrue,
		LastTransitionTime: metav1.Now(),
		Reason:             "ProviderReady",
		Message:            "BizFly Cloud provider is ready",
	}
	
	// Find and update the existing condition or add a new one
	if config.Status.Conditions == nil {
		config.Status.Conditions = []metav1.Condition{readyCondition}
	} else {
		foundIndex := -1
		for i, cond := range config.Status.Conditions {
			if cond.Type == "Ready" {
				foundIndex = i
				break
			}
		}
		
		if foundIndex >= 0 {
			config.Status.Conditions[foundIndex] = readyCondition
		} else {
			config.Status.Conditions = append(config.Status.Conditions, readyCondition)
		}
	}
	
	// Update the provider config status
	return p.client.Status().Update(ctx, config)
}

// getBizFlyClient creates a new BizFly Cloud client
func (p *Provider) getBizFlyClient(ctx context.Context) (*gobizfly.Client, error) {
	// Get the secret containing the credentials
	secret := &corev1.Secret{}
	if err := p.client.Get(ctx, types.NamespacedName{
		Name:      p.config.Spec.SecretRef.Name,
		Namespace: p.config.Spec.SecretRef.Namespace,
	}, secret); err != nil {
		return nil, fmt.Errorf("failed to get credentials secret %q: %w", 
			p.config.Spec.SecretRef.Name, err)
	}
	
	// Get the credentials from the secret
	email, ok := secret.Data["email"]
	if !ok {
		return nil, fmt.Errorf("email not found in secret %q", p.config.Spec.SecretRef.Name)
	}
	
	password, ok := secret.Data["password"]
	if !ok {
		return nil, fmt.Errorf("password not found in secret %q", p.config.Spec.SecretRef.Name)
	}
	
	// Get API endpoint from config
	apiEndpoint := p.config.Spec.CloudConfig.APIEndpoint

	// Convert timeout to duration for client options
	timeout, err := time.ParseDuration(p.config.Spec.CloudConfig.Timeout)
	if err != nil {
		timeout = 30 * time.Second
		p.log.Error(err, "Invalid timeout value, using default", "default", timeout)
	}
	
	// Create a new BizFly Cloud client
	p.log.Info("Creating BizFly Cloud client", "endpoint", apiEndpoint)
	
	// For production environment, use real client
	if os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.log.Info("Using mock BizFly Cloud client")
		return nil, nil
	} else {
		client, err := gobizfly.NewClient(
			gobizfly.WithTLSVerify(true),
			gobizfly.WithRegionName(p.config.Spec.Region),
			gobizfly.WithEndpointURL(apiEndpoint),
			gobizfly.WithTimeout(timeout),
		)
		
		if err != nil {
			return nil, fmt.Errorf("failed to create BizFly Cloud client: %w", err)
		}
		
		// Authenticate with BizFly Cloud
		p.log.Info("Authenticating with BizFly Cloud")
		token, err := client.Token.Create(ctx, &gobizfly.TokenCreateRequest{
			AuthMethod: "password",
			Username:   string(email),
			Password:   string(password),
		})
		
		if err != nil {
			return nil, fmt.Errorf("failed to authenticate with BizFly Cloud: %w", err)
		}
		
		// Set the token in the client
		client.SetKeystoneToken(token.KeystoneToken)
		
		p.log.Info("Successfully authenticated with BizFly Cloud")
		return client, nil
	}
}

// CreateNode creates a new BizFly Cloud server instance
func (p *Provider) CreateNode(ctx context.Context, nodeSpec *corev1.Node) (*corev1.Node, error) {
	start := time.Now()
	p.log.Info("Creating node", "name", nodeSpec.Name)

	// Extract instance type from node labels
	instanceType, exists := nodeSpec.Labels[NodeLabelInstanceType]
	if !exists {
		return nil, fmt.Errorf("node %s does not have an instance-type label", nodeSpec.Name)
	}
	
	// Check if we should create a spot instance
	isSpot := false
	if val, exists := nodeSpec.Annotations[NodeAnnotationIsSpot]; exists {
		isSpot, _ = strconv.ParseBool(val)
	}
	
	// Extract the image ID from node labels if present
	imageID := ""
	if val, exists := nodeSpec.Labels[NodeLabelImageID]; exists {
		imageID = val
	} else if p.config != nil && p.config.Spec.ImageConfig != nil {
		imageID = p.config.Spec.ImageConfig.ImageID
	}

	// Generate user-data for bootstrapping the node
	userData, err := p.generateUserData(nodeSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to generate user data: %w", err)
	}

	// Create the instance
	instance, err := p.createInstance(ctx, nodeSpec.Name, instanceType, imageID, userData, isSpot)
	if err != nil {
		apiErrors.WithLabelValues("create_instance").Inc()
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	// Convert the instance to a node
	node := p.convertInstanceToNode(instance, isSpot)

	// Copy over any other labels from the nodeSpec
	for key, value := range nodeSpec.Labels {
		if _, exists := node.Labels[key]; !exists {
			node.Labels[key] = value
		}
	}

	// Copy over any annotations from the nodeSpec
	if node.Annotations == nil {
		node.Annotations = make(map[string]string)
	}

	for key, value := range nodeSpec.Annotations {
		node.Annotations[key] = value
	}

	// Copy over any taints from the nodeSpec
	node.Spec.Taints = append(node.Spec.Taints, nodeSpec.Spec.Taints...)
	
	// Add spot instance taint if needed
	if isSpot {
		node.Spec.Taints = append(node.Spec.Taints, corev1.Taint{
			Key:    "karpenter.bizflycloud.sh/spot",
			Value:  "true",
			Effect: corev1.TaintEffectNoSchedule,
		})
	}

	p.log.Info("Successfully created node", 
		"name", node.Name, 
		"providerID", node.Spec.ProviderID,
		"isSpot", isSpot,
		"zone", instance.Zone,
		"flavor", instance.Flavor,
		"duration", time.Since(start),
	)
	
	// Record metrics
	instanceCreationDuration.Observe(time.Since(start).Seconds())
	
	return node, nil
}

// generateUserData creates the cloud-init user data for node bootstrapping
func (p *Provider) generateUserData(nodeSpec *corev1.Node) (string, error) {
	// Get API server endpoint and token from environment if defined
	apiServerEndpoint := os.Getenv("KARPENTER_API_SERVER_ENDPOINT")
	if apiServerEndpoint == "" {
		apiServerEndpoint = "${API_SERVER_ENDPOINT}"
	}
	
	token := os.Getenv("KARPENTER_TOKEN")
	if token == "" {
		token = "${TOKEN}"
	}
	
	// Get custom user data template if defined
	customUserData := ""
	if p.config != nil && p.config.Spec.ImageConfig != nil && p.config.Spec.ImageConfig.UserDataTemplate != "" {
		customUserData = p.config.Spec.ImageConfig.UserDataTemplate
	}

	// Build the base user data
	userData := `#cloud-config
write_files:
- path: /etc/kubernetes/bootstrap-kubelet.sh
  permissions: '0755'
  content: |
    #!/bin/bash
    set -e
    
    # Configure kubelet
    mkdir -p /etc/kubernetes/manifests
    
    # Join the cluster
    kubeadm join --token ` + token + ` --discovery-token-unsafe-skip-ca-verification ` + apiServerEndpoint + `
    
    # Apply labels and taints
    NODENAME=$(hostname)
    kubectl --kubeconfig /etc/kubernetes/kubelet.conf label node $NODENAME ` + NodeLabelRegion + `=` + p.region + `
    kubectl --kubeconfig /etc/kubernetes/kubelet.conf label node $NODENAME ` + NodeLabelInstanceType + `=` + nodeSpec.Labels[NodeLabelInstanceType]

	// Add zone label if present
	if zone, exists := nodeSpec.Labels[NodeLabelZone]; exists {
		userData += "\n    kubectl --kubeconfig /etc/kubernetes/kubelet.conf label node $NODENAME " + NodeLabelZone + "=" + zone
	}
	
	// Add image label if present
	if image, exists := nodeSpec.Labels[NodeLabelImageID]; exists {
		userData += "\n    kubectl --kubeconfig /etc/kubernetes/kubelet.conf label node $NODENAME " + NodeLabelImageID + "=" + image
	}
	
	// Add spot annotation if present
	if spot, exists := nodeSpec.Annotations[NodeAnnotationIsSpot]; exists && spot == "true" {
		userData += "\n    kubectl --kubeconfig /etc/kubernetes/kubelet.conf annotate node $NODENAME " + NodeAnnotationIsSpot + "=true"
	}
	
	// Add custom user data if available
	if customUserData != "" {
		userData += "\n" + customUserData
	}
	
	// Add runcmd to execute the bootstrap script
	userData += `

runcmd:
  - /etc/kubernetes/bootstrap-kubelet.sh
`

	return userData, nil
}

// DeleteNode deletes a BizFly Cloud server instance
func (p *Provider) DeleteNode(ctx context.Context, node *corev1.Node) error {
	start := time.Now()
	p.log.Info("Deleting node", "name", node.Name)

	// Extract the instance ID from the provider ID
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return fmt.Errorf("node %s does not have a provider ID", node.Name)
	}

	// Parse the provider ID to get the instance ID
	// Expected format: bizflycloud://instance-id
	instanceID := strings.TrimPrefix(providerID, ProviderIDPrefix)
	if instanceID == providerID {
		return fmt.Errorf("invalid provider ID format: %s", providerID)
	}

	// Delete the instance
	err := p.deleteInstance(ctx, instanceID)
	if err != nil {
		// Check if the error is because the instance doesn't exist
		if strings.Contains(err.Error(), "not found") {
			p.log.Info("Instance already deleted", "name", node.Name, "instanceID", instanceID)
			return nil
		}
		
		apiErrors.WithLabelValues("delete_instance").Inc()
		return fmt.Errorf("failed to delete instance %s: %w", instanceID, err)
	}

	p.log.Info("Successfully deleted node", 
		"name", node.Name, 
		"instanceID", instanceID,
		"duration", time.Since(start),
	)
	
	// Record metrics
	instanceDeletionDuration.Observe(time.Since(start).Seconds())
	
	return nil
}

// GetInstanceTypes returns the available instance types
func (p *Provider) GetInstanceTypes(ctx context.Context) ([]corev1.NodeSelectorRequirement, error) {
	p.log.Info("Getting instance types")

	// Get all instance offerings
	offerings, err := p.getAllInstanceOfferings(ctx)
	if err != nil {
		apiErrors.WithLabelValues("get_instance_types").Inc()
		return nil, fmt.Errorf("failed to get instance offerings: %w", err)
	}

	// Convert offerings to requirements
	requirements := p.convertOfferingsToRequirements(offerings)

	p.log.Info("Retrieved instance types", "count", len(offerings))
	return requirements, nil
}

// GetAvailableZones returns the available zones in the configured region
func (p *Provider) GetAvailableZones(ctx context.Context) ([]string, error) {
	p.log.Info("Getting available zones", "region", p.region)
	
	// In production, this would call the BizFly API to get available zones
	// For now, we'll use hardcoded values based on region
	zones := []string{
		fmt.Sprintf("%s-a", p.region),
		fmt.Sprintf("%s-b", p.region),
	}
	
	p.log.Info("Retrieved available zones", "count", len(zones))
	return zones, nil
}

// GetSpotInstances returns if spot instances are available 
func (p *Provider) GetSpotInstances(ctx context.Context) (bool, error) {
	// In production, this would check if spot instances are available
	// For now, we'll assume they are
	return true, nil
}

// DetectNodeDrift checks if the given node has drifted from its desired state
func (p *Provider) DetectNodeDrift(ctx context.Context, node *corev1.Node) (bool, error) {
	p.log.V(1).Info("Checking for node drift", "name", node.Name)
	
	// Skip if provider ID is not set
	if node.Spec.ProviderID == "" {
		return false, nil
	}
	
	// Extract instance ID
	instanceID := strings.TrimPrefix(node.Spec.ProviderID, ProviderIDPrefix)
	if instanceID == node.Spec.ProviderID {
		return false, fmt.Errorf("invalid provider ID format: %s", node.Spec.ProviderID)
	}
	
	// Get the instance
	instance, err := p.getInstance(ctx, instanceID)
	if err != nil {
		// If the instance is not found, it's definitely drifted
		if strings.Contains(err.Error(), "not found") {
			p.log.Info("Instance not found, node has drifted", "name", node.Name, "instanceID", instanceID)
			return true, nil
		}
		
		return false, fmt.Errorf("failed to get instance %s: %w", instanceID, err)
	}
	
	// Check if instance is in a terminal state
	switch instance.Status {
	case "ERROR", "DELETED", "SHUTOFF":
		p.log.Info("Instance in terminal state, node has drifted", 
			"name", node.Name, 
			"instanceID", instanceID, 
			"status", instance.Status,
		)
		return true, nil
	}
	
	// Instance exists and is running, no drift detected
	return false, nil
}

// ReconcileNodeDrift attempts to fix a drifted node
func (p *Provider) ReconcileNodeDrift(ctx context.Context, node *corev1.Node) error {
	p.log.Info("Reconciling node drift", "name", node.Name)
	
	// For now, we'll just recreate the node
	// In a more advanced implementation, we could try to repair the instance
	
	// We'll need to delete and recreate the node
	// First, get a copy of the node to recreate it later
	nodeCopy := node.DeepCopy()
	
	// Delete the node from the API server
	if err := p.client.Delete(ctx, node); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("failed to delete drifted node %s: %w", node.Name, err)
	}
	
	// Recreate the node
	_, err := p.CreateNode(ctx, nodeCopy)
	if err != nil {
		return fmt.Errorf("failed to recreate drifted node %s: %w", node.Name, err)
	}
	
	p.log.Info("Successfully reconciled drifted node", "name", node.Name)
	return nil
}

// MarshalJSON implements json.Marshaler for custom serialization 
func (p *Provider) MarshalJSON() ([]byte, error) {
	// Only serialize essential information
	return json.Marshal(map[string]interface{}{
		"providerName": p.providerName,
		"region":       p.region,
	})
}

// NeedLeaderElection implements Runnable
func (p *Provider) NeedLeaderElection() bool {
	return false
}
