package bizflycloud

import (
    "context"
    "fmt"
    "strings"
    "time"

    corev1 "k8s.io/api/core/v1"
    "k8s.io/apimachinery/pkg/api/errors"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/runtime/schema"
    "k8s.io/apimachinery/pkg/types"
    v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"

    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis"
    v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/providers/instance"
)

// Create implements cloudprovider.CloudProvider
func (p *CloudProvider) Create(ctx context.Context, nodeClaim *v1.NodeClaim) (*v1.NodeClaim, error) {
    // Throttle node creation
    nodeCreationMutex.Lock()
    timeSinceLastCreation := time.Since(lastNodeCreation)
    if timeSinceLastCreation < minCreationInterval {
        sleepTime := minCreationInterval - timeSinceLastCreation
        p.log.Info("Throttling node creation",
            "name", nodeClaim.Name,
            "sleepTime", sleepTime)
        nodeCreationMutex.Unlock()
        time.Sleep(sleepTime)
        nodeCreationMutex.Lock()
    }
    lastNodeCreation = time.Now()
    nodeCreationMutex.Unlock()

    p.log.V(1).Info("Creating node claim", "name", nodeClaim.Name)

    // Extract instance type from NodeClaim requirements
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

    // Set required labels
    if nodeClaim.Labels == nil {
        nodeClaim.Labels = make(map[string]string)
    }
    
    nodeClaim.Labels[corev1.LabelInstanceTypeStable] = instanceTypeName
    nodeClaim.Labels[corev1.LabelArchStable] = "amd64"
    nodeClaim.Labels["karpenter.sh/capacity-type"] = "on-demand"
    nodeClaim.Labels[NodeLabelRegion] = p.region

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
        p.log.Error(err, "Failed to create node", "name", nodeClaim.Name)
        return nil, err
    }

    // Update NodeClaim status
    nodeClaim.Status.ProviderID = createdNode.Spec.ProviderID
    nodeClaim.Status.Capacity = createdNode.Status.Capacity
    nodeClaim.Status.Allocatable = createdNode.Status.Allocatable

    p.log.Info("Successfully created node claim",
        "name", nodeClaim.Name,
        "providerID", nodeClaim.Status.ProviderID)

    return nodeClaim, nil
}

// Delete deletes a NodeClaim from BizFly Cloud
func (p *CloudProvider) Delete(ctx context.Context, nodeClaim *v1.NodeClaim) error {
    start := time.Now()
    defer func() {
        nodeTerminationDuration.Observe(time.Since(start).Seconds())
    }()

    p.log.Info("Deleting node claim",
        "name", nodeClaim.Name,
        "providerID", nodeClaim.Status.ProviderID)

    // Delete cloud resources
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
                p.log.Info("Instance already deleted", 
                    "name", nodeClaim.Name, 
                    "instanceID", instanceID)
            } else {
                apiErrors.WithLabelValues("delete_instance").Inc()
                return fmt.Errorf("failed to delete instance %s: %w", instanceID, err)
            }
        }
    }

    // Handle stuck finalizers
    if nodeClaim.DeletionTimestamp != nil && 
       time.Since(nodeClaim.DeletionTimestamp.Time) > 5*time.Minute {
        
        p.log.Info("Removing stuck finalizers",
            "name", nodeClaim.Name,
            "terminatingFor", time.Since(nodeClaim.DeletionTimestamp.Time))
        
        var newFinalizers []string
        for _, finalizer := range nodeClaim.Finalizers {
            if !strings.Contains(finalizer, "karpenter.sh") {
                newFinalizers = append(newFinalizers, finalizer)
            }
        }
        
        if len(newFinalizers) != len(nodeClaim.Finalizers) {
            nodeClaim.Finalizers = newFinalizers
            if err := p.kubeClient.Update(ctx, nodeClaim); err != nil {
                p.log.Error(err, "Failed to remove stuck finalizers", "name", nodeClaim.Name)
            }
        }
    }

    p.log.Info("Successfully deleted node claim",
        "name", nodeClaim.Name,
        "duration", time.Since(start))

    return nil
}

// Get retrieves a NodeClaim by ID
func (p *CloudProvider) Get(ctx context.Context, id string) (*v1.NodeClaim, error) {
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

    nodeClaim := &v1.NodeClaim{
        ObjectMeta: metav1.ObjectMeta{
            Name: instance.Name,
        },
        Status: v1.NodeClaimStatus{
            ProviderID: instance.ID,
        },
    }

    return nodeClaim, nil
}

// List returns all instances managed by this provider
func (p *CloudProvider) List(ctx context.Context) ([]*v1.NodeClaim, error) {
    p.log.V(1).Info("Listing all node claims")

    nodes := &corev1.NodeList{}
    if err := p.kubeClient.List(ctx, nodes); err != nil {
        return nil, fmt.Errorf("failed to list nodes: %w", err)
    }

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
        return nil, err
    }

    if !nodeClass.DeletionTimestamp.IsZero() {
        return nil, newTerminatingNodeClassError(nodeClass.Name)
    }

    return nodeClass, nil
}
