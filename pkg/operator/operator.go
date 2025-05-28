package operator

import (
	"context"
	"fmt"
	"os"

	"github.com/bizflycloud/gobizfly"
	v1 "github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis/v1"
	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/provider/instancetype"
	"github.com/go-logr/logr"
	"sigs.k8s.io/controller-runtime/pkg/log"
	coreoperator "sigs.k8s.io/karpenter/pkg/operator"
)

const (
	bizflyCloudAuthMethod      = "BIZFLY_CLOUD_AUTH_METHOD"
	bizflyCloudEmailEnvName    = "BIZFLY_CLOUD_EMAIL"
	bizflyCloudPasswordEnvName = "BIZFLY_CLOUD_PASSWORD"
	bizflyCloudRegionEnvName   = "BIZFLY_CLOUD_REGION"
	bizflyCloudAppCredID       = "BIZFLY_CLOUD_APP_CRED_ID"
	bizflyCloudAppCredSecret   = "BIZFLY_CLOUD_APP_CRED_SECRET"
	bizflyCloudApiUrl          = "BIZFLY_CLOUD_API_URL"
	bizflyCloudTenantID        = "BIZFLY_CLOUD_TENANT_ID"
	defaultRegion              = "HN"
	defaultApiUrl              = "https://manage.bizflycloud.vn"
	authPassword               = "password"
	authAppCred                = "application_credential"
)

// Operator is the main operator struct that holds all the necessary components
type Operator struct {
	*coreoperator.Operator
	Client           *gobizfly.Client
	InstanceProvider instancetype.Provider
	Config           *v1.ProviderConfig
	Log              *logr.Logger
	Region           string
}

// NewOperator creates a new operator instance with all necessary components
// NewOperator creates a new operator instance with all necessary components
func NewOperator(ctx context.Context, operator *coreoperator.Operator) (context.Context, *Operator) {
	// Initialize log
	log := log.FromContext(ctx)

	// Initialize Bizfly client
	client, err := newBizflyClient()
	if err != nil {
		log.Error(err, "failed to create Bizfly client")
		os.Exit(1)
	}

	// Initialize instance type provider - FIXED: Pass both client and logger
	instanceProvider := instancetype.NewDefaultProvider(client, log)

	// Initialize provider config
	config := &v1.ProviderConfig{}

	return ctx, &Operator{
		Operator:         operator,
		Client:           client,
		InstanceProvider: instanceProvider,
		Config:           config,
		Log:              &log,
		Region:           getRegion(),
	}
}


// newBizflyClient creates a new Bizfly Cloud client with proper authentication
func newBizflyClient() (*gobizfly.Client, error) {
	authMethod := getAuthMethod()
	username := getUsername()
	password := getPassword()
	appCredId := getAppCredId()
	appCredSecret := getAppCredSecret()
	apiUrl := getApiUrl()
	tenantId := getTenantId()
	region := getRegion()

	switch authMethod {
	case authPassword:
		if username == "" {
			return nil, fmt.Errorf("you have to provide username variable")
		}
		if password == "" {
			return nil, fmt.Errorf("you have to provide password variable")
		}
	case authAppCred:
		if appCredId == "" {
			return nil, fmt.Errorf("you have to provide application credential ID")
		}
		if appCredSecret == "" {
			return nil, fmt.Errorf("you have to provide application credential secret")
		}
	}

	if region == "" {
		region = defaultRegion
	}

	if apiUrl == "" {
		apiUrl = defaultApiUrl
	}

	bizflyClient, err := gobizfly.NewClient(
		gobizfly.WithAPIURL(apiUrl),
		gobizfly.WithProjectID(tenantId),
		gobizfly.WithRegionName(region),
	)
	if err != nil {
		return nil, fmt.Errorf("cannot create BizFly Cloud Client: %w", err)
	}

	token, err := bizflyClient.Token.Init(
		context.Background(),
		&gobizfly.TokenCreateRequest{
			AuthMethod:    authMethod,
			Username:      username,
			Password:      password,
			AppCredID:     appCredId,
			AppCredSecret: appCredSecret,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("cannot create token: %w", err)
	}

	bizflyClient.SetKeystoneToken(token)

	return bizflyClient, nil
}

func getRegion() string {
	return os.Getenv(bizflyCloudRegionEnvName)
}

func getApiUrl() string {
	return os.Getenv(bizflyCloudApiUrl)
}

func getTenantId() string {
	return os.Getenv(bizflyCloudTenantID)
}

func getAppCredId() string {
	return os.Getenv(bizflyCloudAppCredID)
}

func getAppCredSecret() string {
	return os.Getenv(bizflyCloudAppCredSecret)
}

func getAuthMethod() string {
	return os.Getenv(bizflyCloudAuthMethod)
}

func getUsername() string {
	return os.Getenv(bizflyCloudEmailEnvName)
}

func getPassword() string {
	return os.Getenv(bizflyCloudPasswordEnvName)
}
