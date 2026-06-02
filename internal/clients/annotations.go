package clients

import (
	"context"

	"github.com/crossplane/crossplane-runtime/pkg/meta"
	"github.com/crossplane/crossplane-runtime/pkg/resource"
	"github.com/pkg/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/util/retry"
	"sigs.k8s.io/controller-runtime/pkg/client"
)

const errUpdateCriticalAnnotations = "cannot update critical annotations"

// NamespacedCriticalAnnotationUpdater fixes a bug present in all crossplane-runtime
// v0.x releases where RetryingCriticalAnnotationUpdater omits the namespace from
// the re-fetch NamespacedName. For namespace-scoped resources the Get returns
// NotFound and the annotation update is silently dropped, leaving
// external-create-pending permanently stuck after any failed Create.
type NamespacedCriticalAnnotationUpdater struct {
	client client.Client
}

// NewNamespacedCriticalAnnotationUpdater returns a CriticalAnnotationUpdater
// that correctly handles namespace-scoped managed resources.
func NewNamespacedCriticalAnnotationUpdater(c client.Client) *NamespacedCriticalAnnotationUpdater {
	return &NamespacedCriticalAnnotationUpdater{client: c}
}

func (u *NamespacedCriticalAnnotationUpdater) UpdateCriticalAnnotations(ctx context.Context, o client.Object) error {
	a := o.GetAnnotations()
	err := retry.OnError(retry.DefaultRetry, resource.IsAPIError, func() error {
		nn := types.NamespacedName{
			Name:      o.GetName(),
			Namespace: o.GetNamespace(),
		}
		if err := u.client.Get(ctx, nn, o); err != nil {
			return err
		}
		meta.AddAnnotations(o, a)
		return u.client.Update(ctx, o)
	})
	return errors.Wrap(err, errUpdateCriticalAnnotations)
}
