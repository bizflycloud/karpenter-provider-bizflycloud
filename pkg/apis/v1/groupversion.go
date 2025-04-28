package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"

	"github.com/bizflycloud/karpenter-provider-bizflycloud/pkg/apis"
)

var (
	GroupName          = apis.Group
	SchemeGroupVersion = schema.GroupVersion{Group: GroupName, Version: "v1"}
	SchemeBuilder      = &scheme.Builder{GroupVersion: SchemeGroupVersion}
	AddToScheme        = SchemeBuilder.AddToScheme
)
