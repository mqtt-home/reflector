package config

import "testing"

func TestMapLogLevel(t *testing.T) {
	cases := map[string]string{
		"":            "info",
		"Verbose":     "trace",
		"Debug":       "debug",
		"Information": "info",
		"info":        "info",
		"Warning":     "warn",
		"Error":       "error",
		"Fatal":       "error",
		"nonsense":    "info",
	}
	for in, want := range cases {
		if got := MapLogLevel(in); got != want {
			t.Errorf("MapLogLevel(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestParseTimeout(t *testing.T) {
	cases := map[string]int{
		"":      3600,
		"abc":   3600,
		"0":     3600,
		"-5":    3600,
		"120":   120,
		" 300 ": 300,
	}
	for in, want := range cases {
		if got := parseTimeout(in); got != want {
			t.Errorf("parseTimeout(%q) = %d, want %d", in, got, want)
		}
	}
}

func TestLoadDefaults(t *testing.T) {
	t.Setenv("ES_Serilog__MinimumLevel__Default", "")
	t.Setenv("ES_Reflector__Watcher__Timeout", "")
	t.Setenv("ES_Reflector__Watcher__ExcludedNamespaces", "kube-system")
	t.Setenv("ES_Ignite__KubernetesClient__SkipTlsVerify", "true")
	cfg := Load()
	if cfg.LogLevel != "info" {
		t.Errorf("LogLevel = %q, want info", cfg.LogLevel)
	}
	if cfg.WatcherTimeout != 3600 {
		t.Errorf("WatcherTimeout = %d, want 3600", cfg.WatcherTimeout)
	}
	if cfg.ExcludedNamespaces != "kube-system" {
		t.Errorf("ExcludedNamespaces = %q", cfg.ExcludedNamespaces)
	}
	if !cfg.SkipTLSVerify {
		t.Error("SkipTLSVerify should be true")
	}
	if cfg.HealthAddr != ":8080" {
		t.Errorf("HealthAddr = %q, want :8080", cfg.HealthAddr)
	}
}
