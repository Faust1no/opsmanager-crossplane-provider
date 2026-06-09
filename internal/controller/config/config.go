// Package config wires the ProviderConfig reconcilers that account for usage
// of both the cluster-scoped ClusterProviderConfig and the namespace-scoped
// ProviderConfig. A managed resource can reference either kind via its
// providerConfigRef.kind field.
package config

import (
	"github.com/crossplane/crossplane-runtime/v2/pkg/controller"
	"github.com/crossplane/crossplane-runtime/v2/pkg/event"
	"github.com/crossplane/crossplane-runtime/v2/pkg/ratelimiter"
	"github.com/crossplane/crossplane-runtime/v2/pkg/reconciler/providerconfig"
	"github.com/crossplane/crossplane-runtime/v2/pkg/resource"
	ctrl "sigs.k8s.io/controller-runtime"

	"github.com/crossplane-contrib/provider-opsmanager/apis/v1beta1"
)

// Setup wires both ProviderConfig reconcilers: ClusterProviderConfig
// (cluster-scoped) and ProviderConfig (namespace-scoped).
func Setup(mgr ctrl.Manager, o controller.Options) error {
	if err := setupClusterProviderConfig(mgr, o); err != nil {
		return err
	}
	return setupProviderConfig(mgr, o)
}

func setupClusterProviderConfig(mgr ctrl.Manager, o controller.Options) error {
	name := providerconfig.ControllerName(v1beta1.ClusterProviderConfigGroupKind.Kind)

	of := resource.ProviderConfigKinds{
		Config:    v1beta1.ClusterProviderConfigGroupVersionKind,
		Usage:     v1beta1.ClusterProviderConfigUsageGroupVersionKind,
		UsageList: v1beta1.ClusterProviderConfigUsageListGroupVersionKind,
	}

	r := providerconfig.NewReconciler(mgr, of,
		providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
		providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1beta1.ClusterProviderConfig{}).
		Watches(&v1beta1.ClusterProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}

func setupProviderConfig(mgr ctrl.Manager, o controller.Options) error {
	name := providerconfig.ControllerName(v1beta1.ProviderConfigGroupKind.Kind)

	of := resource.ProviderConfigKinds{
		Config:    v1beta1.ProviderConfigGroupVersionKind,
		Usage:     v1beta1.ProviderConfigUsageGroupVersionKind,
		UsageList: v1beta1.ProviderConfigUsageListGroupVersionKind,
	}

	r := providerconfig.NewReconciler(mgr, of,
		providerconfig.WithLogger(o.Logger.WithValues("controller", name)),
		providerconfig.WithRecorder(event.NewAPIRecorder(mgr.GetEventRecorderFor(name))))

	return ctrl.NewControllerManagedBy(mgr).
		Named(name).
		WithOptions(o.ForControllerRuntime()).
		For(&v1beta1.ProviderConfig{}).
		Watches(&v1beta1.ProviderConfigUsage{}, &resource.EnqueueRequestForProviderConfig{}).
		Complete(ratelimiter.NewReconciler(name, r, o.GlobalRateLimiter))
}
