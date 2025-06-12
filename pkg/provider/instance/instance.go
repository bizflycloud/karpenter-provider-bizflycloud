package instance

import (
	"context"
	"fmt"

	"github.com/bizflycloud/gobizfly"
	v1bizfly "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/go-logr/logr"
	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	karpenterv1 "sigs.k8s.io/karpenter/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance/conversion"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instance/lifecycle"
)

type Provider struct {
	Client           client.Client
	Log              logr.Logger
	BizflyClient     *gobizfly.Client
	Region           string
	Config           *v1bizfly.ProviderConfig
	LifecycleManager *lifecycle.Manager
}

// NewProvider creates a new instance provider
func NewProvider(client client.Client, log logr.Logger, bizflyClient *gobizfly.Client, region string, config *v1bizfly.ProviderConfig) *Provider {
	lifecycleManager := lifecycle.NewManager(client, log, bizflyClient, region, config)
	
	return &Provider{
		Client:           client,
		Log:              log,
		BizflyClient:     bizflyClient,
		Region:           region,
		Config:           config,
		LifecycleManager: lifecycleManager,
	}
}

// CreateInstance creates a new BizFly Cloud server instance and waits for it to be ready
func (p *Provider) CreateInstance(ctx context.Context, nodeClaim *karpenterv1.NodeClaim, nodeClass *v1bizfly.BizflyCloudNodeClass) (*gobizfly.Server, error) {
	return p.LifecycleManager.CreateInstance(ctx, nodeClaim, nodeClass)
}

// GetInstance gets a BizFly Cloud server instance by ID
func (p *Provider) GetInstance(ctx context.Context, instanceID string) (*conversion.Instance, error) {
	server, err := p.LifecycleManager.GetInstance(ctx, instanceID)
	if err != nil {
		return nil, err
	}

	// Convert to Instance
	instance := conversion.ConvertGobizflyServerToInstance(server, p.Region)
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
	return p.LifecycleManager.DeleteInstance(ctx, instanceID)
}

// ConvertToNode converts a BizFly Cloud server instance to a Kubernetes node
func (p *Provider) ConvertToNode(instance *conversion.Instance, isSpot bool) *corev1.Node {
	return conversion.ConvertToNode(instance, isSpot)
}