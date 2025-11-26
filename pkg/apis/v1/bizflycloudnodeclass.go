package v1

import (
    metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BizflyCloudNodeClass is the Schema for the BizflyCloudNodeClass API
// +kubebuilder:object:root=true
// +kubebuilder:printcolumn:name="Ready",type="string",JSONPath=".status.conditions[?(@.type==\"Ready\")].status",description=""
// +kubebuilder:printcolumn:name="Age",type="date",JSONPath=".metadata.creationTimestamp",description=""
// +kubebuilder:printcolumn:name="Category",type="string",JSONPath=".spec.nodeCategory",priority=1,description=""
// +kubebuilder:printcolumn:name="CPU Vendors",type="string",JSONPath=".spec.cpuVendors",priority=1,description=""
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

    // Zones is the list of availability zones where nodes can be created
    // +optional
    Zones []string `json:"zones,omitempty"`

    // Template is the name of the template to use for nodes
    // +required
    Template string `json:"template"`

    // OSType specifies the operating system for the nodes (flatcar or ubuntu)
    // +kubebuilder:validation:Enum=flatcar;ubuntu
    // +kubebuilder:default=ubuntu
    // +optional
    OSType []string `json:"osType"`

    // NodeCategories specifies the category of nodes to provision
    // +optional
    NodeCategories []string `json:"nodeCategories,omitempty"`

    // CPUVendors specifies the CPU vendors to use (e.g., Intel2, AMD4)
    // +kubebuilder:validation:Enum=Intel2;AMD4
    // +optional
    CPUVendors []string `json:"cpuVendors,omitempty"`

    // SSHKeyName is the name of the SSH key pair to use for instances
    // +optional
    SSHKeyName string `json:"sshKeyName,omitempty"`

    // SSHKeys is a list of SSH public keys to inject into instances
    // +optional
    SSHKeys []string `json:"sshKeys,omitempty"`

    // DiskTypes specifies the type of disk to use (e.g., SSD, HDD)
    // +optional
    DiskTypes []string `json:"diskTypes,omitempty"`

    // RootDiskSize specifies the size of the root disk in GB
    // +kubebuilder:validation:Minimum=20
    // +kubebuilder:default=40
    // +optional
    RootDiskSize int `json:"rootDiskSize,omitempty"`

    // NetworkPlans specifies the list of network plans for the instances.
    // +optional
    NetworkPlans []string `json:"networkPlans,omitempty"`

    // VPCNetworkIDs is the list of VPC network IDs to attach to the nodes
    // +optional
    VPCNetworkIDs []string `json:"vpcNetworkIds,omitempty"`

    // PlacementStrategy defines how nodes should be placed across zones
    // +optional
    PlacementStrategy *PlacementStrategy `json:"placementStrategy,omitempty"`

    // Tags to apply to the VMs
    // +optional
    Tags []string `json:"tags,omitempty"`

    // MetadataOptions for the generated launch template of provisioned nodes.
    // +optional
    MetadataOptions *MetadataOptions `json:"metadataOptions,omitempty"`

    // SecurityGroups to apply to the VMs
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
