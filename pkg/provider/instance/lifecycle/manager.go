package lifecycle

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	corev1 "k8s.io/api/core/v1"
)

const (
	// Polling constants
	maxServerWaitTime = 30 * time.Minute
	pollInterval      = 15 * time.Second
)

// Manager handles instance lifecycle operations
type Manager struct {
	Client       client.Client
	Log          logr.Logger
	BizflyClient *gobizfly.Client
	Region       string
	Config       *v1bizfly.ProviderConfig
}

// NewManager creates a new instance lifecycle manager
func NewManager(client client.Client, log logr.Logger, bizflyClient *gobizfly.Client, region string, config *v1bizfly.ProviderConfig) *Manager {
	return &Manager{
		Client:       client,
		Log:          log,
		BizflyClient: bizflyClient,
		Region:       region,
		Config:       config,
	}
}

// CreateInstance creates a new BizFly Cloud server instance and waits for it to be ready
func (m *Manager) CreateInstance(ctx context.Context, nodeClaim *karpenterv1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (*gobizfly.Server, error) {
	// Extract instance type
	instanceTypeName := ""
	for _, req := range nodeClaim.Spec.Requirements {
		if req.Key == corev1.LabelInstanceTypeStable && len(req.Values) > 0 {
			instanceTypeName = req.Values[0]
			break
		}
	}

	if instanceTypeName == "" {
		return nil, fmt.Errorf("no instance type found in requirements")
	}

	// Get configuration from NodeClass or use defaults
	imageID := "5a821700-a184-4f91-8455-205d47d472c0"
	if nodeClass != nil && nodeClass.Spec.ImageID != "" {
		imageID = nodeClass.Spec.ImageID
	}

	diskType := "SSD"
	if nodeClass != nil && nodeClass.Spec.DiskType != "" {
		diskType = nodeClass.Spec.DiskType
	}

	rootDiskSize := 40
	if nodeClass != nil && nodeClass.Spec.RootDiskSize > 0 {
		rootDiskSize = nodeClass.Spec.RootDiskSize
	}

	vpcNetworkIDs := []string{"e84362d6-0632-4950-87ac-e7bc7d74be6d"}
	if nodeClass != nil && len(nodeClass.Spec.VPCNetworkIDs) > 0 {
		vpcNetworkIDs = nodeClass.Spec.VPCNetworkIDs
	}

	// Get availability zone
	zone := "HN2"
	for _, req := range nodeClaim.Spec.Requirements {
		if req.Key == corev1.LabelTopologyZone && len(req.Values) > 0 {
			zone = req.Values[0]
			break
		}
	}

	nodeName := nodeClaim.Name

	// Determine server type
	serverType := "premium"
	if strings.Contains(instanceTypeName, "basic") {
		serverType = "basic"
	} else if strings.Contains(instanceTypeName, "enterprise") {
		serverType = "enterprise"
	}

	// Prepare metadata
	metadata := m.buildMetadata(nodeClass, vpcNetworkIDs[0])
	userData := "#!/bin/bash\n/opt/bizfly-kubernetes-engine/bootstrap.sh"

	m.Log.Info("Creating instance",
		"name", nodeName,
		"instanceType", instanceTypeName,
		"imageID", imageID,
		"zone", zone,
		"serverType", serverType)

	// Create server request
	opts := &gobizfly.ServerCreateRequest{
		Name:             nodeName,
		FlavorName:       instanceTypeName,
		Type:             serverType,
		AvailabilityZone: zone,
		VPCNetworkIDs:    vpcNetworkIDs,
		IsCreatedWan:     utils.BoolPtr(true),
		UserData:         userData,
		Metadata:         metadata,
		OS: &gobizfly.ServerOS{
			Type: "image",
			ID:   imageID,
		},
		RootDisk: &gobizfly.ServerDisk{
			Size: rootDiskSize,
			Type: utils.StringPtr(diskType),
		},
		Password:    true,
		Quantity:    1,
		NetworkPlan: "free_datatransfer",
		Firewalls:   []string{},
	}

	// Create the server
	response, err := m.BizflyClient.CloudServer.Create(ctx, opts)
	if err != nil {
		m.Log.Error(err, "Failed to create instance", "name", nodeName)
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	taskID := response.Task[0]
	m.Log.Info("Server creation initiated", "name", nodeName, "taskID", taskID)

	// Wait for server to be created and get the actual server ID
	serverID, err := m.waitForServerCreation(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("failed to wait for server creation: %w", err)
	}

	// Get the actual server details
	server, err := m.BizflyClient.CloudServer.Get(ctx, serverID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server details: %w", err)
	}

	m.Log.Info("Server created successfully",
		"name", nodeName,
		"serverID", server.ID,
		"status", server.Status)

	return server, nil
}

// GetInstance gets a BizFly Cloud server instance by ID
func (m *Manager) GetInstance(ctx context.Context, instanceID string) (*gobizfly.Server, error) {
	m.Log.V(1).Info("Getting instance", "id", instanceID)

	// Get server from BizFly API
	server, err := m.BizflyClient.CloudServer.Get(ctx, instanceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") {
			return nil, fmt.Errorf("instance not found: %s", instanceID)
		}
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	m.Log.V(1).Info("Retrieved instance details",
		"id", server.ID,
		"name", server.Name,
		"status", server.Status,
		"zone", server.AvailabilityZone,
		"flavor", server.Flavor.Name)

	return server, nil
}

// DeleteInstance deletes a BizFly Cloud server instance
func (m *Manager) DeleteInstance(ctx context.Context, instanceID string) error {
	m.Log.Info("Deleting instance", "id", instanceID)

	// First, check if the instance still exists
	_, err := m.BizflyClient.CloudServer.Get(ctx, instanceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "does not exist") ||
			strings.Contains(err.Error(), "404") {
			m.Log.Info("Instance not found, considering it already deleted", "id", instanceID)
			return nil
		}
		return fmt.Errorf("failed to check instance status: %w", err)
	}

	// Instance exists, proceed with deletion
	_, err = m.BizflyClient.CloudServer.Delete(ctx, instanceID, nil)
	if err != nil {
		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "does not exist") ||
			strings.Contains(err.Error(), "404") {
			m.Log.Info("Instance not found during deletion, considering it already deleted", "id", instanceID)
			return nil
		}
		return fmt.Errorf("failed to delete server: %w", err)
	}

	m.Log.Info("Server successfully deleted", "id", instanceID)
	return nil
}

// buildMetadata prepares metadata for server creation
func (m *Manager) buildMetadata(nodeClass *v1bizfly.BizflyCloudNodeClass, networkID string) map[string]string {
	// Get cluster configuration
	clusterID := os.Getenv("BKE_CLUSTER_ID")
	clusterToken := os.Getenv("BKE_CLUSTER_TOKEN")
	joinEndpoint := os.Getenv("BKE_JOIN_ENDPOINT")
	logEndpoint := os.Getenv("BKE_LOG_ENDPOINT")

	if clusterID == "" {
		clusterID = "letbvcssxzusv4eg"
	}
	if clusterToken == "" {
		clusterToken = "KgrtTbXkdCe9awWE6DDV2Rm4ENo6jPTY"
	}
	if joinEndpoint == "" {
		joinEndpoint = fmt.Sprintf("http://engine.api.k8saas.bizflycloud.vn/engine/cluster_join/%s", clusterID)
	}
	if logEndpoint == "" {
		logEndpoint = fmt.Sprintf("http://engine.api.k8saas.bizflycloud.vn/engine/cluster_log/%s", clusterID)
	}

	metadata := map[string]string{
		"bizfly_cloud_service": "kubernetes_engine",
		"bke_cluster_id":       clusterID,
		"bke_cluster_token":    clusterToken,
		"bke_join_endpoint":    joinEndpoint,
		"bke_log_endpoint":     logEndpoint,
		"bke_node_everywhere":  "false",
		"bke_node_localdns":    "false",
		"bke_node_network_id":  networkID,
		"karpenter-managed":    "true",
		"bke_pool_id":          "682c47eb4af5b281d84ca763",
		"cluster_id":           "627cc2e6-0449-4c78-96c3-ea66d7479c19",
	}

	// Add custom metadata from NodeClass
	if nodeClass != nil && len(nodeClass.Spec.Tags) > 0 {
		for _, tag := range nodeClass.Spec.Tags {
			parts := strings.SplitN(tag, "=", 2)
			if len(parts) == 2 {
				metadata[parts[0]] = parts[1]
			}
		}
	}

	return metadata
}

// waitForServerCreation waits for server creation task to complete and returns server ID
func (m *Manager) waitForServerCreation(ctx context.Context, taskID string) (string, error) {
	timeout := time.After(maxServerWaitTime)
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-timeout:
			return "", fmt.Errorf("timeout waiting for server creation, taskID: %s", taskID)
		case <-ticker.C:
			servers, err := m.BizflyClient.CloudServer.List(ctx, &gobizfly.ServerListOptions{})
			if err != nil {
				m.Log.V(1).Info("Error listing servers", "taskID", taskID, "error", err)
				continue
			}

			// Look for a server with our task ID in metadata or created recently
			for _, server := range servers {
				if server.Metadata != nil {
					if karpenterManaged, exists := server.Metadata["karpenter-managed"]; exists && karpenterManaged == "true" {
						// Check if this server was created recently (within last 5 minutes)
						if server.CreatedAt != "" {
							if createdTime, err := time.Parse("2006-01-02T15:04:05Z", server.CreatedAt); err == nil {
								if time.Since(createdTime) < 5*time.Minute {
									m.Log.Info("Found server created by Karpenter",
										"serverID", server.ID,
										"taskID", taskID,
										"createdAt", server.CreatedAt)
									return server.ID, nil
								}
							}
						}
					}
				}
			}

			m.Log.V(1).Info("Server not found yet, continuing to wait", "taskID", taskID)
			continue

		case <-ctx.Done():
			return "", ctx.Err()
		}
	}
}