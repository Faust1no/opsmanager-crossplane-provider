package project

import (
	"context"
	stderrors "errors"
	"net/http"
	"sort"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/meta"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/managed"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	"github.com/pkg/errors"
	"go.mongodb.org/ops-manager/opsmngr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/clients"
)

const (
	errGetProviderConfig     = "cannot resolve ProviderConfig"
	errTrackUsage            = "cannot track ProviderConfig usage"
	errGetProject            = "cannot get project from Ops Manager"
	errCreateProject         = "cannot create project in Ops Manager"
	errDeleteProject         = "cannot delete project from Ops Manager"
	errUpdateProject         = "cannot update project LDAP group mappings in Ops Manager"
	errListBackupConfigs     = "cannot list backup configs for project"
	errStopBackupConfig      = "cannot stop backup config"
	errTerminateBackupConfig = "cannot terminate backup config"

	backupStatusStarted     = "STARTED"
	backupStatusStopped     = "STOPPED"
	backupStatusTerminating = "TERMINATING"
)

// Setup registers the Project controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.OpsManagerProjectGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.OpsManagerProjectGroupVersionKind),
		managed.WithTypedExternalConnector[*v1alpha1.OpsManagerProject](&connector{
			kube:  mgr.GetClient(),
			usage: clients.NewUsageTracker(mgr.GetClient()),
		}),
		managed.WithLogger(o.Logger.WithValues("controller", name)),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.OpsManagerProject{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube  client.Client
	usage *clients.UsageTracker
}

func (c *connector) Connect(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.TypedExternalClient[*v1alpha1.OpsManagerProject], error) {
	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	opsClient, _, err := clients.Resolve(ctx, c.kube, cr.GetProviderConfigReference(), cr.GetNamespace())
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}
	return &external{
		service:       opsClient.Projects,
		backupConfigs: opsClient.BackupConfigs,
	}, nil
}

type external struct {
	service       opsmngr.ProjectsService
	backupConfigs opsmngr.BackupConfigsService
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalObservation, error) {
	observed, _, err := e.service.GetByName(ctx, cr.Spec.ForProvider.Name)
	if isNotFound(err) {
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetProject)
	}

	cr.Status.AtProvider.ID = observed.ID
	meta.SetExternalName(cr, observed.ID)
	cr.SetConditions(xpv1.Available())

	lateInitProject(&cr.Spec.ForProvider, observed)

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: len(cr.Spec.ForProvider.LDAPGroupMappings) == 0 || ldapMappingsMatch(cr.Spec.ForProvider.LDAPGroupMappings, observed.LDAPGroupMappings),
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalCreation, error) {
	// Check if the project already exists in Ops Manager before creating.
	existing, _, err := e.service.GetByName(ctx, cr.Spec.ForProvider.Name)
	if err == nil {
		cr.Status.AtProvider.ID = existing.ID
		meta.SetExternalName(cr, existing.ID)
		lateInitProject(&cr.Spec.ForProvider, existing)
		return managed.ExternalCreation{}, nil
	}
	if !isNotFound(err) {
		return managed.ExternalCreation{}, errors.Wrap(err, errGetProject)
	}

	f := false
	project := &opsmngr.Project{
		Name:                      cr.Spec.ForProvider.Name,
		OrgID:                     cr.Spec.ForProvider.OrgID,
		LDAPGroupMappings:         toSDKMappings(cr.Spec.ForProvider.LDAPGroupMappings),
		WithDefaultAlertsSettings: &f,
	}

	created, _, err := e.service.Create(ctx, project, nil)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateProject)
	}

	cr.Status.AtProvider.ID = created.ID
	meta.SetExternalName(cr, created.ID)

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalUpdate, error) {
	patch := &opsmngr.Project{
		LDAPGroupMappings: toSDKMappings(cr.Spec.ForProvider.LDAPGroupMappings),
	}

	if _, _, err := e.service.Update(ctx, cr.Status.AtProvider.ID, patch); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalDelete, error) {
	projectID := cr.Status.AtProvider.ID
	if projectID == "" {
		return managed.ExternalDelete{}, nil
	}

	configs, _, err := e.backupConfigs.List(ctx, projectID, nil)
	if err != nil {
		return managed.ExternalDelete{}, errors.Wrap(err, errListBackupConfigs)
	}

	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStarted {
			patch := &opsmngr.BackupConfig{StatusName: backupStatusStopped}
			if _, _, err := e.backupConfigs.Update(ctx, projectID, bc.ClusterID, patch); err != nil {
				return managed.ExternalDelete{}, errors.Wrap(err, errStopBackupConfig)
			}
		}
	}
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStarted {
			return managed.ExternalDelete{}, errors.Errorf("waiting for backup config %s to stop", bc.ClusterID)
		}
	}

	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStopped {
			patch := &opsmngr.BackupConfig{StatusName: backupStatusTerminating}
			if _, _, err := e.backupConfigs.Update(ctx, projectID, bc.ClusterID, patch); err != nil {
				return managed.ExternalDelete{}, errors.Wrap(err, errTerminateBackupConfig)
			}
		}
	}
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStopped || bc.StatusName == backupStatusTerminating {
			return managed.ExternalDelete{}, errors.Errorf("waiting for backup config %s to terminate", bc.ClusterID)
		}
	}

	_, err = e.service.Delete(ctx, projectID)
	if isNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	return managed.ExternalDelete{}, errors.Wrap(err, errDeleteProject)
}

func (e *external) Disconnect(_ context.Context) error { return nil }

// --- helpers ---

func lateInitProject(p *v1alpha1.OpsManagerProjectParameters, o *opsmngr.Project) {
	if len(p.LDAPGroupMappings) == 0 && len(o.LDAPGroupMappings) > 0 {
		p.LDAPGroupMappings = fromSDKMappings(o.LDAPGroupMappings)
	}
}

func ldapMappingsMatch(desired []v1alpha1.LDAPGroupMapping, observed []*opsmngr.LDAPGroupMapping) bool {
	if len(desired) != len(observed) {
		return false
	}
	observedMap := make(map[string][]string, len(observed))
	for _, m := range observed {
		sorted := make([]string, len(m.LDAPGroups))
		copy(sorted, m.LDAPGroups)
		sort.Strings(sorted)
		observedMap[m.RoleName] = sorted
	}
	for _, m := range desired {
		obs, ok := observedMap[m.RoleName]
		if !ok {
			return false
		}
		want := make([]string, len(m.LDAPGroups))
		copy(want, m.LDAPGroups)
		sort.Strings(want)
		if len(want) != len(obs) {
			return false
		}
		for i := range want {
			if want[i] != obs[i] {
				return false
			}
		}
	}
	return true
}

func fromSDKMappings(mappings []*opsmngr.LDAPGroupMapping) []v1alpha1.LDAPGroupMapping {
	result := make([]v1alpha1.LDAPGroupMapping, len(mappings))
	for i, m := range mappings {
		groups := make([]string, len(m.LDAPGroups))
		copy(groups, m.LDAPGroups)
		result[i] = v1alpha1.LDAPGroupMapping{
			RoleName:   m.RoleName,
			LDAPGroups: groups,
		}
	}
	return result
}

func toSDKMappings(mappings []v1alpha1.LDAPGroupMapping) []*opsmngr.LDAPGroupMapping {
	result := make([]*opsmngr.LDAPGroupMapping, len(mappings))
	for i, m := range mappings {
		groups := make([]string, len(m.LDAPGroups))
		copy(groups, m.LDAPGroups)
		result[i] = &opsmngr.LDAPGroupMapping{
			RoleName:   m.RoleName,
			LDAPGroups: groups,
		}
	}
	return result
}

func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	var e *opsmngr.ErrorResponse
	return stderrors.As(err, &e) && e.Response != nil && e.Response.StatusCode == http.StatusNotFound
}
