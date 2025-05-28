package instancetype

import (
	"context"
	"fmt"

	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
)


type Provider interface {
	List(context.Context, *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
}

func NewDefaultProvider(client *gobizfly.Client, log logr.Logger) Provider {
	return &defaultProvider{
		client: client,
		log:    log,
	}
}

type defaultProvider struct {
	client *gobizfly.Client
	log    logr.Logger
}

// List implements Provider.
func (d *defaultProvider) List(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {

	var maxPods int64
	maxPods = 110

	d.log.V(1).Info("Listing instance types", "nodeClass", nodeClass.Name)
	flavors, err := d.client.CloudServer.Flavors().List(ctx)
	if err != nil {
		d.log.Error(err, "Failed to list flavors", "nodeClass", nodeClass.Name)
		return nil, fmt.Errorf("failed to list flavors: %w", err)
	}

	d.log.V(1).Info("Retrieved flavors", "count", len(flavors))

	var instanceTypes []*cloudprovider.InstanceType
	for _, flavor := range flavors {
		d.log.V(1).Info("Processing flavor",
			"name", flavor.Name,
			"vcpus", flavor.VCPUs,
			"ram", flavor.RAM,
		)

	// Create requirements with proper Kubernetes labels
		requirements := scheduling.NewRequirements(
			scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
			scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"), 
			scheduling.NewRequirement("karpenter.sh/capacity-type", corev1.NodeSelectorOpIn, "on-demand"),
		)

		d.log.V(1).Info("Created requirements", 
			"instanceType", flavor.Name,
			"arch", "amd64",
			"capacityType", "on-demand")
	

		// Create capacity for CPU and memory
		cpuQuantity := resource.MustParse(fmt.Sprintf("%d", flavor.VCPUs))
		memoryQuantity := resource.MustParse(fmt.Sprintf("%dMi", flavor.RAM))
		podQuantity := resource.MustParse(fmt.Sprintf("%d", maxPods))
		capacity := corev1.ResourceList{
			corev1.ResourceCPU:    cpuQuantity,
			corev1.ResourceMemory: memoryQuantity,
			corev1.ResourcePods:   podQuantity,
		}
		d.log.V(1).Info("Created capacity",
			"cpu", cpuQuantity.String(),
			"memory", memoryQuantity.String(),
			"pods", podQuantity.String())

		// Create offerings
		offerings := cloudprovider.Offerings{}
		firstOffering := cloudprovider.Offering{
			Requirements:        requirements,
			Price:               0.085,
			Available:           true,
			ReservationCapacity: 1,
		}
		offerings = append(offerings, &firstOffering)
		d.log.V(1).Info("Created offerings", 
			"count", len(offerings),
			"price", firstOffering.Price,
			"available", firstOffering.Available)

		// Create overhead for CPU and memory
		kubeReservedCPU := resource.MustParse("200m")
		kubeReservedMemory := resource.MustParse("200Mi")
		kubeReservedPods := resource.MustParse("4")  // Reserve some pods for system
		
		overhead := cloudprovider.InstanceTypeOverhead{
			KubeReserved: corev1.ResourceList{
				corev1.ResourceCPU:    kubeReservedCPU,
				corev1.ResourceMemory: kubeReservedMemory,
				corev1.ResourcePods:   kubeReservedPods,
			},
			SystemReserved: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
				corev1.ResourcePods:   resource.MustParse("2"),
			},
			EvictionThreshold: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("100m"),
				corev1.ResourceMemory: resource.MustParse("100Mi"),
				corev1.ResourcePods:   resource.MustParse("2"),
			},
		}

		d.log.V(1).Info("Created overhead",
			"kubeReservedCPU", kubeReservedCPU.String(),
			"kubeReservedMemory", kubeReservedMemory.String(),
			"kubeReservedPods", kubeReservedPods.String())


		// Create the instance type with required fields
		instanceType := &cloudprovider.InstanceType{
			Name:         flavor.Name,
			Requirements: requirements,
			Capacity:     capacity,
			Offerings:    offerings,
			Overhead:     &overhead,
		}
		d.log.V(1).Info("Created instance type", 
			"name", instanceType.Name,
			"offeringsCount", len(instanceType.Offerings))

		instanceTypes = append(instanceTypes, instanceType)
	}

	d.log.Info("Successfully listed instance types",
		"count", len(instanceTypes),
		"nodeClass", nodeClass.Name,
	)

	return instanceTypes, nil
}
