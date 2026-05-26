package clients

import (
	"context"
	"net/http"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/mongodb-forks/digest"
	"github.com/pkg/errors"
	"go.mongodb.org/ops-manager/opsmngr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
)

// Credentials holds the public/private API key pair.
type Credentials struct {
	PublicKey  string
	PrivateKey string
}

// NewClient creates an authenticated Ops Manager client using HTTP Digest Auth.
// The Ops Manager API authenticates via digest auth with publicKey as the
// username and privateKey as the password.
func NewClient(baseURL string, creds *Credentials) (*opsmngr.Client, error) {
	transport := digest.NewTransport(creds.PublicKey, creds.PrivateKey)
	httpClient := &http.Client{Transport: transport}

	return opsmngr.New(httpClient, opsmngr.SetBaseURL(baseURL))
}

// GetCredentials reads the public and private API keys from the Kubernetes
// secrets referenced by the ProviderConfig.
func GetCredentials(ctx context.Context, kube client.Client, pc *v1beta1.ProviderConfig) (*Credentials, error) {
	cd := pc.Spec.Credentials
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
