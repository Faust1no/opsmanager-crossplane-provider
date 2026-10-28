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

	// SnapshotSchedule configures the backup snapshot schedule and retention
	// policy on the project's backup config. All fields are optional; any field
	// left unset is not sent in the PATCH payload, so Ops Manager preserves its
	// current value (either the OM default or whatever was last configured).
	//
	// Requires at least one cluster in the project to have backup enabled; if
	// no backup config exists the resource becomes Synced=False until one does.
	// +optional
	SnapshotSchedule *SnapshotSchedule `json:"snapshotSchedule,omitempty"`
}

// SnapshotSchedule configures the per-cluster backup snapshot schedule and
// retention policy. All fields are optional: a nil field is not sent to the
// Ops Manager API, so OM keeps its existing (or default) value. This matches
// the SDK's SnapshotSchedule shape (every field is also optional on the API).
type SnapshotSchedule struct {
	// SnapshotRetentionDays is the number of days to keep recent snapshots.
	// +optional
	SnapshotRetentionDays *int `json:"snapshotRetentionDays,omitempty"`

	// DailySnapshotRetentionDays is the number of days to keep daily snapshots.
	// Set to 0 to disable daily snapshot retention.
	// +optional
	DailySnapshotRetentionDays *int `json:"dailySnapshotRetentionDays,omitempty"`

	// WeeklySnapshotRetentionWeeks is the number of weeks to keep weekly snapshots.
	// Set to 0 to disable weekly snapshot retention.
	// +optional
	WeeklySnapshotRetentionWeeks *int `json:"weeklySnapshotRetentionWeeks,omitempty"`

	// MonthlySnapshotRetentionMonths is the number of months to keep monthly snapshots.
	// Set to 0 to disable monthly snapshot retention.
	// +optional
	MonthlySnapshotRetentionMonths *int `json:"monthlySnapshotRetentionMonths,omitempty"`

	// PointInTimeWindowHours is the number of hours in the past at which a point-in-time restore is possible.
	// +optional
	PointInTimeWindowHours *int `json:"pointInTimeWindowHours,omitempty"`

	// SnapshotIntervalHours is how often (in hours) snapshots are taken.
	// +optional
	SnapshotIntervalHours *int `json:"snapshotIntervalHours,omitempty"`

	// ClusterCheckpointIntervalMin is how often (in minutes) backup takes
	// cluster checkpoints. Only applicable for sharded clusters.
	// +optional
	ClusterCheckpointIntervalMin *int `json:"clusterCheckpointIntervalMin,omitempty"`

	// ReferenceHourOfDay is the hour of the day at which the daily snapshot is taken (0-23).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=23
	ReferenceHourOfDay *int `json:"referenceHourOfDay,omitempty"`

	// ReferenceMinuteOfHour is the minute of the hour at which the daily snapshot is taken (0-59).
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:validation:Maximum=59
	ReferenceMinuteOfHour *int `json:"referenceMinuteOfHour,omitempty"`
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
// +kubebuilder:resource:scope=Cluster,categories=crossplane
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
