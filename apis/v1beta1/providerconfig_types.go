// Package v1beta1 contains the ProviderConfig types for the Ops Manager
// provider.
//
// Only ClusterProviderConfig is supported: every managed resource in this
// provider is cluster-scoped and represents an OM-global singleton, so a
// cluster-wide provider config is the correct data model. See the CHANGELOG
// v3.0.0 entry for the removal of the previously-supported namespace-scoped
// ProviderConfig.
package v1beta1

import (
	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	xpv2 "github.com/crossplane/crossplane-runtime/v2/apis/common/v2"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// ProviderConfigSpec defines the desired state of a ClusterProviderConfig.
// The name is kept generic (not "ClusterProviderConfigSpec") to leave room for
// a future re-introduction of a namespace-scoped variant sharing the same spec.
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

	// PublicKeySecretRef references the secret key containing the Ops Manager API public key.
	// +optional
	PublicKeySecretRef *xpv1.SecretKeySelector `json:"publicKeySecretRef,omitempty"`

	// PrivateKeySecretRef references the secret key containing the Ops Manager API private key.
	// +optional
	PrivateKeySecretRef *xpv1.SecretKeySelector `json:"privateKeySecretRef,omitempty"`
}

// ClusterProviderConfigSpec is an alias of ProviderConfigSpec kept for
// symmetry with the Status type.
type ClusterProviderConfigSpec = ProviderConfigSpec

// ClusterProviderConfigStatus represents the observed state of a ClusterProviderConfig.
type ClusterProviderConfigStatus struct {
	xpv1.ProviderConfigStatus `json:",inline"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="URL",type="string",JSONPath=".spec.baseURL"

// ClusterProviderConfig configures a MongoDB Ops Manager provider for the whole cluster.
type ClusterProviderConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   ClusterProviderConfigSpec   `json:"spec"`
	Status ClusterProviderConfigStatus `json:"status,omitempty"`
}

// GetCondition of this ClusterProviderConfig.
func (pc *ClusterProviderConfig) GetCondition(ct xpv1.ConditionType) xpv1.Condition {
	return pc.Status.GetCondition(ct)
}

// SetConditions of this ClusterProviderConfig.
func (pc *ClusterProviderConfig) SetConditions(c ...xpv1.Condition) {
	pc.Status.SetConditions(c...)
}

// GetUsers returns the number of managed resources using this ClusterProviderConfig.
func (pc *ClusterProviderConfig) GetUsers() int64 { return pc.Status.Users }

// SetUsers sets the number of managed resources using this ClusterProviderConfig.
func (pc *ClusterProviderConfig) SetUsers(i int64) { pc.Status.Users = i }

// +kubebuilder:object:root=true

// ClusterProviderConfigList contains a list of ClusterProviderConfig.
type ClusterProviderConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProviderConfig `json:"items"`
}

// +kubebuilder:object:root=true
// +kubebuilder:storageversion
// +kubebuilder:resource:scope=Cluster,categories=crossplane
// +kubebuilder:printcolumn:name="AGE",type="date",JSONPath=".metadata.creationTimestamp"
// +kubebuilder:printcolumn:name="CONFIG-NAME",type="string",JSONPath=".providerConfigRef.name"
// +kubebuilder:printcolumn:name="RESOURCE-KIND",type="string",JSONPath=".resourceRef.kind"
// +kubebuilder:printcolumn:name="RESOURCE-NAME",type="string",JSONPath=".resourceRef.name"

// ClusterProviderConfigUsage indicates that a resource is using a ClusterProviderConfig.
type ClusterProviderConfigUsage struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	xpv2.TypedProviderConfigUsage `json:",inline"`
}

// +kubebuilder:object:root=true

// ClusterProviderConfigUsageList contains a list of ClusterProviderConfigUsage.
type ClusterProviderConfigUsageList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ClusterProviderConfigUsage `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ClusterProviderConfig{}, &ClusterProviderConfigList{})
	SchemeBuilder.Register(&ClusterProviderConfigUsage{}, &ClusterProviderConfigUsageList{})
}
