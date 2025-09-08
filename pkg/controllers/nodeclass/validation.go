package nodeclass

import (
    "context"
    "fmt"

    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
)

// ValidationReconciler validates nodeclass configuration
type ValidationReconciler struct {
    log logr.Logger
}

func (r *ValidationReconciler) Reconcile(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) (reconcile.Result, error) {
    r.log.V(1).Info("Validating NodeClass", "name", nodeClass.Name)

    // Validate required fields
    if nodeClass.Spec.Template == "" {
        err := fmt.Errorf("template is required")
        r.log.Error(err, "NodeClass validation failed", "name", nodeClass.Name)
        nodeClass.StatusConditions().SetFalse(v1.ConditionTypeInstanceReady, "ValidationFailed", err.Error())
        return reconcile.Result{}, err
    }

    // Validate node category
    if nodeClass.Spec.NodeCategory != "" {
        validCategories := []string{"basic", "premium", "enterprise", "dedicated"}
        valid := false
        for _, category := range validCategories {
            if nodeClass.Spec.NodeCategory == category {
                valid = true
                break
            }
        }
        if !valid {
            err := fmt.Errorf("invalid node category: %s, must be one of: %v", nodeClass.Spec.NodeCategory, validCategories)
            r.log.Error(err, "NodeClass validation failed", "name", nodeClass.Name)
            nodeClass.StatusConditions().SetFalse(v1.ConditionTypeInstanceReady, "ValidationFailed", err.Error())
            return reconcile.Result{}, err
        }
    }

    // Validate disk size
    if nodeClass.Spec.RootDiskSize < 20 || nodeClass.Spec.RootDiskSize > 1000 {
        err := fmt.Errorf("root disk size must be between 20 and 1000 GB, got: %d", nodeClass.Spec.RootDiskSize)
        r.log.Error(err, "NodeClass validation failed", "name", nodeClass.Name)
        nodeClass.StatusConditions().SetFalse(v1.ConditionTypeInstanceReady, "ValidationFailed", err.Error())
        return reconcile.Result{}, err
    }

    // Validate disk type
    if nodeClass.Spec.DiskType != "" {
        validDiskTypes := []string{"SSD", "HDD"}
        valid := false
        for _, diskType := range validDiskTypes {
            if nodeClass.Spec.DiskType == diskType {
                valid = true
                break
            }
        }
        if !valid {
            err := fmt.Errorf("invalid disk type: %s, must be one of: %v", nodeClass.Spec.DiskType, validDiskTypes)
            r.log.Error(err, "NodeClass validation failed", "name", nodeClass.Name)
            nodeClass.StatusConditions().SetFalse(v1.ConditionTypeInstanceReady, "ValidationFailed", err.Error())
            return reconcile.Result{}, err
        }
    }

    // Validate VPC network IDs
    if len(nodeClass.Spec.VPCNetworkIDs) > 5 {
        err := fmt.Errorf("too many VPC network IDs: %d, maximum is 5", len(nodeClass.Spec.VPCNetworkIDs))
        r.log.Error(err, "NodeClass validation failed", "name", nodeClass.Name)
        nodeClass.StatusConditions().SetFalse(v1.ConditionTypeInstanceReady, "ValidationFailed", err.Error())
        return reconcile.Result{}, err
    }

    // Validate security groups
    if len(nodeClass.Spec.SecurityGroups) > 10 {
        err := fmt.Errorf("too many security groups: %d, maximum is 10", len(nodeClass.Spec.SecurityGroups))
        r.log.Error(err, "NodeClass validation failed", "name", nodeClass.Name)
        nodeClass.StatusConditions().SetFalse(v1.ConditionTypeInstanceReady, "ValidationFailed", err.Error())
        return reconcile.Result{}, err
    }

    r.log.V(1).Info("NodeClass validation passed", "name", nodeClass.Name)
    return reconcile.Result{}, nil
}
