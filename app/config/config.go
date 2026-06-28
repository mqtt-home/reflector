// Package config loads the reflector configuration from environment variables.
//
// The variable names mirror the ones the official emberstack Helm chart injects,
// so this binary is a drop-in replacement for the original C# image:
//
//	ES_Serilog__MinimumLevel__Default     log level (Verbose/Debug/Information/Warning/Error/Fatal)
//	ES_Reflector__Watcher__Timeout        watch/resync timeout in seconds (default 3600)
//	ES_Reflector__Watcher__ExcludedNamespaces  comma-separated glob list of namespaces to ignore
//	ES_Ignite__KubernetesClient__SkipTlsVerify skip API server TLS verification
package config

import (
	"os"
	"strconv"
	"strings"
)

// Config holds the resolved runtime configuration.
type Config struct {
	// LogLevel is a go-logger level: trace, debug, info, warn or error.
	LogLevel string
	// WatcherTimeout is the watch/resync period in seconds.
	WatcherTimeout int
	// ExcludedNamespaces is the raw comma-separated glob list.
	ExcludedNamespaces string
	// SkipTLSVerify disables API server certificate verification.
	SkipTLSVerify bool
	// HealthAddr is the listen address for the health endpoints.
	HealthAddr string
}

// Load reads the configuration from the environment.
func Load() Config {
	return Config{
		LogLevel:           MapLogLevel(env("ES_Serilog__MinimumLevel__Default", "LOG_LEVEL")),
		WatcherTimeout:     parseTimeout(env("ES_Reflector__Watcher__Timeout")),
		ExcludedNamespaces: env("ES_Reflector__Watcher__ExcludedNamespaces"),
		SkipTLSVerify:      parseBool(env("ES_Ignite__KubernetesClient__SkipTlsVerify")),
		HealthAddr:         envDefault(":8080", "HEALTH_ADDR"),
	}
}

// env returns the first non-empty value among the given variable names.
func env(names ...string) string {
	for _, n := range names {
		if v := strings.TrimSpace(os.Getenv(n)); v != "" {
			return v
		}
	}
	return ""
}

func envDefault(def string, names ...string) string {
	if v := env(names...); v != "" {
		return v
	}
	return def
}

func parseBool(s string) bool {
	return strings.EqualFold(strings.TrimSpace(s), "true")
}

func parseTimeout(s string) int {
	if v, err := strconv.Atoi(strings.TrimSpace(s)); err == nil && v > 0 {
		return v
	}
	return 3600
}

// MapLogLevel converts a Serilog minimum level to a go-logger level.
func MapLogLevel(level string) string {
	switch strings.ToLower(strings.TrimSpace(level)) {
	case "verbose", "trace":
		return "trace"
	case "debug":
		return "debug"
	case "", "information", "info":
		return "info"
	case "warning", "warn":
		return "warn"
	case "error", "fatal", "critical":
		return "error"
	default:
		return "info"
	}
}
