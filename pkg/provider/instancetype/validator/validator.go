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

	// Filter out very small instances
	if flavor.VCPUs < 1 || flavor.RAM < 1024 {
		v.log.V(3).Info("Excluding instance type - too small",
			"name", flavor.Name,
			"vcpus", flavor.VCPUs,
			"ram", flavor.RAM)
		return false
	}

	// Filter by node category if specified
	if nodeClass != nil && nodeClass.Spec.NodeCategory != "" {
		flavorCategory := v.parser.CategorizeFlavor(flavor.Name)
		requiredCategory := nodeClass.Spec.NodeCategory

		v.log.V(3).Info("Checking category filter",
			"flavorName", flavor.Name,
			"flavorCategory", flavorCategory,
			"requiredCategory", requiredCategory,
			"nodeClass", nodeClass.Name)

		if flavorCategory != requiredCategory {
			v.log.V(3).Info("EXCLUDING instance type - category mismatch",
				"name", flavor.Name,
				"flavorCategory", flavorCategory,
				"requiredCategory", requiredCategory)
			return false
		}
	}

	v.log.V(3).Info("Instance type is USABLE",
		"name", flavor.Name,
		"category", v.parser.CategorizeFlavor(flavor.Name),
		"vcpus", flavor.VCPUs,
		"ram", flavor.RAM)

	return true
}