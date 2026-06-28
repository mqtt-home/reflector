package reflector

import (
	"context"
	"maps"
	"regexp"

	"github.com/philipparndt/go-logger"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/meta"
	"k8s.io/apimachinery/pkg/labels"
	coreinformers "k8s.io/client-go/informers/core/v1"
	listersv1 "k8s.io/client-go/listers/core/v1"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/util/workqueue"
)

// Controller reconciles a single resource type (Secret or ConfigMap). The reflection
// logic itself is type-agnostic and delegates all reads/writes to a Backend.
type Controller struct {
	kind     string
	backend  Backend
	nsLister listersv1.NamespaceLister
	excluded []*regexp.Regexp
	queue    workqueue.TypedRateLimitingInterface[string]
}

// NewController wires a controller for the given backend. The namespace informer is
// shared between controllers; each registers its own handler so namespace changes
// re-evaluate that controller's sources.
func NewController(backend Backend, nsInformer coreinformers.NamespaceInformer, excluded []*regexp.Regexp) *Controller {
	c := &Controller{
		kind:     backend.Kind(),
		backend:  backend,
		nsLister: nsInformer.Lister(),
		excluded: excluded,
		queue: workqueue.NewTypedRateLimitingQueueWithConfig(
			workqueue.DefaultTypedControllerRateLimiter[string](),
			workqueue.TypedRateLimitingQueueConfig[string]{Name: backend.Kind()}),
	}

	backend.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc:    c.onAdd,
		UpdateFunc: func(_, obj any) { c.onAdd(obj) },
		DeleteFunc: c.onDelete,
	})

	nsInformer.Informer().AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(_ any) { c.enqueueAllSources() },
		UpdateFunc: func(oldObj, newObj any) {
			oldNs, ok1 := oldObj.(*corev1.Namespace)
			newNs, ok2 := newObj.(*corev1.Namespace)
			// Reflection eligibility depends only on namespace name and labels, so
			// ignore updates that leave the labels unchanged.
			if ok1 && ok2 && maps.Equal(oldNs.Labels, newNs.Labels) {
				return
			}
			c.enqueueAllSources()
		},
		DeleteFunc: func(_ any) { c.enqueueAllSources() },
	})

	return c
}

func (c *Controller) isExcluded(ns string) bool {
	return IsNamespaceExcluded(ns, c.excluded)
}

func (c *Controller) onAdd(obj any) {
	m, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	if c.isExcluded(m.GetNamespace()) {
		return
	}
	c.queue.Add(NamespacedName{Namespace: m.GetNamespace(), Name: m.GetName()}.String())
}

// onDelete enqueues follow-up work for a deleted object: a deleted source schedules
// cleanup of its auto mirrors, and a deleted auto mirror schedules recreation by
// re-queuing its source.
func (c *Controller) onDelete(obj any) {
	if tomb, ok := obj.(cache.DeletedFinalStateUnknown); ok {
		obj = tomb.Obj
	}
	m, err := meta.Accessor(obj)
	if err != nil {
		return
	}
	props := ParseProperties(m)
	if props.IsReflection() {
		if props.IsAutoReflection {
			c.queue.Add(props.Reflects.String())
		}
		return
	}
	src := NamespacedName{Namespace: m.GetNamespace(), Name: m.GetName()}
	for _, mm := range c.backend.ListByName(src.Name) {
		mp := ParseProperties(mm)
		if mp.IsAutoReflection && mp.ReflectsSource(src) {
			c.queue.Add(NamespacedName{Namespace: mm.GetNamespace(), Name: mm.GetName()}.String())
		}
	}
}

func (c *Controller) enqueueAllSources() {
	for _, k := range c.backend.ListSourceKeys() {
		if c.isExcluded(k.Namespace) {
			continue
		}
		c.queue.Add(k.String())
	}
}

// Run starts the worker loop until the context is cancelled.
func (c *Controller) Run(ctx context.Context, workers int) {
	defer c.queue.ShutDown()
	for range workers {
		go c.runWorker(ctx)
	}
	<-ctx.Done()
}

func (c *Controller) runWorker(ctx context.Context) {
	for c.processNext(ctx) {
	}
}

func (c *Controller) processNext(ctx context.Context) bool {
	item, quit := c.queue.Get()
	if quit {
		return false
	}
	defer c.queue.Done(item)

	ns, name, err := cache.SplitMetaNamespaceKey(item)
	if err != nil {
		c.queue.Forget(item)
		return true
	}
	if err := c.reconcile(ctx, NamespacedName{Namespace: ns, Name: name}); err != nil {
		logger.Error("Reconcile failed", "kind", c.kind, "key", item, "error", err)
		c.queue.AddRateLimited(item)
		return true
	}
	c.queue.Forget(item)
	return true
}

func (c *Controller) reconcile(ctx context.Context, key NamespacedName) error {
	if c.isExcluded(key.Namespace) {
		return nil
	}
	m, ok := c.backend.GetMeta(key.Namespace, key.Name)
	if !ok {
		// Deletions are handled in onDelete via follow-up enqueues.
		return nil
	}
	props := ParseProperties(m)
	if props.IsReflection() {
		return c.reconcileMirror(ctx, key, props)
	}
	return c.reconcileSource(ctx, key, props)
}

// reconcileSource ensures every permitted mirror of the source is up to date and
// removes auto mirrors that are no longer permitted.
func (c *Controller) reconcileSource(ctx context.Context, src NamespacedName, props Properties) error {
	byNs := make(map[string]Properties)
	for _, m := range c.backend.ListByName(src.Name) {
		byNs[m.GetNamespace()] = ParseProperties(m)
	}

	// Direct (user-created) mirrors: keep them in sync but never delete them.
	for ns, mp := range byNs {
		if ns == src.Namespace || c.isExcluded(ns) {
			continue
		}
		if !mp.ReflectsSource(src) || mp.IsAutoReflection {
			continue
		}
		nsObj, err := c.nsLister.Get(ns)
		if err != nil {
			continue
		}
		if !props.CanBeReflectedTo(nsObj) {
			logger.Warn("Source no longer permits the direct reflection",
				"kind", c.kind, "source", src.String(), "target", ns+"/"+src.Name)
			continue
		}
		if mp.ReflectedVersion != props.ResourceVersion {
			c.patch(ctx, NamespacedName{Namespace: ns, Name: src.Name}, src, false)
		}
	}

	// Auto mirrors: create/update/delete across all namespaces.
	namespaces, err := c.nsLister.List(labels.Everything())
	if err != nil {
		return err
	}
	for _, nsObj := range namespaces {
		ns := nsObj.Name
		if ns == src.Namespace || c.isExcluded(ns) {
			continue
		}
		mp, has := byNs[ns]
		if props.CanBeAutoReflectedTo(nsObj) {
			switch {
			case !has:
				c.create(ctx, NamespacedName{Namespace: ns, Name: src.Name}, src)
			case mp.IsAutoReflection && mp.ReflectsSource(src):
				if mp.ReflectedVersion != props.ResourceVersion {
					c.patch(ctx, NamespacedName{Namespace: ns, Name: src.Name}, src, true)
				}
				// else: a same-named non-auto object already occupies the namespace; leave it.
			}
			continue
		}
		// Namespace no longer matches: remove our own auto mirror. When auto is still
		// enabled, any non-match is removed; when auto was disabled we only remove
		// mirrors the source can no longer reflect to at all (others are orphaned,
		// matching the original behaviour).
		if has && mp.IsAutoReflection && mp.ReflectsSource(src) {
			if props.AutoEnabled || !props.CanBeReflectedTo(nsObj) {
				c.delete(ctx, NamespacedName{Namespace: ns, Name: src.Name}, src, "namespace no longer matches")
			}
		}
	}
	return nil
}

// reconcileMirror restores or removes a single mirror after it (or its source) changed.
func (c *Controller) reconcileMirror(ctx context.Context, key NamespacedName, props Properties) error {
	src := *props.Reflects
	srcMeta, ok := c.backend.GetMeta(src.Namespace, src.Name)
	if !ok {
		if props.IsAutoReflection {
			c.delete(ctx, key, src, "source no longer exists")
			return nil
		}
		logger.Warn("Could not update reflection - source not found",
			"kind", c.kind, "target", key.String(), "source", src.String())
		return nil
	}
	sp := ParseProperties(srcMeta)
	nsObj, err := c.nsLister.Get(key.Namespace)
	if err != nil {
		return nil
	}

	if props.IsAutoReflection {
		if !sp.CanBeAutoReflectedTo(nsObj) {
			c.delete(ctx, key, src, "source no longer permits auto reflection")
			return nil
		}
		if props.ReflectedVersion != sp.ResourceVersion {
			c.patch(ctx, key, src, true)
		}
		return nil
	}

	if !sp.CanBeReflectedTo(nsObj) {
		logger.Warn("Source does not permit the reflection",
			"kind", c.kind, "target", key.String(), "source", src.String())
		return nil
	}
	if props.ReflectedVersion != sp.ResourceVersion {
		c.patch(ctx, key, src, false)
	}
	return nil
}

func (c *Controller) create(ctx context.Context, target, source NamespacedName) {
	if err := c.backend.Create(ctx, target, source, true); err != nil {
		logger.Error("Could not create reflection",
			"kind", c.kind, "target", target.String(), "source", source.String(), "error", err)
		return
	}
	logger.Info("Created reflection",
		"kind", c.kind, "target", target.String(), "source", source.String(), "auto", true)
}

func (c *Controller) patch(ctx context.Context, target, source NamespacedName, auto bool) {
	if err := c.backend.Patch(ctx, target, source, auto); err != nil {
		logger.Error("Could not update reflection",
			"kind", c.kind, "target", target.String(), "source", source.String(), "error", err)
		return
	}
	logger.Info("Updated reflection",
		"kind", c.kind, "target", target.String(), "source", source.String(), "auto", auto)
}

func (c *Controller) delete(ctx context.Context, target, source NamespacedName, reason string) {
	if err := c.backend.Delete(ctx, target); err != nil {
		logger.Error("Could not delete reflection",
			"kind", c.kind, "target", target.String(), "source", source.String(), "error", err)
		return
	}
	logger.Info("Deleted reflection",
		"kind", c.kind, "target", target.String(), "source", source.String(), "reason", reason)
}
