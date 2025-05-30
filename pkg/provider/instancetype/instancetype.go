package instancetype

import (
	"context"
	"fmt"
	"sort"

	"github.com/bizflycloud/gobizfly"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"sigs.k8s.io/karpenter/pkg/scheduling"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/calculator"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/parser"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/sorter"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/validator"
)

const (
	NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
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
		sorter:     sorter.NewSorter(log, parserObj),
		validator:  validator.NewValidator(log, parserObj),
	}
}

type defaultProvider struct {
	client     *gobizfly.Client
	log        logr.Logger
	parser     *parser.Parser
	calculator *calculator.CapacityCalculator
	pricing    *calculator.PricingCalculator
	sorter     *sorter.Sorter
	validator  *validator.Validator
}

func (d *defaultProvider) List(ctx context.Context, nodeClass *v1.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {
	d.log.Info("=== STARTING INSTANCE TYPE LISTING ===",
		"nodeClass", nodeClass.Name,
		"nodeCategory", nodeClass.Spec.NodeCategory)

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
			Category: gobizflyFlavor.Category,
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
		offerings := d.createOfferings(flavor)

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

	d.sorter.SortInstanceTypesBySize(instanceTypes)

	// Log the sorted order for debugging
	d.log.Info("=== FINAL SORTED INSTANCE TYPES ===")
	for i, instanceType := range instanceTypes {
		cpu, ram := d.parser.ParseInstanceTypeName(instanceType.Name)
		score := d.sorter.CalculateInstanceScore(cpu, ram)
		d.log.V(1).Info("Sorted instance type",
			"index", i,
			"name", instanceType.Name,
			"cpu", cpu,
			"ramMB", ram,
			"score", score)
	}

	d.log.Info("=== INSTANCE TYPE LISTING COMPLETE ===",
		"totalTypes", len(instanceTypes),
		"smallestFirst", instanceTypes[0].Name,
		"largestLast", instanceTypes[len(instanceTypes)-1].Name)

	return instanceTypes, nil
}

// createOfferings creates cloud provider offerings for an instance type
func (d *defaultProvider) createOfferings(flavor *validator.FlavorResponse) cloudprovider.Offerings {
	var offerings cloudprovider.Offerings
	availableZones := []string{"HN1", "HN2"}
	capacityTypes := []string{"on-demand", "spot", "saving_plan"}

	for _, zone := range availableZones {
		for _, capacityType := range capacityTypes {
			offeringRequirements := scheduling.NewRequirements(
				scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
				scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
				scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
				scheduling.NewRequirement("karpenter.sh/capacity-type", corev1.NodeSelectorOpIn, capacityType),
				scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, d.parser.CategorizeFlavor(flavor.Name)),
			)

			basePrice := d.pricing.CalculateBasePrice(flavor.VCPUs, flavor.RAM)
			price := d.pricing.CalculateCategoryPrice(basePrice, d.parser.CategorizeFlavor(flavor.Name), capacityType)

			offering := cloudprovider.Offering{
				Requirements:        offeringRequirements,
				Price:               price,
				Available:           true,
				ReservationCapacity: 1,
			}
			offerings = append(offerings, &offering)
		}
	}

	sort.Slice(offerings, func(i, j int) bool {
		return offerings[i].Price < offerings[j].Price
	})

	return offerings
}