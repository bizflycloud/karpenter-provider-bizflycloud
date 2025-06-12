package parser

import (
	"strconv"
	"strings"

	"github.com/go-logr/logr"
)

// Parser handles parsing of instance type names and configurations
type Parser struct {
	log logr.Logger
}

// NewParser creates a new instance type parser
func NewParser(log logr.Logger) *Parser {
	return &Parser{
		log: log,
	}
}

// ParseInstanceTypeName extracts CPU and RAM from instance type names
// Handles formats like: nix.3c_4g, 4c_8g_basic, 12c_16g_enterprise
func (p *Parser) ParseInstanceTypeName(instanceType string) (int, int) {
	// Remove "nix." prefix
	name := strings.TrimPrefix(instanceType, "nix.")

	// Split by underscore: ["2c", "3g"] or ["12c", "16g", "basic"]
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		p.log.V(1).Info("Invalid instance type format", "name", instanceType)
		return 999, 999999 // Sort to end
	}

	// Parse CPU (remove 'c' suffix)
	cpuStr := strings.TrimSuffix(parts[0], "c")
	cpu, err := strconv.Atoi(cpuStr)
	if err != nil {
		p.log.V(1).Info("Could not parse CPU", "name", instanceType, "cpuStr", cpuStr)
		return 999, 999999
	}

	// Parse RAM (remove 'g' suffix)
	ramStr := strings.TrimSuffix(parts[1], "g")
	ramGB, err := strconv.Atoi(ramStr)
	if err != nil {
		p.log.V(1).Info("Could not parse RAM", "name", instanceType, "ramStr", ramStr)
		return cpu, 999999
	}

	ramMB := ramGB * 1024
	return cpu, ramMB
}

// CategorizeFlavor determines the category of a flavor
func (p *Parser) CategorizeFlavor(flavorName string) string {
	p.log.V(2).Info("Categorizing flavor", "flavorName", flavorName)

	// CRITICAL: Exclude VPS flavors entirely
	if strings.Contains(flavorName, "_vps") {
		p.log.V(2).Info("Flavor is VPS - categorizing as excluded", "flavorName", flavorName)
		return "vps" // Special category to exclude
	}

	switch {
	case strings.HasPrefix(flavorName, "nix."):
		p.log.V(2).Info("Flavor categorized as premium", "flavorName", flavorName)
		return "premium"
	case strings.HasSuffix(flavorName, "_basic"):
		p.log.V(2).Info("Flavor categorized as basic", "flavorName", flavorName)
		return "basic"
	case strings.HasSuffix(flavorName, "_enterprise"):
		p.log.V(2).Info("Flavor categorized as enterprise", "flavorName", flavorName)
		return "enterprise"
	case strings.HasSuffix(flavorName, "_dedicated"):
		p.log.V(2).Info("Flavor categorized as dedicated", "flavorName", flavorName)
		return "dedicated"
	default:
		p.log.V(2).Info("Flavor categorized as basic (default)", "flavorName", flavorName)
		return "premium" // Default category
	}
}