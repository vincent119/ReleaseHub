package bootstrap

import (
	"testing"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
)

func TestDiscoverOIDCProviderReturnsNilInterfaceWhenDisabled(t *testing.T) {
	discovered, err := discoverOIDCProvider(config.Config{})
	if err != nil {
		t.Fatalf("discover disabled OIDC provider: %v", err)
	}

	var provider identityapp.OIDCProvider = discovered
	if provider != nil {
		t.Fatal("disabled OIDC provider became a non-nil interface")
	}
}

func TestAPIHandlerOptionsReportsOIDCCapability(t *testing.T) {
	options := apiHandlerOptions(apiHandlerDependencies{version: "test"}, apiHandlerParts{oidcEnabled: true})
	if !options.System.OIDCEnabled {
		t.Fatal("OIDC capability was not propagated to the system handler")
	}
}
