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