package configutil

import (
	"fmt"
	"strings"

	"github.com/mitchellh/mapstructure"
)

// DecodeSettings decodes a free-form settings map into a typed struct.
func DecodeSettings(input map[string]any, out any) error {
	if len(input) == 0 {
		return nil
	}
	cfg := &mapstructure.DecoderConfig{
		TagName:          "mapstructure",
		Result:           out,
		WeaklyTypedInput: true,
		MatchName: func(mapKey, fieldName string) bool {
			return normalizeKey(mapKey) == normalizeKey(fieldName)
		},
	}
	decoder, err := mapstructure.NewDecoder(cfg)
	if err != nil {
		return err
	}
	return decoder.Decode(input)
}

// RequireString ensures a value is present for a required config field.
func RequireString(value, path string) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s is required", path)
	}
	return nil
}

// BoolValue returns fallback when value is nil.
func BoolValue(value *bool, fallback bool) bool {
	if value == nil {
		return fallback
	}
	return *value
}

// IntValue returns fallback when value is nil.
func IntValue(value *int, fallback int) int {
	if value == nil {
		return fallback
	}
	return *value
}

func normalizeKey(value string) string {
	value = strings.ToLower(value)
	value = strings.ReplaceAll(value, "_", "")
	value = strings.ReplaceAll(value, "-", "")
	return value
}
