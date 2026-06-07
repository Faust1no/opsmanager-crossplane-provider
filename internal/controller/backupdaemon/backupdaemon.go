package backupdaemon

import (
	"context"
	stderrors "errors"
	"net/http"
	"net/url"
	"strings"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"go.mongodb.org/ops-manager/opsmngr"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/clients"
)

const (
	errGetProviderConfig = "cannot get ProviderConfig"
	errCreateClient      = "cannot create Ops Manager client"
	errTrackUsage        = "cannot track ProviderConfig usage"
	errListDaemons       = "cannot list backup daemons from Ops Manager"
	errUpdateDaemon      = "cannot update backup daemon in Ops Manager"
	errDaemonNotFound    = "backup daemon not found in Ops Manager; ensure the backup agent is running and has registered"

	// annotationLabelsAdopted is set after the first Observe so that labels are
	// adopted from the API exactly once. After that, the spec YAML is authoritative.
	annotationLabelsAdopted = "opsmanager.crossplane.io/labels-adopted"
)

// Setup registers the BackupDaemon controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.BackupDaemonGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.BackupDaemonGroupVersionKind),
		managed.WithTypedExternalConnector[*v1alpha1.BackupDaemon](&connector{
			kube:  mgr.GetClient(),
			usage: resource.NewProviderConfigUsageTracker(mgr.GetClient(), &v1beta1.ProviderConfigUsage{}),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1alpha1.BackupDaemon{}).
		Complete(r)
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, cr *v1alpha1.BackupDaemon) (managed.TypedExternalClient[*v1alpha1.BackupDaemon], error) {
	if err := c.usage.Track(ctx, cr); err != nil {
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

	return &external{service: opsClient.DaemonConfig}, nil
}

type external struct {
	service opsmngr.DaemonConfigService
}

// Observe lists all registered daemons and finds the one matching spec.machine by hostname.
// ResourceExists=false if the daemon hasn't registered with Ops Manager yet, or if the CR
// is being deleted (daemon config is intentionally left in Ops Manager on CR delete).
func (e *external) Observe(ctx context.Context, cr *v1alpha1.BackupDaemon) (managed.ExternalObservation, error) {
	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	observed, err := e.getDaemon(ctx, cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errListDaemons)
	}
	if observed == nil {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.Status.AtProvider.Configured = observed.Configured
	meta.SetExternalName(cr, daemonAPIID(observed))
	cr.SetConditions(xpv1.Available())

	lateInitialized := lateInitDaemon(&cr.Spec.ForProvider, observed)

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

// Create is called when Observe did not find the daemon. If it still isn't
// registered, return an error to requeue and retry.
func (e *external) Create(ctx context.Context, cr *v1alpha1.BackupDaemon) (managed.ExternalCreation, error) {
	existing, err := e.getDaemon(ctx, cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errListDaemons)
	}
	if existing != nil {
		cr.Status.AtProvider.Configured = existing.Configured
		meta.SetExternalName(cr, daemonAPIID(existing))
		lateInitDaemon(&cr.Spec.ForProvider, existing)
		return managed.ExternalCreation{}, nil
	}

	return managed.ExternalCreation{}, errors.New(errDaemonNotFound)
}

// Update fetches the current daemon state and applies the desired parameters,
// preserving any fields not managed by this CR.
func (e *external) Update(ctx context.Context, cr *v1alpha1.BackupDaemon) (managed.ExternalUpdate, error) {
	current, err := e.getDaemon(ctx, cr.Spec.ForProvider)
	if err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errListDaemons)
	}
	if current == nil {
		return managed.ExternalUpdate{}, errors.New(errDaemonNotFound)
	}

	applyParameters(cr.Spec.ForProvider, current)

	if _, _, err := e.service.Update(ctx, daemonAPIID(current), current); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateDaemon)
	}

	return managed.ExternalUpdate{}, nil
}

// Delete removes the CR but does NOT delete the daemon from Ops Manager.
func (e *external) Delete(_ context.Context, _ *v1alpha1.BackupDaemon) (managed.ExternalDelete, error) {
	return managed.ExternalDelete{}, nil
}

func (e *external) Disconnect(_ context.Context) error { return nil }

// --- helpers ---

// getDaemon looks up the daemon in Ops Manager by machine hostname.
func (e *external) getDaemon(ctx context.Context, p v1alpha1.BackupDaemonParameters) (*opsmngr.Daemon, error) {
	if p.HeadRootDirectory != "" {
		id := p.Machine + "/" + url.PathEscape(p.HeadRootDirectory)
		d, _, err := e.service.Get(ctx, id)
		if isNotFound(err) {
			return nil, nil
		}
		return d, err
	}

	list, _, err := e.service.List(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, d := range list.Results {
		if d.Machine != nil && d.Machine.Machine == p.Machine {
			return d, nil
		}
		if d.Machine == nil && strings.HasPrefix(d.ID, p.Machine+"/") {
			return d, nil
		}
	}
	return nil, nil
}

// daemonAPIID constructs the URL path segment used by the Ops Manager API.
func daemonAPIID(d *opsmngr.Daemon) string {
	if d.Machine == nil {
		return d.ID
	}
	return d.Machine.Machine + "/" + url.PathEscape(d.Machine.HeadRootDirectory)
}

func lateInitDaemon(p *v1alpha1.BackupDaemonParameters, d *opsmngr.Daemon) bool {
	changed := false
	// Labels are handled by the annotation-based adoption in Observe; skip here.
	if p.AssignmentEnabled == nil {
		p.AssignmentEnabled = d.AssignmentEnabled
		changed = true
	}
	if p.URI == "" && d.URI != "" {
		p.URI = d.URI
		changed = true
	}
	if p.WriteConcern == "" && d.WriteConcern != "" {
		p.WriteConcern = d.WriteConcern
		changed = true
	}
	if p.SSL == nil {
		p.SSL = d.SSL
		changed = true
	}
	if p.EncryptedCredentials == nil {
		p.EncryptedCredentials = d.EncryptedCredentials
		changed = true
	}
	if p.BackupJobsEnabled == nil {
		p.BackupJobsEnabled = boolPtr(d.BackupJobsEnabled)
		changed = true
	}
	if p.GarbageCollectionEnabled == nil {
		p.GarbageCollectionEnabled = boolPtr(d.GarbageCollectionEnabled)
		changed = true
	}
	if p.ResourceUsageEnabled == nil {
		p.ResourceUsageEnabled = boolPtr(d.ResourceUsageEnabled)
		changed = true
	}
	if p.RestoreQueryableJobsEnabled == nil {
		p.RestoreQueryableJobsEnabled = boolPtr(d.RestoreQueryableJobsEnabled)
		changed = true
	}
	if p.HeadDiskType == "" && d.HeadDiskType != "" {
		p.HeadDiskType = d.HeadDiskType
		changed = true
	}
	if p.NumWorkers == 0 && d.NumWorkers != 0 {
		p.NumWorkers = d.NumWorkers
		changed = true
	}
	if p.HeadRootDirectory == "" && d.Machine != nil && d.Machine.HeadRootDirectory != "" {
		p.HeadRootDirectory = d.Machine.HeadRootDirectory
		changed = true
	}
	return changed
}

func applyParameters(p v1alpha1.BackupDaemonParameters, d *opsmngr.Daemon) {
	if p.Labels != nil {
		d.Labels = p.Labels
	}
	if p.AssignmentEnabled != nil {
		d.AssignmentEnabled = p.AssignmentEnabled
	}
	if p.URI != "" {
		d.URI = p.URI
	}
	if p.WriteConcern != "" {
		d.WriteConcern = p.WriteConcern
	}
	if p.SSL != nil {
		d.SSL = p.SSL
	}
	if p.EncryptedCredentials != nil {
		d.EncryptedCredentials = p.EncryptedCredentials
	}
	if p.BackupJobsEnabled != nil {
		d.BackupJobsEnabled = *p.BackupJobsEnabled
	}
	if p.GarbageCollectionEnabled != nil {
		d.GarbageCollectionEnabled = *p.GarbageCollectionEnabled
	}
	if p.ResourceUsageEnabled != nil {
		d.ResourceUsageEnabled = *p.ResourceUsageEnabled
	}
	if p.RestoreQueryableJobsEnabled != nil {
		d.RestoreQueryableJobsEnabled = *p.RestoreQueryableJobsEnabled
	}
	if p.HeadDiskType != "" {
		d.HeadDiskType = p.HeadDiskType
	}
	if p.NumWorkers != 0 {
		d.NumWorkers = p.NumWorkers
	}
	if p.HeadRootDirectory != "" {
		if d.Machine == nil {
			d.Machine = &opsmngr.Machine{}
		}
		d.Machine.HeadRootDirectory = p.HeadRootDirectory
	}
}

func isUpToDate(p v1alpha1.BackupDaemonParameters, d *opsmngr.Daemon, labelsAdopted bool) bool {
	if labelsAdopted {
		if !stringSlicesEqual(p.Labels, d.Labels) {
			return false
		}
	} else if p.Labels != nil && !stringSlicesEqual(p.Labels, d.Labels) {
		return false
	}
	if p.AssignmentEnabled != nil && !boolPtrEqual(p.AssignmentEnabled, d.AssignmentEnabled) {
		return false
	}
	if p.BackupJobsEnabled != nil && *p.BackupJobsEnabled != d.BackupJobsEnabled {
		return false
	}
	if p.GarbageCollectionEnabled != nil && *p.GarbageCollectionEnabled != d.GarbageCollectionEnabled {
		return false
	}
	if p.ResourceUsageEnabled != nil && *p.ResourceUsageEnabled != d.ResourceUsageEnabled {
		return false
	}
	if p.HeadDiskType != "" && p.HeadDiskType != d.HeadDiskType {
		return false
	}
	return true
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var e *opsmngr.ErrorResponse
	return stderrors.As(err, &e) && e.Response != nil && e.Response.StatusCode == http.StatusNotFound
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

func boolPtr(b bool) *bool { return &b }

func boolPtrEqual(a *bool, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
