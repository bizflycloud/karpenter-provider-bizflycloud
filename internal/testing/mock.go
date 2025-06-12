package testing

import (
	"context"
	"fmt"

	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/cloudprovider/bizflycloud"
	"github.com/go-logr/logr"
	v1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"sigs.k8s.io/karpenter/pkg/cloudprovider"
)

// MockCloudProvider provides a mock implementation for testing
type MockCloudProvider struct {
	instances      []bizflycloud.InstanceInfo
	instanceTypes  []*cloudprovider.InstanceType
	supportedZones []string
}

// NewMockCloudProvider creates a new mock cloud provider
func NewMockCloudProvider() *MockCloudProvider {
	return &MockCloudProvider{
		instances:      []bizflycloud.InstanceInfo{},
		instanceTypes:  []*cloudprovider.InstanceType{},
		supportedZones: []string{"HN1", "HN2"},
	}
}

// MockInstanceProvider provides a mock implementation for testing
type MockInstanceProvider struct {
	instances map[string]*MockInstance
}

// NewMockInstanceProvider creates a new mock instance provider
func NewMockInstanceProvider() *MockInstanceProvider {
	return &MockInstanceProvider{
		instances: make(map[string]*MockInstance),
	}
}

// MockInstance implements the InstanceInfo interface for testing
type MockInstance struct {
	id           string
	name         string
	status       string
	region       string
	zone         string
	instanceType string
	ipAddress    string
	isSpot       bool
	tags         map[string]string
	createdAt    string
}

func (m *MockInstance) GetID() string              { return m.id }
func (m *MockInstance) GetName() string            { return m.name }
func (m *MockInstance) GetStatus() string          { return m.status }
func (m *MockInstance) GetRegion() string          { return m.region }
func (m *MockInstance) GetZone() string            { return m.zone }
func (m *MockInstance) GetInstanceType() string    { return m.instanceType }
func (m *MockInstance) GetIPAddress() string       { return m.ipAddress }
func (m *MockInstance) IsSpot() bool               { return m.isSpot }
func (m *MockInstance) GetTags() map[string]string { return m.tags }
func (m *MockInstance) GetCreatedAt() string       { return m.createdAt }

// CreateInstance creates a mock instance
func (m *MockInstanceProvider) CreateInstance(ctx context.Context, nodeClaim *v1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (bizflycloud.InstanceInfo, error) {
	instance := &MockInstance{
		id:           "mock-instance-" + nodeClaim.Name,
		name:         nodeClaim.Name,
		status:       "ACTIVE",
		region:       "HN",
		zone:         "HN1",
		instanceType: "2c_4g_basic",
		ipAddress:    "192.168.1.100",
		isSpot:       false,
		tags:         map[string]string{"karpenter-managed": "true"},
		createdAt:    "2024-01-01T00:00:00Z",
	}

	m.instances[instance.id] = instance
	return instance, nil
}

// GetInstance retrieves a mock instance
func (m *MockInstanceProvider) GetInstance(ctx context.Context, instanceID string) (bizflycloud.InstanceInfo, error) {
	if instance, exists := m.instances[instanceID]; exists {
		return instance, nil
	}
	return nil, fmt.Errorf("instance not found")
}

// DeleteInstance deletes a mock instance
func (m *MockInstanceProvider) DeleteInstance(ctx context.Context, instanceID string) error {
	delete(m.instances, instanceID)
	return nil
}

// ListInstances lists all mock instances
func (m *MockInstanceProvider) ListInstances(ctx context.Context) ([]bizflycloud.InstanceInfo, error) {
	var instances []bizflycloud.InstanceInfo
	for _, instance := range m.instances {
		instances = append(instances, instance)
	}
	return instances, nil
}

// MockInstanceTypeProvider provides mock instance types for testing
type MockInstanceTypeProvider struct {
	instanceTypes []*cloudprovider.InstanceType
}

// NewMockInstanceTypeProvider creates a new mock instance type provider
func NewMockInstanceTypeProvider() *MockInstanceTypeProvider {
	return &MockInstanceTypeProvider{
		instanceTypes: []*cloudprovider.InstanceType{},
	}
}

// List returns mock instance types
func (m *MockInstanceTypeProvider) List(ctx context.Context, nodeClass *v1bizfly.BizflyCloudNodeClass) ([]*cloudprovider.InstanceType, error) {
	return m.instanceTypes, nil
}

// GetSupportedInstanceTypes returns supported instance types
func (m *MockInstanceTypeProvider) GetSupportedInstanceTypes(ctx context.Context) ([]string, error) {
	return []string{"2c_4g_basic", "4c_8g_basic", "8c_16g_premium"}, nil
}

// NewMockLogger creates a mock logger for testing
func NewMockLogger() logr.Logger {
	return logr.Discard()
}
