package backupdaemon

import (
	"context"
	"net/url"

	xpv1 "github.com/crossplane/crossplane-runtime/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/pkg/controller"
	"github.com/crossplane/crossplane-runtime/pkg/event"
	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
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
	errNotBackupDaemon   = "managed resource is not a BackupDaemon"
	errGetProviderConfig = "cannot get ProviderConfig"
	errCreateClient      = "cannot create Ops Manager client"
	errTrackUsage        = "cannot track ProviderConfig usage"
	errListDaemons       = "cannot list backup daemons from Ops Manager"
	errUpdateDaemon      = "cannot update backup daemon in Ops Manager"
	errDaemonNotFound    = "backup daemon not found in Ops Manager; ensure the backup agent is running and has registered"
)

// Setup registers the BackupDaemon controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.BackupDaemonGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.BackupDaemonGroupVersionKind),
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
		For(&v1alpha1.BackupDaemon{}).
		Complete(r)
}

type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.BackupDaemon)
	if !ok {
		return nil, errors.New(errNotBackupDaemon)
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

	return &external{service: opsClient.DaemonConfig}, nil
}

type external struct {
	service opsmngr.DaemonConfigService
}

// Observe lists all registered daemons and finds the one matching spec.machine by hostname.
// ResourceExists=false if the daemon hasn't registered with Ops Manager yet, or if the CR
// is being deleted (daemon config is intentionally left in Ops Manager on CR delete).
func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr := mg.(*v1alpha1.BackupDaemon)

	if meta.WasDeleted(cr) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	observed, err := e.findByHostname(ctx, cr.Spec.ForProvider.Machine)
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errListDaemons)
	}
	if observed == nil {
		// Daemon hasn't registered yet — keep retrying via Create's error.
		return managed.ExternalObservation{ResourceExists: false}, nil
	}

	cr.Status.AtProvider.Configured = observed.Configured
	meta.SetExternalName(cr, daemonAPIID(observed))
	cr.SetConditions(xpv1.Available())

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: isUpToDate(cr.Spec.ForProvider, observed),
	}, nil
}

// Create is called when the daemon hasn't registered with Ops Manager yet.
// Return an error so the reconciler requeues and retries.
func (e *external) Create(_ context.Context, _ resource.Managed) (managed.ExternalCreation, error) {
	return managed.ExternalCreation{}, errors.New(errDaemonNotFound)
}

// Update fetches the current daemon state and applies the desired parameters,
// preserving any fields not managed by this CR.
func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr := mg.(*v1alpha1.BackupDaemon)

	current, err := e.findByHostname(ctx, cr.Spec.ForProvider.Machine)
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
func (e *external) Delete(_ context.Context, _ resource.Managed) error {
	return nil
}

// --- helpers ---

// findByHostname lists all registered daemons and returns the one whose
// machine hostname matches. Returns nil if not found.
func (e *external) findByHostname(ctx context.Context, hostname string) (*opsmngr.Daemon, error) {
	list, _, err := e.service.List(ctx, nil)
	if err != nil {
		return nil, err
	}
	for _, d := range list.Results {
		if d.Machine != nil && d.Machine.Machine == hostname {
			return d, nil
		}
	}
	return nil, nil
}

// daemonAPIID constructs the URL path segment used by the Ops Manager API:
// "hostname/%2FheadDir%2F" — the head directory is URL-path-encoded.
func daemonAPIID(d *opsmngr.Daemon) string {
	if d.Machine == nil {
		return d.ID
	}
	return d.Machine.Machine + "/" + url.PathEscape(d.Machine.HeadRootDirectory)
}

// applyParameters merges desired spec fields onto the current daemon,
// leaving any fields not specified in the CR unchanged.
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

// isUpToDate checks whether the daemon's current state matches the desired spec.
func isUpToDate(p v1alpha1.BackupDaemonParameters, d *opsmngr.Daemon) bool {
	if p.Labels != nil && !stringSlicesEqual(p.Labels, d.Labels) {
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

func boolPtrEqual(a *bool, b *bool) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}

