package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// BackupDaemonGroupKind is the GroupKind for the BackupDaemon resource.
var BackupDaemonGroupKind = schema.GroupKind{Group: Group, Kind: "BackupDaemon"}

// BackupDaemonGroupVersionKind is the GroupVersionKind for the BackupDaemon resource.
var BackupDaemonGroupVersionKind = SchemeGroupVersion.WithKind("BackupDaemon")

// BackupDaemonParameters mirrors the writable fields of the SDK's Daemon struct
// (which embeds AdminBackupConfig).
type BackupDaemonParameters struct {
	// Machine is the hostname of the backup daemon as shown in Ops Manager
	// (Admin → Backup → Daemons). This is the pod DNS name without the head directory.
	// Example: "ops-manager-backup-daemon-0.ops-manager-backup-daemon-svc.mongodb.svc.cluster.local"
	// +kubebuilder:validation:MinLength=1
	Machine string `json:"machine"`

	// --- From AdminBackupConfig ---

	// Labels are the assignment labels for this daemon.
	// Backup jobs for clusters whose labels match will be handled by this daemon.
	// +optional
	Labels []string `json:"labels,omitempty"`

	// AssignmentEnabled controls whether this daemon accepts new backup job assignments.
	// +optional
	AssignmentEnabled *bool `json:"assignmentEnabled,omitempty"`

	// URI is the connection string for this daemon's MongoDB process.
	// +optional
	URI string `json:"uri,omitempty"`

	// WriteConcern is the write concern for the daemon.
	// +optional
	WriteConcern string `json:"writeConcern,omitempty"`

	// SSL enables SSL for the connection to the daemon.
	// +optional
	SSL *bool `json:"ssl,omitempty"`

	// EncryptedCredentials indicates whether the credentials are encrypted.
	// +optional
	EncryptedCredentials *bool `json:"encryptedCredentials,omitempty"`

	// --- From Daemon ---

	// BackupJobsEnabled controls whether backup jobs run on this daemon.
	// +optional
	BackupJobsEnabled *bool `json:"backupJobsEnabled,omitempty"`

	// GarbageCollectionEnabled controls whether garbage collection runs on this daemon.
	// +optional
	GarbageCollectionEnabled *bool `json:"garbageCollectionEnabled,omitempty"`

	// ResourceUsageEnabled controls whether resource usage reporting is enabled.
	// +optional
	ResourceUsageEnabled *bool `json:"resourceUsageEnabled,omitempty"`

	// RestoreQueryableJobsEnabled controls whether queryable restore jobs run on this daemon.
	// +optional
	RestoreQueryableJobsEnabled *bool `json:"restoreQueryableJobsEnabled,omitempty"`

	// HeadDiskType is the disk type for the daemon's head directory.
	// +optional
	HeadDiskType string `json:"headDiskType,omitempty"`

	// NumWorkers is the number of worker threads for this daemon.
	// +optional
	NumWorkers int64 `json:"numWorkers,omitempty"`

	// HeadRootDirectory is the root directory for the daemon's head files.
	// +optional
	HeadRootDirectory string `json:"headRootDirectory,omitempty"`
}

// BackupDaemonObservation holds read-only state returned by the Ops Manager API.
type BackupDaemonObservation struct {
	// Configured indicates whether the daemon has been fully configured.
	// +optional
	Configured bool `json:"configured,omitempty"`
}

// BackupDaemonSpec defines the desired state of a BackupDaemon.
type BackupDaemonSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       BackupDaemonParameters `json:"forProvider"`
}

// BackupDaemonStatus defines the observed state of a BackupDaemon.
type BackupDaemonStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             BackupDaemonObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="MACHINE",type="string",JSONPath=".spec.forProvider.machine",priority=1

// BackupDaemon is a managed resource representing a Backup Daemon configuration
// in MongoDB Ops Manager. It is used to set the assignment labels on a daemon
// so that it handles backup jobs for specific MongoDB clusters.
type BackupDaemon struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   BackupDaemonSpec   `json:"spec"`
	Status BackupDaemonStatus `json:"status,omitempty"`
}

// --- resource.Managed interface forwarding methods ---

// GetCondition of this BackupDaemon.
func (mg *BackupDaemon) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return mg.Status.GetCondition(ct)
}

// SetConditions of this BackupDaemon.
func (mg *BackupDaemon) SetConditions(c ...xpv1.Condition) {
	mg.Status.SetConditions(c...)
}

// GetDeletionPolicy of this BackupDaemon.
func (mg *BackupDaemon) GetDeletionPolicy() xpv1.DeletionPolicy {
	return mg.Spec.DeletionPolicy
}

// SetDeletionPolicy of this BackupDaemon.
func (mg *BackupDaemon) SetDeletionPolicy(r xpv1.DeletionPolicy) {
	mg.Spec.DeletionPolicy = r
}

// GetManagementPolicies of this BackupDaemon.
func (mg *BackupDaemon) GetManagementPolicies() xpv1.ManagementPolicies {
	return mg.Spec.ManagementPolicies
}

// SetManagementPolicies of this BackupDaemon.
func (mg *BackupDaemon) SetManagementPolicies(r xpv1.ManagementPolicies) {
	mg.Spec.ManagementPolicies = r
}

// GetProviderReference of this BackupDaemon.
func (mg *BackupDaemon) GetProviderReference() *xpv1.Reference { return mg.Spec.ProviderReference }

// SetProviderReference of this BackupDaemon.
func (mg *BackupDaemon) SetProviderReference(r *xpv1.Reference) { mg.Spec.ProviderReference = r }

// GetProviderConfigReference of this BackupDaemon.
func (mg *BackupDaemon) GetProviderConfigReference() *xpv1.Reference {
	return mg.Spec.ProviderConfigReference
}

// SetProviderConfigReference of this BackupDaemon.
func (mg *BackupDaemon) SetProviderConfigReference(r *xpv1.Reference) {
	mg.Spec.ProviderConfigReference = r
}

// GetPublishConnectionDetailsTo of this BackupDaemon.
func (mg *BackupDaemon) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return mg.Spec.PublishConnectionDetailsTo
}

// SetPublishConnectionDetailsTo of this BackupDaemon.
func (mg *BackupDaemon) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	mg.Spec.PublishConnectionDetailsTo = r
}

// GetWriteConnectionSecretToReference of this BackupDaemon.
func (mg *BackupDaemon) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return mg.Spec.WriteConnectionSecretToReference
}

// SetWriteConnectionSecretToReference of this BackupDaemon.
func (mg *BackupDaemon) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	mg.Spec.WriteConnectionSecretToReference = r
}

// +kubebuilder:object:root=true

// BackupDaemonList contains a list of BackupDaemon.
type BackupDaemonList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []BackupDaemon `json:"items"`
}

func init() {
	SchemeBuilder.Register(&BackupDaemon{}, &BackupDaemonList{})
}
