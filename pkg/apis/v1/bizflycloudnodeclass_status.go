package v1

import (
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

const (
	ConditionTypeInstanceReady = "InstanceReady"
)

// BizflyCloudNodeClassStatus defines the observed state of BizflyCloudNodeClass
type BizflyCloudNodeClassStatus struct {
	// Conditions represents the latest available observations of the BizflyCloudNodeClass's current state
	// +optional
	Conditions []status.Condition `json:"conditions,omitempty"`

	// LastUpdated is the timestamp of the last update to the status
	// +optional
	LastUpdated metav1.Time `json:"lastUpdated,omitempty"`

	// InstanceTypes is the list of instance types discovered from BizFly Cloud
	// +optional
	InstanceTypes []string `json:"instanceTypes,omitempty"`

	// Regions is the list of available regions
	// +optional
	Regions []string `json:"regions,omitempty"`
}

// GetConditions returns the conditions of the BizflyCloudNodeClass
func (n *BizflyCloudNodeClass) GetConditions() []status.Condition {
	return n.Status.Conditions
}

// SetConditions sets the conditions of the BizflyCloudNodeClass
func (n *BizflyCloudNodeClass) SetConditions(conditions []status.Condition) {
	n.Status.Conditions = conditions
}

// StatusConditions returns the condition set for the BizflyCloudNodeClass
func (n *BizflyCloudNodeClass) StatusConditions() status.ConditionSet {
	conds := []string{
		ConditionTypeInstanceReady,
	}
	return status.NewReadyConditions(conds...).For(n)
}
