package bizflycloud

import (
	"context"
	"testing"

	"github.com/go-logr/logr"
	"github.com/go-logr/logr/testr"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
)

func TestNewProvider(t *testing.T) {
	// Create a fake client
	client := fake.NewClientBuilder().Build()
	logger := testr.New(t)

	// Create a new provider
	provider, err := NewProvider(client, logger)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Check that the provider was created successfully
	if provider == nil {
		t.Fatal("Provider is nil")
	}

	// Check that the provider has the correct client
	if provider.client == nil {
		t.Fatal("Provider client is nil")
	}

	// Check that the provider has the correct logger
	if provider.log == (logr.Logger{}) {
		t.Fatal("Provider logger is empty")
	}
}

func TestProviderNeedLeaderElection(t *testing.T) {
	// Create a fake client
	client := fake.NewClientBuilder().Build()
	logger := testr.New(t)

	// Create a new provider
	provider, err := NewProvider(client, logger)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Check that the provider does not need leader election
	if provider.NeedLeaderElection() {
		t.Fatal("Provider should not need leader election")
	}
}

func TestProviderStart(t *testing.T) {
	// Skip this test when running with -short flag since it takes a long time
	if testing.Short() {
		t.Skip("Skipping test in short mode")
	}

	// Create a fake client
	client := fake.NewClientBuilder().Build()
	logger := testr.New(t)

	// Create a new provider
	provider, err := NewProvider(client, logger)
	if err != nil {
		t.Fatalf("Failed to create provider: %v", err)
	}

	// Start the provider with a timeout context
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// We expect an error because we haven't set up the BizFly client
	err = provider.Start(ctx)
	if err == nil {
		t.Fatal("Expected error when starting provider without BizFly client")
	}
}
