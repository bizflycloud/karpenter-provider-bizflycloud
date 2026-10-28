package calculator

import "strings"

// PricingCalculator computes the price of an instance type from the Bizfly Cloud
// Server price lists (see pricing_table.go).
type PricingCalculator struct{}

// NewPricingCalculator creates a new pricing calculator
func NewPricingCalculator() *PricingCalculator {
	return &PricingCalculator{}
}

// CalculatePrice returns the hourly on-demand price in VND (excl. VAT) of CPU + RAM
// for a flavor. Add CalculateDiskPrice for the root disk. Image, bandwidth and WAN IP
// are not included: they do not depend on the flavor or disk.
//
// cpuVendor is the parsed vendor and generation ("Intel2", "AMD4", ""). Generations
// without a price list use the closest one: AMD* is priced as AMD Gen 4, everything
// else as Intel Gen 2. AMD is not sold as basic, so basic always uses the Intel list.
func (p *PricingCalculator) CalculatePrice(vcpus, ramMB int, category, cpuVendor string) float64 {
	cores := float64(vcpus)
	ramGB := float64(ramMB) / 1024

	category = priceCategory(category)
	if strings.HasPrefix(cpuVendor, "AMD") {
		if cpuRate, ok := amd4CPUPerCore[category]; ok {
			return cores*cpuRate + ramGB*amd4RAMPerGB
		}
	}
	return tierPrice(intel2CPU[category], cores) + tierPrice(intel2RAM[category], ramGB)
}

// CalculateDiskPrice returns the hourly on-demand price in VND (excl. VAT) of a root
// disk of sizeGB: the first 100 GB at one rate, the rest at a lower one. diskType is
// HDD, SSD or NVME (any case); anything else is priced as SSD, the launch default.
func (p *PricingCalculator) CalculateDiskPrice(sizeGB int, diskType, category string) float64 {
	if sizeGB <= 0 {
		return 0
	}
	category = priceCategory(category)
	diskType = strings.ToUpper(diskType)
	if diskType != "HDD" && diskType != "NVME" {
		diskType = "SSD"
	}
	rate := diskRates[category][diskType]
	first := min(sizeGB, 100)
	return float64(first)*rate.first + float64(sizeGB-first)*rate.tail
}

// priceCategory maps a flavor category onto a price list column. VPS flavors are
// billed as basic; unknown categories as premium, like utils.CategorizeInstanceType.
func priceCategory(category string) string {
	switch category {
	case "basic", "premium", "enterprise", "dedicated":
		return category
	case "vps":
		return "basic"
	default:
		return "premium"
	}
}

// tierPrice prices n units from a sorted price list. Sizes between two rows are
// interpolated linearly; sizes outside the list keep the unit price of the nearest row.
func tierPrice(tiers []tier, n float64) float64 {
	if len(tiers) == 0 || n <= 0 {
		return 0
	}
	first, last := tiers[0], tiers[len(tiers)-1]
	if n <= first.size {
		return first.price / first.size * n
	}
	if n >= last.size {
		return last.price / last.size * n
	}
	for i := 1; i < len(tiers); i++ {
		lo, hi := tiers[i-1], tiers[i]
		if n <= hi.size {
			return lo.price + (hi.price-lo.price)*(n-lo.size)/(hi.size-lo.size)
		}
	}
	return last.price
}
