package lifecycle

import (

	"sort"
	"strconv"
	"strings"

	"context"
	"fmt"
	"os"
	"time"

    "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"


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
	kubernetesVersion string
	versionFetched    bool
}

type InstanceTypeSpec struct {
	Name string
	CPU  int
	RAM  int
	Tier string
}

const (
	NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
	DiskTypeLabel     = "karpenter.bizflycloud.com/disk-type"
	NetworkPlanLabel  = "karpenter.bizflycloud.com/network-plan"
)

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
	instanceTypeName, err := m.getSmallestInstanceType(nodeClaim)
	if err != nil {
		return nil, fmt.Errorf("failed to select instance type: %w", err)
	}

	// Get configuration from NodeClass or use defaults
	imageID, err := m.resolveImageID(nodeClaim, nodeClass)
	if err != nil {
		return nil, fmt.Errorf("failed to resolve image ID: %w", err)
	}

    // Debug: Log all requirements
    m.Log.Info("NodeClaim requirements debug:")
    for i, req := range nodeClaim.Spec.Requirements {
        m.Log.Info("Requirement", "index", i, "key", req.Key, "operator", req.Operator, "values", req.Values)
    }

	// Get disk type from requirements
    diskType, ok := getValueFromRequirements(nodeClaim.Spec.Requirements, DiskTypeLabel)
    if !ok {
        // Fallback: try to find any disk-type related requirement
        for _, req := range nodeClaim.Spec.Requirements {
            if strings.Contains(req.Key, "disk-type") {
                if len(req.Values) > 0 {
                    diskType = req.Values[0]
                    ok = true
                    m.Log.Info("Found disk type with fallback search", "key", req.Key, "value", diskType)
                    break
                }
            }
        }
        
        if !ok {
            return nil, fmt.Errorf("disk type not found in node claim requirements, ensure '%s' is specified in NodePool. Available keys: %v", 
                DiskTypeLabel, m.getRequirementKeys(nodeClaim.Spec.Requirements))
        }
    }

	rootDiskSize := nodeClass.Spec.RootDiskSize
	vpcNetworkIDs := nodeClass.Spec.VPCNetworkIDs

	// Get availability zone
	selectedZone, ok := getValueFromRequirements(nodeClaim.Spec.Requirements, corev1.LabelTopologyZone)
	if !ok {
		return nil, fmt.Errorf("zone not found in node claim requirements, ensure 'topology.kubernetes.io/zone' is specified in NodePool")
	}


	nodeName := nodeClaim.Name

	// Determine server type
	// FIX: Use the plural 'NodeCategories' field and select the first element.
	if len(nodeClass.Spec.NodeCategories) == 0 {
		return nil, fmt.Errorf("nodeCategories must be specified in NodeClass")
	}
	// Get server type (category) from requirements
	serverType, ok := getValueFromRequirements(nodeClaim.Spec.Requirements, NodeCategoryLabel)
	if !ok {
		return nil, fmt.Errorf("node category not found in node claim requirements, ensure '%s' is specified in NodePool", NodeCategoryLabel)
	}

	networkPlan, ok := getValueFromRequirements(nodeClaim.Spec.Requirements, NetworkPlanLabel)
    if !ok {
        // Fallback to the first network plan in the NodeClass if not specified in requirements
        if len(nodeClass.Spec.NetworkPlans) > 0 {
            networkPlan = nodeClass.Spec.NetworkPlans[0]
            m.Log.Info("Network plan not in requirements, falling back to NodeClass default", "plan", networkPlan)
        } else {
            return nil, fmt.Errorf("network plan not found in requirements and none specified in NodeClass")
        }
    }
	// Prepare metadata
	metadata := m.buildMetadata(nodeClass, vpcNetworkIDs[0])
	userData := "#!/bin/bash\n/opt/bizfly-kubernetes-engine/bootstrap.sh"

    var sshKey string
    sshKey = nodeClass.Spec.SSHKeyName
    
	m.Log.Info("Creating instance",
		"name", nodeName,
		"instanceType", instanceTypeName,
		"imageID", imageID,
		"zone", selectedZone,
		"serverType", serverType,
		"vpcNetworkIDs", vpcNetworkIDs,
		"sshKey", sshKey)

	// Create server request
	opts := &gobizfly.ServerCreateRequest{
		Name:             nodeName,
		FlavorName:       instanceTypeName,
		Type:             serverType,
		AvailabilityZone: selectedZone,
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
        SSHKey:      sshKey,
		Password:    true,
		Quantity:    1,
		NetworkPlan: networkPlan,
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
	server, err := m.BizflyClient.CloudServer.Get(ctx, instanceID)
	if err != nil {
		if strings.Contains(err.Error(), "not found") ||
			strings.Contains(err.Error(), "does not exist") ||
			strings.Contains(err.Error(), "404") {
			m.Log.Info("Instance not found, considering it already deleted", "id", instanceID)
			return nil
		}
		return fmt.Errorf("failed to check instance status: %w", err)
	}

	nodeName := server.Name
	m.Log.Info("Retrieved node name from server details", "nodeName", nodeName, "instanceID", instanceID)
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

	clusterID := os.Getenv("BKE_CLUSTER_ID")
	token := os.Getenv("BKE_CLUSTER_TOKEN")

	if clusterID != "" {
		m.Log.Info("Performing BKE cluster cleanup for node", "nodeName", nodeName, "clusterID", clusterID)

		// The request struct is `ClusterLeaveRequest` with a `NodeName` field, as per the function signature provided.
		leaveReq := &gobizfly.ClusterLeaveRequest{
			NodeName: nodeName,
		}

		// The correct method is `ClusterLeave`, and it takes the token as a separate parameter.
		// The second `err` uses `=` instead of `:=` because `err` is already declared in this scope.
		_, err = m.BizflyClient.KubernetesEngine.ClusterLeave(ctx, clusterID, token, leaveReq)
		if err != nil {
			// The `logr.Logger` interface does not have a `Warn` method[2].
			// We log this as `Info` because it's a non-fatal error; the primary resource is deleted.
			m.Log.Info("Failed to remove node from BKE cluster (non-fatal, manual cleanup may be required)", "error", err.Error(), "nodeName", nodeName)
		} else {
			m.Log.Info("Successfully removed node from BKE cluster", "nodeName", nodeName)
		}
	} else {
		m.Log.Info("BKE_CLUSTER_ID not set, skipping cluster cleanup.", "instanceID", instanceID)
	}

	m.Log.Info("Instance deletion process complete", "id", instanceID)
	return nil
}

// buildMetadata prepares metadata for server creation
func (m *Manager) buildMetadata(nodeClass *v1bizfly.BizflyCloudNodeClass, networkID string) map[string]string {
	// Get cluster configuration
	clusterID := os.Getenv("BKE_CLUSTER_ID")
	clusterToken := os.Getenv("BKE_CLUSTER_TOKEN")
	joinEndpoint := os.Getenv("BKE_JOIN_ENDPOINT")
	logEndpoint := os.Getenv("BKE_LOG_ENDPOINT")
	kubernetesVersion := os.Getenv("BKE_KUBERNETES_VERSION")

	metadata := map[string]string{
		"bizfly_cloud_service": "kubernetes_engine",
		"bke_cluster_id":       clusterID,
		"bke_cluster_token":    clusterToken,
		"bke_join_endpoint":    joinEndpoint,
		"bke_log_endpoint":     logEndpoint,
		"kubernetes_version": kubernetesVersion,
		"bke_node_everywhere":  "false",
		"bke_node_localdns":    "false",
		"bke_node_network_id":  networkID,
		"karpenter-managed":    "true",
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

// resolveImageID determines the image ID to use based on Kubernetes version or explicit configuration
func (m *Manager) resolveImageID(nodeClaim *karpenterv1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (string, error) {
	// If explicit imageID is provided in NodeClass, use it
	if nodeClass != nil && nodeClass.Spec.ImageID != "" {
		m.Log.Info("Using explicit image ID from NodeClass", "imageID", nodeClass.Spec.ImageID)
		return nodeClass.Spec.ImageID, nil
	}

	// If imageMapping is provided, try to resolve from Kubernetes version
	if nodeClass != nil && len(nodeClass.Spec.ImageMapping) > 0 {
		// Get Kubernetes version from NodeClaim requirements
		kubeVersion := m.getKubernetesVersion(nodeClaim)
		if kubeVersion == "" {
			return "", fmt.Errorf("kubernetes version not found in NodeClaim requirements, cannot resolve image from mapping")
		}
		
		if imageID, exists := nodeClass.Spec.ImageMapping[kubeVersion]; exists {
			m.Log.Info("Resolved image ID from version mapping", 
				"kubeVersion", kubeVersion, 
				"imageID", imageID)
			return imageID, nil
		}
		return "", fmt.Errorf("no image mapping found for Kubernetes version %s", kubeVersion)
	}

	// No imageID or imageMapping provided - return error
	return "", fmt.Errorf("either imageId or imageMapping must be specified in NodeClass")
}


func (m *Manager) getKubernetesVersion(nodeClaim *karpenterv1.NodeClaim) string {
	if !m.versionFetched {
		version, err := m.getKubernetesVersionFromAPIServer()
		if err != nil {
			m.Log.Error(err, "Failed to get Kubernetes version from API server")
			return ""
		}
		m.kubernetesVersion = version
		m.versionFetched = true
		m.Log.Info("Cached Kubernetes version", "version", version)
	}
	return m.kubernetesVersion
}

func (m *Manager) getKubernetesVersionFromAPIServer() (string, error) {
	config, err := rest.InClusterConfig()
	if err != nil {
		return "", fmt.Errorf("failed to get in-cluster config: %w", err)
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return "", fmt.Errorf("failed to create clientset: %w", err)
	}

	versionInfo, err := clientset.Discovery().ServerVersion()
	if err != nil {
		return "", fmt.Errorf("failed to get server version: %w", err)
	}

	return versionInfo.GitVersion, nil
}


func (m *Manager) getSmallestInstanceType(nodeClaim *karpenterv1.NodeClaim) (string, error) {
	var availableInstanceTypes []string
	
	// Extract all available instance types from requirements
	for _, req := range nodeClaim.Spec.Requirements {
		if req.Key == corev1.LabelInstanceTypeStable {
			m.Log.Info("Values get from label", "values", req.Values)
			availableInstanceTypes = append(availableInstanceTypes, req.Values...)
		}
	}

	// Log the raw list of available instance types
	m.Log.Info("Available instance types from requirements",
		"count", len(availableInstanceTypes),
		"instanceTypes", availableInstanceTypes)

	if len(availableInstanceTypes) == 0 {
		return "", fmt.Errorf("no instance types found in requirements")
	}

	// Parse and sort instance types
	parsedInstances := make([]InstanceTypeSpec, 0, len(availableInstanceTypes))
	var parseErrors []string
	
	for _, instanceType := range availableInstanceTypes {
		spec, err := m.parseInstanceType(instanceType)
		if err != nil {
			m.Log.Error(err, "Failed to parse instance type", "instanceType", instanceType)
			parseErrors = append(parseErrors, fmt.Sprintf("%s: %v", instanceType, err))
			continue
		}
		parsedInstances = append(parsedInstances, spec)
	}

	// Log parsing results
	if len(parseErrors) > 0 {
		m.Log.Info("Instance type parsing errors",
			"errorCount", len(parseErrors),
			"errors", parseErrors)
	}

	// Log all successfully parsed instances with details
	m.Log.Info("Successfully parsed instance types",
		"totalCount", len(parsedInstances),
		"validCount", len(parsedInstances))
	
	for i, spec := range parsedInstances {
		m.Log.V(1).Info("Parsed instance details",
			"index", i,
			"name", spec.Name,
			"cpu", spec.CPU,
			"ram", spec.RAM,
			"tier", spec.Tier) // Include tier if you have it
	}

	if len(parsedInstances) == 0 {
		return "", fmt.Errorf("no valid instance types found after parsing")
	}

	// Sort by CPU first, then by RAM (ascending order)
	sort.Slice(parsedInstances, func(i, j int) bool {
		if parsedInstances[i].CPU == parsedInstances[j].CPU {
			return parsedInstances[i].RAM < parsedInstances[j].RAM
		}
		return parsedInstances[i].CPU < parsedInstances[j].CPU
	})

	// Log sorted instances to see the ordering
	m.Log.Info("Sorted instance types (smallest to largest)")
	for i, spec := range parsedInstances {
		m.Log.V(1).Info("Sorted instance",
			"rank", i+1,
			"name", spec.Name,
			"cpu", spec.CPU,
			"ram", spec.RAM)
	}

	smallestInstance := parsedInstances[0]
	
	// Enhanced logging for the selected smallest instance
	m.Log.Info("Selected smallest instance type",
		"instanceType", smallestInstance.Name,
		"cpu", smallestInstance.CPU,
		"ram", smallestInstance.RAM,
		"totalCandidates", len(parsedInstances),
		"selectionCriteria", "lowest CPU, then lowest RAM")

	// Log comparison with next smallest for context
	if len(parsedInstances) > 1 {
		nextSmallest := parsedInstances[1]
		m.Log.V(1).Info("Next smallest instance for comparison",
			"instanceType", nextSmallest.Name,
			"cpu", nextSmallest.CPU,
			"ram", nextSmallest.RAM)
	}

	return smallestInstance.Name, nil
}


// parseInstanceType parses instance type string format and extracts tier information
func (m *Manager) parseInstanceType(instanceType string) (InstanceTypeSpec, error) {
	var cpu, ram int
	var tier string
	var err error

	// Handle different formats
	if strings.HasPrefix(instanceType, "nix.") {
		// Format: nix.2c_2g => premium
		tier = "premium"
		// Remove "nix." prefix and parse the rest
		remaining := strings.TrimPrefix(instanceType, "nix.")
		cpu, ram, err = m.parseCpuRam(remaining)
		if err != nil {
			return InstanceTypeSpec{}, fmt.Errorf("failed to parse nix format %s: %w", instanceType, err)
		}
	} else {
		// Standard formats: 2c_2g_basic, 2c_2g_enterprise, 2c_2g_dedicated
		parts := strings.Split(instanceType, "_")
		if len(parts) < 3 {
			return InstanceTypeSpec{}, fmt.Errorf("invalid instance type format: %s", instanceType)
		}

		// Parse CPU and RAM from first two parts
		cpu, ram, err = m.parseCpuRam(strings.Join(parts[:2], "_"))
		if err != nil {
			return InstanceTypeSpec{}, fmt.Errorf("failed to parse CPU/RAM from %s: %w", instanceType, err)
		}

		// Extract tier from the last part
		tier = parts[len(parts)-1]
	}

	return InstanceTypeSpec{
		Name: instanceType,
		CPU:  cpu,
		RAM:  ram,
		Tier: tier, // Add this field to your struct
	}, nil
}

func (m *Manager) parseCpuRam(cpuRamStr string) (int, int, error) {
	parts := strings.Split(cpuRamStr, "_")
	if len(parts) != 2 {
		return 0, 0, fmt.Errorf("invalid CPU/RAM format: %s", cpuRamStr)
	}

	// Parse CPU (remove 'c' suffix)
	cpuStr := strings.TrimSuffix(parts[0], "c")
	cpu, err := strconv.Atoi(cpuStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse CPU from %s: %w", parts[0], err)
	}

	// Parse RAM (remove 'g' suffix)
	ramStr := strings.TrimSuffix(parts[1], "g")
	ram, err := strconv.Atoi(ramStr)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to parse RAM from %s: %w", parts[1], err)
	}

	return cpu, ram, nil
}

func getValueFromRequirements(reqs []karpenterv1.NodeSelectorRequirementWithMinValues, key string) (string, bool) {
	for _, req := range reqs {
		if req.Key == key {
			if len(req.Values) > 0 {
				// Return the first value found for the requirement
				return req.Values[0], true
			}
		}
	}
	return "", false
}

func (m *Manager) getRequirementKeys(reqs []karpenterv1.NodeSelectorRequirementWithMinValues) []string {
    keys := make([]string, len(reqs))
    for i, req := range reqs {
        keys[i] = req.Key
    }
    return keys
}