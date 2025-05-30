package instance

import (
    "context"
    "fmt"
    "os"
    "strings"
    "time"

    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
    "k8s.io/apimachinery/pkg/api/resource"
    "sigs.k8s.io/controller-runtime/pkg/client"
    karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
)

// Helper functions for pointers
func stringPtr(s string) *string {
    return &s
}

func boolPtr(b bool) *bool {
    return &b
}

const (
    ProviderIDPrefix = "bizflycloud://"
    NodeLabelInstanceType = "node.kubernetes.io/instance-type"
    NodeLabelRegion = "topology.kubernetes.io/region"
    NodeLabelZone = "topology.kubernetes.io/zone"
    NodeAnnotationIsSpot = "karpenter.bizflycloud.sh/instance-spot"
    NodeCategoryLabel = "karpenter.bizflycloud.com/node-category"
    
    // Polling constants
    maxServerWaitTime = 30 * time.Minute
    pollInterval = 15 * time.Second
)

type Provider struct {
    Client       client.Client
    Log          logr.Logger
    BizflyClient *gobizfly.Client
    Region       string
    Config       *v1bizfly.ProviderConfig
}

type Instance struct {
    ID        string
    Name      string
    Status    string
    Region    string
    Zone      string
    Flavor    string
    IPAddress string
    IsSpot    bool
    Tags      []string
    CreatedAt time.Time
}

// CreateInstance creates a new BizFly Cloud server instance and waits for it to be ready
func (p *Provider) CreateInstance(ctx context.Context, nodeClaim *karpenterv1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (*gobizfly.Server, error) {
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

    // Prepare metadata
    metadata := map[string]string{
        "bizfly_cloud_service":   "kubernetes_engine",
        "bke_cluster_id":         clusterID,
        "bke_cluster_token":      clusterToken,
        "bke_join_endpoint":      joinEndpoint,
        "bke_log_endpoint":       logEndpoint,
        "bke_node_everywhere":    "false",
        "bke_node_localdns":      "false",
        "bke_node_network_id":    vpcNetworkIDs[0],
        "karpenter-managed":      "true",
		"bke_pool_id":            "682c47eb4af5b281d84ca763",
		"cluster_id":             "627cc2e6-0449-4c78-96c3-ea66d7479c19",
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

    userData := "#!/bin/bash\n/opt/bizfly-kubernetes-engine/bootstrap.sh"

    p.Log.Info("Creating instance",
        "name", nodeName,
        "instanceType", instanceTypeName,
        "imageID", imageID,
        "zone", zone,
        "serverType", serverType,
        "clusterID", clusterID)

    // Create server request
    opts := &gobizfly.ServerCreateRequest{
        Name:             nodeName,
        FlavorName:       instanceTypeName,
        Type:             serverType,
        AvailabilityZone: zone,
        VPCNetworkIDs:    vpcNetworkIDs,
        IsCreatedWan:     boolPtr(true),
        UserData:         userData,
        Metadata:         metadata,
        OS: &gobizfly.ServerOS{
            Type: "image",
            ID:   imageID,
        },
        RootDisk: &gobizfly.ServerDisk{
            Size: rootDiskSize,
            Type: stringPtr(diskType),
        },
        Password:    true,
        Quantity:    1,
        NetworkPlan: "free_datatransfer",
        Firewalls:   []string{},
    }

    // Create the server
    response, err := p.BizflyClient.CloudServer.Create(ctx, opts)
    if err != nil {
        p.Log.Error(err, "Failed to create instance", "name", nodeName)
        return nil, fmt.Errorf("failed to create server: %w", err)
    }

    taskID := response.Task[0]
    p.Log.Info("Server creation initiated", "name", nodeName, "taskID", taskID)

    // Wait for server to be created and get the actual server ID
    serverID, err := p.waitForServerCreation(ctx, taskID)
    if err != nil {
        return nil, fmt.Errorf("failed to wait for server creation: %w", err)
    }

    // Get the actual server details
    server, err := p.BizflyClient.CloudServer.Get(ctx, serverID)
    if err != nil {
        return nil, fmt.Errorf("failed to get server details: %w", err)
    }

    p.Log.Info("Server created successfully",
        "name", nodeName,
        "serverID", server.ID,
        "status", server.Status)

    return server, nil
}

// waitForServerCreation waits for server creation task to complete and returns server ID
func (p *Provider) waitForServerCreation(ctx context.Context, taskID string) (string, error) {
    timeout := time.After(maxServerWaitTime)
    ticker := time.NewTicker(pollInterval)
    defer ticker.Stop()

    for {
        select {
        case <-timeout:
            return "", fmt.Errorf("timeout waiting for server creation, taskID: %s", taskID)
        case <-ticker.C:
            // FIXED: Use correct type for server list options
            servers, err := p.BizflyClient.CloudServer.List(ctx, &gobizfly.ServerListOptions{})
            if err != nil {
                p.Log.V(1).Info("Error listing servers", "taskID", taskID, "error", err)
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
                                    p.Log.Info("Found server created by Karpenter", 
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

            // If we can't find the server yet, continue waiting
            p.Log.V(1).Info("Server not found yet, continuing to wait", "taskID", taskID)
            continue
            
        case <-ctx.Done():
            return "", ctx.Err()
        }
    }
}



// GetInstance gets a BizFly Cloud server instance by ID
func (p *Provider) GetInstance(ctx context.Context, instanceID string) (*Instance, error) {
    p.Log.V(1).Info("Getting instance", "id", instanceID)

    // Get server from BizFly API
    server, err := p.BizflyClient.CloudServer.Get(ctx, instanceID)
    if err != nil {
        if strings.Contains(err.Error(), "not found") {
            return nil, fmt.Errorf("instance not found: %s", instanceID)
        }
        return nil, fmt.Errorf("failed to get server: %w", err)
    }

    // Convert to Instance
    instance := p.ConvertGobizflyServerToInstance(server)
    if instance == nil {
        return nil, fmt.Errorf("failed to convert server to instance")
    }

    p.Log.V(1).Info("Retrieved instance details",
        "id", instance.ID,
        "name", instance.Name,
        "status", instance.Status,
        "zone", instance.Zone,
        "flavor", instance.Flavor)

    return instance, nil
}

// DeleteInstance deletes a BizFly Cloud server instance
func (p *Provider) DeleteInstance(ctx context.Context, instanceID string) error {
    p.Log.Info("Deleting instance", "id", instanceID)

    // First, check if the instance still exists
    _, err := p.BizflyClient.CloudServer.Get(ctx, instanceID)
    if err != nil {
        if strings.Contains(err.Error(), "not found") || 
           strings.Contains(err.Error(), "does not exist") ||
           strings.Contains(err.Error(), "404") {
            p.Log.Info("Instance not found, considering it already deleted", "id", instanceID)
            return nil // IMPORTANT: Return nil, not an error
        }
        // For other errors, return them
        return fmt.Errorf("failed to check instance status: %w", err)
    }

    // Instance exists, proceed with deletion
    _, err = p.BizflyClient.CloudServer.Delete(ctx, instanceID, nil)
    if err != nil {
        if strings.Contains(err.Error(), "not found") || 
           strings.Contains(err.Error(), "does not exist") ||
           strings.Contains(err.Error(), "404") {
            p.Log.Info("Instance not found during deletion, considering it already deleted", "id", instanceID)
            return nil // IMPORTANT: Return nil, not an error
        }
        return fmt.Errorf("failed to delete server: %w", err)
    }

    p.Log.Info("Server successfully deleted", "id", instanceID)
    return nil
}


// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func (p *Provider) ConvertToNode(instance *Instance, isSpot bool) *corev1.Node {
    capacity := p.getCapacityFromFlavor(instance.Flavor)
    category := p.categorizeFlavor(instance.Flavor)
    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: instance.Name,
            Labels: map[string]string{
                corev1.LabelInstanceTypeStable:  instance.Flavor,
                corev1.LabelArchStable:          "amd64",
                "karpenter.sh/capacity-type":    "on-demand",
                NodeLabelRegion:                 instance.Region,
                NodeLabelZone:                   instance.Zone,
                NodeCategoryLabel:               category, // Add category label
            },
            Annotations: map[string]string{
                "karpenter.bizflycloud.sh/instance-id": instance.ID,
            },
        },
        Spec: corev1.NodeSpec{
            ProviderID: fmt.Sprintf("%s%s", ProviderIDPrefix, instance.ID),
            Taints: []corev1.Taint{
                {
                    Key:    "node.cloudprovider.kubernetes.io/uninitialized",
                    Value:  "true",
                    Effect: corev1.TaintEffectNoSchedule,
                },
                {
                    Key:    "karpenter.sh/unregistered",
                    Value:  "true",
                    Effect: corev1.TaintEffectNoSchedule,
                },
            },
        },
        Status: corev1.NodeStatus{
            Capacity:    capacity,
            Allocatable: capacity,
            Phase:       corev1.NodePending,
            Conditions: []corev1.NodeCondition{
                {
                    Type:    corev1.NodeReady,
                    Status:  corev1.ConditionFalse,
                    Reason:  "KubeletNotReady",
                    Message: "kubelet is starting",
                },
            },
        },
    }

    if isSpot || instance.IsSpot {
        node.Annotations[NodeAnnotationIsSpot] = "true"
    }

    return node
}


// getCapacityFromFlavor parses flavor name to extract CPU and memory capacity
func (p *Provider) getCapacityFromFlavor(flavorName string) corev1.ResourceList {
    // Default capacity
    capacity := corev1.ResourceList{
        corev1.ResourceCPU:    resource.MustParse("2"),
        corev1.ResourceMemory: resource.MustParse("4Gi"),
        corev1.ResourcePods:   resource.MustParse("110"),
    }

    // Parse flavor name (e.g., "12c_12g_basic" -> 12 CPU, 12GB RAM)
    parts := strings.Split(flavorName, "_")
    if len(parts) >= 2 {
        // Extract CPU
        if strings.HasSuffix(parts[0], "c") {
            cpuStr := strings.TrimSuffix(parts[0], "c")
            if cpu, err := resource.ParseQuantity(cpuStr); err == nil {
                capacity[corev1.ResourceCPU] = cpu
            }
        }
        
        // Extract Memory
        if strings.HasSuffix(parts[1], "g") {
            memStr := strings.TrimSuffix(parts[1], "g") + "Gi"
            if mem, err := resource.ParseQuantity(memStr); err == nil {
                capacity[corev1.ResourceMemory] = mem
            }
        }
    }

    return capacity
}

// ConvertGobizflyServerToInstance converts gobizfly.Server to Instance
func (p *Provider) ConvertGobizflyServerToInstance(server *gobizfly.Server) *Instance {
    if server == nil {
        return nil
    }

    zone := server.AvailabilityZone
    if zone == "" {
        zone = fmt.Sprintf("%s-a", p.Region)
    }

    ipAddress := ""
    if len(server.IPAddresses.WanV4Addresses) > 0 {
        ipAddress = server.IPAddresses.WanV4Addresses[0].Address
    } else if len(server.IPAddresses.LanAddresses) > 0 {
        ipAddress = server.IPAddresses.LanAddresses[0].Address
    }

    var createdAt time.Time
    if server.CreatedAt != "" {
        formats := []string{
            "2006-01-02T15:04:05Z",
            "2006-01-02T15:04:05",
            "2006-01-02 15:04:05",
            time.RFC3339,
        }
        
        for _, format := range formats {
            if parsed, err := time.Parse(format, server.CreatedAt); err == nil {
                createdAt = parsed
                break
            }
        }
        
        if createdAt.IsZero() {
            createdAt = time.Now()
        }
    } else {
        createdAt = time.Now()
    }

    var tags []string
    for key, value := range server.Metadata {
        tags = append(tags, fmt.Sprintf("%s=%s", key, value))
    }

    return &Instance{
        ID:        server.ID,
        Name:      server.Name,
        Status:    server.Status,
        Region:    p.Region,
        Zone:      zone,
        Flavor:    server.Flavor.Name,
        IPAddress: ipAddress,
        IsSpot:    false,
        Tags:      tags,
        CreatedAt: createdAt,
    }
}

// Add this method to instance.go
func (p *Provider) categorizeFlavor(flavorName string) string {
    switch {
    case strings.HasPrefix(flavorName, "nix."):
        return "premium"
    case strings.HasSuffix(flavorName, "_basic"):
        return "basic"
    case strings.HasSuffix(flavorName, "_enterprise"):
        return "enterprise"
    case strings.HasSuffix(flavorName, "_dedicated"):
        return "dedicated"
    default:
        return "premium"
    }
}