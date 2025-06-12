package client

import (
	"context"
	"fmt"
	"os"

	"github.com/bizflycloud/gobizfly"
	"github.com/go-logr/logr"
)

const (
	// Environment variable names
	EnvAuthMethod    = "BIZFLY_CLOUD_AUTH_METHOD"
	EnvEmail         = "BIZFLY_CLOUD_EMAIL"
	EnvPassword      = "BIZFLY_CLOUD_PASSWORD"
	EnvRegion        = "BIZFLY_CLOUD_REGION"
	EnvAppCredID     = "BIZFLY_CLOUD_APP_CRED_ID"
	EnvAppCredSecret = "BIZFLY_CLOUD_APP_CRED_SECRET"
	EnvAPIURL        = "BIZFLY_CLOUD_API_URL"
	EnvTenantID      = "BIZFLY_CLOUD_TENANT_ID"

	// Default values
	DefaultRegion = "HN"
	DefaultAPIURL = "https://manage.bizflycloud.vn"

	// Auth methods
	AuthPassword = "password"
	AuthAppCred  = "application_credential"
)

// Config holds BizflyCloud client configuration
type Config struct {
	AuthMethod    string
	Username      string
	Password      string
	AppCredID     string
	AppCredSecret string
	APIUrl        string
	TenantID      string
	Region        string
}

// LoadConfigFromEnv loads configuration from environment variables
func LoadConfigFromEnv() *Config {
	config := &Config{
		AuthMethod:    getEnvWithDefault(EnvAuthMethod, AuthPassword),
		Username:      os.Getenv(EnvEmail),
		Password:      os.Getenv(EnvPassword),
		AppCredID:     os.Getenv(EnvAppCredID),
		AppCredSecret: os.Getenv(EnvAppCredSecret),
		APIUrl:        getEnvWithDefault(EnvAPIURL, DefaultAPIURL),
		TenantID:      os.Getenv(EnvTenantID),
		Region:        getEnvWithDefault(EnvRegion, DefaultRegion),
	}

	return config
}

// Validate validates the configuration
func (c *Config) Validate() error {
	switch c.AuthMethod {
	case AuthPassword:
		if c.Username == "" {
			return fmt.Errorf("username is required for password authentication")
		}
		if c.Password == "" {
			return fmt.Errorf("password is required for password authentication")
		}
	case AuthAppCred:
		if c.AppCredID == "" {
			return fmt.Errorf("application credential ID is required")
		}
		if c.AppCredSecret == "" {
			return fmt.Errorf("application credential secret is required")
		}
	default:
		return fmt.Errorf("invalid auth method: %s", c.AuthMethod)
	}

	return nil
}

// NewBizflyClient creates a new authenticated BizflyCloud client
func NewBizflyClient(config *Config, log logr.Logger) (*gobizfly.Client, error) {
	if err := config.Validate(); err != nil {
		return nil, fmt.Errorf("invalid configuration: %w", err)
	}

	log.Info("Creating BizflyCloud client",
		"authMethod", config.AuthMethod,
		"region", config.Region,
		"apiUrl", config.APIUrl)

	client, err := gobizfly.NewClient(
		gobizfly.WithAPIURL(config.APIUrl),
		gobizfly.WithProjectID(config.TenantID),
		gobizfly.WithRegionName(config.Region),
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create BizflyCloud client: %w", err)
	}

	token, err := client.Token.Init(
		context.Background(),
		&gobizfly.TokenCreateRequest{
			AuthMethod:    config.AuthMethod,
			Username:      config.Username,
			Password:      config.Password,
			AppCredID:     config.AppCredID,
			AppCredSecret: config.AppCredSecret,
		},
	)
	if err != nil {
		return nil, fmt.Errorf("failed to authenticate: %w", err)
	}

	client.SetKeystoneToken(token)

	log.Info("Successfully authenticated with BizflyCloud",
		"region", config.Region)

	return client, nil
}

// NewBizflyClientFromEnv creates a BizflyCloud client using environment variables
func NewBizflyClientFromEnv(log logr.Logger) (*gobizfly.Client, error) {
	config := LoadConfigFromEnv()
	return NewBizflyClient(config, log)
}

func getEnvWithDefault(key, defaultValue string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return defaultValue
}
