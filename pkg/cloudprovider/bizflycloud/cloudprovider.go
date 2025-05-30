package bizflycloud

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/awslabs/operatorpkg/status"
	"github.com/bizflycloud/gobizfly"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"

	"github.com/bizflycloud/karpenter-provider-bizflycloud/internal/errors"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/internal/logging"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/internal/metrics"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis"
	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance/conversion"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/utils"
)

const (
	// ProviderName is the name of the provider
	ProviderName = "karpenter.bizflycloud.com"

	// ProviderConfigName is the default name for the ProviderConfig
	ProviderConfigName = "default"

	// ProviderID prefix for BizFly Cloud instances
	ProviderIDPrefix = "bizflycloud://"

	// Node labels and annotations
	NodeLabelInstanceType = "node.kubernetes.io/instance-type"
	NodeLabelRegion       = "topology.kubernetes.io/region"
	NodeLabelZone         = "topology.kubernetes.io/zone"
	NodeAnnotationIsSpot  = "karpenter.bizflycloud.sh/instance-spot"
	NodeLabelImageID      = "karpenter.bizflycloud.sh/image-id"
	NodeCategoryLabel     = "karpenter.bizflycloud.com/node-category"

	// Drift reasons
	DriftReasonNodeNotFound     = "NodeNotFound"
	DriftReasonNodeClassDrifted = "NodeClassDrifted"
	DriftReasonInstanceNotFound = "InstanceNotFound"

	// Throttling
	minCreationInterval = 180 * time.Second
)

var (
	nodeCreationMutex sync.Mutex
	lastNodeCreation  time.Time
)

// cloudProviderImpl implements the CloudProvider interface
type cloudProviderImpl struct {
	kubeClient           client.Client
	instanceTypeProvider instancetype.Provider
	bizflyClient         *gobizfly.Client
	log                  *logging.Logger
	region               string
	zone                 string
	config               *v1bizfly.ProviderConfig
}

// New creates a new BizFly Cloud provider
func New(
	instanceTypeProvider instancetype.Provider,
	kubeClient client.Client,
	bizflyClient *gobizfly.Client,
	log *logging.Logger,
	config *v1bizfly.ProviderConfig,
) CloudProvider {
	return &cloudProviderImpl{
		instanceTypeProvider: instanceTypeProvider,
		kubeClient:           kubeClient,
		bizflyClient:         bizflyClient,
		log:                  log,
		region:               config.Spec.Region,
		zone:                 config.Spec.Zone,
		config:               config,
	}
}

// GetInstanceTypes returns the supported instance types for the provider
func (p *cloudProviderImpl) GetInstanceTypes(ctx context.Context, nodePool *v1.NodePool) ([]*cloudprovider.InstanceType, error) {
	p.log.V(1).Info("Getting instance types", "nodePool", nodePool.Name)

	nodeClass, err := p.resolveNodeClassFromNodePool(ctx, nodePool)
	if err != nil {
		if k8serrors.IsNotFound(err) {
			p.log.ErrorWithDetails(err, "NodeClass not found for NodePool",
				"nodePool", nodePool.Name,
				"nodeClassRef", nodePool.Spec.Template.Spec.NodeClassRef.Name)
			return nil, nil
		}
		return nil, fmt.Errorf("resolving node class: %w", err)
	}

	p.log.V(1).Info("Resolved NodeClass",
		"nodeClass", nodeClass.Name,
		"nodePool", nodePool.Name)

	instanceTypes, err := p.instanceTypeProvider.List(ctx, nodeClass)
	if err != nil {
		p.log.ErrorWithDetails(err, "Failed to list instance types",
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

// Create implements cloudprovider.CloudProvider
func (p *cloudProviderImpl) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
	start := time.Now()
	defer func() {
		metrics.RecordInstanceCreation("", "", "", time.Since(start))
	}()

	// Throttle node creation
	p.throttleNodeCreation()

	p.log.V(1).Info("Creating node claim",
		"name", nodeClaim.Name,
		"labels", nodeClaim.Labels,
		"annotations", nodeClaim.Annotations)

	// Extract instance type from NodeClaim requirements
	instanceTypeName := p.extractInstanceTypeFromRequirements(nodeClaim.Spec.Requirements)
	if instanceTypeName == "" {
		return nil, &errors.NodeCreationError{
			NodeName:   nodeClaim.Name,
			Underlying: errors.ErrInvalidInstanceType,
			Retryable:  false,
		}
	}

	// Set required labels
	p.setRequiredLabels(nodeClaim, instanceTypeName)

	// Convert NodeClaim to Node with the updated labels
	node := p.convertNodeClaimToNode(nodeClaim)

	// Create the node
	createdNode, err := p.CreateNode(ctx, node)
	if err != nil {
		return nil, &errors.NodeCreationError{
			NodeName:   nodeClaim.Name,
			Underlying: err,
			Retryable:  errors.IsRetryableError(err),
		}
	}

	// Update NodeClaim status
	p.updateNodeClaimStatus(nodeClaim, createdNode)

	p.log.NodeCreation(nodeClaim.Name, instanceTypeName, p.zone, time.Since(start).String())

	return nodeClaim, nil
}

// Delete deletes a NodeClaim from BizFly Cloud
func (p *cloudProviderImpl) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	start := time.Now()
	defer func() {
		metrics.RecordNodeTermination(time.Since(start))
	}()

	p.log.Info("Deleting node claim for scale-down",
		"name", nodeClaim.Name,
		"providerID", nodeClaim.Status.ProviderID,
		"finalizers", nodeClaim.Finalizers,
		"deletionTimestamp", nodeClaim.DeletionTimestamp)

	// Delete cloud resources first
	if err := p.deleteCloudResources(ctx, nodeClaim); err != nil {
		return &errors.NodeDeletionError{
			NodeName:   nodeClaim.Name,
			Underlying: err,
			Retryable:  errors.IsRetryableError(err),
		}
	}

	// Handle stuck finalizers
	p.handleStuckFinalizers(ctx, nodeClaim)

	p.log.NodeDeletion(nodeClaim.Name, nodeClaim.Status.ProviderID, time.Since(start).String())

	return nil
}

// List returns all instances managed by this provider
func (p *cloudProviderImpl) List(ctx context.Context) ([]*v1.NodeClaim, error) {
	p.log.V(1).Info("Listing all node claims")

	// Get all nodes
	nodes := &corev1.NodeList{}
	if err := p.kubeClient.List(ctx, nodes); err != nil {
		p.log.ErrorWithDetails(err, "Failed to list nodes")
		return nil, fmt.Errorf("failed to list nodes: %w", err)
	}

	// Convert nodes to NodeClaims
	var nodeClaims []*v1.NodeClaim
	for _, node := range nodes.Items {
		nodeClaim := p.convertNodeToNodeClaim(&node)
		nodeClaims = append(nodeClaims, nodeClaim)
	}

	p.log.Info("Successfully listed node claims", "count", len(nodeClaims))

	return nodeClaims, nil
}

// GetSupportedNodeClasses returns the supported node classes for the provider
func (p *cloudProviderImpl) GetSupportedNodeClasses() []status.Object {
	return []status.Object{
		&v1bizfly.BizflyCloudNodeClass{},
	}
}

// IsDrifted checks if the given node claim has drifted from its desired state
func (p *cloudProviderImpl) IsDrifted(ctx context.Context, nodeClaim *v1.NodeClaim) (cloudprovider.DriftReason, error) {
	start := time.Now()
	defer func() {
		metrics.RecordDriftCheck(time.Since(start))
	}()

	p.log.V(1).Info("Checking if node claim is drifted",
		"name", nodeClaim.Name,
		"providerID", nodeClaim.Status.ProviderID)

	// Check if the node claim has a provider ID
	if nodeClaim.Status.ProviderID == "" {
		metrics.RecordDriftDetected(nodeClaim.Name, DriftReasonNodeNotFound)
		return cloudprovider.DriftReason(DriftReasonNodeNotFound), nil
	}

	// Extract instance ID and check if instance exists
	instanceID, err := utils.ExtractInstanceIDFromProviderID(nodeClaim.Status.ProviderID, ProviderIDPrefix)
	if err != nil {
		return "", err
	}

	instanceProvider := p.createInstanceProvider()
	inst, err := instanceProvider.GetInstance(ctx, instanceID)
	if err != nil {
		if errors.IsNotFoundError(err) {
			p.log.Info("Instance not found, node is drifted",
				"name", nodeClaim.Name,
				"instanceID", instanceID)
			metrics.RecordDriftDetected(nodeClaim.Name, DriftReasonNodeNotFound)
			return cloudprovider.DriftReason(DriftReasonNodeNotFound), nil
		}
		return "", err
	}

	// Check instance state for drift
	if p.isInstanceInTerminalState(inst.Status) {
		p.log.Info("Instance in terminal state, node is drifted",
			"name", nodeClaim.Name,
			"instanceID", instanceID,
			"status", inst.Status)
		metrics.RecordDriftDetected(nodeClaim.Name, DriftReasonInstanceNotFound)
		return cloudprovider.DriftReason(DriftReasonInstanceNotFound), nil
	}

	// Check for configuration drift
	if p.hasConfigurationDrift(ctx, inst, nodeClaim) {
		p.log.Info("NodeClass configuration drift detected",
			"name", nodeClaim.Name,
			"instanceID", instanceID)
		metrics.RecordDriftDetected(nodeClaim.Name, DriftReasonNodeClassDrifted)
		return cloudprovider.DriftReason(DriftReasonNodeClassDrifted), nil
	}

	p.log.V(1).Info("NodeClaim is not drifted", "name", nodeClaim.Name)
	return "", nil
}

// Name returns the name of the provider
func (p *cloudProviderImpl) Name() string {
	return ProviderName
}

// RepairPolicies returns the repair policies for the provider
func (p *cloudProviderImpl) RepairPolicies() []cloudprovider.RepairPolicy {
	return nil
}

// Get retrieves a NodeClaim by instance ID
func (p *cloudProviderImpl) Get(ctx context.Context, id string) (*v1.NodeClaim, error) {
	instanceProvider := p.createInstanceProvider()
	inst, err := instanceProvider.GetInstance(ctx, id)
	if err != nil {
		return nil, err
	}

	// Convert to NodeClaim
	nodeClaim := &v1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: inst.Name,
		},
		Status: v1.NodeClaimStatus{
			ProviderID: utils.BuildProviderID(inst.ID, ProviderIDPrefix),
		},
	}

	return nodeClaim, nil
}

// Helper methods

func (p *cloudProviderImpl) throttleNodeCreation() {
	nodeCreationMutex.Lock()
	defer nodeCreationMutex.Unlock()

	timeSinceLastCreation := time.Since(lastNodeCreation)
	if timeSinceLastCreation < minCreationInterval {
		sleepTime := minCreationInterval - timeSinceLastCreation
		p.log.Info("Throttling node creation",
			"sleepTime", sleepTime,
			"timeSinceLastCreation", timeSinceLastCreation)
		time.Sleep(sleepTime)
	}
	lastNodeCreation = time.Now()
}

func (p *cloudProviderImpl) extractInstanceTypeFromRequirements(requirements []v1.NodeSelectorRequirementWithMinValues) string {
	for _, req := range requirements {
		if req.Key == corev1.LabelInstanceTypeStable && len(req.Values) > 0 {
			return req.Values[0]
		}
	}
	return ""
}

func (p *cloudProviderImpl) setRequiredLabels(nodeClaim *v1.NodeClaim, instanceTypeName string) {
	if nodeClaim.Labels == nil {
		nodeClaim.Labels = make(map[string]string)
	}

	nodeClaim.Labels[corev1.LabelInstanceTypeStable] = instanceTypeName
	nodeClaim.Labels[corev1.LabelArchStable] = "amd64"
	nodeClaim.Labels["karpenter.sh/capacity-type"] = "on-demand"
	nodeClaim.Labels[NodeLabelRegion] = p.region
}

func (p *cloudProviderImpl) convertNodeClaimToNode(nodeClaim *v1.NodeClaim) *corev1.Node {
	return &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name:        nodeClaim.Name,
			Labels:      nodeClaim.Labels,
			Annotations: nodeClaim.Annotations,
		},
		Spec: corev1.NodeSpec{
			Taints: nodeClaim.Spec.Taints,
		},
	}
}

func (p *cloudProviderImpl) updateNodeClaimStatus(nodeClaim *v1.NodeClaim, createdNode *corev1.Node) {
	nodeClaim.Status.ProviderID = createdNode.Spec.ProviderID
	nodeClaim.Status.Capacity = createdNode.Status.Capacity
	nodeClaim.Status.Allocatable = createdNode.Status.Allocatable
}

func (p *cloudProviderImpl) convertNodeToNodeClaim(node *corev1.Node) *v1.NodeClaim {
	return &v1.NodeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name: node.Name,
		},
		Status: v1.NodeClaimStatus{
			ProviderID:  node.Spec.ProviderID,
			Capacity:    node.Status.Capacity,
			Allocatable: node.Status.Allocatable,
		},
	}
}

func (p *cloudProviderImpl) deleteCloudResources(ctx context.Context, nodeClaim *v1.NodeClaim) error {
	providerID := nodeClaim.Status.ProviderID
	if providerID == "" {
		return nil
	}

	instanceID, err := utils.ExtractInstanceIDFromProviderID(providerID, ProviderIDPrefix)
	if err != nil {
		return err
	}

	instanceProvider := p.createInstanceProvider()
	return instanceProvider.DeleteInstance(ctx, instanceID)
}

func (p *cloudProviderImpl) handleStuckFinalizers(ctx context.Context, nodeClaim *v1.NodeClaim) {
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
				p.log.ErrorWithDetails(err, "Failed to remove stuck finalizers",
					"name", nodeClaim.Name)
			} else {
				p.log.Info("Removed stuck finalizers from NodeClaim",
					"name", nodeClaim.Name,
					"remainingFinalizers", len(newFinalizers))
			}
		}
	}
}

func (p *cloudProviderImpl) createInstanceProvider() *instance.Provider {
	return instance.NewProvider(
		p.kubeClient,
		p.log.Logger,
		p.bizflyClient,
		p.region,
		p.config,
	)
}

func (p *cloudProviderImpl) isInstanceInTerminalState(status string) bool {
	terminalStates := []string{"ERROR", "DELETED", "SHUTOFF", "SUSPENDED"}
	for _, state := range terminalStates {
		if status == state {
			return true
		}
	}
	return false
}

func (p *cloudProviderImpl) hasConfigurationDrift(ctx context.Context, inst *conversion.Instance, nodeClaim *v1.NodeClaim) bool {
	// Implementation for configuration drift detection
	// This would check if the instance configuration matches the NodeClass requirements
	return false
}

func (p *cloudProviderImpl) resolveNodeClassFromNodePool(ctx context.Context, nodePool *v1.NodePool) (*v1bizfly.BizflyCloudNodeClass, error) {
	p.log.V(1).Info("Resolving NodeClass from NodePool",
		"nodePool", nodePool.Name,
		"nodeClassRef", nodePool.Spec.Template.Spec.NodeClassRef.Name)

	nodeClass := &v1bizfly.BizflyCloudNodeClass{}
	if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: nodePool.Spec.Template.Spec.NodeClassRef.Name}, nodeClass); err != nil {
		return nil, err
	}

	if !nodeClass.DeletionTimestamp.IsZero() {
		return nil, p.newTerminatingNodeClassError(nodeClass.Name)
	}

	return nodeClass, nil
}

// CreateNode creates a new BizFly Cloud server instance
func (p *cloudProviderImpl) CreateNode(ctx context.Context, nodeSpec *corev1.Node) (*corev1.Node, error) {
	start := time.Now()
	defer func() {
		metrics.RecordInstanceCreation("", "", "", time.Since(start))
	}()

	p.log.Info("Creating node",
		"name", nodeSpec.Name,
		"labels", nodeSpec.Labels,
		"annotations", nodeSpec.Annotations)

	// Extract instance type from node labels
	_, exists := nodeSpec.Labels[NodeLabelInstanceType]
	if !exists {
		return nil, fmt.Errorf("node %s does not have an instance-type label", nodeSpec.Name)
	}

	// Resolve nodeClass from nodeSpec
	nodeClass, err := p.resolveNodeClassFromNodeSpec(ctx, nodeSpec)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve node class: %w", err)
	}

	// Create nodeClaim from nodeSpec
	nodeClaim := p.convertNodeSpecToNodeClaim(nodeSpec)

	// Create the instance
	instanceProvider := p.createInstanceProvider()
	gobizflyServer, err := instanceProvider.CreateInstance(ctx, nodeClaim, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("failed to create instance: %w", err)
	}

	// Convert gobizfly.Server to instance.Instance
	instanceObj := conversion.ConvertGobizflyServerToInstance(gobizflyServer, p.region)
	if instanceObj == nil {
		return nil, fmt.Errorf("failed to convert server to instance")
	}

	// Convert the instance to a node
	node := conversion.ConvertToNode(instanceObj, false)
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

	duration := time.Since(start)
	p.log.Info("Successfully created node",
		"name", node.Name,
		"providerID", node.Spec.ProviderID,
		"zone", instanceObj.Zone,
		"flavor", instanceObj.Flavor,
		"duration", duration,
		"labels", node.Labels,
		"annotations", node.Annotations,
		"taints", node.Spec.Taints)

	return node, nil
}

// DeleteNode deletes a BizFly Cloud server instance
func (p *cloudProviderImpl) DeleteNode(ctx context.Context, node *corev1.Node) error {
	start := time.Now()
	defer func() {
		metrics.RecordNodeTermination(time.Since(start))
	}()

	p.log.Info("Deleting node", "name", node.Name)

	// Extract the instance ID from the provider ID
	providerID := node.Spec.ProviderID
	if providerID == "" {
		return fmt.Errorf("node %s does not have a provider ID", node.Name)
	}

	// Parse the provider ID to get the instance ID
	instanceID, err := utils.ExtractInstanceIDFromProviderID(providerID, ProviderIDPrefix)
	if err != nil {
		return err
	}

	// Delete the instance
	instanceProvider := p.createInstanceProvider()
	err = instanceProvider.DeleteInstance(ctx, instanceID)
	if err != nil {
		// Check if the error is because the instance doesn't exist
		if errors.IsNotFoundError(err) {
			p.log.Info("Instance already deleted", "name", node.Name, "instanceID", instanceID)
			return nil
		}

		return fmt.Errorf("failed to delete instance %s: %w", instanceID, err)
	}

	p.log.Info("Successfully deleted node",
		"name", node.Name,
		"instanceID", instanceID,
		"duration", time.Since(start))

	return nil
}

// DetectNodeDrift checks if the given node has drifted from its desired state
func (p *cloudProviderImpl) DetectNodeDrift(ctx context.Context, node *corev1.Node) (bool, error) {
	start := time.Now()
	defer func() {
		metrics.RecordDriftCheck(time.Since(start))
	}()

	p.log.V(1).Info("Checking for node drift", "name", node.Name)

	// Skip if provider ID is not set
	if node.Spec.ProviderID == "" {
		return false, nil
	}

	// Extract instance ID
	instanceID, err := utils.ExtractInstanceIDFromProviderID(node.Spec.ProviderID, ProviderIDPrefix)
	if err != nil {
		return false, err
	}

	// Get the instance
	instanceProvider := p.createInstanceProvider()
	instance, err := instanceProvider.GetInstance(ctx, instanceID)
	if err != nil {
		// If the instance is not found, it's definitely drifted
		if errors.IsNotFoundError(err) {
			p.log.Info("Instance not found, node has drifted", "name", node.Name, "instanceID", instanceID)
			return true, nil
		}

		return false, fmt.Errorf("failed to get instance %s: %w", instanceID, err)
	}

	// Check if instance is in a terminal state
	if p.isInstanceInTerminalState(instance.Status) {
		p.log.Info("Instance in terminal state, node has drifted",
			"name", node.Name,
			"instanceID", instanceID,
			"status", instance.Status)
		return true, nil
	}

	// Instance exists and is running, no drift detected
	return false, nil
}

// ReconcileNodeDrift attempts to fix a drifted node
func (p *cloudProviderImpl) ReconcileNodeDrift(ctx context.Context, node *corev1.Node) error {
	p.log.Info("Reconciling node drift", "name", node.Name)

	// For now, we'll just recreate the node
	// In a more advanced implementation, we could try to repair the instance

	// We'll need to delete and recreate the node
	// First, get a copy of the node to recreate it later
	nodeCopy := node.DeepCopy()

	// Delete the node from the API server
	if err := p.kubeClient.Delete(ctx, node); err != nil && !k8serrors.IsNotFound(err) {
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

// GetAvailableZones returns the available zones in the configured region
func (p *cloudProviderImpl) GetAvailableZones(ctx context.Context) ([]string, error) {
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
func (p *cloudProviderImpl) GetSpotInstances(ctx context.Context) (bool, error) {
	// In production, this would check if spot instances are available
	// For now, we'll assume they are
	return true, nil
}

func (p *cloudProviderImpl) resolveNodeClassFromNodeSpec(ctx context.Context, nodeSpec *corev1.Node) (*v1bizfly.BizflyCloudNodeClass, error) {
	// For now, use the default NodeClass
	nodeClass := &v1bizfly.BizflyCloudNodeClass{}
	if err := p.kubeClient.Get(ctx, types.NamespacedName{Name: "default"}, nodeClass); err != nil {
		return nil, err
	}
	return nodeClass, nil
}

func (p *cloudProviderImpl) convertNodeSpecToNodeClaim(nodeSpec *corev1.Node) *v1.NodeClaim {
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

func (p *cloudProviderImpl) newTerminatingNodeClassError(name string) *k8serrors.StatusError {
	qualifiedResource := schema.GroupResource{Group: apis.Group, Resource: "bizflycloudnodeclasses"}
	err := k8serrors.NewNotFound(qualifiedResource, name)
	err.ErrStatus.Message = fmt.Sprintf("%s %q is terminating, treating as not found", qualifiedResource.String(), name)
	return err
}
