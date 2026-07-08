// Package config wires the ClusterProviderConfig reconciler. Every managed
// resource in this provider is cluster-scoped and references its Ops Manager
// via providerConfigRef.kind: ClusterProviderConfig.
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

// Setup wires the ClusterProviderConfig reconciler.
func Setup(mgr ctrl.Manager, o controller.Options) error {
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
