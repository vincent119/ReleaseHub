package httpserver

import (
	"encoding/json"
	"testing"

	"github.com/google/uuid"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestRuntimeTopologyResponseUsesEmptyArrays(t *testing.T) {
	value := runtimeTopologyResponse(argoapp.RuntimeTopology{
		ApplicationID: uuid.New(), View: "resources",
	})
	encoded, err := json.Marshal(value)
	if err != nil {
		t.Fatalf("marshal runtime topology: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal runtime topology: %v", err)
	}
	for _, field := range []string{"nodes", "edges", "warnings"} {
		if string(fields[field]) != "[]" {
			t.Errorf("%s = %s, want []", field, fields[field])
		}
	}
}

func TestRuntimeResourceResponseUsesEmptyArrays(t *testing.T) {
	encoded, err := json.Marshal(runtimeResourceResponse(argodomain.RuntimeResource{}))
	if err != nil {
		t.Fatalf("marshal runtime resource: %v", err)
	}
	var fields map[string]json.RawMessage
	if err := json.Unmarshal(encoded, &fields); err != nil {
		t.Fatalf("unmarshal runtime resource: %v", err)
	}
	for _, field := range []string{"images", "info", "ingress", "externalUrls"} {
		if string(fields[field]) != "[]" {
			t.Errorf("%s = %s, want []", field, fields[field])
		}
	}
}
