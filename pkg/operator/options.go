package operator

import (
    "flag"
    "fmt"
    "os"
)

// Options holds the configuration options for the operator
type Options struct {
    MetricsAddr          string
    HealthProbeAddr      string
    EnableLeaderElection bool
    LogLevel             string
    Region               string
    Zone                 string
    ClusterName          string
}

// NewOptions creates new options with default values
func NewOptions() *Options {
    return &Options{
        MetricsAddr:          ":8080",
        HealthProbeAddr:      ":8081",
        EnableLeaderElection: false,
        LogLevel:             "info",
        Region:               "HN",
        Zone:                 "HN2",
        ClusterName:          "karpenter-cluster",
    }
}

// AddFlags adds command line flags
func (o *Options) AddFlags() {
    flag.StringVar(&o.MetricsAddr, "metrics-bind-address", o.MetricsAddr, "The address the metric endpoint binds to.")
    flag.StringVar(&o.HealthProbeAddr, "health-probe-bind-address", o.HealthProbeAddr, "The address the probe endpoint binds to.")
    flag.BoolVar(&o.EnableLeaderElection, "leader-elect", o.EnableLeaderElection, "Enable leader election for controller manager.")
    flag.StringVar(&o.LogLevel, "log-level", o.LogLevel, "Log level (debug, info, warn, error)")
    flag.StringVar(&o.Region, "region", o.Region, "BizFly Cloud region")
    flag.StringVar(&o.Zone, "zone", o.Zone, "BizFly Cloud zone")
    flag.StringVar(&o.ClusterName, "cluster-name", o.ClusterName, "Kubernetes cluster name")
}

// Validate validates the options
func (o *Options) Validate() error {
    if o.Region == "" {
        return fmt.Errorf("region is required")
    }
    if o.ClusterName == "" {
        return fmt.Errorf("cluster name is required")
    }
    return nil
}

// LoadFromEnv loads options from environment variables
func (o *Options) LoadFromEnv() {
    if region := os.Getenv("BIZFLY_CLOUD_REGION"); region != "" {
        o.Region = region
    }
    if zone := os.Getenv("BIZFLY_CLOUD_ZONE"); zone != "" {
        o.Zone = zone
    }
    if clusterName := os.Getenv("CLUSTER_NAME"); clusterName != "" {
        o.ClusterName = clusterName
    }
    if logLevel := os.Getenv("LOG_LEVEL"); logLevel != "" {
        o.LogLevel = logLevel
    }
}
