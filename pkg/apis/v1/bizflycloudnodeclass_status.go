package v1

import (
	"github.com/awslabs/operatorpkg/status"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// BizflyCloudNodeClassStatus defines the observed state of BizflyCloudNodeClass
type BizflyCloudNodeClassStatus struct {
	// Conditions represents the latest available observations of the BizflyCloudNodeClass's current state
	// +optional
	Conditions []metav1.Condition `json:"conditions,omitempty"`

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
	conditions := make([]status.Condition, len(n.Status.Conditions))
	for i, c := range n.Status.Conditions {
		conditions[i] = status.Condition{
			Type:    c.Type,
			Status:  c.Status,
			Reason:  c.Reason,
			Message: c.Message,
		}
	}
	return conditions
}

// SetConditions sets the conditions of the BizflyCloudNodeClass
func (n *BizflyCloudNodeClass) SetConditions(conditions []status.Condition) {
	n.Status.Conditions = make([]metav1.Condition, len(conditions))
	for i, c := range conditions {
		n.Status.Conditions[i] = metav1.Condition{
			Type:    c.Type,
			Status:  c.Status,
			Reason:  c.Reason,
			Message: c.Message,
		}
	}
}

// StatusConditions returns the condition set for the BizflyCloudNodeClass
func (n *BizflyCloudNodeClass) StatusConditions() status.ConditionSet {
	return status.NewReadyConditions("Failed").For(n)
}
