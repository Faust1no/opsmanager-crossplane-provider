// Package clients constructs Ops Manager API clients from a ProviderConfig or
// ClusterProviderConfig. It also offers a unified usage tracker that handles
// both ProviderConfig kinds so controllers do not have to duplicate the
// switch-on-kind plumbing themselves.
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

// ProviderConfig kinds accepted in a managed resource's providerConfigRef.
const (
	KindClusterProviderConfig = "ClusterProviderConfig"
	KindProviderConfig        = "ProviderConfig"
)

// Credentials holds the public/private API key pair.
type Credentials struct {
	PublicKey  string
	PrivateKey string
}

// resolvedConfig captures the connection details extracted from either a
// ClusterProviderConfig or a ProviderConfig.
type resolvedConfig struct {
	baseURL string
	creds   ProviderCredentials
}

// ProviderCredentials is a structural copy of v1beta1.ProviderCredentials so
// the resolver can work with both kinds without committing to a single type.
type ProviderCredentials = v1beta1.ProviderCredentials

// NewClient creates an authenticated Ops Manager client using HTTP Digest Auth.
// The Ops Manager API authenticates via digest auth with publicKey as the
// username and privateKey as the password.
func NewClient(baseURL string, creds *Credentials) (*opsmngr.Client, error) {
	transport := digest.NewTransport(creds.PublicKey, creds.PrivateKey)
	httpClient := &http.Client{Transport: transport}
	return opsmngr.New(httpClient, opsmngr.SetBaseURL(baseURL))
}

// Resolve fetches the referenced ProviderConfig (cluster- or namespace-scoped),
// reads the credentials secret, and returns a ready-to-use Ops Manager client.
//
// mrNamespace is the managed resource's namespace, used only for namespace-scoped
// ProviderConfig lookups (it is ignored for ClusterProviderConfig). For
// cluster-scoped managed resources mrNamespace will be the empty string, in
// which case a ref to ProviderConfig is rejected with a clear error.
func Resolve(ctx context.Context, kube client.Client, ref *xpv1.ProviderConfigReference, mrNamespace string) (*opsmngr.Client, string, error) {
	if ref == nil {
		return nil, "", errors.New("managed resource has no providerConfigRef")
	}

	rc, err := lookup(ctx, kube, ref, mrNamespace)
	if err != nil {
		return nil, "", err
	}

	creds, err := readCredentials(ctx, kube, rc.creds)
	if err != nil {
		return nil, rc.baseURL, err
	}

	cli, err := NewClient(rc.baseURL, creds)
	if err != nil {
		return nil, rc.baseURL, errors.Wrap(err, "cannot create Ops Manager client")
	}
	return cli, rc.baseURL, nil
}

func lookup(ctx context.Context, kube client.Client, ref *xpv1.ProviderConfigReference, mrNamespace string) (resolvedConfig, error) {
	switch ref.Kind {
	case KindClusterProviderConfig:
		pc := &v1beta1.ClusterProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Name: ref.Name}, pc); err != nil {
			return resolvedConfig{}, errors.Wrapf(err, "cannot get ClusterProviderConfig %q", ref.Name)
		}
		return resolvedConfig{baseURL: pc.Spec.BaseURL, creds: pc.Spec.Credentials}, nil

	case KindProviderConfig:
		if mrNamespace == "" {
			return resolvedConfig{}, errors.Errorf(
				"cluster-scoped managed resources cannot reference a namespace-scoped ProviderConfig; use kind: ClusterProviderConfig instead")
		}
		pc := &v1beta1.ProviderConfig{}
		if err := kube.Get(ctx, types.NamespacedName{Namespace: mrNamespace, Name: ref.Name}, pc); err != nil {
			return resolvedConfig{}, errors.Wrapf(err, "cannot get ProviderConfig %s/%s", mrNamespace, ref.Name)
		}
		return resolvedConfig{baseURL: pc.Spec.BaseURL, creds: pc.Spec.Credentials}, nil

	default:
		return resolvedConfig{}, errors.Errorf(
			"unsupported providerConfigRef.kind %q; must be %q or %q",
			ref.Kind, KindClusterProviderConfig, KindProviderConfig)
	}
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

// UsageTracker dispatches to either the cluster-scoped or the namespace-scoped
// ProviderConfigUsage tracker based on the managed resource's
// providerConfigRef.kind. It implements resource.Tracker.
type UsageTracker struct {
	cluster    *resource.ProviderConfigUsageTracker
	namespaced *resource.ProviderConfigUsageTracker
}

// NewUsageTracker returns a UsageTracker wired for both ProviderConfig kinds.
func NewUsageTracker(c client.Client) *UsageTracker {
	return &UsageTracker{
		cluster:    resource.NewProviderConfigUsageTracker(c, &v1beta1.ClusterProviderConfigUsage{}),
		namespaced: resource.NewProviderConfigUsageTracker(c, &v1beta1.ProviderConfigUsage{}),
	}
}

// Track records usage against the appropriate ProviderConfigUsage type.
func (u *UsageTracker) Track(ctx context.Context, mg resource.ModernManaged) error {
	ref := mg.GetProviderConfigReference()
	if ref == nil {
		return errors.New("managed resource has no providerConfigRef")
	}
	switch ref.Kind {
	case KindClusterProviderConfig:
		return u.cluster.Track(ctx, mg)
	case KindProviderConfig:
		return u.namespaced.Track(ctx, mg)
	default:
		return errors.Errorf(
			"unsupported providerConfigRef.kind %q; must be %q or %q",
			ref.Kind, KindClusterProviderConfig, KindProviderConfig)
	}
}
