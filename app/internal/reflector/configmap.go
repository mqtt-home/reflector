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

type configMapBackend struct {
	client   kubernetes.Interface
	informer cache.SharedIndexInformer
	lister   listersv1.ConfigMapLister
}

// NewConfigMapBackend builds a ConfigMap backend from the given factory.
func NewConfigMapBackend(client kubernetes.Interface, factory informers.SharedInformerFactory) Backend {
	inf := factory.Core().V1().ConfigMaps()
	informer := inf.Informer()
	informer.SetTransform(stripManagedFields) //nolint:errcheck
	_ = informer.AddIndexers(cache.Indexers{nameIndex: nameIndexFunc})
	return &configMapBackend{client: client, informer: informer, lister: inf.Lister()}
}

func (b *configMapBackend) Kind() string                        { return "ConfigMap" }
func (b *configMapBackend) Informer() cache.SharedIndexInformer { return b.informer }

func (b *configMapBackend) GetMeta(ns, name string) (metav1.Object, bool) {
	c, err := b.lister.ConfigMaps(ns).Get(name)
	if err != nil {
		return nil, false
	}
	return c, true
}

func (b *configMapBackend) ListByName(name string) []metav1.Object {
	objs, err := b.informer.GetIndexer().ByIndex(nameIndex, name)
	if err != nil {
		return nil
	}
	out := make([]metav1.Object, 0, len(objs))
	for _, o := range objs {
		if c, ok := o.(*corev1.ConfigMap); ok {
			out = append(out, c)
		}
	}
	return out
}

func (b *configMapBackend) ListSourceKeys() []NamespacedName {
	var keys []NamespacedName
	for _, o := range b.informer.GetStore().List() {
		c, ok := o.(*corev1.ConfigMap)
		if !ok {
			continue
		}
		if _, isMirror := c.Annotations[AnnReflects]; isMirror {
			continue
		}
		keys = append(keys, NamespacedName{Namespace: c.Namespace, Name: c.Name})
	}
	return keys
}

func (b *configMapBackend) Create(ctx context.Context, target, source NamespacedName, auto bool) error {
	src, err := b.lister.ConfigMaps(source.Namespace).Get(source.Name)
	if err != nil {
		return err
	}
	mirror := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:        target.Name,
			Namespace:   target.Namespace,
			Annotations: mergedAnnotations(nil, auto, source, src.ResourceVersion),
		},
		Data:       src.Data,
		BinaryData: src.BinaryData,
	}
	_, err = b.client.CoreV1().ConfigMaps(target.Namespace).Create(ctx, mirror, metav1.CreateOptions{})
	if apierrors.IsAlreadyExists(err) {
		return b.Patch(ctx, target, source, auto)
	}
	return err
}

func (b *configMapBackend) Patch(ctx context.Context, target, source NamespacedName, auto bool) error {
	src, err := b.lister.ConfigMaps(source.Namespace).Get(source.Name)
	if err != nil {
		return err
	}
	var existing map[string]string
	if cur, cerr := b.lister.ConfigMaps(target.Namespace).Get(target.Name); cerr == nil {
		existing = cur.Annotations
	}
	patch := []jsonPatchOp{
		{Op: "add", Path: "/metadata/annotations", Value: mergedAnnotations(existing, auto, source, src.ResourceVersion)},
		{Op: "add", Path: "/data", Value: src.Data},
		{Op: "add", Path: "/binaryData", Value: src.BinaryData},
	}
	data, err := json.Marshal(patch)
	if err != nil {
		return err
	}
	_, err = b.client.CoreV1().ConfigMaps(target.Namespace).Patch(ctx, target.Name, types.JSONPatchType, data, metav1.PatchOptions{})
	return err
}

func (b *configMapBackend) Delete(ctx context.Context, target NamespacedName) error {
	err := b.client.CoreV1().ConfigMaps(target.Namespace).Delete(ctx, target.Name, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}
