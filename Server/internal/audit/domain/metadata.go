package domain

import (
	"encoding/json"
	"strings"
	"unicode"
)

const (
	maximumMetadataBytes = 16 * 1024
	maximumMetadataDepth = 8
	maximumMetadataKeys  = 100
	maximumArrayItems    = 100
	maximumStringBytes   = 2 * 1024
	redactedValue        = "[REDACTED]"
)

var forbiddenMetadataKeyFragments = []string{
	"password", "passwd", "token", "secret", "authorization", "cookie", "credential",
	"privatekey", "clientsecret", "accesskey", "session",
}

// SanitizeMetadata defensively redacts and bounds historical metadata.
func SanitizeMetadata(raw json.RawMessage) (map[string]any, bool) {
	if len(raw) == 0 {
		return map[string]any{}, false
	}
	var value map[string]any
	if err := json.Unmarshal(raw, &value); err != nil {
		return map[string]any{}, true
	}
	sanitized, truncated := sanitizeObject(value, 1)
	encoded, err := json.Marshal(sanitized)
	if err != nil || len(encoded) > maximumMetadataBytes {
		return map[string]any{"_truncated": true}, true
	}
	return sanitized, truncated
}

func sanitizeObject(value map[string]any, depth int) (map[string]any, bool) {
	if depth > maximumMetadataDepth {
		return map[string]any{"_truncated": true}, true
	}
	result := make(map[string]any, min(len(value), maximumMetadataKeys))
	truncated := len(value) > maximumMetadataKeys
	count := 0
	for key, item := range value {
		if count >= maximumMetadataKeys {
			break
		}
		count++
		if forbiddenMetadataKey(key) {
			result[key] = redactedValue
			truncated = true
			continue
		}
		result[key], truncated = sanitizeValue(item, depth+1, truncated)
	}
	return result, truncated
}

func sanitizeValue(value any, depth int, truncated bool) (any, bool) {
	if depth > maximumMetadataDepth {
		return "[TRUNCATED]", true
	}
	switch typed := value.(type) {
	case map[string]any:
		clean, changed := sanitizeObject(typed, depth)
		return clean, truncated || changed
	case []any:
		limit := min(len(typed), maximumArrayItems)
		result := make([]any, 0, limit)
		truncated = truncated || len(typed) > maximumArrayItems
		for _, item := range typed[:limit] {
			clean, changed := sanitizeValue(item, depth+1, truncated)
			result = append(result, clean)
			truncated = truncated || changed
		}
		return result, truncated
	case string:
		if len(typed) > maximumStringBytes {
			return typed[:maximumStringBytes], true
		}
		return typed, truncated
	default:
		return typed, truncated
	}
}

func forbiddenMetadataKey(value string) bool {
	normalized := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, value)
	for _, fragment := range forbiddenMetadataKeyFragments {
		if strings.Contains(normalized, fragment) {
			return true
		}
	}
	return false
}
