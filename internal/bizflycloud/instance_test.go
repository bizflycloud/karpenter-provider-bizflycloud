package bizflycloud

import (
	"context"
	"testing"
	"time"

	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestCreateInstance(t *testing.T) {
	// Create a provider with test configuration
	client := fake.NewClientBuilder().Build()
	logger := testr.New(t)
	provider, err := NewProvider(client, logger)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Set the region for testing
	provider.region = "HN1"

	// Test creating an instance
	instance, err := provider.createInstance(context.Background(), "test-node", "4c_8g", "ubuntu-20.04", "")
	if err != nil {
		t.Fatalf("Failed to create instance: %v", err)
	}

	// Verify the instance has the expected properties
	if instance.Name != "test-node" {
		t.Errorf("Expected instance name to be 'test-node', got '%s'", instance.Name)
	}
	if instance.Flavor != "4c_8g" {
		t.Errorf("Expected instance flavor to be '4c_8g', got '%s'", instance.Flavor)
	}
	if instance.Region != "HN1" {
		t.Errorf("Expected instance region to be 'HN1', got '%s'", instance.Region)
	}
}

func TestConvertInstanceToNode(t *testing.T) {
	// Create a provider with test configuration
	client := fake.NewClientBuilder().Build()
	logger := testr.New(t)
	provider, err := NewProvider(client, logger)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Create a test instance
	instance := &Instance{
		ID:        "test-id",
		Name:      "test-node",
		Status:    "running",
		Region:    "HN1",
		Zone:      "HN1-zone-1",
		Flavor:    "4c_8g",
		IPAddress: "192.168.1.100",
		CreatedAt: time.Now(),
	}

	// Convert the instance to a node
	node := provider.convertInstanceToNode(instance)

	// Verify the node has the expected properties
	if node.Name != "test-node" {
		t.Errorf("Expected node name to be 'test-node', got '%s'", node.Name)
	}
	if node.Spec.ProviderID != "bizflycloud://test-id" {
		t.Errorf("Expected node provider ID to be 'bizflycloud://test-id', got '%s'", node.Spec.ProviderID)
	}
	if node.Labels["topology.kubernetes.io/region"] != "HN1" {
		t.Errorf("Expected region label to be 'HN1', got '%s'", node.Labels["topology.kubernetes.io/region"])
	}
	if node.Labels["topology.kubernetes.io/zone"] != "HN1-zone-1" {
		t.Errorf("Expected zone label to be 'HN1-zone-1', got '%s'", node.Labels["topology.kubernetes.io/zone"])
	}
	if node.Labels["node.kubernetes.io/instance-type"] != "4c_8g" {
		t.Errorf("Expected instance type label to be '4c_8g', got '%s'", node.Labels["node.kubernetes.io/instance-type"])
	}
}
