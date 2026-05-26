package s3blockstore

import (
	"context"
	stderrors "errors"
	"net/http"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"go.mongodb.org/ops-manager/opsmngr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/clients"
)

const (
	errNotS3Blockstore   = "managed resource is not an S3Blockstore"
	errGetProviderConfig = "cannot get ProviderConfig"
	errCreateClient      = "cannot create Ops Manager client"
	errTrackUsage        = "cannot track ProviderConfig usage"
	errGetBlockstore     = "cannot get S3 blockstore from Ops Manager"
	errCreateBlockstore  = "cannot create S3 blockstore in Ops Manager"
	errUpdateBlockstore  = "cannot update S3 blockstore in Ops Manager"
	errDeleteBlockstore  = "cannot delete S3 blockstore from Ops Manager"
	errGetAWSSecret      = "cannot get AWS secret key from Kubernetes secret"
)

// Setup registers the S3Blockstore controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.S3BlockstoreGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.S3BlockstoreGroupVersionKind),
		managed.WithExternalConnecter(&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &v1beta1.ProviderConfigUsage{}),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.S3Blockstore{}).
		Complete(r)
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.S3Blockstore)
	if !ok {
		return nil, errors.New(errNotS3Blockstore)
	}

	if err := c.usage.Track(ctx, mg); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	pc := &v1beta1.ProviderConfig{}
	if err := c.kube.Get(ctx, types.NamespacedName{Name: cr.GetProviderConfigReference().Name}, pc); err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}

	creds, err := clients.GetCredentials(ctx, c.kube, pc)
	if err != nil {
		return nil, err
	}

	opsClient, err := clients.NewClient(pc.Spec.BaseURL, creds)
	if err != nil {
		return nil, errors.Wrap(err, errCreateClient)
	}

	return &external{service: opsClient.S3BlockstoreConfig, kube: c.kube}, nil
}

type external struct {
	service opsmngr.S3BlockstoreConfigService
	kube    client.Client
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr := mg.(*v1alpha1.S3Blockstore)
	id := cr.Spec.ForProvider.ID

	observed, _, err := e.service.Get(ctx, id)
	if isNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetBlockstore)
	}

	cr.Status.AtProvider.UsedSize = observed.UsedSize
	meta.SetExternalName(cr, id)
	cr.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, observed),
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr := mg.(*v1alpha1.S3Blockstore)

	awsSecretKey, err := e.getAWSSecretKey(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	bs := toSDKBlockstore(cr.Spec.ForProvider, awsSecretKey)
	if _, _, err := e.service.Create(ctx, bs); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateBlockstore)
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr := mg.(*v1alpha1.S3Blockstore)

	awsSecretKey, err := e.getAWSSecretKey(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	bs := toSDKBlockstore(cr.Spec.ForProvider, awsSecretKey)
	if _, _, err := e.service.Update(ctx, cr.Spec.ForProvider.ID, bs); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateBlockstore)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr := mg.(*v1alpha1.S3Blockstore)

	_, err := e.service.Delete(ctx, cr.Spec.ForProvider.ID)
	if isNotFound(err) {
		return nil
	}
	return errors.Wrap(err, errDeleteBlockstore)
}

// getAWSSecretKey fetches the AWS secret access key from the referenced K8s secret.
// Returns an empty string if no secret ref is configured (e.g. IAM_ROLE auth).
func (e *external) getAWSSecretKey(ctx context.Context, cr *v1alpha1.S3Blockstore) (string, error) {
	ref := cr.Spec.ForProvider.AWSSecretKeySecretRef
	if ref == nil {
		return "", nil
	}

	secret := &corev1.Secret{}
	if err := e.kube.Get(ctx, types.NamespacedName{
		Namespace: ref.Namespace,
		Name:      ref.Name,
	}, secret); err != nil {
		return "", errors.Wrap(err, errGetAWSSecret)
	}

	val, ok := secret.Data[ref.Key]
	if !ok {
		return "", errors.Errorf("key %q not found in secret %s/%s", ref.Key, ref.Namespace, ref.Name)
	}
	return string(val), nil
}

// --- helpers ---

// defaultFalse returns a pointer to false — used to satisfy required *bool API fields.
func defaultFalse() *bool { f := false; return &f }

// toSDKBlockstore maps the CRD parameters to the SDK struct.
func toSDKBlockstore(p v1alpha1.S3BlockstoreParameters, awsSecretKey string) *opsmngr.S3Blockstore {
	maxConn := p.S3MaxConnections
	if maxConn == 0 {
		maxConn = 50
	}
	disableProxy := p.DisableProxyS3
	if disableProxy == nil {
		disableProxy = defaultFalse()
	}
	return &opsmngr.S3Blockstore{
		BackupStore: opsmngr.BackupStore{
			AdminBackupConfig: opsmngr.AdminBackupConfig{
				ID:                   p.ID,
				URI:                  p.URI,
				WriteConcern:         p.WriteConcern,
				Labels:               p.Labels,
				SSL:                  p.SSL,
				AssignmentEnabled:    p.AssignmentEnabled,
				EncryptedCredentials: p.EncryptedCredentials,
			},
			LoadFactor:    p.LoadFactor,
			MaxCapacityGB: p.MaxCapacityGB,
			Provisioned:   p.Provisioned,
			SyncSource:    p.SyncSource,
			Username:      p.Username,
		},
		AWSAccessKey:           p.AWSAccessKey,
		AWSSecretKey:           awsSecretKey,
		S3AuthMethod:           p.S3AuthMethod,
		S3BucketEndpoint:       p.S3BucketEndpoint,
		S3BucketName:           p.S3BucketName,
		S3MaxConnections:       maxConn,
		DisableProxyS3:         disableProxy,
		AcceptedTos:            p.AcceptedTos,
		SSEEnabled:             p.SSEEnabled,
		PathStyleAccessEnabled: p.PathStyleAccessEnabled,
	}
}

// isUpToDate compares the desired spec against the observed API state.
// AWS secret key is intentionally excluded — it cannot be read back from the API.
func isUpToDate(p v1alpha1.S3BlockstoreParameters, o *opsmngr.S3Blockstore) bool {
	if p.S3BucketName != o.S3BucketName {
		return false
	}
	if p.S3BucketEndpoint != o.S3BucketEndpoint {
		return false
	}
	if p.S3AuthMethod != o.S3AuthMethod {
		return false
	}
	if p.AWSAccessKey != o.AWSAccessKey {
		return false
	}
	if !stringSlicesEqual(p.Labels, o.Labels) {
		return false
	}
	if !boolPtrEqual(p.AssignmentEnabled, o.AssignmentEnabled) {
		return false
	}
	if !boolPtrEqual(p.PathStyleAccessEnabled, o.PathStyleAccessEnabled) {
		return false
	}
	if !boolPtrEqual(p.SSEEnabled, o.SSEEnabled) {
		return false
	}
	if !boolPtrEqual(p.AcceptedTos, o.AcceptedTos) {
		return false
	}
	return true
}

func stringSlicesEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func boolPtrEqual(a, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var e *opsmngr.ErrorResponse
	return stderrors.As(err, &e) && e.Response != nil && e.Response.StatusCode == http.StatusNotFound
}

