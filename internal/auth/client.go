package auth

import (
    "context"
    "fmt"
    "os"
    "github.com/bizflycloud/gobizfly"
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

// NewBizflyClient creates a new authenticated BizFly Cloud client
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

// Helper functions
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

