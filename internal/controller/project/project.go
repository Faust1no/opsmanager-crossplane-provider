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
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1alpha1"
	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
	"github.com/crossplane-contrib/provider-opsmanager/internal/clients"
)

const (
	errNotProject           = "managed resource is not a Project"
	errGetProviderConfig    = "cannot get ProviderConfig"
	errGetSecret            = "cannot get credentials secret"
	errParseCredentials     = "cannot parse credentials"
	errCreateClient         = "cannot create Ops Manager client"
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
	name := managed.ControllerName(v1alpha1.ProjectGroupKind.Kind)

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.ProjectGroupVersionKind),
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
		For(&v1alpha1.Project{}).
		Complete(r)
}

// connector builds an authenticated ExternalClient for each reconcile.
type connector struct {
	kube  client.Client
	usage *resource.ProviderConfigUsageTracker
}

func (c *connector) Connect(ctx context.Context, mg resource.Managed) (managed.ExternalClient, error) {
	cr, ok := mg.(*v1alpha1.Project)
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

	creds, err := getCredentials(ctx, c.kube, pc)
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
	cr := mg.(*v1alpha1.Project)

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

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: ldapMappingsMatch(cr.Spec.ForProvider.LDAPGroupMappings, observed.LDAPGroupMappings),
	}, nil
}

func (e *external) Create(ctx context.Context, mg resource.Managed) (managed.ExternalCreation, error) {
	cr := mg.(*v1alpha1.Project)

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
	cr := mg.(*v1alpha1.Project)

	patch := &opsmngr.Project{
		LDAPGroupMappings: toSDKMappings(cr.Spec.ForProvider.LDAPGroupMappings),
	}

	if _, _, err := e.service.Update(ctx, cr.Status.AtProvider.ID, patch); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	return managed.ExternalUpdate{}, nil
}

func (e *external) Delete(ctx context.Context, mg resource.Managed) error {
	cr := mg.(*v1alpha1.Project)

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

// getCredentials reads the credentials secret referenced by the ProviderConfig.
func getCredentials(ctx context.Context, kube client.Client, pc *v1beta1.ProviderConfig) (*clients.Credentials, error) {
	cd := pc.Spec.Credentials
	if cd.Source != xpv1.CredentialsSourceSecret || cd.SecretRef == nil {
		return nil, errors.New("credentials source must be Secret with a secretRef")
	}

	secret := &corev1.Secret{}
	if err := kube.Get(ctx, types.NamespacedName{
		Namespace: cd.SecretRef.Namespace,
		Name:      cd.SecretRef.Name,
	}, secret); err != nil {
		return nil, errors.Wrap(err, errGetSecret)
	}

	data, ok := secret.Data[cd.SecretRef.Key]
	if !ok {
		return nil, errors.Errorf("key %q not found in secret %s/%s", cd.SecretRef.Key, cd.SecretRef.Namespace, cd.SecretRef.Name)
	}

	creds, err := clients.ParseCredentials(data)
	return creds, errors.Wrap(err, errParseCredentials)
}
