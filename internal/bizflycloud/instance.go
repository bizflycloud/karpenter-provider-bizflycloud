package bizflycloud

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/wait"
	"k8s.io/utils/ptr"
)

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

// createInstance creates a new BizFly Cloud server instance
func (p *Provider) createInstance(ctx context.Context, nodeName string, flavor string, image string, userData string, isSpot bool) (*Instance, error) {
	p.log.Info("Creating instance", 
		"name", nodeName, 
		"flavor", flavor, 
		"region", p.region, 
		"spot", isSpot,
	)

	// Get config values or use defaults
	rootDiskSize := defaultRootDiskSize
	if p.config != nil && p.config.Spec.ImageConfig != nil && p.config.Spec.ImageConfig.RootDiskSize > 0 {
		rootDiskSize = p.config.Spec.ImageConfig.RootDiskSize
	}

	// Use default image if not specified
	if image == "" {
		if p.config != nil && p.config.Spec.ImageConfig != nil && p.config.Spec.ImageConfig.ImageID != "" {
			image = p.config.Spec.ImageConfig.ImageID
		} else {
			image = defaultImageName
		}
	}

	// Get SSH key if specified
	sshKeyName := defaultSSHKey
	if p.config != nil && p.config.Spec.ImageConfig != nil && p.config.Spec.ImageConfig.SSHKeyName != "" {
		sshKeyName = p.config.Spec.ImageConfig.SSHKeyName
	}

	// Create resource tags
	tags := []string{
		fmt.Sprintf("karpenter-node=%s", nodeName),
		fmt.Sprintf("karpenter-cluster=%s", "kubernetes"),
	}

	// Add additional tags if specified
	if p.config != nil && p.config.Spec.Tagging != nil && p.config.Spec.Tagging.EnableResourceTags {
		// Add provisioner tag
		tags = append(tags, "karpenter-provisioner=default")

		// Add additional tags if any
		if p.config.Spec.Tagging.AdditionalTags != nil {
			for k, v := range p.config.Spec.Tagging.AdditionalTags {
				tags = append(tags, fmt.Sprintf("%s=%s", k, v))
			}
		}
	}

	// If using spot instances, add tag
	if isSpot {
		tags = append(tags, "karpenter-spot=true")
	}

	// In a mock implementation, just return a mock instance
	if p.bizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.log.Info("Using mock instance creation")
		// Create a mock server instance
		instance := &Instance{
			ID:        fmt.Sprintf("instance-%s", nodeName),
			Name:      nodeName,
			Status:    "ACTIVE",
			Region:    p.region,
			Zone:      fmt.Sprintf("%s-a", p.region),
			Flavor:    flavor,
			IPAddress: "192.168.1.100",
			IsSpot:    isSpot,
			Tags:      tags,
			CreatedAt: time.Now(),
		}

		p.log.Info("Server creation completed", "id", instance.ID, "name", instance.Name)
		return instance, nil
	}

	// For a real implementation, determine availability zone
	zones, err := p.GetAvailableZones(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to get available zones: %w", err)
	}

	if len(zones) == 0 {
		return nil, fmt.Errorf("no availability zones found in region %s", p.region)
	}

	// Pick a zone (in production you might use more sophisticated logic)
	zone := zones[0]

	// Get offering details to verify it exists
	offering, err := p.getInstanceOfferingByName(ctx, flavor)
	if err != nil {
		return nil, fmt.Errorf("invalid flavor %s: %w", flavor, err)
	}

	// Setup server creation options
	// In production, we'd convert this to a call to the BizFly Cloud API
	/* For reference, here's a real API call using gobizfly client:
	
	// Create server options
	opts := &gobizfly.ServerCreateRequest{
		Name:             nodeName,
		FlavorName:       flavor,
		ImageName:        image,
		RootDiskSize:     rootDiskSize,
		AvailabilityZone: zone,
		SSHKey:           sshKeyName,
		Userdata:         base64.StdEncoding.EncodeToString([]byte(userData)),
		Tags:             tags,
		Spot:             ptr.To(isSpot),
	}

	// Create the server
	server, err := p.bizflyClient.Server.Create(ctx, opts)
	if err != nil {
		return nil, fmt.Errorf("failed to create server: %w", err)
	}

	instanceID := server.ID
	*/

	// For our mock implementation, create an instance ID
	instanceID := fmt.Sprintf("instance-%s", nodeName)

	// Wait for the server to become active
	p.log.Info("Waiting for server to become active", "name", nodeName)

	var instance *Instance
	err = wait.PollImmediate(pollInterval, maxServerWaitTime, func() (bool, error) {
		// In production, we'd get the server by ID from the API
		// inst, err := p.bizflyClient.Server.Get(ctx, instanceID)
		
		// For mock, just create an active instance
		instance = &Instance{
			ID:        instanceID,
			Name:      nodeName,
			Status:    "ACTIVE",
			Region:    p.region,
			Zone:      zone,
			Flavor:    offering.Name,
			IPAddress: "192.168.1.100",
			IsSpot:    isSpot,
			Tags:      tags,
			CreatedAt: time.Now(),
		}
		
		return true, nil
	})

	if err != nil {
		// If timed out, try to delete the instance to avoid leaks
		p.log.Error(err, "Timed out waiting for server to become active, cleaning up", "name", nodeName)
		if delErr := p.deleteInstance(ctx, instanceID); delErr != nil {
			p.log.Error(delErr, "Failed to clean up server after timeout", "name", nodeName)
		}
		return nil, fmt.Errorf("timed out waiting for server to become active: %w", err)
	}

	p.log.Info("Server creation completed", 
		"id", instance.ID, 
		"name", instance.Name, 
		"zone", instance.Zone,
		"flavor", instance.Flavor,
		"spot", instance.IsSpot,
	)
	
	return instance, nil
}

// deleteInstance deletes a BizFly Cloud server instance
func (p *Provider) deleteInstance(ctx context.Context, instanceID string) error {
	p.log.Info("Deleting instance", "id", instanceID)

	// If in mock mode, just return success
	if p.bizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.log.Info("Using mock instance deletion")
		p.log.Info("Server successfully deleted", "id", instanceID)
		return nil
	}

	// For production, we'd delete the server via the API
	/* For reference, here's a real API call using gobizfly client:
	
	// Delete the server
	err := p.bizflyClient.Server.Delete(ctx, instanceID)
	if err != nil {
		// Check if error is because instance doesn't exist
		if strings.Contains(err.Error(), "not found") {
			p.log.Info("Instance not found, considering it already deleted", "id", instanceID)
			return nil
		}
		return fmt.Errorf("failed to delete server: %w", err)
	}
	*/

	// Wait for the server to be completely deleted
	err := wait.PollImmediate(pollInterval, maxServerWaitTime, func() (bool, error) {
		// For production, we'd check if the server still exists
		/*
		_, err := p.bizflyClient.Server.Get(ctx, instanceID)
		if err != nil {
			if strings.Contains(err.Error(), "not found") {
				return true, nil // Server is gone
			}
			return false, err
		}
		return false, nil // Server still exists
		*/
		
		// For mock, just return success
		return true, nil
	})

	if err != nil {
		return fmt.Errorf("timed out waiting for server to be deleted: %w", err)
	}

	p.log.Info("Server successfully deleted", "id", instanceID)
	return nil
}

// getInstance gets a BizFly Cloud server instance by ID
func (p *Provider) getInstance(ctx context.Context, instanceID string) (*Instance, error) {
	p.log.Info("Getting instance", "id", instanceID)

	// If in mock mode, return a mock instance
	if p.bizflyClient == nil || os.Getenv("KARPENTER_BIZFLYCLOUD_MOCK") == "true" {
		p.log.Info("Using mock instance retrieval")
		
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
			Region:    p.region,
			Zone:      fmt.Sprintf("%s-a", p.region),
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
		Region:    p.region,
		Zone:      fmt.Sprintf("%s-a", p.region),
		Flavor:    "4c_8g",
		IPAddress: "192.168.1.100",
		IsSpot:    false,
		Tags:      []string{"karpenter-node=node-" + instanceID},
		CreatedAt: time.Now(),
	}

	return instance, nil
}

// convertInstanceToNode converts a BizFly Cloud server instance to a Kubernetes node
func (p *Provider) convertInstanceToNode(instance *Instance, isSpot bool) *corev1.Node {
	node := &corev1.Node{
		ObjectMeta: metav1.ObjectMeta{
			Name: instance.Name,
			Labels: map[string]string{
				NodeLabelRegion:        instance.Region,
				NodeLabelZone:          instance.Zone,
				NodeLabelInstanceType:  instance.Flavor,
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
