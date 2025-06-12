package instancetype

import (
	"context"
	"fmt"

	"github.com/bizflycloud/gobizfly"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/calculator"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/parser"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/validator"
)

const (
	NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
	DiskTypeLabel = "karpenter.bizflycloud.com/disk-type"
)

type Provider interface {
	List(context.Context, *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error)
}

func NewDefaultProvider(client *gobizfly.Client, log logr.Logger) Provider {
	parserObj := parser.NewParser(log)
	
	return &defaultProvider{
		client:     client,
		log:        log,
		parser:     parserObj,
		calculator: calculator.NewCapacityCalculator(),
		pricing:    calculator.NewPricingCalculator(),
		validator:  validator.NewValidator(log, parserObj),
	}
}

type defaultProvider struct {
	client     *gobizfly.Client
	log        logr.Logger
	parser     *parser.Parser
	calculator *calculator.CapacityCalculator
	pricing    *calculator.PricingCalculator
	validator  *validator.Validator
}

func (d *defaultProvider) List(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {

	flavors, err := d.client.CloudServer.Flavors().List(ctx)
	if err != nil {
		d.log.Error(err, "Failed to list flavors", "nodeClass", nodeClass.Name)
		return nil, fmt.Errorf("failed to list flavors: %w", err)
	}

	d.log.V(1).Info("Retrieved flavors", "count", len(flavors))

	var instanceTypes []*cloudprovider.InstanceType
	for _, gobizflyFlavor := range flavors {
		flavor := &validator.FlavorResponse{
			ID:       gobizflyFlavor.ID,
			Name:     gobizflyFlavor.Name,
			VCPUs:    gobizflyFlavor.VCPUs,
			RAM:      gobizflyFlavor.RAM,
			Disk:     gobizflyFlavor.Disk,
			Category: d.parser.CategorizeFlavor(gobizflyFlavor.Name),
		}

		// Skip unusable instance types (now includes category filtering)
		if !d.validator.IsInstanceTypeUsable(flavor, nodeClass) {
			continue
		}

		d.log.V(1).Info("Processing flavor",
			"name", flavor.Name,
			"category", d.parser.CategorizeFlavor(flavor.Name),
			"vcpus", flavor.VCPUs,
			"ram", flavor.RAM)

		// Create capacity for CPU, memory, and pods
		capacity := d.calculator.CreateCapacity(flavor.VCPUs, flavor.RAM)

		// Create requirements with proper Kubernetes labels including category
		requirements := scheduling.NewRequirements(
			scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
			scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
			scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, d.parser.CategorizeFlavor(flavor.Name)),
		)

		d.log.V(1).Info("Created requirements",
			"instanceType", flavor.Name,
			"arch", "amd64",
			"capacityType", "on-demand", "saving_plan")

		cpuQuantity := capacity[corev1.ResourceCPU]
		memoryQuantity := capacity[corev1.ResourceMemory]
		podsQuantity := capacity[corev1.ResourcePods]
		
		d.log.V(1).Info("Created capacity",
			"cpu", (&cpuQuantity).String(),
			"memory", (&memoryQuantity).String(),
			"pods", (&podsQuantity).String())

		// Create offerings
		offerings := d.createOfferings(flavor, nodeClass)
		if len(offerings) == 0 {
			d.log.V(1).Info("Skipping instance type with no offerings", "name", flavor.Name)
			continue
		}

		d.log.V(1).Info("Created offerings",
			"instanceType", flavor.Name,
			"totalOfferings", len(offerings))

		// Calculate realistic overhead
		overhead := d.calculator.CalculateOverhead(flavor.VCPUs, flavor.RAM)

		// Use pointer access for String() method
		kubeReservedCPU := overhead.KubeReserved[corev1.ResourceCPU]
		kubeReservedMemory := overhead.KubeReserved[corev1.ResourceMemory]
		kubeReservedPods := overhead.KubeReserved[corev1.ResourcePods]

		d.log.V(1).Info("Created overhead",
			"kubeReservedCPU", (&kubeReservedCPU).String(),
			"kubeReservedMemory", (&kubeReservedMemory).String(),
			"kubeReservedPods", (&kubeReservedPods).String())

		// Create the instance type
		instanceType := &cloudprovider.InstanceType{
			Name:         flavor.Name,
			Requirements: requirements,
			Capacity:     capacity,
			Offerings:    offerings,
			Overhead:     &overhead,
		}

		// Calculate available resources after overhead
		availableCPU := capacity[corev1.ResourceCPU].DeepCopy()
		availableCPU.Sub(overhead.KubeReserved[corev1.ResourceCPU])
		availableCPU.Sub(overhead.SystemReserved[corev1.ResourceCPU])
		availableCPU.Sub(overhead.EvictionThreshold[corev1.ResourceCPU])

		availableMemory := capacity[corev1.ResourceMemory].DeepCopy()
		availableMemory.Sub(overhead.KubeReserved[corev1.ResourceMemory])
		availableMemory.Sub(overhead.SystemReserved[corev1.ResourceMemory])
		availableMemory.Sub(overhead.EvictionThreshold[corev1.ResourceMemory])

		d.log.V(1).Info("Created instance type",
			"name", instanceType.Name,
			"offeringsCount", len(instanceType.Offerings),
			"availableCPU", (&availableCPU).String(),
			"availableMemory", (&availableMemory).String())

		instanceTypes = append(instanceTypes, instanceType)
	}

	return instanceTypes, nil
}

// createOfferings creates cloud provider offerings for an instance type
func (d *defaultProvider) createOfferings(flavor *validator.FlavorResponse, nodeClass *v1.BizflyCloudNodeClass) cloudprovider.Offerings {
	var offerings cloudprovider.Offerings

	// Use zones and disk types from the NodeClass spec.
	availableZones := nodeClass.Spec.Zones
	availableDiskTypes := nodeClass.Spec.DiskTypes
	capacityTypes := []string{"on-demand", "saving-plan"} // Assuming only on-demand for now.

	if len(availableZones) == 0 {
		d.log.V(1).Info("No zones defined in NodeClass, cannot create offerings", "nodeClass", nodeClass.Name)
		return offerings
	}
	if len(availableDiskTypes) == 0 {
		d.log.V(1).Info("No diskTypes defined in NodeClass, cannot create offerings", "nodeClass", nodeClass.Name)
		return offerings
	}
	
	// Create an offering for each combination of Zone, CapacityType, and DiskType.
	for _, zone := range availableZones {
		for _, capacityType := range capacityTypes {
			for _, diskType := range availableDiskTypes {
				offeringRequirements := scheduling.NewRequirements(
					scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
					scheduling.NewRequirement("karpenter.sh/capacity-type", corev1.NodeSelectorOpIn, capacityType),
					scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, flavor.Category),
					// IMPROVEMENT: Add disk type to the offering's requirements.
					scheduling.NewRequirement(DiskTypeLabel, corev1.NodeSelectorOpIn, diskType),
				)

				basePrice := d.pricing.CalculateBasePrice(flavor.VCPUs, flavor.RAM)
				price := d.pricing.CalculateCategoryPrice(basePrice, flavor.Category, capacityType)

				offerings = append(offerings, &cloudprovider.Offering{
					Requirements:        offeringRequirements,
					Price:               price,
					Available:           true,
				})
			}
		}
	}

	return offerings
}