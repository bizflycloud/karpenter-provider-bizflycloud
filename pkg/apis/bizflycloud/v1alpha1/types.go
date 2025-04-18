package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Region",type="string",JSONPath=".spec.region"
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp"

// ProviderConfig defines the configuration for the BizFly Cloud provider
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// ProviderConfigSpec defines the desired state of the BizFly Cloud provider configuration
type ProviderConfigSpec struct {
	// Region is the BizFly Cloud region to use
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=HN1;HN2;HCM1
	Region string `json:"region"`

	// SecretRef is a reference to a secret containing authentication information
	// +kubebuilder:validation:Required
	SecretRef SecretReference `json:"secretRef"`

	// Tagging configures resource tagging behavior
	// +optional
	Tagging *TaggingSpec `json:"tagging,omitempty"`

	// CloudConfig provides additional configuration for the BizFly Cloud API
	// +optional
	CloudConfig *CloudConfigSpec `json:"cloudConfig,omitempty"`

	// ImageConfig specifies the base image and configuration for instances
	// +optional
	ImageConfig *ImageConfigSpec `json:"imageConfig,omitempty"`

	// DriftDetection configures drift detection options
	// +optional
	DriftDetection *DriftDetectionSpec `json:"driftDetection,omitempty"`
}

// SecretReference is a reference to a secret containing authentication information
type SecretReference struct {
	// Name is the name of the secret
	// +kubebuilder:validation:Required
	Name string `json:"name"`

	// Namespace is the namespace of the secret
	// +kubebuilder:validation:Required
	Namespace string `json:"namespace"`
}

// TaggingSpec configures the tagging behavior for BizFly Cloud resources
type TaggingSpec struct {
	// EnableResourceTags enables tagging of created resources
	// +optional
	// +kubebuilder:default=true
	EnableResourceTags bool `json:"enableResourceTags,omitempty"`

	// TagPrefix is the prefix used for all Karpenter-managed resource tags
	// +optional
	// +kubebuilder:default="karpenter.bizflycloud.sh/"
	TagPrefix string `json:"tagPrefix,omitempty"`

	// AdditionalTags are additional tags to apply to all created resources
	// +optional
	AdditionalTags map[string]string `json:"additionalTags,omitempty"`
}

// CloudConfigSpec provides additional configuration for the BizFly Cloud API
type CloudConfigSpec struct {
	// APIEndpoint is the BizFly Cloud API endpoint URL
	// +optional
	// +kubebuilder:default="https://manage.bizflycloud.vn/api"
	APIEndpoint string `json:"apiEndpoint,omitempty"`

	// RetryMaxAttempts is the maximum number of retry attempts for API calls
	// +optional
	// +kubebuilder:validation:Minimum=1
	// +kubebuilder:validation:Maximum=10
	// +kubebuilder:default=5
	RetryMaxAttempts int `json:"retryMaxAttempts,omitempty"`

	// RetryInitialDelay is the initial delay between retry attempts
	// +optional
	// +kubebuilder:default="1s"
	RetryInitialDelay string `json:"retryInitialDelay,omitempty"`

	// Timeout is the maximum time to wait for API operations
	// +optional
	// +kubebuilder:default="30s"
	Timeout string `json:"timeout,omitempty"`
}

// ImageConfigSpec configures the base image and related settings for instances
type ImageConfigSpec struct {
	// ImageID is the ID or name of the BizFly Cloud image to use
	// +optional
	// +kubebuilder:default="ubuntu-20.04"
	ImageID string `json:"imageID,omitempty"`

	// SSHKeyName is the name of the SSH key to add to instances
	// +optional
	SSHKeyName string `json:"sshKeyName,omitempty"`

	// RootDiskSize is the size of the root disk in GB
	// +optional
	// +kubebuilder:validation:Minimum=10
	// +kubebuilder:default=20
	RootDiskSize int `json:"rootDiskSize,omitempty"`

	// EnableIPv6 enables IPv6 on the instances
	// +optional
	// +kubebuilder:default=false
	EnableIPv6 bool `json:"enableIPv6,omitempty"`

	// UserDataTemplate is a template for additional user data to include
	// +optional
	UserDataTemplate string `json:"userDataTemplate,omitempty"`
}

// DriftDetectionSpec configures drift detection options
type DriftDetectionSpec struct {
	// Enabled enables drift detection
	// +optional
	// +kubebuilder:default=true
	Enabled bool `json:"enabled,omitempty"`

	// ReconcileInterval is the interval between drift checks
	// +optional
	// +kubebuilder:default="5m"
	ReconcileInterval string `json:"reconcileInterval,omitempty"`

	// AutoRemediate automatically remediates drift if detected
	// +optional
	// +kubebuilder:default=true
	AutoRemediate bool `json:"autoRemediate,omitempty"`
}

// ProviderConfigStatus defines the observed state of the BizFly Cloud provider configuration
type ProviderConfigStatus struct {
	// LastUpdated is the timestamp of the last update to the provider configuration
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// Conditions represent the latest available observations of the provider configuration's state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

	// InstanceTypes is the list of instance types discovered from BizFly Cloud
	// +optional
	InstanceTypes []string `json:"instanceTypes,omitempty"`

	// Regions is the list of available regions
	// +optional
	Regions []string `json:"regions,omitempty"`
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}
