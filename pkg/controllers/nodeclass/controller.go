package nodeclass

import (
	"context"

	"github.com/awslabs/operatorpkg/reasonable"
	"github.com/go-logr/logr"
	"go.uber.org/multierr"
	"k8s.io/apimachinery/pkg/api/equality"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	controllerruntime "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/builder"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/manager"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"
	"sigs.k8s.io/karpenter/pkg/utils/result"

	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	karpv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

type BizflyCloudNodeClassReconciler struct {
	client.Client
	Scheme *runtime.Scheme
	Log    logr.Logger
}

type Controller struct {
	kubeClient client.Client
	// recorder                events.Recorder
	region string
	// launchTemplateProvider  launchtemplate.Provider
	// instanceProfileProvider instanceprofile.Provider
	// validation              *Validation
	reconcilers []reconcile.TypedReconciler[*v1.BizflyCloudNodeClass]
}

func NewController(kubeClient client.Client, region string) *Controller {
	return &Controller{
		kubeClient: kubeClient,
		region:     region,
		reconcilers: []reconcile.TypedReconciler[*v1.BizflyCloudNodeClass]{
			&StatusReconciler{},
		},
	}
}

func (c *Controller) Name() string {
	return "nodeclass"
}

func (c *Controller) Reconcile(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) (reconcile.Result, error) {
	var results []reconcile.Result
	var errs error
	stored := nodeClass.DeepCopy()

	for _, reconciler := range c.reconcilers {
		res, err := reconciler.Reconcile(ctx, nodeClass)
		errs = multierr.Append(errs, err)
		results = append(results, res)
	}

	if !equality.Semantic.DeepEqual(stored, nodeClass) {
		// We use client.MergeFromWithOptimisticLock because patching a list with a JSON merge patch
		// can cause races due to the fact that it fully replaces the list on a change
		// Here, we are updating the status condition list
		if err := c.kubeClient.Status().Patch(ctx, nodeClass, client.MergeFromWithOptions(stored, client.MergeFromWithOptimisticLock{})); err != nil {
			if errors.IsConflict(err) {
				return reconcile.Result{Requeue: true}, nil
			}
			errs = multierr.Append(errs, client.IgnoreNotFound(err))
		}
	}
	if errs != nil {
		return reconcile.Result{}, errs
	}

	return result.Min(results...), nil
}

// StatusReconciler ensures the nodeclass status is always true
type StatusReconciler struct{}

func (r *StatusReconciler) Reconcile(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) (reconcile.Result, error) {
	// Set the Ready condition to true
	nodeClass.StatusConditions().SetTrue(v1.ConditionTypeInstanceReady)
	return reconcile.Result{}, nil
}

func (c *Controller) Register(_ context.Context, m manager.Manager) error {
	return controllerruntime.NewControllerManagedBy(m).
		Named(c.Name()).
		For(&v1.BizflyCloudNodeClass{}).
		Watches(
			&karpv1.NodeClaim{},
			handler.EnqueueRequestsFromMapFunc(func(_ context.Context, o client.Object) []reconcile.Request {
				nc := o.(*karpv1.NodeClaim)
				if nc.Spec.NodeClassRef == nil {
					return nil
				}
				return []reconcile.Request{{NamespacedName: types.NamespacedName{Name: nc.Spec.NodeClassRef.Name}}}
			}),
			// Watch for NodeClaim deletion events
			builder.WithPredicates(predicate.Funcs{
				CreateFunc: func(e event.CreateEvent) bool { return false },
				UpdateFunc: func(e event.UpdateEvent) bool { return false },
				DeleteFunc: func(e event.DeleteEvent) bool { return true },
			}),
		).
		WithOptions(controller.Options{
			RateLimiter:             reasonable.RateLimiter(),
			MaxConcurrentReconciles: 10,
		}).
		Complete(reconcile.AsReconciler(m.GetClient(), c))
}
