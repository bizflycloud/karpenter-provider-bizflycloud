package operator

import (
	"context"
	"os"

	"github.com/bizflycloud/gobizfly"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/internal/logging"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/client"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
	"sigs.k8s.io/controller-runtime/pkg/log"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
)

// Operator is the main operator struct that holds all the necessary components
type Operator struct {
	*coreoperator.Operator
	Client           *gobizfly.Client
	InstanceProvider instancetype.Provider
	Config           *v1.ProviderConfig
	Log              *logging.Logger
	Region           string
}

// NewOperator creates a new operator instance with all necessary components
func NewOperator(ctx context.Context, operator *coreoperator.Operator) (context.Context, *Operator) {
	// Initialize log from context
	baseLog := log.FromContext(ctx)
	log := &logging.Logger{Logger: baseLog}

	// Load configuration from environment
	config := loadConfigFromEnv()

	// Initialize Bizfly client
	clientConfig := client.LoadConfigFromEnv()
	bizflyClient, err := client.NewBizflyClient(clientConfig, baseLog)
	if err != nil {
		log.ErrorWithDetails(err, "Failed to create Bizfly client")
		os.Exit(1)
	}

	// Initialize instance type provider
	instanceProvider := instancetype.NewDefaultProvider(bizflyClient, baseLog)

	return ctx, &Operator{
		Operator:         operator,
		Client:           bizflyClient,
		InstanceProvider: instanceProvider,
		Config:           config,
		Log:              log,
		Region:           clientConfig.Region,
	}
}

// loadConfigFromEnv loads the provider configuration from environment variables
func loadConfigFromEnv() *v1.ProviderConfig {
	region := os.Getenv("BIZFLY_CLOUD_REGION")
	if region == "" {
		region = "HN"
	}

	zone := os.Getenv("BIZFLY_CLOUD_ZONE")
	if zone == "" {
		zone = "HN1"
	}

	imageID := os.Getenv("BIZFLY_CLOUD_IMAGE_ID")
	if imageID == "" {
		imageID = "5a821700-a184-4f91-8455-205d47d472c0" // Default Ubuntu image
	}

	return &v1.ProviderConfig{
		Spec: v1.ProviderConfigSpec{
			Region: region,
			Zone:   zone,
			ImageConfig: &v1.ImageConfigSpec{
				ImageID: imageID,
			},
		},
	}
}
