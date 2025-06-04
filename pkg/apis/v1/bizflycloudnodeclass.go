package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BizflyCloudNodeClass is the Schema for the BizflyCloudNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""
// +kubebuilder:printcolumn:name="Category",type="string",JSONPath=".spec.nodeCategory",priority=1,description=""
// +kubebuilder:resource:path=bizflycloudnodeclasses,scope=Cluster,categories=karpenter,shortName={bizflycloudnc,bizflycloudncs}
// +kubebuilder:storageversion
// +kubebuilder:subresource:status
type BizflyCloudNodeClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BizflyCloudNodeClassSpec   `json:"spec,omitempty"`
	Status BizflyCloudNodeClassStatus `json:"status,omitempty"`
}

type BizflyCloudNodeClassSpec struct {
	// +optional
	Region string `json:"region"`

	// Zone is the availability zone where nodes will be created
	// If not specified, zones will be automatically selected based on placement strategy
	// +optional
	Zone string `json:"zone,omitempty"`

	// Template is the name of the template to use for nodes
	// +required
	Template string `json:"template"`

	// NodeCategory specifies the category of nodes to provision
	// +kubebuilder:validation:Enum=basic;premium;enterprise;dedicated
	// +kubebuilder:default="basic"
	// +optional
	NodeCategory string `json:"nodeCategory,omitempty"`

	// ImageID is the ID of the image to use for nodes
	// +optional
	ImageID string `json:"imageId,omitempty"`
	// SSHKeyName is the name of the SSH key pair to use for instances
	// +optional
	// ImageMapping maps Kubernetes versions to image IDs
	// +optional
	ImageMapping map[string]string `json:"imageMapping,omitempty"`
	SSHKeyName string `json:"sshKeyName,omitempty"`

	// SSHKeys is a list of SSH public keys to inject into instances
	// +optional
	SSHKeys []string `json:"sshKeys,omitempty"`

	// DiskType specifies the type of disk to use (SSD, HDD)
	// +kubebuilder:validation:Enum=SSD;HDD
	// +kubebuilder:default="SSD"
	// +optional
	DiskType string `json:"diskType,omitempty"`

	// RootDiskSize specifies the size of the root disk in GB
	// +kubebuilder:validation:Minimum=20
	// +kubebuilder:validation:Maximum=1000
	// +kubebuilder:default=40
	// +optional
	RootDiskSize int `json:"rootDiskSize,omitempty"`
	// NetworkPlan specifies the network plan for the instances
	// +kubebuilder:validation:Enum=free_datatransfer;paid_datatransfer
	// +kubebuilder:default="free_datatransfer"
	// +optional
	NetworkPlan string `json:"networkPlan,omitempty"`
	// VPCNetworkIDs is the list of VPC network IDs to attach to the nodes
	// +optional
	VPCNetworkIDs []string `json:"vpcNetworkIds,omitempty"`

	// PlacementStrategy defines how nodes should be placed across zones
	// Only used when Zone or Subnet is not specified
	// +optional
	PlacementStrategy *PlacementStrategy `json:"placementStrategy,omitempty"`

	// Tags to apply to the VMs
	// +optional
	Tags []string `json:"tags,omitempty"`

	// MetadataOptions for the generated launch template of provisioned nodes.
	// +kubebuilder:default={"type":"template"}
	// +optional
	MetadataOptions *MetadataOptions `json:"metadataOptions,omitempty"`

	// SecurityGroups to apply to the VMs
	// +kubebuilder:validation:MaxItems:=10
	// +optional
	SecurityGroups []SecurityGroupsTerm `json:"securityGroups,omitempty"`
}

// Rest of your existing types remain the same...
type PlacementStrategy struct {
	Spread *SpreadStrategy `json:"spread,omitempty"`
}

type SpreadStrategy struct {
	Zones []string `json:"zones,omitempty"`
}

type MetadataOptions struct {
	Type string `json:"type"`
}

type SecurityGroupsTerm struct {
	SecurityGroupID string `json:"securityGroupID"`
}

// BizflyCloudNodeClassList contains a list of BizflyCloudNodeClass
// +kubebuilder:object:root=true
type BizflyCloudNodeClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BizflyCloudNodeClass `json:"items"`
}
