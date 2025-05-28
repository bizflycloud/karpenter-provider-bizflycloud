package bizflycloud

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/awslabs/operatorpkg/status"
	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
	"github.com/prometheus/client_golang/prometheus"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis"
	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
)

const (
	// ProviderName is the name of the provider
	ProviderName = "karpenter.bizflycloud.com"

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
	// Create a new registry for provider-specific metrics
	providerRegistry := prometheus.NewRegistry()

	// Register metrics with the provider registry
	providerRegistry.MustRegister(
		instanceCreationDuration,
		instanceDeletionDuration,
		apiErrors,
	)

	// Register the provider registry with the global registry
	metrics.Registry.MustRegister(providerRegistry)
}

// CloudProvider implements the CloudProvider interface for BizFly Cloud
type CloudProvider struct {
	kubeClient client.Client

	instanceTypeProvider instancetype.Provider
	client               gobizfly.Client
	bizflyClient         *gobizfly.Client
	log                  logr.Logger
	region               string
	zone                 string
	config               *v1bizfly.ProviderConfig
}

// NewProvider creates a new BizFly Cloud provider
func New(
	instanceTypeProvider instancetype.Provider,
	kubeClient client.Client,
	bizflyClient *gobizfly.Client,
	log logr.Logger,
	config *v1bizfly.ProviderConfig,
) *CloudProvider {
	return &CloudProvider{
		instanceTypeProvider: instanceTypeProvider,
		client:               *bizflyClient,
		kubeClient:           kubeClient,
		bizflyClient:         bizflyClient,
		log:                  log,
		region:               config.Spec.Region,
		zone:                 config.Spec.Zone,
		config:               config,
	}
}

// GetInstanceTypes returns the supported instance types for the provider
func (p *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*cloudprovider.InstanceType, error) {
	p.log.V(1).Info("Getting instance types", "nodePool", nodePool.Name)

	nodeClass, err := p.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		if errors.IsNotFound(err) {
			p.log.Error(err, "NodeClass not found for NodePool", 
				"nodePool", nodePool.Name,
				"nodeClassRef", nodePool.Spec.Template.Spec.NodeClassRef.Name)
			return nil, nil
		}
		return nil, fmt.Errorf("resolving node class, %w", err)
	}

	p.log.V(1).Info("Resolved NodeClass", 
		"nodeClass", nodeClass.Name,
		"nodePool", nodePool.Name)

	instanceTypes, err := p.instanceTypeProvider.List(ctx, nodeClass)
	if err != nil {
		p.log.Error(err, "Failed to list instance types",
			"nodeClass", nodeClass.Name,
			"nodePool", nodePool.Name)
		return nil, err
	}

	p.log.Info("Successfully retrieved instance types",
		"count", len(instanceTypes),
		"nodeClass", nodeClass.Name,
		"nodePool", nodePool.Name)

	return instanceTypes, nil
}

func (p *CloudProvider) resolveNodeClassFromNodePool(ctx context.Context, nodePool *v1.NodePool) (*v1bizfly.BizflyCloudNodeClass, error) {
	p.log.V(1).Info("Resolving NodeClass from NodePool",
		"nodePool", nodePool.Name,
		"nodeClassRef", nodePool.Spec.Template.Spec.NodeClassRef.Name)

	nodeClass := &v1bizfly.BizflyCloudNodeClass{}
	if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: nodePool.Spec.Template.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		p.log.Error(err, "Failed to get NodeClass",
			"nodePool", nodePool.Name,
			"nodeClassRef", nodePool.Spec.Template.Spec.NodeClassRef.Name)
		return nil, err
	}

	if !nodeClass.DeletionTimestamp.IsZero() {
		p.log.Info("NodeClass is being deleted",
			"nodeClass", nodeClass.Name,
			"deletionTimestamp", nodeClass.DeletionTimestamp)
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}

	p.log.V(1).Info("Successfully resolved NodeClass",
		"nodeClass", nodeClass.Name,
		"nodePool", nodePool.Name)

	return nodeClass, nil
}

// newTerminatingNodeClassError returns a NotFound error for handling by
func newTerminatingNodeClassError(name string) *errors.StatusError {
	qualifiedResource := schema.GroupResource{Group: apis.Group, Resource: "bizflycloudnodeclasses"}
	err := errors.NewNotFound(qualifiedResource, name)
	err.ErrStatus.Message = fmt.Sprintf("%s %q is terminating, treating as not found", qualifiedResource.String(), name)
	return err
}

// GetSupportedNodeClasses returns the supported node classes for the provider
func (p *CloudProvider) GetSupportedNodeClasses() []status.Object {
	return []status.Object{
		&v1bizfly.BizflyCloudNodeClass{},
	}
}

// Create implements cloudprovider.CloudProvider
// Create implements cloudprovider.CloudProvider
func (p *CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	p.log.V(1).Info("Creating node claim",
		"name", nodeClaim.Name,
		"labels", nodeClaim.Labels,
		"annotations", nodeClaim.Annotations)

	// Extract instance type from NodeClaim requirements (NOT from labels)
	instanceTypeName := ""
	for _, req := range nodeClaim.Spec.Requirements {
		if req.Key == corev1.LabelInstanceTypeStable && len(req.Values) > 0 {
			instanceTypeName = req.Values[0]
			break
		}
	}

	if instanceTypeName == "" {
		return nil, fmt.Errorf("no instance type found in NodeClaim requirements")
	}

	// SET the instance-type label (don't check for it)
	if nodeClaim.Labels == nil {
		nodeClaim.Labels = make(map[string]string)
	}
	
	// Add required labels
	nodeClaim.Labels[corev1.LabelInstanceTypeStable] = instanceTypeName
	nodeClaim.Labels[corev1.LabelArchStable] = "amd64"
	nodeClaim.Labels["karpenter.sh/capacity-type"] = "on-demand"
	nodeClaim.Labels[NodeLabelRegion] = p.region

	p.log.Info("Set instance-type label", 
		"name", nodeClaim.Name,
		"instanceType", instanceTypeName,
		"labels", nodeClaim.Labels)

	// Convert NodeClaim to Node with the updated labels
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nodeClaim.Name,
			Labels:      nodeClaim.Labels,  // Use the updated labels
			Annotations: nodeClaim.Annotations,
		},
		Spec: corev1.NodeSpec{
			Taints: nodeClaim.Spec.Taints,
		},
	}

	// Create the node
	createdNode, err := p.CreateNode(ctx, node)
	if err != nil {
		p.log.Error(err, "Failed to create node",
			"name", nodeClaim.Name)
		return nil, err
	}

	// Convert back to NodeClaim
	nodeClaim.Status.ProviderID = createdNode.Spec.ProviderID
	nodeClaim.Status.Capacity = createdNode.Status.Capacity
	nodeClaim.Status.Allocatable = createdNode.Status.Allocatable

	p.log.Info("Successfully created node claim",
		"name", nodeClaim.Name,
		"providerID", nodeClaim.Status.ProviderID,
		"capacity", nodeClaim.Status.Capacity,
		"allocatable", nodeClaim.Status.Allocatable)

	return nodeClaim, nil
}

// Delete deletes an instance from BizFly Cloud
func (p *CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	p.log.V(1).Info("Deleting node claim",
		"name", nodeClaim.Name,
		"providerID", nodeClaim.Status.ProviderID)

	// Convert NodeClaim to Node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeClaim.Name,
		},
		Spec: corev1.NodeSpec{
			ProviderID: nodeClaim.Status.ProviderID,
		},
	}

	err := p.DeleteNode(ctx, node)
	if err != nil {
		p.log.Error(err, "Failed to delete node",
			"name", nodeClaim.Name,
			"providerID", nodeClaim.Status.ProviderID)
		return err
	}

	p.log.Info("Successfully deleted node claim",
		"name", nodeClaim.Name,
		"providerID", nodeClaim.Status.ProviderID)

	return nil
}

// List returns all instances managed by this provider
func (p *CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	p.log.V(1).Info("Listing all node claims")

	// Get all nodes
	nodes := &corev1.NodeList{}
	if err := p.kubeClient.List(ctx, nodes); err != nil {
		p.log.Error(err, "Failed to list nodes")
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Convert nodes to NodeClaims
	var nodeClaims []*v1.NodeClaim
	for _, node := range nodes.Items {
		nodeClaim := &v1.NodeClaim{
			ObjectMeta: metav1.ObjectMeta{
				Name: node.Name,
			},
			Status: v1.NodeClaimStatus{
				ProviderID:  node.Spec.ProviderID,
				Capacity:    node.Status.Capacity,
				Allocatable: node.Status.Allocatable,
			},
		}
		nodeClaims = append(nodeClaims, nodeClaim)
	}

	p.log.Info("Successfully listed node claims",
		"count", len(nodeClaims))

	return nodeClaims, nil
}

// GetOptions returns the options for the provider
func (p *CloudProvider) GetOptions() *v1.NodePoolSpec {
	return &v1.NodePoolSpec{}
}

// CreateNode creates a new BizFly Cloud server instance
func (p *CloudProvider) CreateNode(ctx context.Context, nodeSpec *corev1.Node) (*corev1.Node, error) {
    start := time.Now()
    p.log.Info("Creating node",
        "name", nodeSpec.Name,
        "labels", nodeSpec.Labels,
        "annotations", nodeSpec.Annotations)

    // Extract instance type from node labels
    instanceType, exists := nodeSpec.Labels[NodeLabelInstanceType]
    if !exists {
        p.log.Error(nil, "Node missing instance-type label",
            "name", nodeSpec.Name,
            "labels", nodeSpec.Labels)
        return nil, fmt.Errorf("node %s does not have an instance-type label", nodeSpec.Name)
    }

    // Check if we should create a spot instance
    isSpot := false
    if val, exists := nodeSpec.Annotations[NodeAnnotationIsSpot]; exists {
        isSpot, _ = strconv.ParseBool(val)
        p.log.V(1).Info("Spot instance requested",
            "name", nodeSpec.Name,
            "isSpot", isSpot)
    }

    // Extract the image ID from node labels if present
    imageID := ""
    if val, exists := nodeSpec.Labels[NodeLabelImageID]; exists {
        imageID = val
        p.log.V(1).Info("Using image from node labels",
            "name", nodeSpec.Name,
            "imageID", imageID)
    } else if p.config != nil && p.config.Spec.ImageConfig != nil {
        imageID = p.config.Spec.ImageConfig.ImageID
        p.log.V(1).Info("Using image from config",
            "name", nodeSpec.Name,
            "imageID", imageID)
    }

    // Create the instance
    instanceProvider := &instance.Provider{
        Client:       p.kubeClient,
        Log:          p.log,
        BizflyClient: p.bizflyClient,
        Region:       p.region,
        Config:       p.config,
    }

    p.log.V(1).Info("Creating instance",
        "name", nodeSpec.Name,
        "instanceType", instanceType,
        "imageID", imageID,
        "isSpot", isSpot)

    // Resolve nodeClass from nodeSpec
    nodeClass, err := p.resolveNodeClassFromNodeSpec(ctx, nodeSpec)
    if err != nil {
        return nil, fmt.Errorf("failed to resolve node class: %w", err)
    }

    // Create nodeClaim from nodeSpec
    nodeClaim := p.convertNodeSpecToNodeClaim(nodeSpec)

    // Create the instance
    gobizflyServer, err := instanceProvider.CreateInstance(ctx, nodeClaim, nodeClass)
    if err != nil {
        return nil, fmt.Errorf("failed to create instance: %w", err)
    }

    // Convert gobizfly.Server to instance.Instance
    instanceObj := instanceProvider.ConvertGobizflyServerToInstance(gobizflyServer)
    if instanceObj == nil {
        return nil, fmt.Errorf("failed to convert server to instance")
    }

    // Convert the instance to a node
    node := instanceProvider.ConvertToNode(instanceObj, isSpot)

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

    duration := time.Since(start)
    p.log.Info("Successfully created node",
        "name", node.Name,
        "providerID", node.Spec.ProviderID,
        "isSpot", isSpot,
        "zone", instanceObj.Zone,
        "flavor", instanceObj.Flavor,
        "duration", duration,
        "labels", node.Labels,
        "annotations", node.Annotations,
        "taints", node.Spec.Taints)

    // Record metrics
    instanceCreationDuration.Observe(duration.Seconds())

    return node, nil
}


// generateUserData creates the cloud-init user data for node bootstrapping
func (p *CloudProvider) generateUserData(nodeSpec *corev1.Node) (string, error) {
	p.log.V(1).Info("Generating user data",
		"name", nodeSpec.Name)

	// Get API server endpoint and token from environment if defined
	apiServerEndpoint := os.Getenv("KARPENTER_API_SERVER_ENDPOINT")
	if apiServerEndpoint == "" {
		apiServerEndpoint = "${API_SERVER_ENDPOINT}"
		p.log.V(1).Info("Using default API server endpoint template")
	}

	token := os.Getenv("KARPENTER_TOKEN")
	if token == "" {
		token = "${TOKEN}"
		p.log.V(1).Info("Using default token template")
	}

	// Get custom user data template if defined
	customUserData := ""
	if p.config != nil && p.config.Spec.ImageConfig != nil && p.config.Spec.ImageConfig.UserDataTemplate != "" {
		customUserData = p.config.Spec.ImageConfig.UserDataTemplate
		p.log.V(1).Info("Using custom user data template from config")
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
		p.log.V(1).Info("Added zone label to user data",
			"zone", zone)
	}

	// Add image label if present
	if image, exists := nodeSpec.Labels[NodeLabelImageID]; exists {
		userData += "\n    kubectl --kubeconfig /etc/kubernetes/kubelet.conf label node $NODENAME " + NodeLabelImageID + "=" + image
		p.log.V(1).Info("Added image label to user data",
			"image", image)
	}

	// Add custom user data if available
	if customUserData != "" {
		userData += "\n" + customUserData
		p.log.V(1).Info("Added custom user data template")
	}

	// Add runcmd to execute the bootstrap script
	userData += `

runcmd:
  - /etc/kubernetes/bootstrap-kubelet.sh
`

	p.log.V(1).Info("Successfully generated user data",
		"name", nodeSpec.Name,
		"hasCustomTemplate", customUserData != "")

	return userData, nil
}

// DeleteNode deletes a BizFly Cloud server instance
func (p *CloudProvider) DeleteNode(ctx context.Context, node *corev1.Node) error {
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
	instanceProvider := &instance.Provider{
		Client:       p.kubeClient,
		Log:          p.log,
		BizflyClient: p.bizflyClient,
		Region:       p.region,
		Config:       p.config,
	}
	err := instanceProvider.DeleteInstance(ctx, instanceID)
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

// GetAvailableZones returns the available zones in the configured region
func (p *CloudProvider) GetAvailableZones(ctx context.Context) ([]string, error) {
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
func (p *CloudProvider) GetSpotInstances(ctx context.Context) (bool, error) {
	// In production, this would check if spot instances are available
	// For now, we'll assume they are
	return true, nil
}

// DetectNodeDrift checks if the given node has drifted from its desired state
func (p *CloudProvider) DetectNodeDrift(ctx context.Context, node *corev1.Node) (bool, error) {
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
	instanceProvider := &instance.Provider{
		Client:       p.kubeClient,
		Log:          p.log,
		BizflyClient: p.bizflyClient,
		Region:       p.region,
		Config:       p.config,
	}
	instance, err := instanceProvider.GetInstance(ctx, instanceID)
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
func (p *CloudProvider) ReconcileNodeDrift(ctx context.Context, node *corev1.Node) error {
	p.log.Info("Reconciling node drift", "name", node.Name)

	// For now, we'll just recreate the node
	// In a more advanced implementation, we could try to repair the instance

	// We'll need to delete and recreate the node
	// First, get a copy of the node to recreate it later
	nodeCopy := node.DeepCopy()

	// Delete the node from the API server
	if err := p.kubeClient.Delete(ctx, node); err != nil && !errors.IsNotFound(err) {
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

// NeedLeaderElection implements Runnable
func (p *CloudProvider) NeedLeaderElection() bool {
	return false
}

func (p *CloudProvider) Get(ctx context.Context, id string) (*v1.NodeClaim, error) {
	// Get the instance
	instanceProvider := &instance.Provider{
		Client:       p.kubeClient,
		Log:          p.log,
		BizflyClient: p.bizflyClient,
		Region:       p.region,
		Config:       p.config,
	}
	instance, err := instanceProvider.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}

	// Convert to NodeClaim
	nodeClaim := &v1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name,
		},
		Status: v1.NodeClaimStatus{
			ProviderID: instance.ID,
			// Capacity:    instance.Flavor,
			// Allocatable: instance.Flavor,
		},
	}

	return nodeClaim, nil
}

func (c *CloudProvider) IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error) {
	// Not needed when GetInstanceTypes removes nodepool dependency
	nodePoolName, ok := nodeClaim.Labels[v1.NodePoolLabelKey]
	if !ok {
		return "", nil
	}
	nodePool := &v1.NodePool{}
	if err := c.kubeClient.Get(ctx, types.NamespacedName{Name: nodePoolName}, nodePool); err != nil {
		return "", client.IgnoreNotFound(err)
	}
	if nodePool.Spec.Template.Spec.NodeClassRef == nil {
		return "", nil
	}
	nodeClass, err := c.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		if errors.IsNotFound(err) {
			// We can't determine the drift status for the NodeClaim if we can no longer resolve the NodeClass
			// p.recorder.Publish(cloudproviderevents.NodePoolFailedToResolveNodeClass(nodePool))
			return "", nil
		}
		return "", fmt.Errorf("resolving node class, %w", err)
	}
	driftReason, err := c.isNodeClassDrifted(ctx, nodeClaim, nodePool, nodeClass)
	if err != nil {
		return "", err
	}
	return driftReason, nil
}

// Name returns the name of the provider
func (p *CloudProvider) Name() string {
	return ProviderName
}

// RepairPolicies returns the repair policies for the provider
func (p *CloudProvider) RepairPolicies() []cloudprovider.RepairPolicy {
	return nil
}

// resolveNodeClassFromNodeSpec resolves the NodeClass from a node spec
func (p *CloudProvider) resolveNodeClassFromNodeSpec(ctx context.Context, nodeSpec *corev1.Node) (*v1bizfly.BizflyCloudNodeClass, error) {
    // For now, use the default NodeClass
    // In a real implementation, you might extract this from node labels or annotations
    nodeClass := &v1bizfly.BizflyCloudNodeClass{}
    if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: "default"}, nodeClass); err != nil {
        return nil, err
    }
    return nodeClass, nil
}

// convertNodeSpecToNodeClaim converts a Node spec to a NodeClaim
func (p *CloudProvider) convertNodeSpecToNodeClaim(nodeSpec *corev1.Node) *v1.NodeClaim {
    // Create requirements from node labels
    var requirements []v1.NodeSelectorRequirementWithMinValues
    
    if instanceType, exists := nodeSpec.Labels[corev1.LabelInstanceTypeStable]; exists {
        requirements = append(requirements, v1.NodeSelectorRequirementWithMinValues{
            NodeSelectorRequirement: corev1.NodeSelectorRequirement{
                Key:      corev1.LabelInstanceTypeStable,
                Operator: corev1.NodeSelectorOpIn,
                Values:   []string{instanceType},
            },
        })
    }
    
    if arch, exists := nodeSpec.Labels[corev1.LabelArchStable]; exists {
        requirements = append(requirements, v1.NodeSelectorRequirementWithMinValues{
            NodeSelectorRequirement: corev1.NodeSelectorRequirement{
                Key:      corev1.LabelArchStable,
                Operator: corev1.NodeSelectorOpIn,
                Values:   []string{arch},
            },
        })
    }

    // Add capacity type requirement
    requirements = append(requirements, v1.NodeSelectorRequirementWithMinValues{
        NodeSelectorRequirement: corev1.NodeSelectorRequirement{
            Key:      "karpenter.sh/capacity-type",
            Operator: corev1.NodeSelectorOpIn,
            Values:   []string{"on-demand"},
        },
    })

    return &v1.NodeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name:        nodeSpec.Name,
            Labels:      nodeSpec.Labels,
            Annotations: nodeSpec.Annotations,
        },
        Spec: v1.NodeClaimSpec{
            Requirements: requirements,
            Taints:       nodeSpec.Spec.Taints,
        },
    }
}
