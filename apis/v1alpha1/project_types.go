package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
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
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       OpsManagerProjectParameters `json:"forProvider"`
}

// OpsManagerProjectStatus defines the observed state of an OpsManagerProject.
type OpsManagerProjectStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             OpsManagerProjectObservation `json:"atProvider,omitempty"`
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

// --- resource.Managed interface forwarding methods ---

// GetCondition of this OpsManagerProject.
func (mg *OpsManagerProject) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return mg.Status.GetCondition(ct)
}

// SetConditions of this OpsManagerProject.
func (mg *OpsManagerProject) SetConditions(c ...xpv1.Condition) {
	mg.Status.SetConditions(c...)
}

// GetDeletionPolicy of this OpsManagerProject.
func (mg *OpsManagerProject) GetDeletionPolicy() xpv1.DeletionPolicy {
	return mg.Spec.DeletionPolicy
}

// SetDeletionPolicy of this OpsManagerProject.
func (mg *OpsManagerProject) SetDeletionPolicy(r xpv1.DeletionPolicy) {
	mg.Spec.DeletionPolicy = r
}

// GetManagementPolicies of this OpsManagerProject.
func (mg *OpsManagerProject) GetManagementPolicies() xpv1.ManagementPolicies {
	return mg.Spec.ManagementPolicies
}

// SetManagementPolicies of this OpsManagerProject.
func (mg *OpsManagerProject) SetManagementPolicies(r xpv1.ManagementPolicies) {
	mg.Spec.ManagementPolicies = r
}

// GetProviderReference of this OpsManagerProject.
func (mg *OpsManagerProject) GetProviderReference() *xpv1.Reference {
	return mg.Spec.ProviderReference
}

// SetProviderReference of this OpsManagerProject.
func (mg *OpsManagerProject) SetProviderReference(r *xpv1.Reference) {
	mg.Spec.ProviderReference = r
}

// GetProviderConfigReference of this OpsManagerProject.
func (mg *OpsManagerProject) GetProviderConfigReference() *xpv1.Reference {
	return mg.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this OpsManagerProject.
func (mg *OpsManagerProject) SetProviderConfigReference(r *xpv1.Reference) {
	mg.Spec.ProviderConfigReference = r
}

// GetPublishConnectionDetailsTo of this OpsManagerProject.
func (mg *OpsManagerProject) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return mg.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this OpsManagerProject.
func (mg *OpsManagerProject) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	mg.Spec.PublishConnectionDetailsTo = r
}

// GetWriteConnectionSecretToReference of this OpsManagerProject.
func (mg *OpsManagerProject) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return mg.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this OpsManagerProject.
func (mg *OpsManagerProject) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	mg.Spec.WriteConnectionSecretToReference = r
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
