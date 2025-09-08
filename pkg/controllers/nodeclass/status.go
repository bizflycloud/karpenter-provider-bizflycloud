package nodeclass

import (
    "context"
    "time"

    "github.com/go-logr/logr"
    "sigs.k8s.io/controller-runtime/pkg/reconcile"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
)

type StatusReconciler struct {
    log logr.Logger
}

func (r *StatusReconciler) Reconcile(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) (reconcile.Result, error) {
    r.log.V(1).Info("Reconciling NodeClass status", "name", nodeClass.Name)

    // Fix: Use metav1.Now() directly without taking its address
    now := metav1.Now()
    nodeClass.Status.LastUpdated = &now

    // Set the Ready condition to true
    nodeClass.StatusConditions().SetTrue(v1.ConditionTypeInstanceReady)

    // Update instance types status
    nodeClass.Status.InstanceTypes = []string{
        "nix.2c_4g", "nix.4c_8g", "nix.8c_16g", "nix.12c_24g",
        "2c_4g_basic", "4c_8g_basic", "8c_16g_basic",
        "4c_8g_enterprise", "8c_16g_enterprise",
        "4c_8g_dedicated", "8c_16g_dedicated",
    }

    nodeClass.Status.Regions = []string{"HN", "HCM"}

    return reconcile.Result{RequeueAfter: 5 * time.Minute}, nil
}
