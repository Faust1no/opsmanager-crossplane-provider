package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// OpsManagerProjectGroupKind is the GroupKind for the OpsManagerProject resource.
var OpsManagerProjectGroupKind = schema.GroupKind{Group: Group, Kind: "OpsManagerProject"}

// OpsManagerProjectGroupVersionKind is the GroupVersionKind for the OpsManagerProject resource.
var OpsManagerProjectGroupVersionKind = SchemeGroupVersion.WithKind("OpsManagerProject")

// LDAPGroupMapping maps an Ops Manager role to one or more LDAP groups.
type LDAPGroupMapping struct {
	// RoleName is the Ops Manager project role.
	// Valid values: GROUP_OWNER, GROUP_CLUSTER_MANAGER, GROUP_DATA_ACCESS_ADMIN,
	// GROUP_DATA_ACCESS_READ_WRITE, GROUP_DATA_ACCESS_READ_ONLY, GROUP_READ_ONLY, GROUP_AUTOMATION_ADMIN.
	// +kubebuilder:validation:MinLength=1
	RoleName string `json:"roleName"`

	// LDAPGroups is the list of LDAP group DNs mapped to this role.
	// +kubebuilder:validation:MinItems=1
	LDAPGroups []string `json:"ldapGroups"`
}

// OpsManagerProjectParameters are the configurable fields of an OpsManagerProject.
type OpsManagerProjectParameters struct {
	// Name is the display name for the project in Ops Manager.
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// OrgID is the ID of the organization that owns this project.
	// +kubebuilder:validation:MinLength=1
	OrgID string `json:"orgId"`

	// LDAPGroupMappings maps Ops Manager project roles to LDAP groups.
	// Only applicable when Ops Manager is backed by LDAP.
	// +optional
	LDAPGroupMappings []LDAPGroupMapping `json:"ldapGroupMappings,omitempty"`
}

// OpsManagerProjectObservation holds the observed state of the OpsManagerProject.
type OpsManagerProjectObservation struct {
	// ID is the project ID assigned by Ops Manager.
	// +optional
	ID string `json:"id,omitempty"`
}

// OpsManagerProjectSpec defines the desired state of an OpsManagerProject.
type OpsManagerProjectSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              OpsManagerProjectParameters `json:"forProvider"`
}

// OpsManagerProjectStatus defines the observed state of an OpsManagerProject.
type OpsManagerProjectStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          OpsManagerProjectObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Namespaced,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="PROJECT-ID",type="string",JSONPath=".status.atProvider.id",priority=1

// OpsManagerProject is a managed resource representing an Ops Manager project.
// It can configure LDAP group permission mappings for the project.
type OpsManagerProject struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   OpsManagerProjectSpec   `json:"spec"`
	Status OpsManagerProjectStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// OpsManagerProjectList contains a list of OpsManagerProject.
type OpsManagerProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []OpsManagerProject `json:"items"`
}

func init() {
	SchemeBuilder.Register(&OpsManagerProject{}, &OpsManagerProjectList{})
}
