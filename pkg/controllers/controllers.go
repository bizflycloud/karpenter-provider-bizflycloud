package controllers

import (
	"context"

	"github.com/awslabs/operatorpkg/controller"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/controllers/nodeclass"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

func NewControllers(
	ctx context.Context,
	kubeClient client.Client,
	region string,
) []controller.Controller {
	controllers := []controller.Controller{
		nodeclass.NewController(kubeClient, region),
	}

	return controllers
}
