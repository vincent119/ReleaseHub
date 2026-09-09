package bootstrap

import (
	"testing"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	"github.com/vincent119/ReleaseHub/Server/internal/observability"
)

func TestArgoCDClientRequirementByProcess(t *testing.T) {
	tests := []struct {
		name      string
		component string
		cfg       config.ArgoCDConfig
		want      bool
	}{
		{name: "API without Argo CD", component: observability.ComponentAPI, want: false},
		{name: "API with address only validates partial configuration", component: observability.ComponentAPI, cfg: config.ArgoCDConfig{Address: "argocd:443"}, want: true},
		{name: "API with token only validates partial configuration", component: observability.ComponentAPI, cfg: config.ArgoCDConfig{Token: "token"}, want: true},
		{name: "API with Argo CD", component: observability.ComponentAPI, cfg: config.ArgoCDConfig{Address: "argocd:443", Token: "token"}, want: true},
		{name: "Worker without Argo CD", component: observability.ComponentWorker, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := shouldCreateArgoCDClient(test.component, test.cfg); got != test.want {
				t.Fatalf("Argo CD client requirement mismatch, got %t", got)
			}
		})
	}
}
