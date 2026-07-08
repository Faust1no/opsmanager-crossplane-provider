package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// S3BlockstoreGroupKind is the GroupKind for the S3Blockstore resource.
var S3BlockstoreGroupKind = schema.GroupKind{Group: Group, Kind: "S3Blockstore"}

// S3BlockstoreGroupVersionKind is the GroupVersionKind for the S3Blockstore resource.
var S3BlockstoreGroupVersionKind = SchemeGroupVersion.WithKind("S3Blockstore")

// S3BlockstoreParameters mirrors the fields of the SDK's S3Blockstore struct
// (which embeds BackupStore -> AdminBackupConfig), covering all writable API fields.
type S3BlockstoreParameters struct {
	// ID is the unique identifier for this S3 blockstore in Ops Manager.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// URI is the connection string for the blockstore's MongoDB process (if applicable).
	// +optional
	URI string `json:"uri,omitempty"`

	// Labels are the assignment labels for this blockstore.
	// Backup jobs whose cluster assignmentLabels match will be routed to this store.
	// +optional
	Labels []string `json:"labels,omitempty"`

	// AssignmentEnabled controls whether this blockstore accepts new backup assignments.
	// +optional
	// +kubebuilder:default=true
	AssignmentEnabled *bool `json:"assignmentEnabled,omitempty"`

	// SSL enables SSL for the connection to the blockstore.
	// +optional
	SSL *bool `json:"ssl,omitempty"`

	// WriteConcern is the write concern for the blockstore.
	// +optional
	WriteConcern string `json:"writeConcern,omitempty"`

	// EncryptedCredentials indicates whether the credentials are encrypted.
	// +optional
	EncryptedCredentials *bool `json:"encryptedCredentials,omitempty"`

	// LoadFactor is the relative weight for backup job distribution across stores.
	// +optional
	LoadFactor *int64 `json:"loadFactor,omitempty"`

	// MaxCapacityGB is the maximum storage capacity for this blockstore in GB.
	// +optional
	MaxCapacityGB *int64 `json:"maxCapacityGB,omitempty"`

	// Provisioned indicates whether the blockstore has been provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`

	// SyncSource is the source for syncing the blockstore.
	// +optional
	SyncSource string `json:"syncSource,omitempty"`

	// Username for authentication to the blockstore.
	// +optional
	Username string `json:"username,omitempty"`

	// S3BucketEndpoint is the S3-compatible endpoint URL.
	// +kubebuilder:validation:MinLength=1
	S3BucketEndpoint string `json:"s3BucketEndpoint"`

	// S3BucketName is the name of the S3 bucket used for backup storage.
	// +kubebuilder:validation:MinLength=1
	S3BucketName string `json:"s3BucketName"`

	// S3AuthMethod controls how the backup agent authenticates to S3.
	// Use "KEYS" for access/secret key auth or "IAM_ROLE" for instance role.
	// +kubebuilder:validation:Enum=KEYS;IAM_ROLE
	// +kubebuilder:default=KEYS
	S3AuthMethod string `json:"s3AuthMethod"`

	// AWSAccessKey is the AWS access key ID. Required when S3AuthMethod is KEYS.
	// +optional
	AWSAccessKey string `json:"awsAccessKey,omitempty"`

	// AWSSecretKeySecretRef references a Kubernetes Secret containing the AWS secret
	// access key under the specified key. Required when S3AuthMethod is KEYS.
	// +optional
	AWSSecretKeySecretRef *xpv1.SecretKeySelector `json:"awsSecretKeySecretRef,omitempty"`

	// S3MaxConnections is the maximum number of concurrent S3 connections.
	// +optional
	S3MaxConnections int64 `json:"s3MaxConnections,omitempty"`

	// PathStyleAccessEnabled enables path-style S3 URLs.
	// Required for non-AWS S3-compatible endpoints.
	// +optional
	PathStyleAccessEnabled *bool `json:"pathStyleAccessEnabled,omitempty"`

	// SSEEnabled enables server-side encryption on the S3 bucket.
	// +optional
	SSEEnabled *bool `json:"sseEnabled,omitempty"`

	// AcceptedTos indicates acceptance of the S3 terms of service.
	// +optional
	AcceptedTos *bool `json:"acceptedTos,omitempty"`

	// DisableProxyS3 disables proxy usage for S3 connections.
	// +optional
	DisableProxyS3 *bool `json:"disableProxyS3,omitempty"`
}

// S3BlockstoreObservation holds read-only state returned by the Ops Manager API.
type S3BlockstoreObservation struct {
	// UsedSize is the storage currently consumed by this blockstore in bytes.
	// +optional
	UsedSize int64 `json:"usedSize,omitempty"`
}

// S3BlockstoreSpec defines the desired state of an S3Blockstore.
type S3BlockstoreSpec struct {
	xpv2.ManagedResourceSpec `json:",inline"`
	ForProvider              S3BlockstoreParameters `json:"forProvider"`
}

// S3BlockstoreStatus defines the observed state of an S3Blockstore.
type S3BlockstoreStatus struct {
	xpv1.ResourceStatus `json:",inline"`
	AtProvider          S3BlockstoreObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="BUCKET",type="string",JSONPath=".spec.forProvider.s3BucketName",priority=1

// S3Blockstore is a managed resource representing an S3-backed backup blockstore
// in MongoDB Ops Manager. The labels field controls which backup daemons and
// MongoDB clusters route their backups to this store.
type S3Blockstore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3BlockstoreSpec   `json:"spec"`
	Status S3BlockstoreStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// S3BlockstoreList contains a list of S3Blockstore.
type S3BlockstoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3Blockstore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3Blockstore{}, &S3BlockstoreList{})
}
