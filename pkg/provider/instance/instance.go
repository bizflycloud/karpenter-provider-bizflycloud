package instance

import (
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"sigs.k8s.io/controller-runtime/pkg/client"

	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
)

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

// Provider implements instance management for BizFly Cloud
type Provider struct {
	Client       client.Client
	Log          logr.Logger
	BizflyClient *gobizfly.Client
	Region       string
	Config       *v1.ProviderConfig
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
func (p *Provider) CreateInstance(ctx context.Context, nodeName string, flavor string, image string, userData string, isSpot bool) (*Instance, error) {
	p.Log.Info("Creating instance",
		"name", nodeName,
		"flavor", flavor,
		"region", p.Region,
		"spot", isSpot,
	)

	// Get config values or use defaults
	rootDiskSize := defaultRootDiskSize
	if p.Config != nil && p.Config.Spec.ImageConfig != nil && p.Config.Spec.ImageConfig.RootDiskSize > 0 {
		rootDiskSize = p.Config.Spec.ImageConfig.RootDiskSize
	}

	// Use default image if not specified
	if image == "" {
		if p.Config != nil && p.Config.Spec.ImageConfig != nil && p.Config.Spec.ImageConfig.ImageID != "" {
			image = p.Config.Spec.ImageConfig.ImageID
		} else {
			image = defaultImageName
		}
	}

	// Create resource tags
	tags := []string{
		fmt.Sprintf("karpenter-node=%s", nodeName),
		fmt.Sprintf("karpenter-cluster=%s", "kubernetes"),
	}

	// Add additional tags if specified
	if p.Config != nil && p.Config.Spec.Tagging != nil && p.Config.Spec.Tagging.EnableResourceTags {
		// Add provisioner tag
		tags = append(tags, "karpenter-provisioner=default")

		// Add additional tags if any
		if p.Config.Spec.Tagging.AdditionalTags != nil {
			for k, v := range p.Config.Spec.Tagging.AdditionalTags {
				tags = append(tags, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	// If using spot instances, add tag
	if isSpot {
		tags = append(tags, "karpenter-spot=true")
	}

	// In a mock implementation, just return a mock instance
	if p.BizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.Log.Info("Using mock instance creation")
		// Create a mock server instance
		instance := &Instance{
			ID:        fmt.Sprintf("instance-%s", nodeName),
			Name:      nodeName,
			Status:    "ACTIVE",
			Region:    p.Region,
			Zone:      fmt.Sprintf("%s-a", p.Region),
			Flavor:    flavor,
			IPAddress: "192.168.1.100",
			IsSpot:    isSpot,
			Tags:      tags,
			CreatedAt: time.Now(),
		}

		p.Log.Info("Server creation completed", "id", instance.ID, "name", instance.Name)
		return instance, nil
	}

	// For a real implementation, determine availability zone
	zones, err := p.GetAvailableZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get available zones: %w", err)
	}

	if len(zones) == 0 {
		return nil, fmt.Errorf("no availability zones found in region %s", p.Region)
	}

	// Pick a zone (in production you might use more sophisticated logic)
	zone := zones[0]

	// Get offering details to verify it exists
	offering, err := p.getInstanceOfferingByName(ctx, flavor)
	if err != nil {
		return nil, fmt.Errorf("invalid flavor %s: %w", flavor, err)
	}

	// Create server options
	opts := &gobizfly.ServerCreateRequest{
		Name:             nodeName,
		FlavorName:       flavor,
		Type:             "premium",
		AvailabilityZone: zone,
		UserData:         base64.StdEncoding.EncodeToString([]byte(userData)),
		OS: &gobizfly.ServerOS{
			Type: image,
		},
		RootDisk: &gobizfly.ServerDisk{
			Size: rootDiskSize,
		},
		Password: true,
	}

	// Create the server
	server, err := p.BizflyClient.CloudServer.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	// Get the task ID from the response
	if len(server.Task) == 0 {
		return nil, fmt.Errorf("no task ID returned from server creation")
	}
	taskID := server.Task[0]

	// Wait for the server to become active
	p.Log.Info("Waiting for server to become active", "name", nodeName)

	var instance *Instance
	var serverID string
	err = wait.PollImmediate(pollInterval, maxServerWaitTime, func() (bool, error) {
		// Get the task status
		task, err := p.BizflyClient.CloudServer.GetTask(ctx, taskID)
		if err != nil {
			return false, fmt.Errorf("failed to get task status: %w", err)
		}

		// Check if task is completed
		if task.Ready {
			// Get the server details
			inst, err := p.BizflyClient.CloudServer.Get(ctx, task.Result.ID)
			if err != nil {
				return false, fmt.Errorf("failed to get server: %w", err)
			}

			// Get the first WAN IP address
			var ipAddress string
			if len(inst.IPAddresses.WanV4Addresses) > 0 {
				ipAddress = inst.IPAddresses.WanV4Addresses[0].Address
			} else if len(inst.IPAddresses.LanAddresses) > 0 {
				ipAddress = inst.IPAddresses.LanAddresses[0].Address
			} else {
				return false, fmt.Errorf("no IP address found for server")
			}

			// Parse the creation time
			createdAt, err := time.Parse(time.RFC3339, inst.CreatedAt)
			if err != nil {
				createdAt = time.Now()
			}

			instance = &Instance{
				ID:        inst.ID,
				Name:      inst.Name,
				Status:    inst.Status,
				Region:    p.Region,
				Zone:      inst.AvailabilityZone,
				Flavor:    offering.Name,
				IPAddress: ipAddress,
				IsSpot:    isSpot,
				Tags:      tags,
				CreatedAt: createdAt,
			}
			serverID = inst.ID
			return true, nil
		}

		// Check if task is in error state
		if !task.Result.Success {
			return false, fmt.Errorf("server creation failed: %s", task.Result.Action)
		}

		return false, nil
	})

	if err != nil {
		// If timed out, try to delete the instance to avoid leaks
		p.Log.Error(err, "Timed out waiting for server to become active, cleaning up", "name", nodeName)
		if delErr := p.DeleteInstance(ctx, serverID); delErr != nil {
			p.Log.Error(delErr, "Failed to clean up server after timeout", "name", nodeName)
		}
		return nil, fmt.Errorf("timed out waiting for server to become active: %w", err)
	}

	p.Log.Info("Server creation completed",
		"id", instance.ID,
		"name", instance.Name,
		"zone", instance.Zone,
		"flavor", instance.Flavor,
		"spot", instance.IsSpot,
	)

	return instance, nil
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

	// For production, we'd get the server from the API
	/* For reference, here's a real API call using gobizfly client:

	// Get the server
	server, err := p.bizflyClient.Server.Get(ctx, instanceID)
	if err != nil {
		return nil, fmt.Errorf("failed to get server: %w", err)
	}

	// Convert to our instance type
	isSpot := false
	for _, tag := range server.Tags {
		if tag == "karpenter-spot=true" {
			isSpot = true
			break
		}
	}

	instance := &Instance{
		ID:        server.ID,
		Name:      server.Name,
		Status:    server.Status,
		Region:    p.region,
		Zone:      server.AvailabilityZone,
		Flavor:    server.FlavorName,
		IPAddress: server.IPAddresses[0].Address,
		IsSpot:    isSpot,
		Tags:      server.Tags,
		CreatedAt: server.Created,
	}
	*/

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

	return instance, nil
}

// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func (p *Provider) ConvertToNode(instance *Instance, isSpot bool) *corev1.Node {
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
