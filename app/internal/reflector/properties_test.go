package reflector

import (
	"testing"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func TestParseNamespacedName(t *testing.T) {
	cases := []struct {
		in    string
		ns    string
		name  string
		valid bool
	}{
		{"default/secret", "default", "secret", true},
		{" default/secret ", "default", "secret", true},
		{"secret", "", "", false},
		{"/secret", "", "", false},
		{"default/", "", "", false},
	}
	for _, c := range cases {
		got, ok := ParseNamespacedName(c.in)
		if ok != c.valid {
			t.Errorf("ParseNamespacedName(%q) valid = %v, want %v", c.in, ok, c.valid)
			continue
		}
		if ok && (got.Namespace != c.ns || got.Name != c.name) {
			t.Errorf("ParseNamespacedName(%q) = %v, want %s/%s", c.in, got, c.ns, c.name)
		}
	}
}

func TestParseProperties(t *testing.T) {
	meta := &metav1.ObjectMeta{
		ResourceVersion: "42",
		Annotations: map[string]string{
			AnnReflectionAllowed:           "true",
			AnnReflectionAllowedNamespaces: "team-.*",
			AnnReflectionAutoEnabled:       "True",
			AnnReflectionAutoNamespaces:    "team-prod",
			AnnReflects:                    "source-ns/source",
			AnnMetaAutoReflects:            "True",
			AnnMetaReflectedVersion:        "41",
		},
	}
	p := ParseProperties(meta)
	if !p.Allowed || !p.AutoEnabled {
		t.Error("expected allowed and auto-enabled to be true")
	}
	if p.AllowedNamespaces != "team-.*" || p.AutoNamespaces != "team-prod" {
		t.Error("namespace patterns not parsed")
	}
	if !p.IsReflection() || p.Reflects.String() != "source-ns/source" {
		t.Errorf("reflects not parsed: %+v", p.Reflects)
	}
	if !p.IsAutoReflection {
		t.Error("auto-reflects not parsed")
	}
	if p.ReflectedVersion != "41" || p.ResourceVersion != "42" {
		t.Error("versions not parsed")
	}

	// No annotations -> empty source properties.
	empty := ParseProperties(&metav1.ObjectMeta{})
	if empty.Allowed || empty.IsReflection() {
		t.Error("empty metadata should produce empty properties")
	}
}

func TestBoolStr(t *testing.T) {
	if boolStr(true) != "True" || boolStr(false) != "False" {
		t.Error("boolStr must produce .NET-style capitalised booleans")
	}
}
