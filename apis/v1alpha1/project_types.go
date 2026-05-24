package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// ProjectGroupKind is the GroupKind for the Project resource.
var ProjectGroupKind = schema.GroupKind{Group: Group, Kind: "Project"}

// ProjectGroupVersionKind is the GroupVersionKind for the Project resource.
var ProjectGroupVersionKind = SchemeGroupVersion.WithKind("Project")

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

// ProjectParameters are the configurable fields of a Project.
type ProjectParameters struct {
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

// ProjectObservation holds the observed state of the Project.
type ProjectObservation struct {
	// ID is the project ID assigned by Ops Manager.
	// +optional
	ID string `json:"id,omitempty"`
}

// ProjectSpec defines the desired state of a Project.
type ProjectSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       ProjectParameters `json:"forProvider"`
}

// ProjectStatus defines the observed state of a Project.
type ProjectStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             ProjectObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="PROJECT-ID",type="string",JSONPath=".status.atProvider.id",priority=1

// Project is a managed resource representing an Ops Manager project.
// It can configure LDAP group permission mappings for the project.
type Project struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProjectSpec   `json:"spec"`
	Status ProjectStatus `json:"status,omitempty"`
}

// --- resource.Managed interface forwarding methods ---

// GetCondition of this Project.
func (mg *Project) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return mg.Status.GetCondition(ct)
}

// SetConditions of this Project.
func (mg *Project) SetConditions(c ...xpv1.Condition) {
	mg.Status.SetConditions(c...)
}

// GetDeletionPolicy of this Project.
func (mg *Project) GetDeletionPolicy() xpv1.DeletionPolicy {
	return mg.Spec.DeletionPolicy
}

// SetDeletionPolicy of this Project.
func (mg *Project) SetDeletionPolicy(r xpv1.DeletionPolicy) {
	mg.Spec.DeletionPolicy = r
}

// GetManagementPolicies of this Project.
func (mg *Project) GetManagementPolicies() xpv1.ManagementPolicies {
	return mg.Spec.ManagementPolicies
}

// SetManagementPolicies of this Project.
func (mg *Project) SetManagementPolicies(r xpv1.ManagementPolicies) {
	mg.Spec.ManagementPolicies = r
}

// GetProviderReference of this Project.
func (mg *Project) GetProviderReference() *xpv1.Reference { return mg.Spec.ProviderReference }

// SetProviderReference of this Project.
func (mg *Project) SetProviderReference(r *xpv1.Reference) { mg.Spec.ProviderReference = r }

// GetProviderConfigReference of this Project.
func (mg *Project) GetProviderConfigReference() *xpv1.Reference {
	return mg.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this Project.
func (mg *Project) SetProviderConfigReference(r *xpv1.Reference) {
	mg.Spec.ProviderConfigReference = r
}

// GetPublishConnectionDetailsTo of this Project.
func (mg *Project) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return mg.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this Project.
func (mg *Project) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	mg.Spec.PublishConnectionDetailsTo = r
}

// GetWriteConnectionSecretToReference of this Project.
func (mg *Project) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return mg.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this Project.
func (mg *Project) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	mg.Spec.WriteConnectionSecretToReference = r
}

// +kubebuilder:object:root=true

// ProjectList contains a list of Project.
type ProjectList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Project `json:"items"`
}

func init() {
	SchemeBuilder.Register(&Project{}, &ProjectList{})
}
