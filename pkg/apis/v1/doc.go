// +k8s:openapi-gen=true
// +k8s:deepcopy-gen=package,register
// +k8s:defaulter-gen=TypeMeta
// +groupName=karpenter.bizflycloud.com
package v1

import (
	corev1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/kubernetes/scheme"

	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis"
)

func init() {
	gv := schema.GroupVersion{Group: apis.Group, Version: "v1"}
	corev1.AddToGroupVersion(scheme.Scheme, gv)
	scheme.Scheme.AddKnownTypes(gv,
		&BizflyCloudNodeClass{},
		&BizflyCloudNodeClassList{},
	)
}
