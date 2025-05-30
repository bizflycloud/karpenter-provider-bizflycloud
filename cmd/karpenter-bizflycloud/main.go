package main

import (
	"context"
	"os"

	"sigs.k8s.io/karpenter/pkg/cloudprovider/metrics"
	corescontrollers "sigs.k8s.io/karpenter/pkg/controllers"
	"sigs.k8s.io/karpenter/pkg/controllers/state"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"

	"github.com/bizflycloud/karpenter-provider-bizflycloud/internal/logging"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/client"
	bizflycloud "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/cloudprovider/bizflycloud"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/controllers"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/operator"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
)

func main() {
	// Initialize logger
	log := logging.NewLogger("karpenter-bizflycloud")

	ctx := context.Background()

	// Create core operator
	opCtx, coreOp := coreoperator.NewOperator()

	// Create our operator with enhanced configuration
	ctx, op := operator.NewOperator(opCtx, coreOp)
	if op == nil {
		log.Error(nil, "Failed to create operator")
		os.Exit(1)
	}

	// Create BizflyCloud client
	bizflyClient, err2 := client.NewBizflyClientFromEnv(log.Logger)
	if err2 != nil {
		log.ErrorWithDetails(err2, "Failed to create BizflyCloud client")
		os.Exit(1)
	}

	// Create instance type provider
	instanceTypeProvider := instancetype.NewDefaultProvider(bizflyClient, log.Logger)

	// Create cloud provider
	cloudProvider := bizflycloud.New(
		instanceTypeProvider,
		op.GetClient(),
		bizflyClient,
		log,
		op.Config,
	)

	// Decorate with metrics
	decoratedCloudProvider := metrics.Decorate(cloudProvider)

	// Create cluster state
	clusterState := state.NewCluster(op.Clock, op.GetClient(), decoratedCloudProvider)

	log.Info("Starting Karpenter BizflyCloud Provider",
		"version", getVersion(),
		"region", op.Region)

	// Start operator with controllers
	op.
		WithControllers(ctx, corescontrollers.NewControllers(
			ctx,
			op.Manager,
			op.Clock,
			op.GetClient(),
			op.EventRecorder,
			decoratedCloudProvider,
			clusterState,
		)...).
		WithControllers(ctx, controllers.NewControllers(
			ctx,
			op.GetClient(),
			op.Region,
		)...).
		Start(ctx)
}

func getVersion() string {
	version := os.Getenv("VERSION")
	if version == "" {
		return "development"
	}
	return version
}
