package validator

import (
    "strings"

    "github.com/go-logr/logr"
    v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/parser"
)

// FlavorResponse represents a BizflyCloud flavor
type FlavorResponse struct {
    ID       string `json:"id"`
    Name     string `json:"name"`
    VCPUs    int    `json:"vcpus"`
    RAM      int    `json:"ram"`
    Disk     int    `json:"disk"`
    Category string `json:"category"`
}

// Validator handles validation of instance types
type Validator struct {
    log    logr.Logger
    parser *parser.Parser
}

// NewValidator creates a new instance type validator
func NewValidator(log logr.Logger, parser *parser.Parser) *Validator {
    return &Validator{
        log:    log,
        parser: parser,
    }
}

// IsInstanceTypeUsable checks if an instance type is usable
func (v *Validator) IsInstanceTypeUsable(flavor *FlavorResponse, nodeClass *v1.BizflyCloudNodeClass) bool {
    // CRITICAL: Exclude VPS flavors entirely
    if strings.Contains(flavor.Name, "_vps") {
        v.log.V(3).Info("Excluding VPS flavor",
            "name", flavor.Name,
            "reason", "VPS flavors not allowed")
        return false
    }

    // Only accept new format with dot notation (e.g., p4a.2c_2g, b2.4c_8g)
    if !strings.Contains(flavor.Name, ".") {
        v.log.V(3).Info("Excluding flavor - old format not supported",
            "name", flavor.Name,
            "reason", "Only new format (e.g., p4a.2c_2g) is supported")
        return false
    }

    // Filter out very small instances
    if flavor.VCPUs < 1 || flavor.RAM < 1024 { // Assuming RAM is in MB
        v.log.V(3).Info("Excluding instance type - too small",
            "name", flavor.Name,
            "vcpus", flavor.VCPUs,
            "ram_mb", flavor.RAM)
        return false
    }

    // Parse CPU vendor from flavor name
    cpuVendor := v.parser.ParseCPUVendor(flavor.Name)

    // Filter by CPU vendor if specified in the NodeClass
    if nodeClass != nil && len(nodeClass.Spec.CPUVendors) > 0 {
        requiredVendors := nodeClass.Spec.CPUVendors

        v.log.V(3).Info("Checking CPU vendor filter",
            "flavorName", flavor.Name,
            "flavorCPUVendor", cpuVendor,
            "requiredCPUVendors", requiredVendors,
            "nodeClass", nodeClass.Name)

        vendorMatchFound := false
        for _, requiredVendor := range requiredVendors {
            if cpuVendor == requiredVendor {
                vendorMatchFound = true
                break
            }
        }

        if !vendorMatchFound {
            v.log.V(3).Info("EXCLUDING instance type - CPU vendor mismatch",
                "name", flavor.Name,
                "flavorCPUVendor", cpuVendor,
                "requiredCPUVendors", requiredVendors)
            return false
        }
        v.log.V(3).Info("Instance type CPU vendor MATCHES one of the required vendors",
            "name", flavor.Name,
            "flavorCPUVendor", cpuVendor,
            "requiredCPUVendors", requiredVendors)
    }

    // Filter by node category if specified in the NodeClass
    if nodeClass != nil && len(nodeClass.Spec.NodeCategories) > 0 {
        flavorCategory := v.parser.CategorizeFlavor(flavor.Name)
        requiredCategories := nodeClass.Spec.NodeCategories

        v.log.V(3).Info("Checking category filter",
            "flavorName", flavor.Name,
            "flavorCategory", flavorCategory,
            "requiredNodeCategories", requiredCategories,
            "nodeClass", nodeClass.Name)

        categoryMatchFound := false
        for _, requiredCategory := range requiredCategories {
            if flavorCategory == requiredCategory {
                categoryMatchFound = true
                break
            }
        }

        if !categoryMatchFound {
            v.log.V(3).Info("EXCLUDING instance type - category mismatch",
                "name", flavor.Name,
                "flavorCategory", flavorCategory,
                "requiredNodeCategories", requiredCategories)
            return false
        }
        v.log.V(3).Info("Instance type category MATCHES one of the required categories",
            "name", flavor.Name,
            "flavorCategory", flavorCategory,
            "requiredNodeCategories", requiredCategories)
    }

    v.log.V(3).Info("Instance type is USABLE",
        "name", flavor.Name,
        "category", v.parser.CategorizeFlavor(flavor.Name),
        "cpuVendor", cpuVendor,
        "vcpus", flavor.VCPUs,
        "ram_mb", flavor.RAM)

    return true
}
