package domain

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestSanitizeMetadataRedactsAndBoundsHistoricalValues(t *testing.T) {
	raw, _ := json.Marshal(map[string]any{
		"safe":   "visible",
		"nested": map[string]any{"Access_Token": "must-not-leak"},
		"long":   strings.Repeat("x", maximumStringBytes+1),
	})
	value, truncated := SanitizeMetadata(raw)
	if !truncated || value["safe"] != "visible" {
		t.Fatalf("sanitized metadata = %#v, truncated=%v", value, truncated)
	}
	nested := value["nested"].(map[string]any)
	if nested["Access_Token"] != redactedValue {
		t.Fatalf("sensitive value was not redacted: %#v", nested)
	}
	if len(value["long"].(string)) != maximumStringBytes {
		t.Fatal("long string was not bounded")
	}
	invalid, invalidTruncated := SanitizeMetadata(json.RawMessage(`[]`))
	if !invalidTruncated || len(invalid) != 0 {
		t.Fatalf("invalid metadata = %#v, truncated=%v", invalid, invalidTruncated)
	}
}
