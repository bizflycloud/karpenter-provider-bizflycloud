package main

import (
	"os"

	"github.com/bizflycloud/gobizfly"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/bizflycloud"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	"sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
)

func main() {
	ctx, op := coreoperator.NewOperator()
	log := op.GetLogger()
	log.Info("Karpenter BizFly Cloud Provider version", "version", coreoperator.Version)
	config := v1.ProviderConfig{}

	bizflyClient, err := gobizfly.NewClient()
	if err != nil {
		log.Error(err, "failed creating BizFly Cloud client")
		os.Exit(1)
	}

	instanceTypeProvider := instancetype.NewDefaultProvider(*bizflyClient)

	cloudcapacityProvider, err := bizflycloud.New(
		instanceTypeProvider,
		op.GetClient(),
		bizflyClient,
		log,
		&config,
	)
	if err != nil {
		log.Error(err, "failed creating instance provider")

		os.Exit(1)
	}

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
