package instancetype

import (
	"context"
	"fmt"

	"github.com/bizflycloud/gobizfly"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)

type Provider interface {
	List(context.Context, *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
}

func NewDefaultProvider(client *gobizfly.Client) Provider {
	return &defaultProvider{client: client}
}

type defaultProvider struct {
	client *gobizfly.Client
}

// List implements Provider.
func (d *defaultProvider) List(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {
	flavors, err := d.client.CloudServer.Flavors().List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list flavors: %w", err)
	}

	var instanceTypes []*cloudprovider.InstanceType
	for _, flavor := range flavors {
		instanceType := &cloudprovider.InstanceType{
			Name: flavor.Name,
			Requirements: scheduling.NewRequirements(
				scheduling.NewRequirement("cpu", corev1.NodeSelectorOpIn, fmt.Sprintf("%d", flavor.VCPUs)),
				scheduling.NewRequirement("memory", corev1.NodeSelectorOpIn, fmt.Sprintf("%dGi", flavor.RAM)),
			),
			Capacity: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", flavor.VCPUs)),
				corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dGi", flavor.RAM)),
			},
		}
		instanceTypes = append(instanceTypes, instanceType)
	}

	return instanceTypes, nil
}
