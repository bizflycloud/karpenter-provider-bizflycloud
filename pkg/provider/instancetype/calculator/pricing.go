package calculator

// PricingCalculator handles pricing calculations for instance types
type PricingCalculator struct{}

// NewPricingCalculator creates a new pricing calculator
func NewPricingCalculator() *PricingCalculator {
	return &PricingCalculator{}
}

// CalculateBasePrice calculates the base price for an instance type
func (p *PricingCalculator) CalculateBasePrice(vcpus int, ramMB int) float64 {
	cpuCost := float64(vcpus) * 0.01          // $0.01 per vCPU per hour
	memoryCost := float64(ramMB) / 1024 * 0.001 // $0.001 per GB per hour
	return cpuCost + memoryCost
}

// LaunchPrice is the on-demand price a flavor's offering is given (instancetype.go).
// Create must launch the cheapest candidate by this same price: Karpenter assumes
// it did, and consolidation replaces any pricier node right after it initializes.
func (p *PricingCalculator) LaunchPrice(category string, vcpus, ramGB int) float64 {
	return p.CalculateCategoryPrice(p.CalculateBasePrice(vcpus, ramGB*1024), category, "on-demand")
}

// CheaperFirst orders flavors by LaunchPrice, then CPU, RAM and name, so the
// first one is what Karpenter expects to be launched.
func (p *PricingCalculator) CheaperFirst(aName, aCategory string, aCPU, aRAM int, bName, bCategory string, bCPU, bRAM int) bool {
	if pa, pb := p.LaunchPrice(aCategory, aCPU, aRAM), p.LaunchPrice(bCategory, bCPU, bRAM); pa != pb {
		return pa < pb
	}
	if aCPU != bCPU {
		return aCPU < bCPU
	}
	if aRAM != bRAM {
		return aRAM < bRAM
	}
	return aName < bName
}

// CalculateCategoryPrice adjusts price based on category and capacity type
func (p *PricingCalculator) CalculateCategoryPrice(basePrice float64, category, capacityType string) float64 {
	// Adjust price based on category
	switch category {
	case "basic":
		basePrice *= 1.0 // Standard pricing
	case "premium":
		basePrice *= 1.5 // 50% more expensive
	case "enterprise":
		basePrice *= 2.0 // 100% more expensive
	case "dedicated":
		basePrice *= 2.5 // 150% more expensive
	}

	// Adjust for capacity type
	if capacityType == "spot" {
		basePrice *= 0.3 // Spot is 70% cheaper
	}

	return basePrice
}