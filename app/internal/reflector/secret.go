package reflector

import (
	"context"
	"encoding/json"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
)

type secretBackend struct {
	client   kubernetes.Interface
	informer cache.SharedIndexInformer
	lister   listersv1.SecretLister
}

// NewSecretBackend builds a Secret backend from the given factory. The factory is
// expected to exclude helm release secrets via a field selector to keep churn and
// memory low.
func NewSecretBackend(client kubernetes.Interface, factory informers.SharedInformerFactory) Backend {
	inf := factory.Core().V1().Secrets()
	informer := inf.Informer()
	informer.SetTransform(stripManagedFields) //nolint:errcheck
	_ = informer.AddIndexers(cache.Indexers{nameIndex: nameIndexFunc})
	return &secretBackend{client: client, informer: informer, lister: inf.Lister()}
}

func (b *secretBackend) Kind() string                        { return "Secret" }
func (b *secretBackend) Informer() cache.SharedIndexInformer { return b.informer }

func (b *secretBackend) GetMeta(ns, name string) (metav1.Object, bool) {
	s, err := b.lister.Secrets(ns).Get(name)
	if err != nil {
		return nil, false
	}
	return s, true
}

func (b *secretBackend) ListByName(name string) []metav1.Object {
	objs, err := b.informer.GetIndexer().ByIndex(nameIndex, name)
	if err != nil {
		return nil
	}
	out := make([]metav1.Object, 0, len(objs))
	for _, o := range objs {
		if s, ok := o.(*corev1.Secret); ok {
			out = append(out, s)
		}
	}
	return out
}

func (b *secretBackend) ListSourceKeys() []NamespacedName {
	var keys []NamespacedName
	for _, o := range b.informer.GetStore().List() {
		s, ok := o.(*corev1.Secret)
		if !ok {
			continue
		}
		if _, isMirror := s.Annotations[AnnReflects]; isMirror {
			continue
		}
		keys = append(keys, NamespacedName{Namespace: s.Namespace, Name: s.Name})
	}
	return keys
}

func (b *secretBackend) Create(ctx context.Context, target, source NamespacedName, auto bool) error {
	src, err := b.lister.Secrets(source.Namespace).Get(source.Name)
	if err != nil {
		return err
	}
	mirror := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:        target.Name,
			Namespace:   target.Namespace,
			Annotations: mergedAnnotations(nil, auto, source, src.ResourceVersion),
		},
		Type: src.Type,
		Data: src.Data,
	}
	_, err = b.client.CoreV1().Secrets(target.Namespace).Create(ctx, mirror, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return b.Patch(ctx, target, source, auto)
	}
	return err
}

func (b *secretBackend) Patch(ctx context.Context, target, source NamespacedName, auto bool) error {
	src, err := b.lister.Secrets(source.Namespace).Get(source.Name)
	if err != nil {
		return err
	}
	var existing map[string]string
	if cur, cerr := b.lister.Secrets(target.Namespace).Get(target.Name); cerr == nil {
		existing = cur.Annotations
	}
	patch := []jsonPatchOp{
		{Op: "add", Path: "/metadata/annotations", Value: mergedAnnotations(existing, auto, source, src.ResourceVersion)},
		{Op: "add", Path: "/data", Value: src.Data},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = b.client.CoreV1().Secrets(target.Namespace).Patch(ctx, target.Name, types.JSONPatchType, data, metav1.PatchOptions{})
	return err
}

func (b *secretBackend) Delete(ctx context.Context, target NamespacedName) error {
	err := b.client.CoreV1().Secrets(target.Namespace).Delete(ctx, target.Name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
