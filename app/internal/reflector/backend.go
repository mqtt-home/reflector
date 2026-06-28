package reflector

import (
	"context"
	"maps"
	"time"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/tools/cache"
)

// nameIndex indexes objects by their metadata name so all same-named objects across
// namespaces (potential mirrors of a single source) can be looked up cheaply.
const nameIndex = "name"

func nameIndexFunc(obj any) ([]string, error) {
	m, err := meta.Accessor(obj)
	if err != nil {
		return nil, err
	}
	return []string{m.GetName()}, nil
}

// Backend abstracts the per-resource-type (Secret / ConfigMap) operations that the
// generic controller needs. Both implementations read source/target objects from
// their informer cache and only hit the API server for writes.
type Backend interface {
	// Kind is the human-readable resource kind, used in logs.
	Kind() string
	// Informer returns the shared informer backing the cache.
	Informer() cache.SharedIndexInformer
	// GetMeta returns the object's metadata from the cache, or ok=false if absent.
	GetMeta(ns, name string) (metav1.Object, bool)
	// ListByName returns the metadata of every cached object with the given name.
	ListByName(name string) []metav1.Object
	// ListSourceKeys returns the keys of every cached object that is not a mirror.
	ListSourceKeys() []NamespacedName
	// Create makes a new mirror of source in target's namespace.
	Create(ctx context.Context, target, source NamespacedName, auto bool) error
	// Patch updates an existing mirror's payload and reflector annotations.
	Patch(ctx context.Context, target, source NamespacedName, auto bool) error
	// Delete removes a mirror. A missing object is not an error.
	Delete(ctx context.Context, target NamespacedName) error
}

// timeNow is overridable in tests.
var timeNow = time.Now

func boolStr(b bool) string {
	if b {
		return "True"
	}
	return "False"
}

// reflectedAt renders the current time in the ISO-8601 "round-trip" form used by
// the original implementation for the reflected-at annotation.
func reflectedAt() string {
	return timeNow().UTC().Format("2006-01-02T15:04:05.0000000Z07:00")
}

// mergedAnnotations returns a copy of existing with the four reflector meta
// annotations set/overwritten. User annotations on the mirror are preserved.
func mergedAnnotations(existing map[string]string, auto bool, source NamespacedName, rv string) map[string]string {
	out := make(map[string]string, len(existing)+4)
	maps.Copy(out, existing)
	out[AnnMetaAutoReflects] = boolStr(auto)
	out[AnnReflects] = source.String()
	out[AnnMetaReflectedVersion] = rv
	out[AnnMetaReflectedAt] = reflectedAt()
	return out
}

// jsonPatchOp is a single RFC-6902 JSON patch operation. "add" on an existing
// object member replaces it wholesale, which gives us replace-semantics for the
// payload (so removed keys disappear from the mirror).
type jsonPatchOp struct {
	Op    string `json:"op"`
	Path  string `json:"path"`
	Value any    `json:"value"`
}

// stripManagedFields is an informer transform that drops bookkeeping data we never
// use, substantially reducing the in-memory cache footprint.
func stripManagedFields(obj any) (any, error) {
	if m, err := meta.Accessor(obj); err == nil {
		m.SetManagedFields(nil)
		if a := m.GetAnnotations(); a != nil {
			delete(a, "kubectl.kubernetes.io/last-applied-configuration")
			m.SetAnnotations(a)
		}
	}
	return obj, nil
}
