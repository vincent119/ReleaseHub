package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/vincent119/ReleaseHub/Server/internal/config"
)

func TestLoadAppliesDefaultsYAMLEnvironmentAndCLIInOrder(t *testing.T) {
	t.Setenv("RELEASEHUB_MANAGER_PASSWORD", "manager-secret")
	t.Setenv("RELEASEHUB_API_ADDRESS", ":9100")
	t.Setenv("RELEASEHUB_LOG_LEVEL", "warn")
	t.Setenv("RELEASEHUB_SESSION_COOKIE_SECURE", "false")

	path := writeConfig(t, `
api:
  address: ":9000"
database:
  server: postgres.internal
  port: 5432
  user: releasehub
  password: database-secret
  database: releasehub
redis:
  address: redis.internal:6379
log:
  level: debug
`)

	apiAddress := ":9200"
	cfg, err := config.Load(path, config.Overrides{APIAddress: &apiAddress})
	if err != nil {
		t.Fatalf("load configuration: %v", err)
	}

	if cfg.API.Address != ":9200" {
		t.Fatalf("CLI should have highest precedence, got %q", cfg.API.Address)
	}
	if cfg.Log.Level != "warn" {
		t.Fatalf("environment variable should override YAML, got %q", cfg.Log.Level)
	}
	if cfg.Runtime.ShutdownTimeout != 30*time.Second {
		t.Fatalf("unset field should use default value, got %s", cfg.Runtime.ShutdownTimeout)
	}
	if cfg.Database.Server != "postgres.internal" {
		t.Fatalf("YAML mapping mismatch, got %q", cfg.Database.Server)
	}
	if cfg.ManagerPassword != "manager-secret" {
		t.Fatalf("manager password environment mapping mismatch")
	}
	if cfg.Session.CookieSecure {
		t.Fatal("session cookie secure environment mapping mismatch")
	}
	if cfg.OIDC.AccessTokenMaxTTL != 5*time.Minute {
		t.Fatalf("OIDC access token maximum should default to five minutes, got %s", cfg.OIDC.AccessTokenMaxTTL)
	}
	if cfg.Worker.DeploymentPollInterval != time.Second || cfg.Worker.MaxParallelDeployments != 10 {
		t.Fatalf("deployment worker defaults mismatch: %#v", cfg.Worker)
	}
	if cfg.Notifications.Retention != 7*24*time.Hour || cfg.Notifications.ProjectionInterval != time.Second {
		t.Fatalf("notification defaults mismatch: %#v", cfg.Notifications)
	}
}

func TestConfigRejectsInvalidDeploymentWorkerRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Database.Server, cfg.Database.User = "postgres", "releasehub"
	cfg.Database.Password, cfg.Database.Database = "secret", "releasehub"
	cfg.Redis.Address = "redis:6379"
	cfg.Worker.MaxParallelDeployments = 0
	if err := cfg.Validate(); err == nil || !strings.Contains(err.Error(), "worker") {
		t.Fatalf("invalid deployment worker runtime should fail: %v", err)
	}
}

func TestConfigRejectsInvalidNotificationRuntime(t *testing.T) {
	cfg := config.Default()
	cfg.Database.Server, cfg.Database.User = "postgres", "releasehub"
	cfg.Database.Password, cfg.Database.Database = "secret", "releasehub"
	cfg.Redis.Address = "redis:6379"
	cfg.Notifications.Retention = 0
	if err := cfg.Validate(); err == nil {
		t.Fatal("expected invalid notification runtime")
	}
}

func TestLoadRejectsMissingRequiredDatabasePassword(t *testing.T) {
	path := writeConfig(t, `
database:
  server: postgres.internal
  port: 5432
  user: releasehub
  database: releasehub
redis:
  address: redis.internal:6379
`)

	_, err := config.Load(path, config.Overrides{})
	if err == nil || !strings.Contains(err.Error(), "database.password") {
		t.Fatalf("missing database.password should be rejected, got %v", err)
	}
}

func TestLoadTreatsExplicitEmptyEnvironmentValueAsOverride(t *testing.T) {
	t.Setenv("RELEASEHUB_DATABASE_PASSWORD", "")
	path := writeConfig(t, `
database:
  server: postgres.internal
  port: 5432
  user: releasehub
  password: yaml-secret
  database: releasehub
redis:
  address: redis.internal:6379
`)

	_, err := config.Load(path, config.Overrides{})
	if err == nil || !strings.Contains(err.Error(), "database.password") {
		t.Fatalf("explicit empty environment variable should override YAML and fail validation, got %v", err)
	}
}

func TestConfigStringRedactsSecrets(t *testing.T) {
	cfg := config.Default()
	cfg.Database.Password = "database-secret"
	cfg.Redis.Password = "redis-secret"
	cfg.OIDC.ClientSecret = "oidc-secret"
	cfg.ArgoCD.Token = "argocd-secret"
	cfg.Session.EncryptionKey = "session-secret"
	cfg.ManagerPassword = "manager-secret"

	text := cfg.String()
	for _, secret := range []string{
		"database-secret",
		"redis-secret",
		"oidc-secret",
		"argocd-secret",
		"session-secret",
		"manager-secret",
	} {
		if strings.Contains(text, secret) {
			t.Fatalf("configuration output leaked secret %q", secret)
		}
	}
	if !strings.Contains(text, "[REDACTED]") {
		t.Fatal("configuration output should mark redacted fields")
	}
}

func TestSingleTenantHiddenOrganization(t *testing.T) {
	path := writeConfig(t, `
database:
  server: postgres.internal
  user: releasehub
  password: database-secret
  database: releasehub
redis:
  address: redis.internal:6379
tenancy:
  mode: single
`)

	cfg, err := config.Load(path, config.Overrides{})
	if err != nil {
		t.Fatalf("load single-tenant configuration: %v", err)
	}
	if cfg.Tenancy.Mode != "single" || cfg.Tenancy.DefaultOrganizationID == "" {
		t.Fatalf("single-tenant mode must retain a default Organization: %#v", cfg.Tenancy)
	}
}

func TestSingleTenantRejectsInvalidDefaultOrganization(t *testing.T) {
	path := writeConfig(t, `
database:
  server: postgres.internal
  user: releasehub
  password: database-secret
  database: releasehub
redis:
  address: redis.internal:6379
tenancy:
  mode: single
  default_organization_id: invalid
`)

	if _, err := config.Load(path, config.Overrides{}); err == nil {
		t.Fatal("invalid default Organization ID should be rejected")
	}
}

func TestArgoCDConfigRequiresAddressTokenAndTimeout(t *testing.T) {
	valid := config.ArgoCDConfig{Address: "argocd-server.argocd.svc:443", Token: "token", RequestTimeout: 10 * time.Second, ApplicationNamespace: "argocd"}
	if err := valid.Validate(); err != nil {
		t.Fatalf("valid Argo CD configuration: %v", err)
	}
	for _, value := range []config.ArgoCDConfig{
		{Token: "token", RequestTimeout: time.Second, ApplicationNamespace: "argocd"},
		{Address: "argocd-server.argocd.svc:443", RequestTimeout: time.Second, ApplicationNamespace: "argocd"},
		{Address: "argocd-server.argocd.svc:443", Token: "token", ApplicationNamespace: "argocd"},
		{Address: "argocd-server.argocd.svc:443", Token: "token", RequestTimeout: time.Second},
	} {
		if err := value.Validate(); err == nil {
			t.Fatalf("invalid Argo CD configuration should fail: %#v", value)
		}
	}
}

func TestArgoCDConfigRejectsConflictingTransportModes(t *testing.T) {
	cfg := config.ArgoCDConfig{
		Address:              "argocd-server.argocd.svc:80",
		Token:                "token",
		Plaintext:            true,
		Insecure:             true,
		RequestTimeout:       10 * time.Second,
		ApplicationNamespace: "argocd",
	}

	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "argocd.plaintext") {
		t.Fatalf("conflicting Argo CD transport modes should fail: %v", err)
	}
}

func TestLoadMapsArgoCDTransportEnvironment(t *testing.T) {
	t.Setenv("RELEASEHUB_ARGOCD_PLAINTEXT", "true")
	t.Setenv("RELEASEHUB_ARGOCD_INSECURE", "false")
	path := writeConfig(t, `
database:
  server: postgres.internal
  user: releasehub
  password: database-secret
  database: releasehub
redis:
  address: redis.internal:6379
argocd:
  address: argocd-server.argocd.svc:80
  token: token
`)

	cfg, err := config.Load(path, config.Overrides{})
	if err != nil {
		t.Fatalf("load Argo CD transport environment: %v", err)
	}
	if !cfg.ArgoCD.Plaintext || cfg.ArgoCD.Insecure {
		t.Fatalf("Argo CD transport environment mapping mismatch: %#v", cfg.ArgoCD)
	}
}

func TestAWSConfigValidatesECRScope(t *testing.T) {
	valid := config.AWSConfig{AccountID: "123456789012", Region: "ap-northeast-1", ECRRepositories: []string{"platform/admin", "payments/api"}}
	if err := valid.ValidateECR(); err != nil {
		t.Fatalf("valid ECR configuration: %v", err)
	}
	for _, value := range []config.AWSConfig{
		{AccountID: "123", Region: "ap-northeast-1", ECRRepositories: []string{"platform/admin"}},
		{AccountID: "123456789012", ECRRepositories: []string{"platform/admin"}},
		{AccountID: "123456789012", Region: "ap-northeast-1"},
		{AccountID: "123456789012", Region: "ap-northeast-1", ECRRepositories: []string{"platform/admin", "platform/admin"}},
		{AccountID: "123456789012", Region: "ap-northeast-1", ECRRepositories: []string{"../platform/admin"}},
	} {
		if err := value.ValidateECR(); err == nil {
			t.Fatalf("invalid ECR configuration should fail: %#v", value)
		}
	}
}

func writeConfig(t *testing.T, body string) string {
	t.Helper()

	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatalf("create test configuration file: %v", err)
	}
	return path
}
