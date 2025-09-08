package bizflycloud

import (
    "context"
    "strings"
    "time"

    "k8s.io/apimachinery/pkg/types"
    v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"

    v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/providers/instance"
)

// IsDrifted checks if a NodeClaim has drifted from its desired state
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
