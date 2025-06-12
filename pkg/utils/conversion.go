package utils

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
)

// ParseFlavorResources extracts CPU and memory from flavor names like "12c_16g_basic"
func ParseFlavorResources(flavorName string) (cpu int, memoryMB int, err error) {
	// Remove "nix." prefix if present
	name := strings.TrimPrefix(flavorName, "nix.")

	// Split by underscore: ["2c", "3g"] or ["12c", "16g", "basic"]
	parts := strings.Split(name, "_")
	if len(parts) < 2 {
		return 0, 0, fmt.Errorf("invalid flavor format: %s", flavorName)
	}

	// Parse CPU (remove 'c' suffix)
	cpuStr := strings.TrimSuffix(parts[0], "c")
	cpu, err = strconv.Atoi(cpuStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid CPU format in flavor %s: %v", flavorName, err)
	}

	// Parse RAM (remove 'g' suffix)
	ramStr := strings.TrimSuffix(parts[1], "g")
	ramGB, err := strconv.Atoi(ramStr)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid RAM format in flavor %s: %v", flavorName, err)
	}

	return cpu, ramGB * 1024, nil
}

// CreateResourceList creates a Kubernetes ResourceList from CPU and memory values
func CreateResourceList(cpuCores int, memoryMB int, maxPods int64) corev1.ResourceList {
	return corev1.ResourceList{
		corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", cpuCores)),
		corev1.ResourceMemory: resource.MustParse(fmt.Sprintf("%dMi", memoryMB)),
		corev1.ResourcePods:   resource.MustParse(fmt.Sprintf("%d", maxPods)),
	}
}

// CalculatePodCapacity calculates realistic pod capacity based on instance size
func CalculatePodCapacity(vcpus int, ramMB int) int64 {
	// Base pod capacity calculation
	var maxPods int64

	// Calculate based on CPU and memory
	cpuBasedPods := int64(vcpus * 8)      // 8 pods per CPU core
	memoryBasedPods := int64(ramMB / 256) // 256MB per pod minimum

	// Take the minimum of the two
	maxPods = cpuBasedPods
	if memoryBasedPods < cpuBasedPods {
		maxPods = memoryBasedPods
	}

	// Apply reasonable limits based on instance size
	switch {
	case vcpus <= 2:
		if maxPods > 20 {
			maxPods = 20
		}
	case vcpus <= 4:
		if maxPods > 40 {
			maxPods = 40
		}
	case vcpus <= 8:
		if maxPods > 80 {
			maxPods = 80
		}
	default:
		if maxPods > 110 {
			maxPods = 110
		}
	}

	// Minimum of 5 pods for any instance
	if maxPods < 5 {
		maxPods = 5
	}

	return maxPods
}

// CategorizeInstanceType categorizes instance types based on naming patterns
func CategorizeInstanceType(instanceType string) string {
	switch {
	case strings.HasPrefix(instanceType, "nix."):
		return "premium"
	case strings.HasSuffix(instanceType, "_basic"):
		return "basic"
	case strings.HasSuffix(instanceType, "_enterprise"):
		return "enterprise"
	case strings.HasSuffix(instanceType, "_dedicated"):
		return "dedicated"
	case strings.Contains(instanceType, "_vps"):
		return "vps"
	default:
		return "premium"
	}
}

// IsLargeInstance determines if an instance is considered "large" for consolidation
func IsLargeInstance(instanceType string) bool {
	cpu, ram, err := ParseFlavorResources(instanceType)
	if err != nil {
		return false
	}

	// Define what constitutes a "large" instance
	return cpu >= 12 || ram >= 16384 // 12+ cores or 16+ GB RAM
}

// ParseTimeWithFormats attempts to parse time with multiple formats
func ParseTimeWithFormats(timeStr string) (time.Time, error) {
	formats := []string{
		"2006-01-02T15:04:05Z",
		"2006-01-02T15:04:05",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}

	for _, format := range formats {
		if parsed, err := time.Parse(format, timeStr); err == nil {
			return parsed, nil
		}
	}

	return time.Time{}, fmt.Errorf("unable to parse time: %s", timeStr)
}

// StringPtr returns a pointer to a string
func StringPtr(s string) *string {
	return &s
}

// BoolPtr returns a pointer to a bool
func BoolPtr(b bool) *bool {
	return &b
}

// IntPtr returns a pointer to an int
func IntPtr(i int) *int {
	return &i
}

// SafeStringDeref safely dereferences a string pointer
func SafeStringDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// SafeIntDeref safely dereferences an int pointer
func SafeIntDeref(i *int) int {
	if i == nil {
		return 0
	}
	return *i
}

// SafeBoolDeref safely dereferences a bool pointer
func SafeBoolDeref(b *bool) bool {
	if b == nil {
		return false
	}
	return *b
}

// ExtractInstanceIDFromProviderID extracts instance ID from provider ID
func ExtractInstanceIDFromProviderID(providerID, prefix string) (string, error) {
	if !strings.HasPrefix(providerID, prefix) {
		return "", fmt.Errorf("invalid provider ID format: %s", providerID)
	}
	return strings.TrimPrefix(providerID, prefix), nil
}

// BuildProviderID builds a provider ID from instance ID
func BuildProviderID(instanceID, prefix string) string {
	return prefix + instanceID
}

// ContainsIgnoreCase checks if a string contains a substring (case-insensitive)
func ContainsIgnoreCase(s, substr string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(substr))
}
