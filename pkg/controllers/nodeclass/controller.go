package nodeclass

import (
	"context"
	"time"

	"github.com/go-logr/logr"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	karpv1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
)

type BizflyCloudNodeClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

func (r *BizflyCloudNodeClassReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("nodeclass", req.NamespacedName)

	var nc karpv1.BizflyCloudNodeClass
	if err := r.Get(ctx, req.NamespacedName, &nc); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	ready := nc.Spec.Region != "" && nc.Spec.Template != "" && nc.Spec.Zone != ""

	condition := metav1.Condition{
		Type:               "Ready",
		Status:             metav1.ConditionFalse,
		Reason:             "MissingFields",
		Message:            "Required fields are missing",
		LastTransitionTime: metav1.Now(),
	}

	if ready {
		condition.Status = metav1.ConditionTrue
		condition.Reason = "ConfigurationValid"
		condition.Message = "NodeClass configuration is valid"
	}

	nc.Status.Conditions = []metav1.Condition{condition}
	if err := r.Status().Update(ctx, &nc); err != nil {
		log.Error(err, "unable to update NodeClass status")
		return ctrl.Result{}, err
	}

	log.Info("updated NodeClass status", "ready", condition.Status)
	return ctrl.Result{RequeueAfter: 60 * time.Second}, nil
}

func (r *BizflyCloudNodeClassReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&karpv1.BizflyCloudNodeClass{}).
		Complete(r)
}
