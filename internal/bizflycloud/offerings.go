package bizflycloud

import (
	"context"
	"fmt"
	"math"
	"os"
	"sort"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/samber/lo"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/metrics"
)

// InstanceOffering represents a BizFly Cloud flavor offering
type InstanceOffering struct {
	Name        string
	ID          string
	CPU         int64
	Memory      int64
	Disk        int64
	GPU         int64
	Region      string
	Zone        string
	PriceHourly float64
	IsSpot      bool
	SpotPrice   float64
}

const (
	// Megabyte conversion to bytes
	MiB int64 = 1024 * 1024
	// Gigabyte conversion to bytes
	GiB int64 = 1024 * 1024 * 1024
	
	// Labels for metrics
	LabelRegion       = "region"
	LabelZone         = "zone"
	LabelInstanceType = "instance_type"
	LabelSpot         = "spot"
)

var (
	// offeringCache to improve performance
	offeringCache struct {
		offerings []InstanceOffering
		expiresAt time.Time
		sync.RWMutex
	}
	
	// Metrics for instance offerings
	instanceTypesAvailable = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "karpenter_bizflycloud_instance_types_available",
			Help: "Number of instance types available by region and zone",
		},
		[]string{LabelRegion, LabelZone},
	)
	
	instanceTypeSelections = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "karpenter_bizflycloud_instance_type_selections_total",
			Help: "Total number of instance type selections by instance type and spot",
		},
		[]string{LabelInstanceType, LabelSpot},
	)
	
	instanceTypePrice = prometheus.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "karpenter_bizflycloud_instance_type_price",
			Help: "Price of instance types by instance type and spot",
		},
		[]string{LabelInstanceType, LabelSpot},
	)
)

func init() {
	// Register metrics with the global prometheus registry
	metrics.Registry.MustRegister(
		instanceTypesAvailable,
		instanceTypeSelections,
		instanceTypePrice,
	)
}

const (
	// CacheTTL defines how long to cache offerings before refreshing
	CacheTTL = 10 * time.Minute
)

// getAllInstanceOfferings returns all available instance types from BizFly Cloud
func (p *Provider) getAllInstanceOfferings(ctx context.Context) ([]InstanceOffering, error) {
	p.log.V(1).Info("Getting all instance offerings")

	// Check the cache first
	offeringCache.RLock()
	if offeringCache.offerings != nil && time.Now().Before(offeringCache.expiresAt) {
		offerings := offeringCache.offerings
		offeringCache.RUnlock()
		return offerings, nil
	}
	offeringCache.RUnlock()

	// Cache miss or expired, need to refresh
	offeringCache.Lock()
	defer offeringCache.Unlock()

	// Double-check to avoid race condition where another routine might have updated while we were waiting
	if offeringCache.offerings != nil && time.Now().Before(offeringCache.expiresAt) {
		return offeringCache.offerings, nil
	}

	// For mock mode, use static data
	if p.bizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.log.Info("Using mock instance offerings")
		
		// Generate offerings for all supported regions
		offerings := []InstanceOffering{}
		
		// Add regular instance types
		standardTypes := []struct {
			name    string
			cpu     int64
			memory  int64
			disk    int64
			hourly  float64
		}{
			{"2c_2g", 2, 2 * GiB, 40 * GiB, 0.03},
			{"4c_8g", 4, 8 * GiB, 60 * GiB, 0.12},
			{"8c_16g", 8, 16 * GiB, 80 * GiB, 0.24},
			{"16c_32g", 16, 32 * GiB, 160 * GiB, 0.48},
			{"32c_64g", 32, 64 * GiB, 320 * GiB, 0.96},
		}
		
		// Add GPU instance types
		gpuTypes := []struct {
			name    string
			cpu     int64
			memory  int64
			disk    int64
			gpu     int64
			hourly  float64
		}{
			{"2c_4g_gpu", 2, 4 * GiB, 40 * GiB, 1, 0.40},
			{"4c_8g_gpu", 4, 8 * GiB, 60 * GiB, 1, 0.80},
			{"8c_16g_gpu", 8, 16 * GiB, 80 * GiB, 1, 1.20},
		}
		
		// Define regions and zones
		regions := []string{"HN1", "HN2", "HCM1"}
		
		// Generate offerings for all regions and zones
		for _, region := range regions {
			for _, t := range standardTypes {
				// Create standard instances
				for zoneIdx := 0; zoneIdx < 2; zoneIdx++ {
					zone := fmt.Sprintf("%s-%c", region, 'a'+zoneIdx)
					
					// Regular instance
					offerings = append(offerings, InstanceOffering{
						Name:        t.name,
						ID:          fmt.Sprintf("flavor-%s-%s", t.name, region),
						CPU:         t.cpu,
						Memory:      t.memory,
						Disk:        t.disk,
						Region:      region,
						Zone:        zone,
						PriceHourly: t.hourly,
						IsSpot:      false,
					})
					
					// Spot instance (70% discount)
					offerings = append(offerings, InstanceOffering{
						Name:        t.name,
						ID:          fmt.Sprintf("flavor-%s-%s-spot", t.name, region),
						CPU:         t.cpu,
						Memory:      t.memory,
						Disk:        t.disk,
						Region:      region,
						Zone:        zone,
						PriceHourly: t.hourly * 0.3, // 70% discount
						IsSpot:      true,
						SpotPrice:   t.hourly * 0.3,
					})
				}
			}
			
			// Add GPU instances (only available in zone a)
			for _, t := range gpuTypes {
				zone := fmt.Sprintf("%s-a", region)
				
				// Regular GPU instance
				offerings = append(offerings, InstanceOffering{
					Name:        t.name,
					ID:          fmt.Sprintf("flavor-%s-%s", t.name, region),
					CPU:         t.cpu,
					Memory:      t.memory,
					Disk:        t.disk,
					GPU:         t.gpu,
					Region:      region,
					Zone:        zone,
					PriceHourly: t.hourly,
					IsSpot:      false,
				})
				
				// Spot GPU instance (70% discount)
				offerings = append(offerings, InstanceOffering{
					Name:        t.name,
					ID:          fmt.Sprintf("flavor-%s-%s-spot", t.name, region),
					CPU:         t.cpu,
					Memory:      t.memory,
					Disk:        t.disk,
					GPU:         t.gpu,
					Region:      region,
					Zone:        zone,
					PriceHourly: t.hourly * 0.3, // 70% discount
					IsSpot:      true,
					SpotPrice:   t.hourly * 0.3,
				})
			}
		}
		
		// Export pricing metrics
		p.updateOfferingMetrics(offerings)
		
		// Cache the offerings
		offeringCache.offerings = offerings
		offeringCache.expiresAt = time.Now().Add(CacheTTL)
		
		return offerings, nil
	}

	// For real implementation, get offerings from BizFly Cloud API
	/* For reference, here's a real API call using gobizfly client:
	
	flavors, err := p.bizflyClient.Flavor.List(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list flavors: %w", err)
	}
	
	offerings := make([]InstanceOffering, 0, len(flavors))
	for _, flavor := range flavors {
		memoryBytes := int64(flavor.RAM) * MiB
		diskBytes := int64(flavor.Disk) * GiB
		
		// Create offering for each region/zone combination this flavor is available in
		for _, region := range getRegionsFromFlavor(flavor) {
			for _, zone := range getZonesFromFlavorAndRegion(flavor, region) {
				// Regular instance
				offerings = append(offerings, InstanceOffering{
					Name:        flavor.Name,
					ID:          flavor.ID,
					CPU:         int64(flavor.VCPUs),
					Memory:      memoryBytes,
					Disk:        diskBytes,
					GPU:         int64(flavor.GPUs),
					Region:      region,
					Zone:        zone,
					PriceHourly: getFlavorPrice(flavor),
					IsSpot:      false,
				})
				
				// Check if spot instances are available for this flavor
				if isSpotAvailable(flavor, region, zone) {
					spotPrice := getSpotFlavorPrice(flavor, region, zone)
					offerings = append(offerings, InstanceOffering{
						Name:        flavor.Name,
						ID:          flavor.ID + "-spot",
						CPU:         int64(flavor.VCPUs),
						Memory:      memoryBytes,
						Disk:        diskBytes,
						GPU:         int64(flavor.GPUs),
						Region:      region,
						Zone:        zone,
						PriceHourly: spotPrice,
						IsSpot:      true,
						SpotPrice:   spotPrice,
					})
				}
			}
		}
	}
	*/
	
	// Export pricing metrics
	p.updateOfferingMetrics(offeringCache.offerings)
	
	// Cache the offerings
	offeringCache.expiresAt = time.Now().Add(CacheTTL)
	
	return offeringCache.offerings, nil
}

// updateOfferingMetrics updates Prometheus metrics for instance offerings
func (p *Provider) updateOfferingMetrics(offerings []InstanceOffering) {
	// Reset metrics
	instanceTypesAvailable.Reset()
	instanceTypePrice.Reset()
	
	// Count offerings by region/zone
	regionZoneCount := make(map[string]map[string]int)
	
	for _, o := range offerings {
		// Initialize region map if needed
		if _, ok := regionZoneCount[o.Region]; !ok {
			regionZoneCount[o.Region] = make(map[string]int)
		}
		
		// Increment count for this region/zone
		regionZoneCount[o.Region][o.Zone]++
		
		// Export price metrics
		spotLabel := "false"
		if o.IsSpot {
			spotLabel = "true"
		}
		
		instanceTypePrice.WithLabelValues(o.Name, spotLabel).Set(o.PriceHourly)
	}
	
	// Update instance type availability metrics
	for region, zones := range regionZoneCount {
		for zone, count := range zones {
			instanceTypesAvailable.WithLabelValues(region, zone).Set(float64(count))
		}
	}
}

// getInstanceOfferingByName returns a specific instance type by name
func (p *Provider) getInstanceOfferingByName(ctx context.Context, name string) (*InstanceOffering, error) {
	offerings, err := p.getAllInstanceOfferings(ctx)
	if err != nil {
		return nil, err
	}

	// Find the offering with the matching name in the current region
	offering, found := lo.Find(offerings, func(o InstanceOffering) bool {
		return o.Name == name && o.Region == p.region && !o.IsSpot
	})

	if !found {
		return nil, fmt.Errorf("instance offering %s not found in region %s", name, p.region)
	}

	return &offering, nil
}

// getSpotInstanceOfferingByName returns a specific spot instance type by name
func (p *Provider) getSpotInstanceOfferingByName(ctx context.Context, name string) (*InstanceOffering, error) {
	offerings, err := p.getAllInstanceOfferings(ctx)
	if err != nil {
		return nil, err
	}

	// Find the spot offering with the matching name in the current region
	offering, found := lo.Find(offerings, func(o InstanceOffering) bool {
		return o.Name == name && o.Region == p.region && o.IsSpot
	})

	if !found {
		return nil, fmt.Errorf("spot instance offering %s not found in region %s", name, p.region)
	}

	return &offering, nil
}

// convertOfferingsToRequirements converts BizFly Cloud offerings to Kubernetes node selector requirements
func (p *Provider) convertOfferingsToRequirements(offerings []InstanceOffering) []corev1.NodeSelectorRequirement {
	// Filter offerings for the current region and only regular instances (non-spot)
	regionOfferings := lo.Filter(offerings, func(o InstanceOffering, _ int) bool {
		return o.Region == p.region && !o.IsSpot
	})
	
	// Extract unique instance types
	instanceTypes := lo.UniqBy(regionOfferings, func(o InstanceOffering) string {
		return o.Name
	})

	// Extract unique zones
	zones := lo.Uniq(lo.Map(regionOfferings, func(o InstanceOffering, _ int) string {
		return o.Zone
	}))

	// Prepare requirements
	requirements := []corev1.NodeSelectorRequirement{
		{
			Key:      NodeLabelInstanceType,
			Operator: corev1.NodeSelectorOpIn,
			Values:   lo.Map(instanceTypes, func(o InstanceOffering, _ int) string { return o.Name }),
		},
		{
			Key:      NodeLabelRegion,
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{p.region},
		},
		{
			Key:      NodeLabelZone,
			Operator: corev1.NodeSelectorOpIn,
			Values:   zones,
		},
	}

	// Add CPU requirements
	cpuValues := lo.Uniq(lo.Map(regionOfferings, func(o InstanceOffering, _ int) string {
		return resource.NewQuantity(o.CPU, resource.DecimalSI).String()
	}))

	if len(cpuValues) > 0 {
		requirements = append(requirements, corev1.NodeSelectorRequirement{
			Key:      "karpenter.bizflycloud.sh/instance-cpu",
			Operator: corev1.NodeSelectorOpIn,
			Values:   cpuValues,
		})
	}

	// Add memory requirements
	memValues := lo.Uniq(lo.Map(regionOfferings, func(o InstanceOffering, _ int) string {
		return resource.NewQuantity(o.Memory, resource.BinarySI).String()
	}))

	if len(memValues) > 0 {
		requirements = append(requirements, corev1.NodeSelectorRequirement{
			Key:      "karpenter.bizflycloud.sh/instance-memory",
			Operator: corev1.NodeSelectorOpIn,
			Values:   memValues,
		})
	}

	// Add GPU requirements if any offerings have GPUs
	gpuOfferings := lo.Filter(regionOfferings, func(o InstanceOffering, _ int) bool {
		return o.GPU > 0
	})

	if len(gpuOfferings) > 0 {
		gpuValues := lo.Uniq(lo.Map(gpuOfferings, func(o InstanceOffering, _ int) string {
			return resource.NewQuantity(o.GPU, resource.DecimalSI).String()
		}))

		requirements = append(requirements, corev1.NodeSelectorRequirement{
			Key:      "karpenter.bizflycloud.sh/instance-gpu",
			Operator: corev1.NodeSelectorOpIn,
			Values:   gpuValues,
		})
		
		// Add GPU instance type selector
		requirements = append(requirements, corev1.NodeSelectorRequirement{
			Key:      "karpenter.bizflycloud.sh/instance-gpu-enabled",
			Operator: corev1.NodeSelectorOpIn,
			Values:   []string{"true"},
		})
	}
	
	// Add spot instance selector
	requirements = append(requirements, corev1.NodeSelectorRequirement{
		Key:      NodeAnnotationIsSpot,
		Operator: corev1.NodeSelectorOpIn,
		Values:   []string{"true", "false"},
	})

	return requirements
}

// findMatchingOffering finds an offering that matches the given CPU, memory, and GPU requirements
func (p *Provider) findMatchingOffering(ctx context.Context, cpuRequest, memoryRequest, gpuRequest int64, zone string, useSpot bool) (*InstanceOffering, error) {
	p.log.Info("Finding matching offering", 
		"cpu", cpuRequest, 
		"memory", memoryRequest, 
		"gpu", gpuRequest,
		"zone", zone,
		"spot", useSpot,
	)

	// Get all offerings
	allOfferings, err := p.getAllInstanceOfferings(ctx)
	if err != nil {
		return nil, err
	}
	
	// Filter offerings by region, zone (if specified), and spot flag
	offerings := lo.Filter(allOfferings, func(o InstanceOffering, _ int) bool {
		regionMatch := o.Region == p.region
		zoneMatch := zone == "" || o.Zone == zone
		spotMatch := o.IsSpot == useSpot
		return regionMatch && zoneMatch && spotMatch
	})
	
	if len(offerings) == 0 {
		return nil, fmt.Errorf("no offerings found in region %s, zone %s, spot %v", p.region, zone, useSpot)
	}

	// Filter offerings that meet the requirements
	var matchingOfferings []InstanceOffering
	for _, o := range offerings {
		cpuOK := o.CPU >= cpuRequest
		memOK := o.Memory >= memoryRequest
		gpuOK := o.GPU >= gpuRequest
		if cpuOK && memOK && gpuOK {
			matchingOfferings = append(matchingOfferings, o)
		}
	}

	if len(matchingOfferings) == 0 {
		return nil, fmt.Errorf(
			"no matching offerings found for cpu=%d, memory=%d, gpu=%d, zone=%s, spot=%v",
			cpuRequest, memoryRequest, gpuRequest, zone, useSpot,
		)
	}

	// Sort by "closest fit" to minimize waste and then by price
	sort.Slice(matchingOfferings, func(i, j int) bool {
		a := matchingOfferings[i]
		b := matchingOfferings[j]

		// Calculate waste score (lower is better)
		// This prioritizes instances that are closest to the requested resources
		aWasteCPU := float64(a.CPU - cpuRequest) / float64(cpuRequest)
		bWasteCPU := float64(b.CPU - cpuRequest) / float64(cpuRequest)
		
		aWasteMem := float64(a.Memory - memoryRequest) / float64(memoryRequest)
		bWasteMem := float64(b.Memory - memoryRequest) / float64(memoryRequest)
		
		// Weight CPU and memory equally
		aWasteScore := aWasteCPU + aWasteMem
		bWasteScore := bWasteCPU + bWasteMem
		
		// If GPU is requested, include GPU waste in the score
		if gpuRequest > 0 {
			aWasteGPU := float64(a.GPU - gpuRequest) / float64(gpuRequest)
			bWasteGPU := float64(b.GPU - gpuRequest) / float64(gpuRequest)
			aWasteScore += aWasteGPU
			bWasteScore += bWasteGPU
		}
		
		// If waste scores are significantly different, use that for sorting
		const wasteTolerance = 0.1
		wasteDiff := math.Abs(aWasteScore - bWasteScore)
		if wasteDiff > wasteTolerance {
			return aWasteScore < bWasteScore
		}
		
		// If waste scores are similar, choose by price
		return a.PriceHourly < b.PriceHourly
	})

	// Return the best match
	bestMatch := matchingOfferings[0]
	p.log.Info("Found matching offering", 
		"name", bestMatch.Name, 
		"cpu", bestMatch.CPU, 
		"memory", bestMatch.Memory, 
		"gpu", bestMatch.GPU,
		"zone", bestMatch.Zone,
		"price", bestMatch.PriceHourly,
	)
	
	// Record the selection
	spotLabel := "false"
	if useSpot {
		spotLabel = "true"
	}
	instanceTypeSelections.WithLabelValues(bestMatch.Name, spotLabel).Inc()

	return &bestMatch, nil
}
