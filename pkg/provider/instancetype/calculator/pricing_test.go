package calculator

import (
	"math"
	"testing"
)

func TestCalculatePrice(t *testing.T) {
	p := NewPricingCalculator()
	tests := []struct {
		name     string
		vcpus    int
		ramMB    int
		category string
		vendor   string
		want     float64
	}{
		// exact rows of the Intel Gen 2 on-demand list (CPU + RAM)
		{"intel premium 2c4g", 2, 4096, "premium", "Intel2", 488 + 735},
		{"intel basic 12c16g", 12, 16384, "basic", "Intel2", 3330 + 1980},
		{"intel enterprise 32c64g", 32, 65536, "enterprise", "Intel2", 19935 + 18435},
		{"intel dedicated 8c32g", 8, 32768, "dedicated", "Intel2", 5415 + 9338},
		// between rows: 5 cores is halfway through 4 (1170) .. 6 (1755)
		{"interpolated cores", 5, 4096, "premium", "Intel2", 1462.5 + 735},
		// beyond the last basic row (12 cores / 24 GB) keeps the unit price
		{"above the list", 16, 24576, "basic", "Intel2", 3330.0/12*16 + 3300},
		// AMD Gen 4 is linear
		{"amd premium 4c8g", 4, 8192, "premium", "AMD4", 4*292 + 8*177},
		{"amd enterprise 2c4g", 2, 4096, "enterprise", "AMD4", 2*396 + 4*177},
		{"amd dedicated 1c1g", 1, 1024, "dedicated", "AMD4", 1167 + 177},
		// generations without a list use the closest one
		{"amd5 as amd4", 4, 8192, "premium", "AMD5", 4*292 + 8*177},
		{"intel3 as intel2", 2, 4096, "premium", "Intel3", 488 + 735},
		{"no vendor as intel2", 2, 4096, "premium", "", 488 + 735},
		// AMD has no basic: falls back to the Intel basic list
		{"amd basic", 2, 4096, "basic", "AMD4", 150 + 518},
		// categories without a column
		{"vps as basic", 2, 4096, "vps", "Intel2", 150 + 518},
		{"unknown as premium", 2, 4096, "", "Intel2", 488 + 735},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.CalculatePrice(tt.vcpus, tt.ramMB, tt.category, tt.vendor)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("CalculatePrice(%d, %d, %q, %q) = %v, want %v",
					tt.vcpus, tt.ramMB, tt.category, tt.vendor, got, tt.want)
			}
		})
	}
}

func TestCalculatePriceNeverDropsWithSize(t *testing.T) {
	p := NewPricingCalculator()
	for _, vendor := range []string{"Intel2", "AMD4"} {
		for _, category := range []string{"basic", "premium", "enterprise", "dedicated"} {
			prev := 0.0
			for vcpus := 1; vcpus <= 96; vcpus++ {
				got := p.CalculatePrice(vcpus, vcpus*2048, category, vendor)
				if got < prev {
					t.Errorf("%s %s: %d vCPU costs %v, less than %d vCPU (%v)",
						vendor, category, vcpus, got, vcpus-1, prev)
				}
				prev = got
			}
		}
	}
}

func TestCalculatePriceInvalidSize(t *testing.T) {
	if got := NewPricingCalculator().CalculatePrice(0, 0, "premium", "Intel2"); got != 0 {
		t.Errorf("empty flavor = %v, want 0", got)
	}
}

func TestDiskPricesOrdered(t *testing.T) {
	p := NewPricingCalculator()
	for _, category := range []string{"basic", "premium", "enterprise", "dedicated"} {
		for _, size := range []int{40, 100, 500} {
			hdd := p.CalculateDiskPrice(size, "HDD", category)
			ssd := p.CalculateDiskPrice(size, "SSD", category)
			nvme := p.CalculateDiskPrice(size, "NVME", category)
			if !(hdd < ssd && ssd < nvme) {
				t.Errorf("%s %dGB: want HDD < SSD < NVMe, got %v, %v, %v", category, size, hdd, ssd, nvme)
			}
		}
	}
}

func TestCalculateDiskPrice(t *testing.T) {
	p := NewPricingCalculator()
	tests := []struct {
		name     string
		sizeGB   int
		diskType string
		category string
		want     float64
	}{
		{"premium ssd within 100GB", 40, "SSD", "premium", 40 * 12.6},
		{"premium ssd beyond 100GB", 150, "SSD", "premium", 100*12.6 + 50*8.3},
		{"premium hdd", 40, "HDD", "premium", 40 * 3.6},
		{"basic hdd beyond 100GB", 120, "HDD", "basic", 100*2.0 + 20*1.4},
		// blank cells derived by ratio: SSD tail/first = 8.3/12.6, basic/premium = 6.2/12.6
		{"basic ssd beyond 100GB", 120, "SSD", "basic", 100*6.2 + 20*6.2*8.3/12.6},
		{"enterprise nvme", 120, "NVME", "enterprise", 100*19.0 + 20*12.5},
		{"disk type is case-insensitive", 40, "nvme", "dedicated", 40 * 19.0},
		{"premium nvme as enterprise", 40, "NVMe", "premium", 40 * 19.0},
		{"basic nvme", 120, "NVME", "basic", 100*19.0*6.2/12.6 + 20*12.5*6.2/12.6},
		// unknown disk types are priced as SSD, which is what the launch uses
		{"unknown type as ssd", 40, "", "premium", 40 * 12.6},
		{"vps as basic", 40, "SSD", "vps", 40 * 6.2},
		{"no disk", 0, "SSD", "premium", 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := p.CalculateDiskPrice(tt.sizeGB, tt.diskType, tt.category)
			if math.Abs(got-tt.want) > 1e-6 {
				t.Errorf("CalculateDiskPrice(%d, %q, %q) = %v, want %v",
					tt.sizeGB, tt.diskType, tt.category, got, tt.want)
			}
		})
	}
}
