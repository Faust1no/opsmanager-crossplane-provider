package v1alpha1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// S3OplogStoreGroupKind is the GroupKind for the S3OplogStore resource.
var S3OplogStoreGroupKind = schema.GroupKind{Group: Group, Kind: "S3OplogStore"}

// S3OplogStoreGroupVersionKind is the GroupVersionKind for the S3OplogStore resource.
var S3OplogStoreGroupVersionKind = SchemeGroupVersion.WithKind("S3OplogStore")

// S3OplogStoreParameters mirrors the fields of the SDK's S3Blockstore struct
// as used by the oplog S3 store endpoint. The API shape is identical to the
// snapshot blockstore — only the endpoint differs.
type S3OplogStoreParameters struct {
	// ID is the unique identifier for this S3 oplog store in Ops Manager.
	// +kubebuilder:validation:MinLength=1
	ID string `json:"id"`

	// URI is the connection string for the store's MongoDB process (if applicable).
	// +optional
	URI string `json:"uri,omitempty"`

	// Labels are the assignment labels for this oplog store.
	// Backup jobs whose cluster assignmentLabels match will have their oplogs routed here.
	// +optional
	Labels []string `json:"labels,omitempty"`

	// AssignmentEnabled controls whether this store accepts new backup assignments.
	// +optional
	// +kubebuilder:default=true
	AssignmentEnabled *bool `json:"assignmentEnabled,omitempty"`

	// SSL enables SSL for the connection to the store.
	// +optional
	SSL *bool `json:"ssl,omitempty"`

	// WriteConcern is the write concern for the store.
	// +optional
	WriteConcern string `json:"writeConcern,omitempty"`

	// EncryptedCredentials indicates whether the credentials are encrypted.
	// +optional
	EncryptedCredentials *bool `json:"encryptedCredentials,omitempty"`

	// LoadFactor is the relative weight for backup job distribution across stores.
	// +optional
	LoadFactor *int64 `json:"loadFactor,omitempty"`

	// MaxCapacityGB is the maximum storage capacity for this store in GB.
	// +optional
	MaxCapacityGB *int64 `json:"maxCapacityGB,omitempty"`

	// Provisioned indicates whether the store has been provisioned.
	// +optional
	Provisioned *bool `json:"provisioned,omitempty"`

	// SyncSource is the source for syncing the store.
	// +optional
	SyncSource string `json:"syncSource,omitempty"`

	// Username for authentication to the store.
	// +optional
	Username string `json:"username,omitempty"`

	// S3BucketEndpoint is the S3-compatible endpoint URL.
	// +kubebuilder:validation:MinLength=1
	S3BucketEndpoint string `json:"s3BucketEndpoint"`

	// S3BucketName is the name of the S3 bucket used for oplog storage.
	// +kubebuilder:validation:MinLength=1
	S3BucketName string `json:"s3BucketName"`

	// S3AuthMethod controls how the backup agent authenticates to S3.
	// +kubebuilder:validation:Enum=KEYS;IAM_ROLE
	// +kubebuilder:default=KEYS
	S3AuthMethod string `json:"s3AuthMethod"`

	// AWSAccessKey is the AWS access key ID. Required when S3AuthMethod is KEYS.
	// +optional
	AWSAccessKey string `json:"awsAccessKey,omitempty"`

	// AWSSecretKeySecretRef references a Kubernetes Secret containing the AWS secret
	// access key. Required when S3AuthMethod is KEYS.
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

// S3OplogStoreObservation holds read-only state returned by the Ops Manager API.
type S3OplogStoreObservation struct {
	// UsedSize is the storage currently consumed by this oplog store in bytes.
	// +optional
	UsedSize int64 `json:"usedSize,omitempty"`
}

// S3OplogStoreSpec defines the desired state of an S3OplogStore.
type S3OplogStoreSpec struct {
	xpv1.ResourceSpec `json:",inline"`
	ForProvider       S3OplogStoreParameters `json:"forProvider"`
}

// S3OplogStoreStatus defines the observed state of an S3OplogStore.
type S3OplogStoreStatus struct {
	xpv1.ConditionedStatus `json:",inline"`
	AtProvider             S3OplogStoreObservation `json:"atProvider,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="READY",type="string",JSONPath=".status.conditions[?(@.type=='Ready')].status"
// +kubebuilder:printcolumn:name="SYNCED",type="string",JSONPath=".status.conditions[?(@.type=='Synced')].status"
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="BUCKET",type="string",JSONPath=".spec.forProvider.s3BucketName",priority=1

// S3OplogStore is a managed resource representing an S3-backed oplog store
// in MongoDB Ops Manager. In a multi-cluster setup this store is typically shared —
// each new cluster appends its label so its oplogs are routed here.
type S3OplogStore struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   S3OplogStoreSpec   `json:"spec"`
	Status S3OplogStoreStatus `json:"status,omitempty"`
}

func (mg *S3OplogStore) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return mg.Status.GetCondition(ct)
}
func (mg *S3OplogStore) SetConditions(c ...xpv1.Condition) { mg.Status.SetConditions(c...) }
func (mg *S3OplogStore) GetDeletionPolicy() xpv1.DeletionPolicy {
	return mg.Spec.DeletionPolicy
}
func (mg *S3OplogStore) SetDeletionPolicy(r xpv1.DeletionPolicy) { mg.Spec.DeletionPolicy = r }
func (mg *S3OplogStore) GetManagementPolicies() xpv1.ManagementPolicies {
	return mg.Spec.ManagementPolicies
}
func (mg *S3OplogStore) SetManagementPolicies(r xpv1.ManagementPolicies) {
	mg.Spec.ManagementPolicies = r
}
func (mg *S3OplogStore) GetProviderReference() *xpv1.Reference { return mg.Spec.ProviderReference }
func (mg *S3OplogStore) SetProviderReference(r *xpv1.Reference) { mg.Spec.ProviderReference = r }
func (mg *S3OplogStore) GetProviderConfigReference() *xpv1.Reference {
	return mg.Spec.ProviderConfigReference
}
func (mg *S3OplogStore) SetProviderConfigReference(r *xpv1.Reference) {
	mg.Spec.ProviderConfigReference = r
}
func (mg *S3OplogStore) GetPublishConnectionDetailsTo() *xpv1.PublishConnectionDetailsTo {
	return mg.Spec.PublishConnectionDetailsTo
}
func (mg *S3OplogStore) SetPublishConnectionDetailsTo(r *xpv1.PublishConnectionDetailsTo) {
	mg.Spec.PublishConnectionDetailsTo = r
}
func (mg *S3OplogStore) GetWriteConnectionSecretToReference() *xpv1.SecretReference {
	return mg.Spec.WriteConnectionSecretToReference
}
func (mg *S3OplogStore) SetWriteConnectionSecretToReference(r *xpv1.SecretReference) {
	mg.Spec.WriteConnectionSecretToReference = r
}

// +kubebuilder:object:root=true

// S3OplogStoreList contains a list of S3OplogStore.
type S3OplogStoreList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []S3OplogStore `json:"items"`
}

func init() {
	SchemeBuilder.Register(&S3OplogStore{}, &S3OplogStoreList{})
}
