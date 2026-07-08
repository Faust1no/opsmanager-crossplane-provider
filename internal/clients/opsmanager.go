// Package clients constructs Ops Manager API clients from a ClusterProviderConfig
// and offers a usage tracker that keeps ClusterProviderConfigUsage records in
// sync so a PC in use cannot be deleted from under a managed resource.
package clients

import (
	"context"
	"net/http"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/mongodb-forks/digest"
	"github.com/pkg/errors"
	"go.mongodb.org/ops-manager/opsmngr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
)

// KindClusterProviderConfig is the only providerConfigRef.kind accepted by
// managed resources in this provider.
const KindClusterProviderConfig = "ClusterProviderConfig"

// Credentials holds the public/private API key pair.
type Credentials struct {
	PublicKey  string
	PrivateKey string
}

// ProviderCredentials is a structural copy of v1beta1.ProviderCredentials.
type ProviderCredentials = v1beta1.ProviderCredentials

// NewClient creates an authenticated Ops Manager client using HTTP Digest Auth.
// The Ops Manager API authenticates via digest auth with publicKey as the
// username and privateKey as the password.
func NewClient(baseURL string, creds *Credentials) (*opsmngr.Client, error) {
	transport := digest.NewTransport(creds.PublicKey, creds.PrivateKey)
	httpClient := &http.Client{Transport: transport}
	return opsmngr.New(httpClient, opsmngr.SetBaseURL(baseURL))
}

// Resolve fetches the referenced ClusterProviderConfig, reads its credentials
// secret, and returns a ready-to-use Ops Manager client.
func Resolve(ctx context.Context, kube client.Client, ref *xpv1.ProviderConfigReference) (*opsmngr.Client, string, error) {
	if ref == nil {
		return nil, "", errors.New("managed resource has no providerConfigRef")
	}
	if ref.Kind != KindClusterProviderConfig {
		return nil, "", errors.Errorf(
			"unsupported providerConfigRef.kind %q; must be %q",
			ref.Kind, KindClusterProviderConfig)
	}

	pc := &v1beta1.ClusterProviderConfig{}
	if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, pc); err != nil {
		return nil, "", errors.Wrapf(err, "cannot get ClusterProviderConfig %q", ref.Name)
	}

	creds, err := readCredentials(ctx, kube, pc.Spec.Credentials)
	if err != nil {
		return nil, pc.Spec.BaseURL, err
	}

	cli, err := NewClient(pc.Spec.BaseURL, creds)
	if err != nil {
		return nil, pc.Spec.BaseURL, errors.Wrap(err, "cannot create Ops Manager client")
	}
	return cli, pc.Spec.BaseURL, nil
}

func readCredentials(ctx context.Context, kube client.Client, cd ProviderCredentials) (*Credentials, error) {
	if cd.Source != xpv1.CredentialsSourceSecret {
		return nil, errors.New("credentials source must be Secret")
	}
	if cd.PublicKeySecretRef == nil || cd.PrivateKeySecretRef == nil {
		return nil, errors.New("publicKeySecretRef and privateKeySecretRef must both be set")
	}

	publicKey, err := getSecretValue(ctx, kube, cd.PublicKeySecretRef)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get public key")
	}
	privateKey, err := getSecretValue(ctx, kube, cd.PrivateKeySecretRef)
	if err != nil {
		return nil, errors.Wrap(err, "cannot get private key")
	}
	return &Credentials{PublicKey: publicKey, PrivateKey: privateKey}, nil
}

func getSecretValue(ctx context.Context, kube client.Client, ref *xpv1.SecretKeySelector) (string, error) {
	secret := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}, secret); err != nil {
		return "", errors.Wrap(err, "cannot get credentials secret")
	}
	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", errors.Errorf("key %q not found in secret %s/%s", ref.Key, ref.Namespace, ref.Name)
	}
	return string(val), nil
}

// UsageTracker records ClusterProviderConfigUsage against a managed resource.
// It implements resource.Tracker.
type UsageTracker struct {
	tracker *resource.ProviderConfigUsageTracker
}

// NewUsageTracker returns a UsageTracker wired for ClusterProviderConfigUsage.
func NewUsageTracker(c client.Client) *UsageTracker {
	return &UsageTracker{
		tracker: resource.NewProviderConfigUsageTracker(c, &v1beta1.ClusterProviderConfigUsage{}),
	}
}

// Track records usage against the ClusterProviderConfigUsage type.
func (u *UsageTracker) Track(ctx context.Context, mg resource.ModernManaged) error {
	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return errors.New("managed resource has no providerConfigRef")
	}
	if ref.Kind != KindClusterProviderConfig {
		return errors.Errorf(
			"unsupported providerConfigRef.kind %q; must be %q",
			ref.Kind, KindClusterProviderConfig)
	}
	return u.tracker.Track(ctx, mg)
}
