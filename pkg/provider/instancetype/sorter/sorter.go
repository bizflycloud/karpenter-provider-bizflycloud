package sorter

import (
	"sort"

	"github.com/go-logr/logr"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype/parser"
)

// Sorter handles sorting of instance types
type Sorter struct {
	log    logr.Logger
	parser *parser.Parser
}

// NewSorter creates a new instance type sorter
func NewSorter(log logr.Logger, parser *parser.Parser) *Sorter {
	return &Sorter{
		log:    log,
		parser: parser,
	}
}

// CalculateInstanceScore calculates a size score for sorting
// Lower score = smaller instance = higher priority
func (s *Sorter) CalculateInstanceScore(vcpus int, ramMB int) float64 {
	// Normalize CPU and RAM to comparable scales
	// Weight CPU more heavily as it's usually the primary constraint
	cpuScore := float64(vcpus) * 100.0          // CPU weight: 100
	ramScore := float64(ramMB) / 1024.0 * 100.0 // RAM weight: 50 (per GB)

	return cpuScore + ramScore
}

// SortInstanceTypesBySize sorts instance types by size (smallest first)
func (s *Sorter) SortInstanceTypesBySize(instanceTypes []*cloudprovider.InstanceType) {
	sort.Slice(instanceTypes, func(i, j int) bool {
		nameI := instanceTypes[i].Name
		nameJ := instanceTypes[j].Name

		// Parse both instance types
		cpuI, ramI := s.parser.ParseInstanceTypeName(nameI)
		cpuJ, ramJ := s.parser.ParseInstanceTypeName(nameJ)

		// Calculate total resource score (CPU weighted more heavily)
		scoreI := float64(cpuI)*1000 + float64(ramI)/1024 // CPU * 1000 + RAM in GB
		scoreJ := float64(cpuJ)*1000 + float64(ramJ)/1024

		// Sort by total score (smallest first)
		if scoreI != scoreJ {
			return scoreI < scoreJ
		}

		// If scores are equal, prefer fewer CPUs
		if cpuI != cpuJ {
			return cpuI < cpuJ
		}

		// If CPU is equal, prefer less RAM
		return ramI < ramJ
	})

	// Log the sorted order for debugging
	s.log.Info("Instance types sorted by size")
	for i, instanceType := range instanceTypes[:min(10, len(instanceTypes))] {
		cpu, ram := s.parser.ParseInstanceTypeName(instanceType.Name)
		s.log.Info("Sorted instance",
			"index", i,
			"name", instanceType.Name,
			"cpu", cpu,
			"ramGB", ram/1024,
			"score", float64(cpu)*1000+float64(ram)/1024)
	}
}

// Helper function for min
func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}