package config

import (
    "os"
)

// Config holds the configuration for the operator
type Config struct {
    Region           string
    Zone             string
    ClusterName      string
    ClusterEndpoint  string
    NodeImageID      string
    LogLevel         string
}

// NewConfig creates a new configuration from environment variables
func NewConfig() *Config {
    return &Config{
        Region:          getEnvOrDefault("BIZFLY_CLOUD_REGION", "HN"),
        Zone:            getEnvOrDefault("BIZFLY_CLOUD_ZONE", "HN2"),
        ClusterName:     getEnvOrDefault("CLUSTER_NAME", "karpenter-cluster"),
        ClusterEndpoint: getEnvOrDefault("CLUSTER_ENDPOINT", ""),
        NodeImageID:     getEnvOrDefault("NODE_IMAGE_ID", "5a821700-a184-4f91-8455-205d47d472c0"),
        LogLevel:        getEnvOrDefault("LOG_LEVEL", "info"),
    }
}

func getEnvOrDefault(key, defaultValue string) string {
    if value := os.Getenv(key); value != "" {
        return value
    }
    return defaultValue
}
