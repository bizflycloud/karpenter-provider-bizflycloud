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

func (p *Parser) ParseInstanceTypeName(instanceType string) (int, int) {
    // New format must contain a dot: p4a.2c_2g or b2.4c_8g
    if !strings.Contains(instanceType, ".") {
        p.log.V(1).Info("Invalid instance type format - missing dot separator", 
            "name", instanceType)
        return 999, 999999
    }

    parts := strings.Split(instanceType, ".")
    if len(parts) != 2 {
        p.log.V(1).Info("Invalid instance type format", "name", instanceType)
        return 999, 999999
    }

    cpuRamPart := parts[1]  // e.g., "4c_8g"
    
    cpuRamParts := strings.Split(cpuRamPart, "_")
    if len(cpuRamParts) < 2 {
        p.log.V(1).Info("Invalid CPU/RAM format", "name", instanceType)
        return 999, 999999
    }

    // Parse CPU
    cpuStr := strings.TrimSuffix(cpuRamParts[0], "c")
    cpu, err := strconv.Atoi(cpuStr)
    if err != nil {
        p.log.V(1).Info("Could not parse CPU", "name", instanceType)
        return 999, 999999
    }

    // Parse RAM
    ramStr := strings.TrimSuffix(cpuRamParts[1], "g")
    ramGB, err := strconv.Atoi(ramStr)
    if err != nil {
        p.log.V(1).Info("Could not parse RAM", "name", instanceType)
        return cpu, 999999
    }

    ramMB := ramGB * 1024
    return cpu, ramMB
}


// CategorizeFlavor determines the category/tier of a flavor from the first character
// Examples:
//   - "p4a.2c_2g" -> "premium"
//   - "b2.4c_8g" -> "basic"
//   - "e4a.1c_1g" -> "enterprise"
//   - "d2.1c_1g" -> "dedicated"
func (p *Parser) CategorizeFlavor(flavorName string) string {
    p.log.V(2).Info("Categorizing flavor", "flavorName", flavorName)

    // CRITICAL: Exclude VPS flavors entirely
    if strings.Contains(flavorName, "_vps") {
        p.log.V(2).Info("Flavor is VPS - categorizing as excluded", "flavorName", flavorName)
        return "vps" // Special category to exclude
    }

    // Get first character which represents the tier
    if len(flavorName) == 0 {
        p.log.V(1).Info("Empty flavor name", "flavorName", flavorName)
        return ""
    }

    tierChar := string(flavorName[0])
    
    var category string
    switch tierChar {
    case "p":
        category = "premium"
    case "b":
        category = "basic"
    case "e":
        category = "enterprise"
    case "d":
        category = "dedicated"
    default:
        p.log.V(1).Info("Unknown tier prefix, defaulting to premium", 
            "flavorName", flavorName,
            "tierChar", tierChar)
        category = "premium" // Default category
    }

    p.log.V(2).Info("Flavor categorized", 
        "flavorName", flavorName, 
        "category", category)
    
    return category
}

// ParseCPUVendor extracts CPU vendor with generation from instance type
// Only supports:
//   - "p2.1c_1g" or "b2.1c_1g" -> "Intel2"
//   - "p4a.1c_1g" or "b4a.1c_1g" -> "AMD4"
func (p *Parser) ParseCPUVendor(flavorName string) string {
    p.log.V(2).Info("Parsing CPU vendor", "flavorName", flavorName)

    if !strings.Contains(flavorName, ".") {
        p.log.V(1).Info("Cannot parse CPU vendor - invalid format", "flavorName", flavorName)
        return ""
    }

    parts := strings.Split(flavorName, ".")
    if len(parts) < 2 {
        p.log.V(1).Info("Cannot parse CPU vendor - invalid format", "flavorName", flavorName)
        return ""
    }

    prefix := parts[0]
    
    // Remove tier prefix (first character: p, b, e, d)
    if len(prefix) < 2 {
        p.log.V(1).Info("Cannot parse CPU vendor - prefix too short", 
            "flavorName", flavorName,
            "prefix", prefix)
        return ""
    }
    
    genString := prefix[1:] // e.g., "4a" or "2"
    
    var cpuVendor string
    // Only handle Intel2 and AMD4
    if genString == "4a" {
        cpuVendor = "AMD4"
    } else if genString == "2" {
        cpuVendor = "Intel2"
    } else {
        p.log.V(1).Info("Unknown CPU vendor format", 
            "flavorName", flavorName,
            "genString", genString)
        return ""
    }

    p.log.V(2).Info("Parsed CPU vendor",
        "flavorName", flavorName,
        "cpuVendor", cpuVendor)
    
    return cpuVendor
}

// ParseTier extracts the tier from the flavor name
// Examples:
//   - "p4a.2c_2g" -> "p"
//   - "b2.4c_8g" -> "b"
func (p *Parser) ParseTier(flavorName string) string {
    if len(flavorName) == 0 {
        return ""
    }
    return string(flavorName[0])
}

// IsValidFlavorFormat checks if the flavor name follows the expected format
// Valid format: <tier><gen><vendor>.<cpu>c_<ram>g
func (p *Parser) IsValidFlavorFormat(flavorName string) bool {
    // Must contain a dot
    if !strings.Contains(flavorName, ".") {
        p.log.V(2).Info("Invalid format - missing dot", "flavorName", flavorName)
        return false
    }

    parts := strings.Split(flavorName, ".")
    if len(parts) != 2 {
        p.log.V(2).Info("Invalid format - incorrect parts", "flavorName", flavorName)
        return false
    }

    prefix := parts[0]
    cpuRamPart := parts[1]

    // Check prefix (should be at least 2 chars: tier + gen)
    if len(prefix) < 2 {
        p.log.V(2).Info("Invalid format - prefix too short", 
            "flavorName", flavorName,
            "prefix", prefix)
        return false
    }

    // Check CPU/RAM part format
    if !strings.Contains(cpuRamPart, "_") {
        p.log.V(2).Info("Invalid format - missing underscore in CPU/RAM", 
            "flavorName", flavorName)
        return false
    }

    cpuRamParts := strings.Split(cpuRamPart, "_")
    if len(cpuRamParts) != 2 {
        p.log.V(2).Info("Invalid format - incorrect CPU/RAM parts", 
            "flavorName", flavorName)
        return false
    }

    // Check CPU format (should end with 'c')
    if !strings.HasSuffix(cpuRamParts[0], "c") {
        p.log.V(2).Info("Invalid format - CPU doesn't end with 'c'", 
            "flavorName", flavorName)
        return false
    }

    // Check RAM format (should end with 'g')
    if !strings.HasSuffix(cpuRamParts[1], "g") {
        p.log.V(2).Info("Invalid format - RAM doesn't end with 'g'", 
            "flavorName", flavorName)
        return false
    }

    p.log.V(2).Info("Valid flavor format", "flavorName", flavorName)
    return true
}
