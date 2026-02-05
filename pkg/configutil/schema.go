package configutil

import (
	"errors"
	"sort"
	"strings"
)

// Schema defines required and optional keys for a settings map.
type Schema struct {
	Required     []string
	Optional     []string
	AllowUnknown bool
}

// ValidateSettings validates a settings map against a schema.
// Keys are normalized to be case/underscore/hyphen insensitive.
func ValidateSettings(input map[string]any, schema Schema) error {
	required := make(map[string]string, len(schema.Required))
	optional := make(map[string]struct{}, len(schema.Optional))
	for _, k := range schema.Required {
		required[normalizeKey(k)] = k
	}
	for _, k := range schema.Optional {
		optional[normalizeKey(k)] = struct{}{}
	}
	allowed := make(map[string]struct{}, len(required)+len(optional))
	for k := range required {
		allowed[k] = struct{}{}
	}
	for k := range optional {
		allowed[k] = struct{}{}
	}

	missing := make([]string, 0)
	unknown := make([]string, 0)
	seen := make(map[string]bool)

	for k, v := range input {
		nk := normalizeKey(k)
		seen[nk] = true
		if _, ok := allowed[nk]; !ok && !schema.AllowUnknown {
			unknown = append(unknown, k)
		}
		if reqKey, ok := required[nk]; ok {
			if isEmptyValue(v) {
				missing = append(missing, reqKey)
			}
		}
	}

	for nk, reqKey := range required {
		if !seen[nk] {
			missing = append(missing, reqKey)
		}
	}

	if len(missing) == 0 && len(unknown) == 0 {
		return nil
	}
	sort.Strings(missing)
	sort.Strings(unknown)
	var parts []string
	if len(missing) > 0 {
		parts = append(parts, "missing: "+strings.Join(missing, ", "))
	}
	if len(unknown) > 0 {
		parts = append(parts, "unknown: "+strings.Join(unknown, ", "))
	}
	return errors.New(strings.Join(parts, "; "))
}

func isEmptyValue(v any) bool {
	if v == nil {
		return true
	}
	switch val := v.(type) {
	case string:
		return strings.TrimSpace(val) == ""
	default:
		return false
	}
}
