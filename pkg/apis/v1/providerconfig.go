package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status

// ProviderConfig defines the configuration for the BizFly Cloud provider
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status"`
}

type ProviderConfigSpec struct {
	Region         string              `json:"region"`
	Zone           string              `json:"zone"`
	SecretRef      SecretReference     `json:"secretRef"`
	Tagging        *TaggingSpec        `json:"tagging,omitempty"`
	CloudConfig    *CloudConfigSpec    `json:"cloudConfig,omitempty"`
	ImageConfig    *ImageConfigSpec    `json:"imageConfig,omitempty"`
	DriftDetection *DriftDetectionSpec `json:"driftDetection,omitempty"`
}

type SecretReference struct {
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

type TaggingSpec struct {
	EnableResourceTags bool              `json:"enableResourceTags,omitempty"`
	TagPrefix          string            `json:"tagPrefix,omitempty"`
	AdditionalTags     map[string]string `json:"additionalTags,omitempty"`
}

type CloudConfigSpec struct {
	APIEndpoint       string `json:"apiEndpoint,omitempty"`
	RetryMaxAttempts  int    `json:"retryMaxAttempts,omitempty"`
	RetryInitialDelay string `json:"retryInitialDelay,omitempty"`
	Timeout           string `json:"timeout,omitempty"`
}

type ImageConfigSpec struct {
	ImageID          string `json:"imageID,omitempty"`
	RootDiskSize     int    `json:"rootDiskSize,omitempty"`
	UserDataTemplate string `json:"userDataTemplate,omitempty"`
}

type DriftDetectionSpec struct {
	Enabled           bool   `json:"enabled,omitempty"`
	ReconcileInterval string `json:"reconcileInterval,omitempty"`
	AutoRemediate     bool   `json:"autoRemediate,omitempty"`
}

type ProviderConfigStatus struct {
	LastUpdated   metav1.Time        `json:"lastUpdated,omitempty"`
	Conditions    []metav1.Condition `json:"conditions,omitempty"`
	InstanceTypes []string           `json:"instanceTypes,omitempty"`
	Regions       []string           `json:"regions,omitempty"`
}
