package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BizflyCloudNodeClass is the Schema for the BizflyCloudNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""
// +kubebuilder:printcolumn:name="Role",type="string",JSONPath=".spec.role",priority=1,description=""
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
	// Region is the Proxmox Cloud region where nodes will be created
	// +optional
	Region string `json:"region"`

	// Zone is the availability zone where nodes will be created
	// If not specified, zones will be automatically selected based on placement strategy
	// +optional
	Zone string `json:"zone,omitempty"`

	// Template is the name of the template to use for nodes
	// +required
	Template string `json:"template"`

	// BlockDevicesStorageID is the storage ID to create/clone the VM
	// +required
	BlockDevicesStorageID string `json:"blockDevicesStorageID,omitempty"`

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
