package s3oplogstore

import (
	"context"
	stderrors "errors"
	"net/http"
	"time"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"go.mongodb.org/ops-manager/opsmngr"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/clients"
)

const (
	errGetProviderConfig = "cannot resolve ProviderConfig"
	errTrackUsage        = "cannot track ProviderConfig usage"
	errGetOplogStore     = "cannot get S3 oplog store from Ops Manager"
	errCreateOplogStore  = "cannot create S3 oplog store in Ops Manager"
	errUpdateOplogStore  = "cannot update S3 oplog store in Ops Manager"
	errDeleteOplogStore  = "cannot delete S3 oplog store from Ops Manager"
	errGetAWSSecret      = "cannot get AWS secret key from Kubernetes secret"

	// annotationLabelsAdopted is set after the first Observe so that labels are
	// adopted from the API exactly once. After that, the spec YAML is authoritative.
	annotationLabelsAdopted = "opsmanager.crossplane.io/labels-adopted"
)

// Setup registers the S3OplogStore controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.S3OplogStoreGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.S3OplogStoreGroupVersionKind),
		managed.WithTypedExternalConnector[*v1alpha1.S3OplogStore](&connector{
			kube:  mgr.GetClient(),
			usage: clients.NewUsageTracker(mgr.GetClient()),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
		managed.WithTimeout(5*time.Minute),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.S3OplogStore{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *clients.UsageTracker
}

func (c *connector) Connect(ctx context.Context, cr *v1alpha1.S3OplogStore) (managed.TypedExternalClient[*v1alpha1.S3OplogStore], error) {
	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	opsClient, _, err := clients.Resolve(ctx, c.kube, cr.GetProviderConfigReference())
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}
	return &external{service: opsClient.S3OplogStoreConfig, kube: c.kube}, nil
}

type external struct {
	service opsmngr.S3OplogStoreConfigService
	kube    client.Client
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.S3OplogStore) (managed.ExternalObservation, error) {
	id := cr.Spec.ForProvider.ID

	observed, _, err := e.service.Get(ctx, id)
	if isNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetOplogStore)
	}

	cr.Status.AtProvider.UsedSize = observed.UsedSize
	meta.SetExternalName(cr, id)
	cr.SetConditions(xpv1.Available())

	lateInitialized := lateInitOplogStore(&cr.Spec.ForProvider, observed)

	// Adopt labels from the API exactly once. After the annotation is set,
	// the spec YAML is authoritative and labels are never overwritten.
	ann := cr.GetAnnotations()
	labelsAdopted := ann[annotationLabelsAdopted] == "true"
	if !labelsAdopted {
		if cr.Spec.ForProvider.Labels == nil && len(observed.Labels) > 0 {
			cr.Spec.ForProvider.Labels = observed.Labels
		}
		if ann == nil {
			ann = make(map[string]string)
		}
		ann[annotationLabelsAdopted] = "true"
		cr.SetAnnotations(ann)
		lateInitialized = true
		labelsAdopted = true
	}

	return managed.ExternalObservation{
		ResourceExists:          true,
		ResourceUpToDate:        isUpToDate(cr.Spec.ForProvider, observed, labelsAdopted),
		ResourceLateInitialized: lateInitialized,
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.S3OplogStore) (managed.ExternalCreation, error) {
	// Check if the oplog store already exists in Ops Manager before creating.
	existing, _, err := e.service.Get(ctx, cr.Spec.ForProvider.ID)
	if err == nil {
		// Oplog store exists — adopt it and populate all optional spec fields from API.
		meta.SetExternalName(cr, cr.Spec.ForProvider.ID)
		lateInitOplogStore(&cr.Spec.ForProvider, existing)
		return managed.ExternalCreation{}, nil
	}
	if !isNotFound(err) {
		return managed.ExternalCreation{}, errors.Wrap(err, errGetOplogStore)
	}

	// Oplog store does not exist — create it.
	awsSecretKey, err := e.getAWSSecretKey(ctx, cr)
	if err != nil {
		return managed.ExternalCreation{}, err
	}

	bs := toSDKStore(cr.Spec.ForProvider, awsSecretKey)
	if _, _, err := e.service.Create(ctx, bs); err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateOplogStore)
	}

	meta.SetExternalName(cr, cr.Spec.ForProvider.ID)
	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.S3OplogStore) (managed.ExternalUpdate, error) {
	awsSecretKey, err := e.getAWSSecretKey(ctx, cr)
	if err != nil {
		return managed.ExternalUpdate{}, err
	}

	bs := toSDKStore(cr.Spec.ForProvider, awsSecretKey)
	// Ops Manager re-runs S3 bucket validation on every oplog-store update.
	// The first probe right after a config change occasionally hits a transient
	// 403 from S3, which OM surfaces as 409 BACKUP-S3-VALIDATION_FAILED.
	// A short in-line retry avoids waiting a full --poll-interval for recovery.
	if err := clients.RetryOnS3Validation(ctx, 3, 5*time.Second, func() error {
		_, _, updErr := e.service.Update(ctx, cr.Spec.ForProvider.ID, bs)
		return updErr
	}); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateOplogStore)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.S3OplogStore) (managed.ExternalDelete, error) {
	id := cr.Spec.ForProvider.ID

	current, _, err := e.service.Get(ctx, id)
	if isNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errGetOplogStore)
	}

	// Ops Manager rejects deletion with 409 if assignmentEnabled is true.
	if current.AssignmentEnabled != nil && *current.AssignmentEnabled {
		f := false
		current.AssignmentEnabled = &f
		if err := clients.RetryOnS3Validation(ctx, 3, 5*time.Second, func() error {
			_, _, updErr := e.service.Update(ctx, id, current)
			return updErr
		}); err != nil {
			return managed.ExternalDelete{}, errors.Wrap(err, errUpdateOplogStore)
		}
	}

	_, err = e.service.Delete(ctx, id)
	if isNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	return managed.ExternalDelete{}, errors.Wrap(err, errDeleteOplogStore)
}

func (e *external) Disconnect(_ context.Context) error { return nil }

func (e *external) getAWSSecretKey(ctx context.Context, cr *v1alpha1.S3OplogStore) (string, error) {
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

func lateInitOplogStore(p *v1alpha1.S3OplogStoreParameters, o *opsmngr.S3Blockstore) bool {
	changed := false
	set := func(dst *string, src string) {
		if *dst == "" && src != "" {
			*dst = src
			changed = true
		}
	}
	setPtr := func(dst **bool, src *bool) {
		if *dst == nil && src != nil {
			*dst = src
			changed = true
		}
	}
	setPtrInt64 := func(dst **int64, src *int64) {
		if *dst == nil && src != nil {
			*dst = src
			changed = true
		}
	}
	setInt := func(dst *int64, src int64) {
		if *dst == 0 && src != 0 {
			*dst = src
			changed = true
		}
	}
	set(&p.URI, o.URI)
	set(&p.S3BucketEndpoint, o.S3BucketEndpoint)
	set(&p.S3AuthMethod, o.S3AuthMethod)
	set(&p.WriteConcern, o.WriteConcern)
	set(&p.SyncSource, o.SyncSource)
	set(&p.Username, o.Username)
	set(&p.AWSAccessKey, o.AWSAccessKey)
	setPtr(&p.AssignmentEnabled, o.AssignmentEnabled)
	setPtr(&p.SSL, o.SSL)
	setPtr(&p.EncryptedCredentials, o.EncryptedCredentials)
	setPtrInt64(&p.LoadFactor, o.LoadFactor)
	setPtrInt64(&p.MaxCapacityGB, o.MaxCapacityGB)
	setPtr(&p.Provisioned, o.Provisioned)
	setInt(&p.S3MaxConnections, o.S3MaxConnections)
	setPtr(&p.PathStyleAccessEnabled, o.PathStyleAccessEnabled)
	setPtr(&p.SSEEnabled, o.SSEEnabled)
	setPtr(&p.AcceptedTos, o.AcceptedTos)
	setPtr(&p.DisableProxyS3, o.DisableProxyS3)
	return changed
}

func defaultFalse() *bool { f := false; return &f }

func toSDKStore(p v1alpha1.S3OplogStoreParameters, awsSecretKey string) *opsmngr.S3Blockstore {
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

func isUpToDate(p v1alpha1.S3OplogStoreParameters, o *opsmngr.S3Blockstore, labelsAdopted bool) bool {
	if p.S3BucketName != o.S3BucketName {
		return false
	}
	if p.S3BucketEndpoint != o.S3BucketEndpoint {
		return false
	}
	if p.S3AuthMethod != o.S3AuthMethod {
		return false
	}
	if p.AWSAccessKey != "" && p.AWSAccessKey != o.AWSAccessKey {
		return false
	}
	if labelsAdopted {
		if !stringSlicesEqual(p.Labels, o.Labels) {
			return false
		}
	} else if p.Labels != nil && !stringSlicesEqual(p.Labels, o.Labels) {
		return false
	}
	if p.AssignmentEnabled != nil && !boolPtrEqual(p.AssignmentEnabled, o.AssignmentEnabled) {
		return false
	}
	if p.PathStyleAccessEnabled != nil && !boolPtrEqual(p.PathStyleAccessEnabled, o.PathStyleAccessEnabled) {
		return false
	}
	if p.SSEEnabled != nil && !boolPtrEqual(p.SSEEnabled, o.SSEEnabled) {
		return false
	}
	if p.AcceptedTos != nil && !boolPtrEqual(p.AcceptedTos, o.AcceptedTos) {
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
