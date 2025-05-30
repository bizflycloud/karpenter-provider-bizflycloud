package bizflycloud

import (
	"context"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"
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

    NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
    
	DriftReasonNodeNotFound      = "NodeNotFound"
    DriftReasonNodeClassDrifted  = "NodeClassDrifted"
    DriftReasonInstanceNotFound  = "InstanceNotFound"
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

    // Add these new metrics to your existing metrics
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

    nodeCreationMutex sync.Mutex
    lastNodeCreation  time.Time
    minCreationInterval = 180 * time.Second
)

// Update your init() function to register these metrics
func init() {
    providerRegistry := prometheus.NewRegistry()
    providerRegistry.MustRegister(
        instanceCreationDuration,
        instanceDeletionDuration,
        driftCheckDuration,
        apiErrors,
        nodeConsolidationEvents,      // Add this
        nodeTerminationDuration,      // Add this
        underutilizedNodesDetected,   // Add this
    )
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
func (p *CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {

    nodeCreationMutex.Lock()
    timeSinceLastCreation := time.Since(lastNodeCreation)
    if timeSinceLastCreation < minCreationInterval {
        sleepTime := minCreationInterval - timeSinceLastCreation
        p.log.Info("Throttling node creation",
            "name", nodeClaim.Name,
            "sleepTime", sleepTime,
            "timeSinceLastCreation", timeSinceLastCreation)
        nodeCreationMutex.Unlock()
        time.Sleep(sleepTime)
        nodeCreationMutex.Lock()
    }
    lastNodeCreation = time.Now()
    nodeCreationMutex.Unlock()

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
			Labels:      nodeClaim.Labels,
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

// Delete deletes a NodeClaim from BizFly Cloud
func (p *CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
    start := time.Now()
    defer func() {
        nodeTerminationDuration.Observe(time.Since(start).Seconds())
    }()

    p.log.Info("Deleting node claim for scale-down",
        "name", nodeClaim.Name,
        "providerID", nodeClaim.Status.ProviderID,
        "finalizers", nodeClaim.Finalizers,
        "deletionTimestamp", nodeClaim.DeletionTimestamp)

    // Delete cloud resources first
    providerID := nodeClaim.Status.ProviderID
    if providerID != "" {
        instanceID := strings.TrimPrefix(providerID, ProviderIDPrefix)
        if instanceID == providerID {
            return fmt.Errorf("invalid provider ID format: %s", providerID)
        }

        instanceProvider := &instance.Provider{
            Client:       p.kubeClient,
            Log:          p.log,
            BizflyClient: p.bizflyClient,
            Region:       p.region,
            Config:       p.config,
        }
        
        err := instanceProvider.DeleteInstance(ctx, instanceID)
        if err != nil {
            if strings.Contains(err.Error(), "not found") || 
               strings.Contains(err.Error(), "does not exist") ||
               strings.Contains(err.Error(), "404") {
                p.log.Info("Instance already deleted in cloud provider", 
                    "name", nodeClaim.Name, 
                    "instanceID", instanceID)
            } else {
                apiErrors.WithLabelValues("delete_instance").Inc()
                return fmt.Errorf("failed to delete instance %s: %w", instanceID, err)
            }
        }
    }

    // WORKAROUND: Explicitly remove Karpenter finalizers if they're stuck
    if nodeClaim.DeletionTimestamp != nil && 
       time.Since(nodeClaim.DeletionTimestamp.Time) > 5*time.Minute {
        
        p.log.Info("NodeClaim has been terminating for too long, removing finalizers",
            "name", nodeClaim.Name,
            "terminatingFor", time.Since(nodeClaim.DeletionTimestamp.Time))
        
        // Remove Karpenter finalizers
        var newFinalizers []string
        for _, finalizer := range nodeClaim.Finalizers {
            if !strings.Contains(finalizer, "karpenter.sh") {
                newFinalizers = append(newFinalizers, finalizer)
            }
        }
        
        if len(newFinalizers) != len(nodeClaim.Finalizers) {
            nodeClaim.Finalizers = newFinalizers
            if err := p.kubeClient.Update(ctx, nodeClaim); err != nil {
                p.log.Error(err, "Failed to remove stuck finalizers", 
                    "name", nodeClaim.Name)
                // Don't return error - let Karpenter retry
            } else {
                p.log.Info("Removed stuck finalizers from NodeClaim", 
                    "name", nodeClaim.Name,
                    "remainingFinalizers", len(newFinalizers))
            }
        }
    }

    p.log.Info("Successfully deleted cloud resources for node claim",
        "name", nodeClaim.Name,
        "duration", time.Since(start))

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
	node.Status.Phase = corev1.NodeRunning
	node.Status.Conditions = []corev1.NodeCondition{
		{
			Type:               corev1.NodeReady,
			Status:             corev1.ConditionTrue,
			LastTransitionTime: metav1.Now(),
			Reason:             "KubeletReady",
			Message:            "kubelet is posting ready status",
		},
	}
	
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

func (p *CloudProvider) IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error) {
    start := time.Now()
    defer func() {
        driftCheckDuration.Observe(time.Since(start).Seconds())
    }()

    p.log.V(1).Info("Checking if node claim is drifted",
        "name", nodeClaim.Name,
        "providerID", nodeClaim.Status.ProviderID)

    // Check if the node claim has a provider ID
    if nodeClaim.Status.ProviderID == "" {
        p.log.V(1).Info("NodeClaim has no provider ID, considering it drifted",
            "name", nodeClaim.Name)
        return cloudprovider.DriftReason(DriftReasonNodeNotFound), nil
    }

    // Extract instance ID and check if instance exists
    instanceID := strings.TrimPrefix(nodeClaim.Status.ProviderID, ProviderIDPrefix)
    instanceProvider := &instance.Provider{
        Client:       p.kubeClient,
        Log:          p.log,
        BizflyClient: p.bizflyClient,
        Region:       p.region,
        Config:       p.config,
    }
    
    inst, err := instanceProvider.GetInstance(ctx, instanceID)
    if err != nil {
        if strings.Contains(err.Error(), "not found") {
            p.log.Info("Instance not found, node is drifted",
                "name", nodeClaim.Name,
                "instanceID", instanceID)
            return cloudprovider.DriftReason(DriftReasonNodeNotFound), nil
        }
        return "", err
    }

    // Check instance state for drift
    switch inst.Status {
    case "ERROR", "DELETED", "SHUTOFF", "SUSPENDED":
        p.log.Info("Instance in terminal state, node is drifted",
            "name", nodeClaim.Name,
            "instanceID", instanceID,
            "status", inst.Status)
        return cloudprovider.DriftReason(DriftReasonInstanceNotFound), nil
    }

    // Get NodeClass for drift comparison
    nodeClassRef := nodeClaim.Labels["karpenter.bizflycloud.com/bizflycloudnodeclass"]
    if nodeClassRef == "" {
        p.log.V(1).Info("No NodeClass reference found", "name", nodeClaim.Name)
        return "", nil
    }

    nodeClass := &v1bizfly.BizflyCloudNodeClass{}
    if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: nodeClassRef}, nodeClass); err != nil {
        p.log.Error(err, "Failed to get NodeClass", "name", nodeClassRef)
        return "", nil // Don't fail drift check if NodeClass is missing
    }

    // Check for NodeClass configuration drift
    if p.hasConfigurationDrift(inst, nodeClass, nodeClaim) {
        p.log.Info("NodeClass configuration drift detected",
            "name", nodeClaim.Name,
            "instanceID", instanceID)
        return cloudprovider.DriftReason(DriftReasonNodeClassDrifted), nil
    }

    // Check for instance type drift (for consolidation)
    if p.hasInstanceTypeDrift(inst, nodeClaim) {
        p.log.Info("Instance type drift detected for consolidation",
            "name", nodeClaim.Name,
            "instanceID", instanceID)
        return cloudprovider.DriftReason("ConsolidationCandidate"), nil
    }

    p.log.V(1).Info("NodeClaim is not drifted", "name", nodeClaim.Name)
    return "", nil
}



func (p *CloudProvider) isNodeClassDrifted(ctx context.Context, nodeClaim *v1.NodeClaim, nodePool *v1.NodePool, nodeClass *v1bizfly.BizflyCloudNodeClass) (cloudprovider.DriftReason, error) {
    // Extract instance ID from provider ID
    if nodeClaim.Status.ProviderID == "" {
        return DriftReasonNodeNotFound, nil
    }

    instanceID := strings.TrimPrefix(nodeClaim.Status.ProviderID, ProviderIDPrefix)
    if instanceID == nodeClaim.Status.ProviderID {
        return "", fmt.Errorf("invalid provider ID format: %s", nodeClaim.Status.ProviderID)
    }

    // Get the instance
    instanceProvider := &instance.Provider{
        Client:       p.kubeClient,
        Log:          p.log,
        BizflyClient: p.bizflyClient,
        Region:       p.region,
        Config:       p.config,
    }
    
    inst, err := instanceProvider.GetInstance(ctx, instanceID)
    if err != nil {
        if strings.Contains(err.Error(), "not found") {
            return DriftReasonNodeNotFound, nil
        }
        return "", err
    }

    // Check if instance configuration matches NodeClass requirements
    if nodeClass.Spec.ImageID != "" && inst.Tags != nil {
        // Check image ID in instance metadata/tags
        for _, tag := range inst.Tags {
            if strings.HasPrefix(tag, "image_id=") && !strings.Contains(tag, nodeClass.Spec.ImageID) {
                return DriftReasonNodeClassDrifted, nil
            }
        }
    }

    // Check instance state
    switch inst.Status {
    case "ERROR", "DELETED", "SHUTOFF":
        return DriftReasonNodeNotFound, nil
    }

    return "", nil
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

// In your cloudprovider.go, add this helper
func (p *CloudProvider) isRetryableError(err error) bool {
    if err == nil {
        return false
    }
    
    // Don't retry for "not found" errors
    if strings.Contains(err.Error(), "not found") || 
       strings.Contains(err.Error(), "does not exist") ||
       strings.Contains(err.Error(), "404") {
        return false
    }
    
    // Retry for network errors, rate limits, etc.
    return strings.Contains(err.Error(), "timeout") ||
           strings.Contains(err.Error(), "rate limit") ||
           strings.Contains(err.Error(), "connection") ||
           strings.Contains(err.Error(), "temporary")

}

// hasConfigurationDrift checks if the instance configuration differs from NodeClass
func (p *CloudProvider) hasConfigurationDrift(inst *instance.Instance, nodeClass *v1bizfly.BizflyCloudNodeClass, nodeClaim *v1.NodeClaim) bool {
    // Check image drift
    if nodeClass.Spec.ImageID != "" {
        currentImageID := p.extractImageIDFromInstance(inst)
        if currentImageID != "" && currentImageID != nodeClass.Spec.ImageID {
            p.log.Info("Image drift detected",
                "current", currentImageID,
                "expected", nodeClass.Spec.ImageID)
            return true
        }
    }

    // Check disk type drift
    if nodeClass.Spec.DiskType != "" {
        currentDiskType := p.extractDiskTypeFromInstance(inst)
        if currentDiskType != "" && currentDiskType != nodeClass.Spec.DiskType {
            p.log.Info("Disk type drift detected",
                "current", currentDiskType,
                "expected", nodeClass.Spec.DiskType)
            return true
        }
    }

    // Check VPC network drift
    if len(nodeClass.Spec.VPCNetworkIDs) > 0 {
        currentNetworkID := p.extractNetworkIDFromInstance(inst)
        expectedNetworkID := nodeClass.Spec.VPCNetworkIDs[0]
        if currentNetworkID != "" && currentNetworkID != expectedNetworkID {
            p.log.Info("VPC network drift detected",
                "current", currentNetworkID,
                "expected", expectedNetworkID)
            return true
        }
    }

    return false
}

// hasInstanceTypeDrift checks if a smaller/cheaper instance could be used
func (p *CloudProvider) hasInstanceTypeDrift(inst *instance.Instance, nodeClaim *v1.NodeClaim) bool {
    // This is where you'd implement consolidation logic
    // For now, we'll check if the instance has been underutilized
    
    // Check if instance is significantly oversized for current workload
    // This would require integration with metrics to determine actual usage
    // For simplicity, we'll check instance age and type patterns
    
    instanceAge := time.Since(inst.CreatedAt)
    
    // If instance is old and large, it might be a consolidation candidate
    if instanceAge > 30*time.Minute && p.isLargeInstance(inst.Flavor) {
        p.log.V(1).Info("Large instance detected as potential consolidation candidate",
            "flavor", inst.Flavor,
            "age", instanceAge)
        return true
    }
    
    return false
}

// Helper methods for extracting instance metadata
func (p *CloudProvider) extractImageIDFromInstance(inst *instance.Instance) string {
    for _, tag := range inst.Tags {
        if strings.HasPrefix(tag, "image_id=") {
            return strings.TrimPrefix(tag, "image_id=")
        }
    }
    return ""
}

func (p *CloudProvider) extractDiskTypeFromInstance(inst *instance.Instance) string {
    for _, tag := range inst.Tags {
        if strings.HasPrefix(tag, "disk_type=") {
            return strings.TrimPrefix(tag, "disk_type=")
        }
    }
    return ""
}

func (p *CloudProvider) extractNetworkIDFromInstance(inst *instance.Instance) string {
    for _, tag := range inst.Tags {
        if strings.HasPrefix(tag, "bke_node_network_id=") {
            return strings.TrimPrefix(tag, "bke_node_network_id=")
        }
    }
    return ""
}

func (p *CloudProvider) isLargeInstance(flavor string) bool {
    // Define what constitutes a "large" instance for consolidation
    largeInstancePatterns := []string{
        "12c_", "16c_", "24c_", "32c_",  // High CPU instances
        "_16g", "_24g", "_32g", "_64g",  // High memory instances
    }
    
    for _, pattern := range largeInstancePatterns {
        if strings.Contains(flavor, pattern) {
            return true
        }
    }
    return false
}
