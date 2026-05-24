package v1beta1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderConfigSpec defines the desired state of a ProviderConfig.
type ProviderConfigSpec struct {
	// BaseURL is the base URL of the MongoDB Ops Manager API.
	// e.g. "https://my-ops-manager.example.com/"
	// +kubebuilder:validation:MinLength=1
	BaseURL string `json:"baseURL"`

	// Credentials required to authenticate to the Ops Manager API.
	// The referenced secret must contain "publicKey" and "privateKey" keys.
	Credentials ProviderCredentials `json:"credentials"`
}

// ProviderCredentials required to authenticate.
type ProviderCredentials struct {
	// Source of the provider credentials.
	// +kubebuilder:validation:Enum=None;Secret
	Source xpv1.CredentialsSource `json:"source"`

	// SecretRef references a Kubernetes Secret that contains the credentials.
	// The secret must have keys "publicKey" and "privateKey".
	// +optional
	SecretRef *xpv1.SecretKeySelector `json:"secretRef,omitempty"`
}

// ProviderConfigStatus represents the observed state of a ProviderConfig.
type ProviderConfigStatus struct {
	xpv1.ConditionedStatus `json:",inline"`

	// Users is the number of managed resources currently using this ProviderConfig.
	// +optional
	Users int64 `json:"users,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="SECRET-NAME",type="string",JSONPath=".spec.credentials.secretRef.name",priority=1

// ProviderConfig configures a MongoDB Ops Manager provider.
type ProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ProviderConfigSpec   `json:"spec"`
	Status ProviderConfigStatus `json:"status,omitempty"`
}

// GetCondition of this ProviderConfig.
func (pc *ProviderConfig) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return pc.Status.GetCondition(ct)
}

// SetConditions of this ProviderConfig.
func (pc *ProviderConfig) SetConditions(c ...xpv1.Condition) {
	pc.Status.SetConditions(c...)
}

// GetUsers returns the number of managed resources using this ProviderConfig.
func (pc *ProviderConfig) GetUsers() int64 {
	return pc.Status.Users
}

// SetUsers sets the number of managed resources using this ProviderConfig.
func (pc *ProviderConfig) SetUsers(i int64) {
	pc.Status.Users = i
}

// +kubebuilder:object:root=true

// ProviderConfigList contains a list of ProviderConfig.
type ProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:resource:scope=Cluster,categories=crossplane

// ProviderConfigUsage indicates that a resource is using a ProviderConfig.
type ProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	// ProviderConfigReference to the provider config being used.
	ProviderConfigReference xpv1.Reference `json:"providerConfigRef"`

	// ResourceReference to the managed resource using the provider config.
	ResourceReference xpv1.TypedReference `json:"resourceRef"`
}

// GetProviderConfigReference of this ProviderConfigUsage.
func (p *ProviderConfigUsage) GetProviderConfigReference() xpv1.Reference {
	return p.ProviderConfigReference
}

// SetProviderConfigReference of this ProviderConfigUsage.
func (p *ProviderConfigUsage) SetProviderConfigReference(r xpv1.Reference) {
	p.ProviderConfigReference = r
}

// GetResourceReference of this ProviderConfigUsage.
func (p *ProviderConfigUsage) GetResourceReference() xpv1.TypedReference {
	return p.ResourceReference
}

// SetResourceReference of this ProviderConfigUsage.
func (p *ProviderConfigUsage) SetResourceReference(r xpv1.TypedReference) {
	p.ResourceReference = r
}

// +kubebuilder:object:root=true

// ProviderConfigUsageList contains a list of ProviderConfigUsage.
type ProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ProviderConfigUsage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ProviderConfig{}, &ProviderConfigList{})
	SchemeBuilder.Register(&ProviderConfigUsage{}, &ProviderConfigUsageList{})
}
