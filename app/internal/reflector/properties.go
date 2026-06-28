package reflector

import (
	"strings"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// NamespacedName identifies a namespaced resource. Its string form ("namespace/name")
// is the exact format used by the reflects annotation.
type NamespacedName struct {
	Namespace string
	Name      string
}

func (n NamespacedName) String() string { return n.Namespace + "/" + n.Name }

// ParseNamespacedName parses "namespace/name". It returns false when the value is
// not a valid reference.
func ParseNamespacedName(s string) (NamespacedName, bool) {
	parts := strings.SplitN(strings.TrimSpace(s), "/", 2)
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return NamespacedName{}, false
	}
	return NamespacedName{Namespace: parts[0], Name: parts[1]}, true
}

// Properties is the parsed reflector state of a single object, derived purely from
// its annotations and resource version. It mirrors MirroringProperties in the
// original implementation.
type Properties struct {
	ResourceVersion string

	Allowed                   bool
	AllowedNamespaces         string
	AllowedNamespacesSelector string
	AutoEnabled               bool
	AutoNamespaces            string
	AutoNamespacesSelector    string

	// Reflects is non-nil when the object is a mirror.
	Reflects *NamespacedName

	IsAutoReflection bool
	ReflectedVersion string
}

// IsReflection reports whether the object is a mirror (has a reflects annotation).
func (p Properties) IsReflection() bool { return p.Reflects != nil }

// ReflectsSource reports whether the object is a mirror of the given source.
func (p Properties) ReflectsSource(src NamespacedName) bool {
	return p.Reflects != nil && *p.Reflects == src
}

func parseAnnotationBool(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "true")
}

// ParseProperties extracts the reflector properties from an object's metadata.
func ParseProperties(meta metav1.Object) Properties {
	a := meta.GetAnnotations()
	p := Properties{ResourceVersion: meta.GetResourceVersion()}
	if a == nil {
		return p
	}
	p.Allowed = parseAnnotationBool(a[AnnReflectionAllowed])
	p.AllowedNamespaces = a[AnnReflectionAllowedNamespaces]
	p.AllowedNamespacesSelector = a[AnnReflectionAllowedNsSelector]
	p.AutoEnabled = parseAnnotationBool(a[AnnReflectionAutoEnabled])
	p.AutoNamespaces = a[AnnReflectionAutoNamespaces]
	p.AutoNamespacesSelector = a[AnnReflectionAutoNsSelector]
	if v, ok := a[AnnReflects]; ok {
		if nn, ok := ParseNamespacedName(v); ok {
			p.Reflects = &nn
		}
	}
	p.IsAutoReflection = parseAnnotationBool(a[AnnMetaAutoReflects])
	p.ReflectedVersion = strings.TrimSpace(a[AnnMetaReflectedVersion])
	return p
}
