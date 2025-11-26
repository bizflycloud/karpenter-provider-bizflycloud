package instancetype

import (
    "context"
    "fmt"

    "github.com/bizflycloud/gobizfly"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/calculator"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/parser"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/validator"
    "github.com/go-logr/logr"
    corev1 "k8s.io/api/core/v1"
    "sigs.k8s.io/karpenter/pkg/cloudprovider"
    "sigs.k8s.io/karpenter/pkg/scheduling"
)

const (
    NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
    DiskTypeLabel     = "karpenter.bizflycloud.com/disk-type"
    OSTypeLabel       = "karpenter.bizflycloud.com/os-type"
    CPUVendorLabel    = "karpenter.bizflycloud.com/cpu-vendor"
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
    // Get flavors from BizflyCloud API
    flavors, err := d.client.CloudServer.Flavors().List(ctx)
    if err != nil {
        d.log.Error(err, "Failed to list flavors", "nodeClass", nodeClass.Name)
        return nil, fmt.Errorf("failed to list flavors: %w", err)
    }
    d.log.V(1).Info("Retrieved flavors", "count", len(flavors))

    var instanceTypes []*cloudprovider.InstanceType
    for _, gobizflyFlavor := range flavors {
        
        // Parse CPU vendor directly from flavor name (e.g., p4a -> AMD4, p2 -> Intel2)
        cpuVendor := d.parser.ParseCPUVendor(gobizflyFlavor.Name)
        
        // Parse category from flavor name (e.g., p4a -> premium, b2 -> basic)
        category := d.parser.CategorizeFlavor(gobizflyFlavor.Name)

        flavor := &validator.FlavorResponse{
            ID:       gobizflyFlavor.ID,
            Name:     gobizflyFlavor.Name,
            VCPUs:    gobizflyFlavor.VCPUs,
            RAM:      gobizflyFlavor.RAM,
            Disk:     gobizflyFlavor.Disk,
            Category: category,
        }

        // Validate if this flavor is usable based on NodeClass requirements
        if !d.validator.IsInstanceTypeUsable(flavor, nodeClass) {
            d.log.V(2).Info("Skipping unusable flavor",
                "name", flavor.Name,
                "category", category,
                "cpuVendor", cpuVendor)
            continue
        }

        // Calculate resource capacity
        capacity := d.calculator.CreateCapacity(flavor.VCPUs, flavor.RAM)

        // Create requirements (labels that describe this instance type)
        // Use pointers directly - Karpenter expects *Requirement
        reqs := []*scheduling.Requirement{
            scheduling.NewRequirement(corev1.LabelInstanceTypeStable, corev1.NodeSelectorOpIn, flavor.Name),
            scheduling.NewRequirement(corev1.LabelArchStable, corev1.NodeSelectorOpIn, "amd64"),
            scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, category),
        }

        // Add CPU vendor requirement if available
        if cpuVendor != "" {
            reqs = append(reqs, scheduling.NewRequirement(CPUVendorLabel, corev1.NodeSelectorOpIn, cpuVendor))
            d.log.V(2).Info("Added CPU vendor requirement",
                "flavor", flavor.Name,
                "cpuVendor", cpuVendor)
        }

        requirements := scheduling.NewRequirements(reqs...)

        // Create offerings (combinations of zones and disk types)
        offerings := d.createOfferings(flavor, nodeClass, category, cpuVendor)
        if len(offerings) == 0 {
            d.log.V(2).Info("No offerings available for flavor", "name", flavor.Name)
            continue
        }

        // Calculate overhead
        overhead := d.calculator.CalculateOverhead(flavor.VCPUs, flavor.RAM)

        instanceType := &cloudprovider.InstanceType{
            Name:         flavor.Name,
            Requirements: requirements,
            Capacity:     capacity,
            Offerings:    offerings,
            Overhead:     &overhead,
        }

        d.log.V(2).Info("Created instance type",
            "name", flavor.Name,
            "category", category,
            "cpuVendor", cpuVendor,
            "vcpus", flavor.VCPUs,
            "ram", flavor.RAM,
            "offerings", len(offerings))

        instanceTypes = append(instanceTypes, instanceType)
    }

    d.log.Info("Successfully created instance types",
        "total", len(instanceTypes),
        "nodeClass", nodeClass.Name)

    return instanceTypes, nil
}

func (d *defaultProvider) createOfferings(flavor *validator.FlavorResponse, nodeClass *v1.BizflyCloudNodeClass, category string, cpuVendor string) cloudprovider.Offerings {
    var offerings cloudprovider.Offerings

    availableZones := nodeClass.Spec.Zones
    availableDiskTypes := nodeClass.Spec.DiskTypes

    if len(availableZones) == 0 || len(availableDiskTypes) == 0 {
        d.log.V(2).Info("No zones or disk types configured",
            "zones", len(availableZones),
            "diskTypes", len(availableDiskTypes))
        return offerings
    }

    for _, zone := range availableZones {
        for _, diskType := range availableDiskTypes {
            // Calculate price
            basePrice := d.pricing.CalculateBasePrice(flavor.VCPUs, flavor.RAM)
            price := d.pricing.CalculateCategoryPrice(basePrice, category, "on-demand")

            // Create offering requirements
            offeringReqs := []*scheduling.Requirement{
                scheduling.NewRequirement(corev1.LabelTopologyZone, corev1.NodeSelectorOpIn, zone),
                scheduling.NewRequirement(NodeCategoryLabel, corev1.NodeSelectorOpIn, category),
                scheduling.NewRequirement(DiskTypeLabel, corev1.NodeSelectorOpIn, diskType),
            }

            // Add CPU vendor to offering requirements
            if cpuVendor != "" {
                offeringReqs = append(offeringReqs, 
                    scheduling.NewRequirement(CPUVendorLabel, corev1.NodeSelectorOpIn, cpuVendor))
            }

            offeringRequirements := scheduling.NewRequirements(offeringReqs...)

            offerings = append(offerings, &cloudprovider.Offering{
                Requirements: offeringRequirements,
                Price:        price,
                Available:    true,
            })
        }
    }

    d.log.V(3).Info("Created offerings",
        "flavor", flavor.Name,
        "zones", len(availableZones),
        "diskTypes", len(availableDiskTypes),
        "totalOfferings", len(offerings))

    return offerings
}
