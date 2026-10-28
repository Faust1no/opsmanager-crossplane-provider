package project

import (
	"context"
	stderrors "errors"
	"net/http"
	"sort"

	xpv1 "github.com/crossplane/crossplane-runtime/v2/apis/common/v1"
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/logging"
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
	errGetSnapshotSchedule   = "cannot get snapshot schedule from Ops Manager"
	errUpdateSnapSchedule    = "cannot update snapshot schedule in Ops Manager"
	errNoBackupConfig        = "snapshotSchedule declared but no cluster in the project has backup enabled; waiting for a backup config to appear"

	backupStatusStarted     = "STARTED"
	backupStatusStopped     = "STOPPED"
	backupStatusTerminating = "TERMINATING"

	// Event reasons recorded on the OpsManagerProject CR. Emitted only on
	// state transitions (one-shot adopt/create/delete, and backup-config
	// stop/terminate transitions) — never on per-reconcile Update, which
	// would spam the event stream every poll on drift.
	reasonAdoptedProject    event.Reason = "AdoptedProject"
	reasonCreatedProject    event.Reason = "CreatedProject"
	reasonStoppingBackup    event.Reason = "StoppingBackupConfig"
	reasonTerminatingBackup event.Reason = "TerminatingBackupConfig"
	reasonDeletedProject    event.Reason = "DeletedProject"
	reasonNoBackupConfig    event.Reason = "NoBackupConfig"
)

// Setup registers the Project controller with the manager.
func Setup(mgr ctrl.Manager, o controller.Options) error {
	name := managed.ControllerName(v1alpha1.OpsManagerProjectGroupKind.Kind)
	log := o.Logger.WithValues("controller", name)
	rec := event.NewAPIRecorder(mgr.GetEventRecorderFor(name))

	r := managed.NewReconciler(mgr,
		resource.ManagedKind(v1alpha1.OpsManagerProjectGroupVersionKind),
		managed.WithTypedExternalConnector[*v1alpha1.OpsManagerProject](&connector{
			kube:     mgr.GetClient(),
			usage:    clients.NewUsageTracker(mgr.GetClient()),
			log:      log,
			recorder: rec,
		}),
		managed.WithLogger(log),
		managed.WithPollInterval(o.PollInterval),
		managed.WithRecorder(rec),
	)

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		WithEventFilter(resource.DesiredStateChanged()).
		For(&v1alpha1.OpsManagerProject{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

type connector struct {
	kube     client.Client
	usage    *clients.UsageTracker
	log      logging.Logger
	recorder event.Recorder
}

func (c *connector) Connect(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.TypedExternalClient[*v1alpha1.OpsManagerProject], error) {
	if err := c.usage.Track(ctx, cr); err != nil {
		return nil, errors.Wrap(err, errTrackUsage)
	}

	opsClient, _, err := clients.Resolve(ctx, c.kube, cr.GetProviderConfigReference())
	if err != nil {
		return nil, errors.Wrap(err, errGetProviderConfig)
	}
	return &external{
		service:          opsClient.Projects,
		backupConfigs:    opsClient.BackupConfigs,
		snapshotSchedule: opsClient.SnapshotSchedule,
		log:              c.log.WithValues("project", cr.Spec.ForProvider.Name),
		recorder:         c.recorder,
	}, nil
}

type external struct {
	service          opsmngr.ProjectsService
	backupConfigs    opsmngr.BackupConfigsService
	snapshotSchedule opsmngr.SnapshotScheduleService
	log              logging.Logger
	recorder         event.Recorder
}

func (e *external) Observe(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalObservation, error) {
	observed, _, err := e.service.GetByName(ctx, cr.Spec.ForProvider.Name)
	if isNotFound(err) {
		e.log.Debug("Observe: project not found in Ops Manager")
		return managed.ExternalObservation{ResourceExists: false}, nil
	}
	if err != nil {
		return managed.ExternalObservation{}, errors.Wrap(err, errGetProject)
	}

	cr.Status.AtProvider.ID = observed.ID
	meta.SetExternalName(cr, observed.ID)
	cr.SetConditions(xpv1.Available())

	lateInitProject(&cr.Spec.ForProvider, observed)

	ldapUpToDate := len(cr.Spec.ForProvider.LDAPGroupMappings) == 0 ||
		ldapMappingsMatch(cr.Spec.ForProvider.LDAPGroupMappings, observed.LDAPGroupMappings)

	// Snapshot schedule check. Skipped during delete so the Delete path can
	// progress even when backup has been torn down. If declared but no backup
	// config exists in the project yet, surface a clear error so the CR shows
	// Synced=False with a readable reason (per the "say so loudly" choice).
	scheduleUpToDate := true
	if cr.Spec.ForProvider.SnapshotSchedule != nil && !meta.WasDeleted(cr) {
		clusterID, err := e.findBackupClusterID(ctx, cr, observed.ID)
		if err != nil {
			return managed.ExternalObservation{}, err
		}
		currentSchedule, _, err := e.snapshotSchedule.Get(ctx, observed.ID, clusterID)
		if isNotFound(err) {
			// Backup config listed but inactive — the SnapshotSchedule endpoint
			// returns 404 BACKUP_CONFIG_NOT_FOUND until the config is activated.
			// Treat identically to "no backup config" so the CR surfaces one
			// consistent reason regardless of which OM API path revealed it.
			e.recorder.Event(cr, event.Warning(reasonNoBackupConfig, errors.New(errNoBackupConfig)))
			return managed.ExternalObservation{}, errors.New(errNoBackupConfig)
		}
		if err != nil {
			return managed.ExternalObservation{}, errors.Wrap(err, errGetSnapshotSchedule)
		}
		scheduleUpToDate = snapshotScheduleMatches(cr.Spec.ForProvider.SnapshotSchedule, currentSchedule)
	}

	upToDate := ldapUpToDate && scheduleUpToDate
	e.log.Debug("Observe: project found", "id", observed.ID, "upToDate", upToDate,
		"ldapUpToDate", ldapUpToDate, "scheduleUpToDate", scheduleUpToDate,
		"ldapMappings", len(cr.Spec.ForProvider.LDAPGroupMappings),
		"snapshotSchedule", cr.Spec.ForProvider.SnapshotSchedule != nil)

	return managed.ExternalObservation{
		ResourceExists:   true,
		ResourceUpToDate: upToDate,
	}, nil
}

func (e *external) Create(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalCreation, error) {
	// Check if the project already exists in Ops Manager before creating.
	existing, _, err := e.service.GetByName(ctx, cr.Spec.ForProvider.Name)
	if err == nil {
		cr.Status.AtProvider.ID = existing.ID
		meta.SetExternalName(cr, existing.ID)
		lateInitProject(&cr.Spec.ForProvider, existing)
		e.log.Debug("Create: adopted existing project", "id", existing.ID)
		e.recorder.Event(cr, event.Normal(reasonAdoptedProject,
			"adopted existing Ops Manager project "+existing.ID))
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

	e.log.Debug("Create: creating project in Ops Manager", "orgId", cr.Spec.ForProvider.OrgID,
		"ldapMappings", len(cr.Spec.ForProvider.LDAPGroupMappings))
	created, _, err := e.service.Create(ctx, project, nil)
	if err != nil {
		return managed.ExternalCreation{}, errors.Wrap(err, errCreateProject)
	}

	cr.Status.AtProvider.ID = created.ID
	meta.SetExternalName(cr, created.ID)
	e.recorder.Event(cr, event.Normal(reasonCreatedProject,
		"created Ops Manager project "+created.ID))

	return managed.ExternalCreation{}, nil
}

func (e *external) Update(ctx context.Context, cr *v1alpha1.OpsManagerProject) (managed.ExternalUpdate, error) {
	patch := &opsmngr.Project{
		LDAPGroupMappings: toSDKMappings(cr.Spec.ForProvider.LDAPGroupMappings),
	}

	e.log.Debug("Update: patching LDAP group mappings", "id", cr.Status.AtProvider.ID,
		"mappings", len(cr.Spec.ForProvider.LDAPGroupMappings))
	if _, _, err := e.service.Update(ctx, cr.Status.AtProvider.ID, patch); err != nil {
		return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateProject)
	}

	if cr.Spec.ForProvider.SnapshotSchedule != nil {
		clusterID, err := e.findBackupClusterID(ctx, cr, cr.Status.AtProvider.ID)
		if err != nil {
			return managed.ExternalUpdate{}, err
		}
		schedulePatch := toSDKSchedule(cr.Spec.ForProvider.SnapshotSchedule)
		e.log.Debug("Update: patching snapshot schedule",
			"id", cr.Status.AtProvider.ID, "clusterId", clusterID)
		if _, _, err := e.snapshotSchedule.Update(ctx, cr.Status.AtProvider.ID, clusterID, schedulePatch); err != nil {
			return managed.ExternalUpdate{}, errors.Wrap(err, errUpdateSnapSchedule)
		}
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

	e.log.Debug("Delete: scanning backup configs before project delete",
		"id", projectID, "configs", len(configs.Results))

	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStarted {
			e.log.Debug("Delete: stopping backup config", "clusterId", bc.ClusterID)
			e.recorder.Event(cr, event.Normal(reasonStoppingBackup,
				"stopping backup config for cluster "+bc.ClusterID))
			patch := &opsmngr.BackupConfig{StatusName: backupStatusStopped}
			if _, _, err := e.backupConfigs.Update(ctx, projectID, bc.ClusterID, patch); err != nil {
				return managed.ExternalDelete{}, errors.Wrap(err, errStopBackupConfig)
			}
		}
	}
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStarted {
			e.log.Debug("Delete: waiting for backup config to reach STOPPED", "clusterId", bc.ClusterID)
			return managed.ExternalDelete{}, errors.Errorf("waiting for backup config %s to stop", bc.ClusterID)
		}
	}

	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStopped {
			e.log.Debug("Delete: terminating backup config", "clusterId", bc.ClusterID)
			e.recorder.Event(cr, event.Normal(reasonTerminatingBackup,
				"terminating backup config for cluster "+bc.ClusterID))
			patch := &opsmngr.BackupConfig{StatusName: backupStatusTerminating}
			if _, _, err := e.backupConfigs.Update(ctx, projectID, bc.ClusterID, patch); err != nil {
				return managed.ExternalDelete{}, errors.Wrap(err, errTerminateBackupConfig)
			}
		}
	}
	for _, bc := range configs.Results {
		if bc.StatusName == backupStatusStopped || bc.StatusName == backupStatusTerminating {
			e.log.Debug("Delete: waiting for backup config to terminate",
				"clusterId", bc.ClusterID, "status", bc.StatusName)
			return managed.ExternalDelete{}, errors.Errorf("waiting for backup config %s to terminate", bc.ClusterID)
		}
	}

	e.log.Debug("Delete: deleting project", "id", projectID)
	_, err = e.service.Delete(ctx, projectID)
	if isNotFound(err) {
		return managed.ExternalDelete{}, nil
	}
	if err == nil {
		e.recorder.Event(cr, event.Normal(reasonDeletedProject,
			"deleted Ops Manager project "+projectID))
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

// --- snapshotSchedule helpers ---

// findBackupClusterID locates the clusterId of the project's single backup config.
// Returns a sentinel error (recorded as a NoBackupConfig event) when the project
// has no backup enabled yet — the SnapshotSchedule API only exists per clusterId.
// If multiple configs exist (unexpected for a 1:1 project→cluster topology),
// the first is used and a Debug line is emitted.
func (e *external) findBackupClusterID(ctx context.Context, cr *v1alpha1.OpsManagerProject, projectID string) (string, error) {
	configs, _, err := e.backupConfigs.List(ctx, projectID, nil)
	if err != nil {
		return "", errors.Wrap(err, errListBackupConfigs)
	}
	if len(configs.Results) == 0 {
		e.recorder.Event(cr, event.Warning(reasonNoBackupConfig, errors.New(errNoBackupConfig)))
		return "", errors.New(errNoBackupConfig)
	}
	if len(configs.Results) > 1 {
		e.log.Debug("found multiple backup configs; using the first",
			"count", len(configs.Results), "clusterId", configs.Results[0].ClusterID)
	}
	return configs.Results[0].ClusterID, nil
}

// snapshotScheduleMatches reports whether every field explicitly set in the
// desired spec equals the value observed on the API. Nil fields in the spec
// are not compared ("nil = don't manage"), consistent with the LDAP pattern.
func snapshotScheduleMatches(desired *v1alpha1.SnapshotSchedule, observed *opsmngr.SnapshotSchedule) bool {
	if desired == nil || observed == nil {
		return true
	}
	// Plain int fields on the SDK (observed is int, not *int).
	if desired.SnapshotRetentionDays != nil && *desired.SnapshotRetentionDays != observed.SnapshotRetentionDays {
		return false
	}
	if desired.SnapshotIntervalHours != nil && *desired.SnapshotIntervalHours != observed.SnapshotIntervalHours {
		return false
	}
	if desired.ClusterCheckpointIntervalMin != nil && *desired.ClusterCheckpointIntervalMin != observed.ClusterCheckpointIntervalMin {
		return false
	}
	// *int fields: a nil observed field never matches a non-nil desired.
	if !intPtrEq(desired.DailySnapshotRetentionDays, observed.DailySnapshotRetentionDays) {
		return false
	}
	if !intPtrEq(desired.WeeklySnapshotRetentionWeeks, observed.WeeklySnapshotRetentionWeeks) {
		return false
	}
	if !intPtrEq(desired.MonthlySnapshotRetentionMonths, observed.MonthlySnapshotRetentionMonths) {
		return false
	}
	if !intPtrEq(desired.PointInTimeWindowHours, observed.PointInTimeWindowHours) {
		return false
	}
	if !intPtrEq(desired.ReferenceHourOfDay, observed.ReferenceHourOfDay) {
		return false
	}
	if !intPtrEq(desired.ReferenceMinuteOfHour, observed.ReferenceMinuteOfHour) {
		return false
	}
	return true
}

// intPtrEq returns true when desired is nil (we don't care about this field)
// or when both desired and observed are non-nil and equal.
func intPtrEq(desired, observed *int) bool {
	if desired == nil {
		return true
	}
	if observed == nil {
		return false
	}
	return *desired == *observed
}

// toSDKSchedule builds a PATCH payload that only sets the fields the user
// declared in spec; unset fields are omitted so Ops Manager preserves them.
func toSDKSchedule(s *v1alpha1.SnapshotSchedule) *opsmngr.SnapshotSchedule {
	out := &opsmngr.SnapshotSchedule{}
	if s.SnapshotRetentionDays != nil {
		out.SnapshotRetentionDays = *s.SnapshotRetentionDays
	}
	if s.SnapshotIntervalHours != nil {
		out.SnapshotIntervalHours = *s.SnapshotIntervalHours
	}
	if s.ClusterCheckpointIntervalMin != nil {
		out.ClusterCheckpointIntervalMin = *s.ClusterCheckpointIntervalMin
	}
	if s.DailySnapshotRetentionDays != nil {
		v := *s.DailySnapshotRetentionDays
		out.DailySnapshotRetentionDays = &v
	}
	if s.WeeklySnapshotRetentionWeeks != nil {
		v := *s.WeeklySnapshotRetentionWeeks
		out.WeeklySnapshotRetentionWeeks = &v
	}
	if s.MonthlySnapshotRetentionMonths != nil {
		v := *s.MonthlySnapshotRetentionMonths
		out.MonthlySnapshotRetentionMonths = &v
	}
	if s.PointInTimeWindowHours != nil {
		v := *s.PointInTimeWindowHours
		out.PointInTimeWindowHours = &v
	}
	if s.ReferenceHourOfDay != nil {
		v := *s.ReferenceHourOfDay
		out.ReferenceHourOfDay = &v
	}
	if s.ReferenceMinuteOfHour != nil {
		v := *s.ReferenceMinuteOfHour
		out.ReferenceMinuteOfHour = &v
	}
	return out
}
