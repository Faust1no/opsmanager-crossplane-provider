package project

import (
	"context"
	stderrors "errors"
	"net/http"
	"sort"

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
	errNotProject        = "managed resource is not an OpsManagerProject"
	errGetProviderConfig = "cannot get ProviderConfig"
	errCreateClient      = "cannot create Ops Manager client"
	errTrackUsage           = "cannot track ProviderConfig usage"
	errGetProject           = "cannot get project from Ops Manager"
	errCreateProject        = "cannot create project in Ops Manager"
	errDeleteProject        = "cannot delete project from Ops Manager"
	errUpdateProject        = "cannot update project LDAP group mappings in Ops Manager"
	errListBackupConfigs    = "cannot list backup configs for project"
	errStopBackupConfig     = "cannot stop backup config"
	errTerminateBackupConfig = "cannot terminate backup config"

	backupStatusStarted    = "STARTED"
	backupStatusStopped    = "STOPPED"
	backupStatusTerminating = "TERMINATING"
	backupStatusInactive   = "INACTIVE"
)

// Setup registers the Project controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.OpsManagerProjectGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.OpsManagerProjectGroupVersionKind),
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
		For(&v1alpha1.OpsManagerProject{}).
		Complete(r)
}

// connector builds an authenticated ExternalClient for each reconcile.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.OpsManagerProject)
	if !ok {
		return nil, errors.New(errNotProject)
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

	return &external{
		service:       opsClient.Projects,
		backupConfigs: opsClient.BackupConfigs,
	}, nil
}

// external implements managed.ExternalClient against the Ops Manager Projects API.
type external struct {
	service       opsmngr.ProjectsService
	backupConfigs opsmngr.BackupConfigsService
}

func (e *external) Observe(ctx context.Context, mg resource.Managed) (managed.ExternalObservation, error) {
	cr := mg.(*v1alpha1.OpsManagerProject)

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

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr := mg.(*v1alpha1.OpsManagerProject)

	// Check if the project already exists in Ops Manager before creating.
	existing, _, err := e.service.GetByName(ctx, cr.Spec.ForProvider.Name)
	if err == nil {
		// Project exists — adopt it and populate spec from API.
		cr.Status.AtProvider.ID = existing.ID
		meta.SetExternalName(cr, existing.ID)
		lateInitProject(&cr.Spec.ForProvider, existing)
		return managed.ExternalCreation{}, nil
	}
	if !isNotFound(err) {
		return managed.ExternalCreation{}, errors.Wrap(err, errGetProject)
	}

	// Project does not exist — create it.
	project := &opsmngr.Project{
		Name:              cr.Spec.ForProvider.Name,
		OrgID:             cr.Spec.ForProvider.OrgID,
		LDAPGroupMappings: toSDKMappings(cr.Spec.ForProvider.LDAPGroupMappings),
	}

	created, _, err := e.service.Create(ctx, project, nil)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateProject)
	}

	cr.Status.AtProvider.ID = created.ID
	meta.SetExternalName(cr, created.ID)

	return managed.ExternalCreation{}, nil
}

// Update patches the project's ldapGroupMappings to match the desired spec.
// Only the mappings field is sent so no other project state is affected.
func (e *external) Update(ctx context.Context, mg resource.Managed) (managed.ExternalUpdate, error) {
	cr := mg.(*v1alpha1.OpsManagerProject)

	patch := &opsmngr.Project{
		LDAPGroupMappings: toSDKMappings(cr.Spec.ForProvider.LDAPGroupMappings),
	}

	if _, _, err := e.service.Update(ctx, cr.Status.AtProvider.ID, patch); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr := mg.(*v1alpha1.OpsManagerProject)

	projectID := cr.Status.AtProvider.ID
	if projectID == "" {
		return nil
	}

	configs, _, err := e.backupConfigs.List(ctx, projectID, nil)
	if err != nil {
		return errors.Wrap(err, errListBackupConfigs)
	}

	// Stop any started backups, return error to requeue until stopped.
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStarted {
			patch := &opsmngr.BackupConfig{StatusName: backupStatusStopped}
			if _, _, err := e.backupConfigs.Update(ctx, projectID, bc.ClusterID, patch); err != nil {
				return errors.Wrap(err, errStopBackupConfig)
			}
		}
	}
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStarted {
			return errors.Errorf("waiting for backup config %s to stop", bc.ClusterID)
		}
	}

	// Terminate any stopped backups, return error to requeue until inactive.
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStopped {
			patch := &opsmngr.BackupConfig{StatusName: backupStatusTerminating}
			if _, _, err := e.backupConfigs.Update(ctx, projectID, bc.ClusterID, patch); err != nil {
				return errors.Wrap(err, errTerminateBackupConfig)
			}
		}
	}
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStopped || bc.StatusName == backupStatusTerminating {
			return errors.Errorf("waiting for backup config %s to terminate", bc.ClusterID)
		}
	}

	_, err = e.service.Delete(ctx, projectID)
	if isNotFound(err) {
		return nil
	}
	return errors.Wrap(err, errDeleteProject)
}

// --- helpers ---

// lateInitProject populates empty optional spec fields from the API response.
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

