package instancetype

import (
	"context"

	"github.com/bizflycloud/gobizfly"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

type Provider interface {
	List(context.Context, *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
}

func NewDefaultProvider(client gobizfly.Client) Provider {
	return &defaultProvider{client: client}
}

type defaultProvider struct {
	client gobizfly.Client
}

// List implements Provider.
func (d *defaultProvider) List(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {
	panic("unimplemented")
}
