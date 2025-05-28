package instance

import (
    "context"
    // "encoding/base64"
    "fmt"
    "os"
    "strings"
    "time"

    "github.com/bizflycloud/gobizfly"
    "github.com/go-logr/logr"
    v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
    corev1 "k8s.io/api/core/v1"
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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

func extractTagsFromMetadata(metadata map[string]string) []string {
    var tags []string
    for key, value := range metadata {
        tags = append(tags, fmt.Sprintf("%s=%s", key, value))
    }
    return tags
}


const (
	// ProviderID prefix for BizFly Cloud instances
	ProviderIDPrefix = "bizflycloud://"

	// NodeLabelInstanceType is the label for instance type
	NodeLabelInstanceType = "node.kubernetes.io/instance-type"

	// NodeLabelRegion is the label for region
	NodeLabelRegion = "topology.kubernetes.io/region"

	// NodeLabelZone is the label for zone
	NodeLabelZone = "topology.kubernetes.io/zone"

	// NodeAnnotationSpot is the annotation for spot instances
	NodeAnnotationIsSpot = "karpenter.bizflycloud.sh/instance-spot"
)

type Provider struct {
    Client       client.Client
    Log          logr.Logger
    BizflyClient *gobizfly.Client
    Region       string
    Config       *v1bizfly.ProviderConfig
}
// Instance represents a BizFly Cloud server instance
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

// Default values for instance creation
const (
	defaultRootDiskSize = 20 // GB
	defaultSSHKey       = "" // No SSH key by default
	defaultImageName    = "ubuntu-20.04"
	maxServerWaitTime   = 10 * time.Minute
	pollInterval        = 10 * time.Second
)

// CreateInstance creates a new BizFly Cloud server instance
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

    // Use default image ID for now
    imageID := "61304d7b-f5fc-42ac-bbcd-50d73df967da"

    // Get availability zone
    zone := "HN2" // Default
    for _, req := range nodeClaim.Spec.Requirements {
        if req.Key == corev1.LabelTopologyZone && len(req.Values) > 0 {
            zone = req.Values[0]
            break
        }
    }

    // Generate node name
    nodeName := nodeClaim.Name

    // Determine server type and disk configuration
    serverType := "premium"
    diskType := "SSD"
    rootDiskSize := 40

    if strings.Contains(instanceTypeName, "basic") {
        serverType = "basic"
        diskType = "HDD"
    } else if strings.Contains(instanceTypeName, "enterprise") {
        serverType = "enterprise"
        diskType = "SSD"
    }

    p.Log.Info("Creating instance",
        "name", nodeName,
        "instanceType", instanceTypeName,
        "imageID", imageID,
        "zone", zone,
        "serverType", serverType)

    // Create server options with all required fields
    opts := &gobizfly.ServerCreateRequest{
        Name:             nodeName,
        FlavorName:       instanceTypeName,
        Type:             serverType,
        AvailabilityZone: zone,
        VPCNetworkIDs:    []string{"e84362d6-0632-4950-87ac-e7bc7d74be6d"},
        IsCreatedWan:     boolPtr(true),
        OS: &gobizfly.ServerOS{
            Type: "image",
            ID:   imageID,
        },
        RootDisk: &gobizfly.ServerDisk{
            Size: rootDiskSize,
            Type: stringPtr(diskType),
        },
        Password:    true,
        Quantity:    1, // FIXED: Use int instead of *bool
        NetworkPlan: "free_datatransfer",
        Firewalls:   []string{},
    }

    // Create the server
    response, err := p.BizflyClient.CloudServer.Create(ctx, opts)
    if err != nil {
        p.Log.Error(err, "Failed to create instance",
            "name", nodeName,
            "instanceType", instanceTypeName,
            "imageID", imageID,
            "request", opts)
        return nil, fmt.Errorf("failed to create server: %w", err)
    }

    p.Log.Info("Server creation initiated",
        "name", nodeName,
        "taskID", response.Task[0])

    // TODO: Implement polling logic to wait for server creation
    return nil, nil
}

// DeleteInstance deletes a BizFly Cloud server instance
func (p *Provider) DeleteInstance(ctx context.Context, instanceID string) error {
	p.Log.Info("Deleting instance", "id", instanceID)

	// If in mock mode, just return success
	if p.BizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.Log.Info("Using mock instance deletion")
		p.Log.Info("Server successfully deleted", "id", instanceID)
		return nil
	}

	// Delete the server via the API
	_, err := p.BizflyClient.CloudServer.Delete(ctx, instanceID, nil)
	if err != nil {
		// Check if error is because instance doesn't exist
		if strings.Contains(err.Error(), "not found") {
			p.Log.Info("Instance not found, considering it already deleted", "id", instanceID)
			return nil
		}
		return fmt.Errorf("failed to delete server: %w", err)
	}

	p.Log.Info("Server successfully deleted", "id", instanceID)
	return nil
}

// GetInstance gets a BizFly Cloud server instance by ID
func (p *Provider) GetInstance(ctx context.Context, instanceID string) (*Instance, error) {
	p.Log.Info("Getting instance", "id", instanceID)

	// If in mock mode, return a mock instance
	if p.BizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.Log.Info("Using mock instance retrieval")

		// Simulate a "not found" error for instances with "notfound" in the ID
		if strings.Contains(instanceID, "notfound") {
			return nil, fmt.Errorf("instance not found: %s", instanceID)
		}

		// Simulate an "error" state for instances with "error" in the ID
		status := "ACTIVE"
		if strings.Contains(instanceID, "error") {
			status = "ERROR"
		}

		// Create a mock server instance
		instance := &Instance{
			ID:        instanceID,
			Name:      fmt.Sprintf("node-%s", instanceID),
			Status:    status,
			Region:    p.Region,
			Zone:      fmt.Sprintf("%s-a", p.Region),
			Flavor:    "4c_8g",
			IPAddress: "192.168.1.100",
			IsSpot:    strings.Contains(instanceID, "spot"),
			Tags:      []string{"karpenter-node=node-" + instanceID},
			CreatedAt: time.Now(),
		}

		return instance, nil
	}

	// For mock, just return a default instance
	instance := &Instance{
		ID:        instanceID,
		Name:      fmt.Sprintf("node-%s", instanceID),
		Status:    "ACTIVE",
		Region:    p.Region,
		Zone:      fmt.Sprintf("%s-a", p.Region),
		Flavor:    "4c_8g",
		IPAddress: "192.168.1.100",
		IsSpot:    false,
		Tags:      []string{"karpenter-node=node-" + instanceID},
		CreatedAt: time.Now(),
	}

	p.Log.Info("Retrieved instance details",
		"id", instance.ID,
		"name", instance.Name,
		"status", instance.Status,
		"zone", instance.Zone,
		"flavor", instance.Flavor,
		"spot", instance.IsSpot,
	)

	return instance, nil
}

func (p *Provider) generateUserData(nodeClaim *karpenterv1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (string, error) {
    // Basic user data for node bootstrapping
    userData := `#!/bin/bash
# Basic node setup
apt-get update
apt-get install -y kubelet kubeadm kubectl
systemctl enable kubelet
systemctl start kubelet
`

    // TODO: Add UserData field to BizflyCloudNodeClassSpec if needed
    // For now, just return the basic user data
    return userData, nil
}




// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func (p *Provider) ConvertToNode(instance *Instance, isSpot bool) *corev1.Node {
    p.Log.V(1).Info("Converting instance to node",
        "instanceID", instance.ID,
        "name", instance.Name,
        "flavor", instance.Flavor,
        "spot", isSpot,
    )

    node := &corev1.Node{
        ObjectMeta: metav1.ObjectMeta{
            Name: instance.Name,
            Labels: map[string]string{
                NodeLabelRegion:                 instance.Region,
                NodeLabelZone:                   instance.Zone,
                NodeLabelInstanceType:           instance.Flavor,
                "karpenter.sh/provisioner-name": "default",
            },
            Annotations: map[string]string{
                "karpenter.bizflycloud.sh/instance-id": instance.ID,
            },
        },
        Spec: corev1.NodeSpec{
            ProviderID: fmt.Sprintf("%s%s", ProviderIDPrefix, instance.ID),
        },
    }

    // Add spot instance annotation if needed
    if isSpot || instance.IsSpot {
        node.Annotations[NodeAnnotationIsSpot] = "true"
    }

    // Extract labels and annotations from tags
    for _, tag := range instance.Tags {
        parts := strings.SplitN(tag, "=", 2)
        if len(parts) != 2 {
            continue
        }

        key, value := parts[0], parts[1]

        // Add tag as label if it starts with the label prefix
        if strings.HasPrefix(key, "label/") {
            labelKey := strings.TrimPrefix(key, "label/")
            if labelKey != "" {
                node.Labels[labelKey] = value
            }
        } else if strings.HasPrefix(key, "annotation/") {
            // Add tag as annotation if it starts with the annotation prefix
            annotKey := strings.TrimPrefix(key, "annotation/")
            if annotKey != "" {
                node.Annotations[annotKey] = value
            }
        }
    }

    return node
}


// GetAvailableZones returns the available zones in the configured region
func (p *Provider) GetAvailableZones(ctx context.Context) ([]string, error) {
	// In production, this would call the BizFly API to get available zones
	// For now, we'll use hardcoded values based on region
	zones := []string{
		fmt.Sprintf("%s-a", p.Region),
		fmt.Sprintf("%s-b", p.Region),
	}
	return zones, nil
}

// getInstanceOfferingByName gets an instance offering by name
func (p *Provider) getInstanceOfferingByName(ctx context.Context, name string) (*InstanceOffering, error) {
	// In production, this would call the BizFly API to get the offering
	// For now, we'll return a mock offering
	return &InstanceOffering{
		Name:   name,
		Region: p.Region,
	}, nil
}

// InstanceOffering represents a BizFly Cloud instance offering
type InstanceOffering struct {
	Name   string
	Region string
}

// Add this conversion function to your instance.go:
// Add this conversion function to your instance.go:
func (p *Provider) ConvertGobizflyServerToInstance(server *gobizfly.Server) *Instance {
    if server == nil {
        return nil
    }

    // Extract zone from availability zone or use default
    zone := server.AvailabilityZone
    if zone == "" {
        zone = fmt.Sprintf("%s-a", p.Region)
    }

    // Extract IP address from IPAddresses structure
    ipAddress := ""
    if len(server.IPAddresses.WanV4Addresses) > 0 {
        ipAddress = server.IPAddresses.WanV4Addresses[0].Address
    } else if len(server.IPAddresses.LanAddresses) > 0 {
        ipAddress = server.IPAddresses.LanAddresses[0].Address
    }

    // Convert CreatedAt string to time.Time
    var createdAt time.Time
    if server.CreatedAt != "" {
        // Try different time formats that BizFly might use
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
        
        // If all parsing fails, use current time
        if createdAt.IsZero() {
            p.Log.V(1).Info("Failed to parse CreatedAt, using current time", 
                "createdAt", server.CreatedAt)
            createdAt = time.Now()
        }
    } else {
        createdAt = time.Now()
    }

    // Extract tags from metadata
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
        IsSpot:    false, // You can determine this from metadata or server type
        Tags:      tags,
        CreatedAt: createdAt,
    }
}

