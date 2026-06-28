package reflector

import (
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestPatternListMatch(t *testing.T) {
	cases := []struct {
		patterns string
		value    string
		want     bool
	}{
		{"", "anything", true},
		{"ns-a,ns-b", "ns-a", true},
		{"ns-a,ns-b", "ns-c", false},
		{"namespace-[0-9]*", "namespace-12", true},
		{"namespace-[0-9]*", "namespace-x", false},
		{"team-.*", "team-alpha", true},
		{"prod", "prod-1", false}, // must match the whole value
		{"prod", "myprod", false},
		{" team-a , team-b ", "team-b", true}, // patterns are trimmed
	}
	for _, c := range cases {
		if got := patternListMatch(c.patterns, c.value); got != c.want {
			t.Errorf("patternListMatch(%q, %q) = %v, want %v", c.patterns, c.value, got, c.want)
		}
	}
}

func TestGlobPatterns(t *testing.T) {
	patterns := ParseGlobPatterns("kube-system,*-temp,prefix-*")
	cases := map[string]bool{
		"kube-system": true,
		"foo-temp":    true,
		"prefix-bar":  true,
		"other":       false,
		"kube-public": false,
	}
	for ns, want := range cases {
		if got := IsNamespaceExcluded(ns, patterns); got != want {
			t.Errorf("IsNamespaceExcluded(%q) = %v, want %v", ns, got, want)
		}
	}
	if IsNamespaceExcluded("anything", ParseGlobPatterns("")) {
		t.Error("empty pattern list must not exclude")
	}
}

func nsWithLabels(name string, labels map[string]string) *corev1.Namespace {
	return &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: name, Labels: labels}}
}

func TestLabelSelectorMatch(t *testing.T) {
	ns := nsWithLabels("team", map[string]string{"env": "prod"})
	cases := []struct {
		selector string
		want     bool
	}{
		{"", true},
		{"env=prod", true},
		{"env=dev", false},
		{"env in (prod,staging)", true},
		{"env notin (dev)", true},
		{"!disabled", true},
		{"env", true},
		{"=invalid", false}, // malformed -> fail closed
	}
	for _, c := range cases {
		if got := labelSelectorMatch(c.selector, ns); got != c.want {
			t.Errorf("labelSelectorMatch(%q) = %v, want %v", c.selector, got, c.want)
		}
	}
}

func TestCanBeReflectedTo(t *testing.T) {
	prod := nsWithLabels("prod-1", map[string]string{"env": "prod"})
	dev := nsWithLabels("dev-1", map[string]string{"env": "dev"})

	// Not allowed at all.
	p := Properties{Allowed: false, AllowedNamespaces: "prod-1"}
	if p.CanBeReflectedTo(prod) {
		t.Error("must not reflect when not allowed")
	}

	// Allowed, name pattern.
	p = Properties{Allowed: true, AllowedNamespaces: "prod-.*"}
	if !p.CanBeReflectedTo(prod) || p.CanBeReflectedTo(dev) {
		t.Error("name pattern matching failed")
	}

	// Allowed, empty namespaces -> all namespaces.
	p = Properties{Allowed: true}
	if !p.CanBeReflectedTo(prod) || !p.CanBeReflectedTo(dev) {
		t.Error("empty namespaces should allow all")
	}

	// Allowed, label selector only.
	p = Properties{Allowed: true, AllowedNamespacesSelector: "env=prod"}
	if !p.CanBeReflectedTo(prod) || p.CanBeReflectedTo(dev) {
		t.Error("label selector matching failed")
	}

	// Auto requires auto-enabled and auto-namespaces.
	p = Properties{Allowed: true, AllowedNamespaces: "prod-.*", AutoEnabled: false}
	if p.CanBeAutoReflectedTo(prod) {
		t.Error("auto must require auto-enabled")
	}
	p.AutoEnabled = true
	if !p.CanBeAutoReflectedTo(prod) {
		t.Error("auto should be permitted when enabled and reflectable")
	}
}
