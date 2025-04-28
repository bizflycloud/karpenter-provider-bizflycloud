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
) (*CloudProvider, error) {
	return &CloudProvider{
		instanceTypeProvider: instanceTypeProvider,
		client:               *bizflyClient,
		kubeClient:           kubeClient,
		bizflyClient:         bizflyClient,
		log:                  log,
		region:               config.Spec.Region,
		zone:                 config.Spec.Zone,
		config:               config,
	}, nil
}

// GetInstanceTypes returns the supported instance types for the provider
func (p *CloudProvider) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*cloudprovider.InstanceType, error) {
	nodeClass, err := p.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		if errors.IsNotFound(err) {
			// If we can't resolve the NodeClass, then it's impossible for us to resolve the instance types
			// p.recorder.Publish(cloudproviderevents.NodePoolFailedToResolveNodeClass(nodePool))
			return nil, nil
		}
		return nil, fmt.Errorf("resolving node class, %w", err)
	}
	// TODO, break this coupling
	instanceTypes, err := p.instanceTypeProvider.List(ctx, nodeClass)
	if err != nil {
		return nil, err
	}
	return instanceTypes, nil
}

func (p *CloudProvider) resolveNodeClassFromNodePool(ctx context.Context, nodePool *v1.NodePool) (*v1bizfly.BizflyCloudNodeClass, error) {
	nodeClass := &v1bizfly.BizflyCloudNodeClass{}
	if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: nodePool.Spec.Template.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}
	if !nodeClass.DeletionTimestamp.IsZero() {
		// For the purposes of NodeClass CloudProvider resolution, we treat deleting NodeClasses as NotFound,
		// but we return a different error message to be clearer to users
		return nil, newTerminatingNodeClassError(nodeClass.Name)
	}
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
	// Convert NodeClaim to Node
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
		return nil, err
	}

	// Convert back to NodeClaim
	nodeClaim.Status.ProviderID = createdNode.Spec.ProviderID
	nodeClaim.Status.Capacity = createdNode.Status.Capacity
	nodeClaim.Status.Allocatable = createdNode.Status.Allocatable

	return nodeClaim, nil
}

// Delete deletes an instance from BizFly Cloud
func (p *CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	// Convert NodeClaim to Node
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: nodeClaim.Name,
		},
		Spec: corev1.NodeSpec{
			ProviderID: nodeClaim.Status.ProviderID,
		},
	}
	return p.DeleteNode(ctx, node)
}

// List returns all instances managed by this provider
func (p *CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	// Get all nodes
	nodes := &corev1.NodeList{}
	if err := p.kubeClient.List(ctx, nodes); err != nil {
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

	return nodeClaims, nil
}

// GetOptions returns the options for the provider
func (p *CloudProvider) GetOptions() *v1.NodePoolSpec {
	return &v1.NodePoolSpec{}
}

// getProviderConfig retrieves the ProviderConfig CRD
func (p *CloudProvider) getProviderConfig(ctx context.Context) (*v1bizfly.ProviderConfig, error) {
	// Get the provider config
	config := &v1bizfly.ProviderConfig{}
	if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: ProviderConfigName}, config); err != nil {
		return nil, fmt.Errorf("failed to get ProviderConfig %q: %w", ProviderConfigName, err)
	}

	// Ensure default values are set if not specified
	if config.Spec.Tagging == nil {
		config.Spec.Tagging = &v1bizfly.TaggingSpec{
			EnableResourceTags: true,
			TagPrefix:          "karpenter.bizflycloud.sh/",
		}
	}

	if config.Spec.CloudConfig == nil {
		config.Spec.CloudConfig = &v1bizfly.CloudConfigSpec{
			APIEndpoint:       "https://manage.bizflycloud.vn/api",
			RetryMaxAttempts:  5,
			RetryInitialDelay: "1s",
			Timeout:           "30s",
		}
	}

	if config.Spec.ImageConfig == nil {
		config.Spec.ImageConfig = &v1bizfly.ImageConfigSpec{
			ImageID:      "ubuntu-20.04",
			RootDiskSize: 20,
		}
	}

	if config.Spec.DriftDetection == nil {
		config.Spec.DriftDetection = &v1bizfly.DriftDetectionSpec{
			Enabled:           true,
			ReconcileInterval: "5m",
			AutoRemediate:     true,
		}
	}

	return config, nil
}

// getBizFlyClient creates a new BizFly Cloud client
func (p *CloudProvider) getBizFlyClient(ctx context.Context) (*gobizfly.Client, error) {
	// Get the secret containing the credentials
	secret := &corev1.Secret{}
	if err := p.kubeClient.Get(ctx, types.NamespacedName{
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

	// Create a new BizFly Cloud client
	p.log.Info("Creating BizFly Cloud client", "endpoint", apiEndpoint)
	client, err := gobizfly.NewClient(
		gobizfly.WithAPIURL(apiEndpoint),
		gobizfly.WithProjectID(p.config.Spec.SecretRef.Name),
		gobizfly.WithRegionName(p.config.Spec.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create BizFly Cloud client: %w", err)
	}

	// Authenticate with BizFly Cloud
	p.log.Info("Authenticating with BizFly Cloud")
	token, err := client.Token.Init(
		ctx,
		&gobizfly.TokenCreateRequest{
			AuthMethod:    "password",
			Username:      string(email),
			Password:      string(password),
			AppCredID:     "",
			AppCredSecret: "",
		})
	if err != nil {
		return nil, fmt.Errorf("cannot create token: %s", err)
	}

	client.SetKeystoneToken(token)

	p.log.Info("Successfully authenticated with BizFly Cloud")
	return client, nil
}

// CreateNode creates a new BizFly Cloud server instance
func (p *CloudProvider) CreateNode(ctx context.Context, nodeSpec *corev1.Node) (*corev1.Node, error) {
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
	instanceProvider := &instance.Provider{
		Client:       p.kubeClient,
		Log:          p.log,
		BizflyClient: p.bizflyClient,
		Region:       p.region,
		Config:       p.config,
	}
	instance, err := instanceProvider.CreateInstance(ctx, nodeSpec.Name, instanceType, imageID, userData, isSpot)
	if err != nil {
		apiErrors.WithLabelValues("create_instance").Inc()
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	// Convert the instance to a node
	node := instanceProvider.ConvertToNode(instance, isSpot)

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
func (p *CloudProvider) generateUserData(nodeSpec *corev1.Node) (string, error) {
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
