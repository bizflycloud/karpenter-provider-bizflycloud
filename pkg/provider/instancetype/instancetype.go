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
		fmt.Printf("[ERROR] Failed to list flavors: %v\n", err)
		return nil, fmt.Errorf("failed to list flavors: %w", err)
	}

	var instanceTypes []*cloudprovider.InstanceType
	for _, flavor := range flavors {
		// Create requirements for CPU and memory
		requirements := scheduling.NewRequirements(
			scheduling.NewRequirement("cpu", corev1.NodeSelectorOpIn, fmt.Sprintf("%d", flavor.VCPUs)),
			scheduling.NewRequirement("memory", corev1.NodeSelectorOpIn, fmt.Sprintf("%dMi", flavor.RAM)),
		)

		// Create capacity for CPU and memory
		capacity := corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", flavor.VCPUs)),
			corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", flavor.RAM)),
		}

		// Create offerings
		offerings := cloudprovider.Offerings{}
		firstOffering := cloudprovider.Offering{
			Requirements:        requirements,
			Price:               0.085,
			Available:           true,
			ReservationCapacity: 1,
		}
		offerings = append(offerings, &firstOffering)

		// Create overhead for CPU and memory
		overhead := cloudprovider.InstanceTypeOverhead{
			KubeReserved: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("200m"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
			SystemReserved: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
			EvictionThreshold: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
			},
		}

		// Create the instance type with required fields
		instanceType := &cloudprovider.InstanceType{
			Name:         flavor.Name,
			Requirements: requirements,
			Capacity:     capacity,
			Offerings:    offerings,
			Overhead:     &overhead,
		}

		instanceTypes = append(instanceTypes, instanceType)
	}

	return instanceTypes, nil
}
