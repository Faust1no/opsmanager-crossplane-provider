// Package v1beta1 contains managed resources and provider config for MongoDB Ops Manager.
// +kubebuilder:object:generate=true
// +groupName=opsmanager.crossplane.io
// +versionName=v1beta1
package v1beta1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// Package type metadata.
const (
	Group   = "opsmanager.crossplane.io"
	Version = "v1beta1"
)

// SchemeGroupVersion is group version used to register these objects.
var SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

// SchemeBuilder is used to add functions to this group's scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}

// ClusterProviderConfig type metadata.
var (
	ClusterProviderConfigGroupKind = schema.GroupKind{
		Group: Group,
		Kind:  "ClusterProviderConfig",
	}
	ClusterProviderConfigGroupVersionKind = schema.GroupVersionKind{
		Group:   Group,
		Version: Version,
		Kind:    "ClusterProviderConfig",
	}
	ClusterProviderConfigUsageGroupVersionKind = schema.GroupVersionKind{
		Group:   Group,
		Version: Version,
		Kind:    "ClusterProviderConfigUsage",
	}
	ClusterProviderConfigUsageListGroupVersionKind = schema.GroupVersionKind{
		Group:   Group,
		Version: Version,
		Kind:    "ClusterProviderConfigUsageList",
	}
)

