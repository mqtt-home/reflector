package reflector

import (
	"regexp"
	"strings"
)

// ParseGlobPatterns compiles a comma-separated list of namespace glob patterns into
// anchored regular expressions. Supported wildcards are '*' (any characters) and
// '?' (single character). Patterns are lowercased because Kubernetes namespace
// names are always lowercase DNS-1123 labels. This matches the original GlobMatcher.
func ParseGlobPatterns(patterns string) []*regexp.Regexp {
	patterns = strings.ToLower(strings.TrimSpace(patterns))
	if patterns == "" {
		return nil
	}
	var result []*regexp.Regexp
	for _, raw := range strings.Split(patterns, ",") {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		rx := "^" + regexp.QuoteMeta(p) + "$"
		rx = strings.ReplaceAll(rx, `\*`, ".*")
		rx = strings.ReplaceAll(rx, `\?`, ".")
		re, err := regexp.Compile(rx)
		if err != nil {
			continue
		}
		result = append(result, re)
	}
	return result
}

// IsNamespaceExcluded reports whether the namespace matches any exclusion pattern.
func IsNamespaceExcluded(ns string, patterns []*regexp.Regexp) bool {
	if len(patterns) == 0 || ns == "" {
		return false
	}
	for _, p := range patterns {
		if p.MatchString(ns) {
			return true
		}
	}
	return false
}
