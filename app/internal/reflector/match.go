package reflector

import (
	"regexp"
	"strings"
	"sync"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/labels"
)

// regexCache memoises compiled namespace patterns. A nil entry records a pattern
// that failed to compile so it is skipped on every subsequent evaluation.
var regexCache sync.Map // map[string]*regexp.Regexp

func compilePattern(p string) *regexp.Regexp {
	if v, ok := regexCache.Load(p); ok {
		re, _ := v.(*regexp.Regexp)
		return re
	}
	re, err := regexp.Compile(p)
	if err != nil {
		re = nil
	}
	regexCache.Store(p, re)
	return re
}

// patternListMatch matches a value against a comma-separated list of regular
// expressions. Each pattern must match the whole value (the matched span has to
// cover the entire string). An empty list matches everything. This reproduces the
// original PatternListMatch semantics exactly.
func patternListMatch(patternList, value string) bool {
	if patternList == "" {
		return true
	}
	for _, raw := range strings.Split(patternList, ",") {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		re := compilePattern(p)
		if re == nil {
			continue
		}
		if loc := re.FindStringIndex(value); loc != nil && loc[1]-loc[0] == len(value) {
			return true
		}
	}
	return false
}

// labelSelectorMatch evaluates a standard Kubernetes label selector against a
// namespace's labels. Invalid selectors fail closed (return false).
func labelSelectorMatch(selector string, ns *corev1.Namespace) bool {
	if strings.TrimSpace(selector) == "" {
		return true
	}
	sel, err := labels.Parse(selector)
	if err != nil {
		return false
	}
	return sel.Matches(labels.Set(ns.Labels))
}

// matchNamespace returns true when the namespace matches either the name pattern
// list or the label selector. If both are empty it matches all namespaces.
func matchNamespace(patternList, selector string, ns *corev1.Namespace) bool {
	hasPatterns := patternList != ""
	hasSelector := selector != ""
	if !hasPatterns && !hasSelector {
		return true
	}
	if hasPatterns && patternListMatch(patternList, ns.Name) {
		return true
	}
	if hasSelector && labelSelectorMatch(selector, ns) {
		return true
	}
	return false
}

// CanBeReflectedTo reports whether the source permits reflection to the namespace.
func (p Properties) CanBeReflectedTo(ns *corev1.Namespace) bool {
	return p.Allowed && matchNamespace(p.AllowedNamespaces, p.AllowedNamespacesSelector, ns)
}

// CanBeAutoReflectedTo reports whether the source permits automatic reflection to
// the namespace.
func (p Properties) CanBeAutoReflectedTo(ns *corev1.Namespace) bool {
	return p.CanBeReflectedTo(ns) &&
		p.AutoEnabled &&
		matchNamespace(p.AutoNamespaces, p.AutoNamespacesSelector, ns)
}
