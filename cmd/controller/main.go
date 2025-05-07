package main

import (
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/bizflycloud"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/operator"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	"sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
)

func main() {
	ctx, op := operator.NewOperator(coreoperator.NewOperator())

	cloudcapacityProvider := bizflycloud.New(
		op.InstanceProvider,
		op.GetClient(),
		op.Client,
		*op.Log,
		op.Config,
	)

	// Use the provider directly as the cloud provider
	cloudProvider := metrics.Decorate(cloudcapacityProvider)
	clusterState := state.NewCluster(op.Clock, op.GetClient(), cloudProvider)

	op.
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			cloudProvider,
			clusterState,
		)...).
		Start(ctx)
}
