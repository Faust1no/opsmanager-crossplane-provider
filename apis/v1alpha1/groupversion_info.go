// Package v1alpha1 contains managed resources for MongoDB Ops Manager.
// +kubebuilder:object:generate=true
// +groupName=opsmanager.crossplane.io
// +versionName=v1alpha1
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

// Package type metadata.
const (
	Group   = "opsmanager.crossplane.io"
	Version = "v1alpha1"
)

// SchemeGroupVersion is group version used to register these objects.
var SchemeGroupVersion = schema.GroupVersion{Group: Group, Version: Version}

// SchemeBuilder is used to add functions to this group's scheme.
var SchemeBuilder = &scheme.Builder{GroupVersion: SchemeGroupVersion}
