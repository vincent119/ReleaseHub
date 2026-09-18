// Package config provides typed runtime configuration for ReleaseHub.
package config

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/spf13/viper"
)

const (
	// DefaultPath is the default configuration path in the container runtime.
	DefaultPath = "/app/configs/config.yaml"
	envPrefix   = "RELEASEHUB"
)

// Config is the validated configuration snapshot after source precedence is applied.
type Config struct {
	ManagerPassword string              `mapstructure:"manager_password" json:"managerPassword"`
	Runtime         RuntimeConfig       `mapstructure:"runtime" json:"runtime"`
	API             APIConfig           `mapstructure:"api" json:"api"`
	Worker          WorkerConfig        `mapstructure:"worker" json:"worker"`
	Database        DatabaseConfig      `mapstructure:"database" json:"database"`
	Redis           RedisConfig         `mapstructure:"redis" json:"redis"`
	Log             LogConfig           `mapstructure:"log" json:"log"`
	Observability   ObservabilityConfig `mapstructure:"observability" json:"observability"`
	OIDC            OIDCConfig          `mapstructure:"oidc" json:"oidc"`
	ArgoCD          ArgoCDConfig        `mapstructure:"argocd" json:"argocd"`
	AWS             AWSConfig           `mapstructure:"aws" json:"aws"`
	Session         SessionConfig       `mapstructure:"session" json:"session"`
	Identity        IdentityConfig      `mapstructure:"identity" json:"identity"`
	Notifications   NotificationConfig  `mapstructure:"notifications" json:"notifications"`
	Tenancy         TenancyConfig       `mapstructure:"tenancy" json:"tenancy"`
}

type RuntimeConfig struct {
	ShutdownTimeout time.Duration `mapstructure:"shutdown_timeout" json:"shutdownTimeout"`
}

type APIConfig struct {
	Address      string        `mapstructure:"address" json:"address"`
	ReadTimeout  time.Duration `mapstructure:"read_timeout" json:"readTimeout"`
	WriteTimeout time.Duration `mapstructure:"write_timeout" json:"writeTimeout"`
	IdleTimeout  time.Duration `mapstructure:"idle_timeout" json:"idleTimeout"`
}

type WorkerConfig struct {
	ReconcileInterval       time.Duration `mapstructure:"reconcile_interval" json:"reconcileInterval"`
	DeploymentPollInterval  time.Duration `mapstructure:"deployment_poll_interval" json:"deploymentPollInterval"`
	JobLeaseDuration        time.Duration `mapstructure:"job_lease_duration" json:"jobLeaseDuration"`
	JobRetryDelay           time.Duration `mapstructure:"job_retry_delay" json:"jobRetryDelay"`
	ApplicationLockDuration time.Duration `mapstructure:"application_lock_duration" json:"applicationLockDuration"`
	MaxParallelDeployments  int           `mapstructure:"max_parallel_deployments" json:"maxParallelDeployments"`
}

// NotificationConfig defines global notification retention and projection timing.
type NotificationConfig struct {
	Retention          time.Duration `mapstructure:"retention" json:"retention"`
	ProjectionInterval time.Duration `mapstructure:"projection_interval" json:"projectionInterval"`
}

type DatabaseConfig struct {
	Server   string             `mapstructure:"server" json:"server"`
	Port     int                `mapstructure:"port" json:"port"`
	User     string             `mapstructure:"user" json:"user"`
	Password string             `mapstructure:"password" json:"password"`
	Database string             `mapstructure:"database" json:"database"`
	SSLMode  string             `mapstructure:"ssl_mode" json:"sslMode"`
	Timezone string             `mapstructure:"timezone" json:"timezone"`
	Pools    DatabasePoolConfig `mapstructure:"pools" json:"pools"`
}

type DatabasePoolConfig struct {
	API     PoolConfig `mapstructure:"api" json:"api"`
	Worker  PoolConfig `mapstructure:"worker" json:"worker"`
	Migrate PoolConfig `mapstructure:"migrate" json:"migrate"`
}

type PoolConfig struct {
	MaxOpenConnections int           `mapstructure:"max_open_connections" json:"maxOpenConnections"`
	MaxIdleConnections int           `mapstructure:"max_idle_connections" json:"maxIdleConnections"`
	ConnectionLifetime time.Duration `mapstructure:"connection_lifetime" json:"connectionLifetime"`
}

type RedisConfig struct {
	Address      string `mapstructure:"address" json:"address"`
	Username     string `mapstructure:"username" json:"username"`
	Password     string `mapstructure:"password" json:"password"`
	Database     int    `mapstructure:"database" json:"database"`
	PoolSize     int    `mapstructure:"pool_size" json:"poolSize"`
	MinIdleConns int    `mapstructure:"min_idle_connections" json:"minIdleConnections"`
}

type LogConfig struct {
	Level         string   `mapstructure:"level" json:"level"`
	Format        string   `mapstructure:"format" json:"format"`
	Outputs       []string `mapstructure:"outputs" json:"outputs"`
	AddCaller     bool     `mapstructure:"add_caller" json:"addCaller"`
	AddStacktrace bool     `mapstructure:"add_stacktrace" json:"addStacktrace"`
	Development   bool     `mapstructure:"development" json:"development"`
	ColorEnabled  bool     `mapstructure:"color_enabled" json:"colorEnabled"`
}

type ObservabilityConfig struct {
	MetricsPath string        `mapstructure:"metrics_path" json:"metricsPath"`
	Tracing     TracingConfig `mapstructure:"tracing" json:"tracing"`
}

type TracingConfig struct {
	Enabled      bool    `mapstructure:"enabled" json:"enabled"`
	OTLPEndpoint string  `mapstructure:"otlp_endpoint" json:"otlpEndpoint"`
	Insecure     bool    `mapstructure:"insecure" json:"insecure"`
	SampleRatio  float64 `mapstructure:"sample_ratio" json:"sampleRatio"`
}

type OIDCConfig struct {
	Issuer                string        `mapstructure:"issuer" json:"issuer"`
	ClientID              string        `mapstructure:"client_id" json:"clientId"`
	ClientSecret          string        `mapstructure:"client_secret" json:"clientSecret"`
	RedirectURL           string        `mapstructure:"redirect_url" json:"redirectUrl"`
	WebRedirectURL        string        `mapstructure:"web_redirect_url" json:"webRedirectUrl"`
	LogoutURL             string        `mapstructure:"logout_url" json:"logoutUrl"`
	PostLogoutRedirectURL string        `mapstructure:"post_logout_redirect_url" json:"postLogoutRedirectUrl"`
	AccessTokenMaxTTL     time.Duration `mapstructure:"access_token_max_ttl" json:"accessTokenMaxTTL"`
}

type ArgoCDConfig struct {
	Address              string        `mapstructure:"address" json:"address"`
	Token                string        `mapstructure:"token" json:"token"`
	Plaintext            bool          `mapstructure:"plaintext" json:"plaintext"`
	Insecure             bool          `mapstructure:"insecure" json:"insecure"`
	RequestTimeout       time.Duration `mapstructure:"request_timeout" json:"requestTimeout"`
	ApplicationNamespace string        `mapstructure:"application_namespace" json:"applicationNamespace"`
}

// Validate checks the Argo CD client configuration when the Worker enables reconciliation.
func (c ArgoCDConfig) Validate() error {
	if strings.TrimSpace(c.Address) == "" {
		return errors.New("argocd.address is required")
	}
	if strings.TrimSpace(c.Token) == "" {
		return errors.New("argocd.token is required")
	}
	if c.RequestTimeout <= 0 {
		return errors.New("argocd.request_timeout must be greater than 0")
	}
	if strings.TrimSpace(c.ApplicationNamespace) == "" {
		return errors.New("argocd.application_namespace is required")
	}
	if c.Plaintext && c.Insecure {
		return errors.New("argocd.plaintext and argocd.insecure cannot both be true")
	}
	return nil
}

type AWSConfig struct {
	AccountID       string   `mapstructure:"account_id" json:"accountId"`
	Region          string   `mapstructure:"region" json:"region"`
	ECRRepositories []string `mapstructure:"ecr_repositories" json:"ecrRepositories"`
}

// ValidateECR checks the immutable AWS boundary used by the ECR digest resolver.
func (c AWSConfig) ValidateECR() error {
	if len(strings.TrimSpace(c.AccountID)) != 12 {
		return errors.New("aws.account_id must contain 12 digits")
	}
	for _, character := range c.AccountID {
		if character < '0' || character > '9' {
			return errors.New("aws.account_id must contain 12 digits")
		}
	}
	if strings.TrimSpace(c.Region) == "" {
		return errors.New("aws.region is required")
	}
	if len(c.ECRRepositories) == 0 {
		return errors.New("aws.ecr_repositories must contain at least one repository")
	}
	seen := make(map[string]struct{}, len(c.ECRRepositories))
	for _, repository := range c.ECRRepositories {
		value := strings.TrimSpace(repository)
		if value == "" || strings.HasPrefix(value, "/") || strings.Contains(value, "..") {
			return errors.New("aws.ecr_repositories contains an invalid repository")
		}
		if _, exists := seen[value]; exists {
			return errors.New("aws.ecr_repositories must not contain duplicates")
		}
		seen[value] = struct{}{}
	}
	return nil
}

type SessionConfig struct {
	EncryptionKey string        `mapstructure:"encryption_key" json:"encryptionKey"`
	IdleTimeout   time.Duration `mapstructure:"idle_timeout" json:"idleTimeout"`
	AbsoluteTTL   time.Duration `mapstructure:"absolute_ttl" json:"absoluteTTL"`
	LoginStateTTL time.Duration `mapstructure:"login_state_ttl" json:"loginStateTTL"`
	CookieSecure  bool          `mapstructure:"cookie_secure" json:"cookieSecure"`
}

type IdentityConfig struct {
	SyncInterval time.Duration `mapstructure:"sync_interval" json:"syncInterval"`
}

// TenancyConfig controls presentation behavior without removing the Organization data boundary.
type TenancyConfig struct {
	Mode                  string `mapstructure:"mode" json:"mode"`
	DefaultOrganizationID string `mapstructure:"default_organization_id" json:"defaultOrganizationId"`
}

// Overrides contains runtime options explicitly overridden by command-line flags.
type Overrides struct {
	APIAddress *string
	LogLevel   *string
}

// Default returns the default configuration without secrets.
func Default() Config {
	defaultPool := PoolConfig{
		MaxOpenConnections: 20,
		MaxIdleConnections: 5,
		ConnectionLifetime: 30 * time.Minute,
	}
	return Config{
		Runtime: RuntimeConfig{ShutdownTimeout: 30 * time.Second},
		API: APIConfig{
			Address:      ":8080",
			ReadTimeout:  15 * time.Second,
			WriteTimeout: 30 * time.Second,
			IdleTimeout:  60 * time.Second,
		},
		Worker: WorkerConfig{
			ReconcileInterval: 30 * time.Second, DeploymentPollInterval: time.Second,
			JobLeaseDuration: 30 * time.Second, JobRetryDelay: 5 * time.Second,
			ApplicationLockDuration: 30 * time.Second, MaxParallelDeployments: 10,
		},
		Database: DatabaseConfig{
			Port:     5432,
			SSLMode:  "require",
			Timezone: "UTC",
			Pools: DatabasePoolConfig{
				API: defaultPool, Worker: defaultPool, Migrate: defaultPool,
			},
		},
		Redis: RedisConfig{Database: 0, PoolSize: 20, MinIdleConns: 2},
		Log: LogConfig{
			Level: "info", Format: "json", Outputs: []string{"console"}, AddCaller: true,
		},
		Observability: ObservabilityConfig{
			MetricsPath: "/metrics",
			Tracing:     TracingConfig{SampleRatio: 0.1},
		},
		OIDC:   OIDCConfig{AccessTokenMaxTTL: 5 * time.Minute},
		ArgoCD: ArgoCDConfig{RequestTimeout: 10 * time.Second, ApplicationNamespace: "argocd"},
		Session: SessionConfig{
			IdleTimeout: 30 * time.Minute, AbsoluteTTL: 12 * time.Hour,
			LoginStateTTL: 5 * time.Minute, CookieSecure: true,
		},
		Identity: IdentityConfig{SyncInterval: 5 * time.Minute},
		Notifications: NotificationConfig{
			Retention: 7 * 24 * time.Hour, ProjectionInterval: time.Second,
		},
		Tenancy: TenancyConfig{Mode: "single", DefaultOrganizationID: "00000000-0000-0000-0000-000000000001"},
	}
}

// Load creates configuration using defaults, YAML, environment variables, and CLI overrides.
func Load(path string, overrides Overrides) (Config, error) {
	if path == "" {
		path = DefaultPath
	}

	v := viper.New()
	v.SetConfigFile(path)
	v.SetConfigType("yaml")
	v.SetEnvPrefix(envPrefix)
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AllowEmptyEnv(true)
	v.AutomaticEnv()

	defaults := Default()
	setDefaults(v, defaults)
	if err := bindEnvironment(v); err != nil {
		return Config{}, err
	}

	if err := v.ReadInConfig(); err != nil {
		return Config{}, fmt.Errorf("read configuration file %q: %w", path, err)
	}

	var cfg Config
	if err := v.Unmarshal(&cfg); err != nil {
		return Config{}, fmt.Errorf("unmarshal configuration: %w", err)
	}
	applyOverrides(&cfg, overrides)
	if err := cfg.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate configuration: %w", err)
	}
	return cfg, nil
}

// Validate checks shared safety constraints before any runtime is created.
func (c Config) Validate() error {
	checks := []struct {
		name  string
		value string
	}{
		{"database.server", c.Database.Server},
		{"database.user", c.Database.User},
		{"database.password", c.Database.Password},
		{"database.database", c.Database.Database},
		{"redis.address", c.Redis.Address},
	}
	for _, check := range checks {
		if strings.TrimSpace(check.value) == "" {
			return fmt.Errorf("%s is required", check.name)
		}
	}
	if c.Database.Port < 1 || c.Database.Port > 65535 {
		return errors.New("database.port must be between 1 and 65535")
	}
	if c.Runtime.ShutdownTimeout <= 0 {
		return errors.New("runtime.shutdown_timeout must be greater than 0")
	}
	if c.Worker.ReconcileInterval <= 0 || c.Worker.DeploymentPollInterval <= 0 ||
		c.Worker.JobLeaseDuration <= 0 || c.Worker.JobRetryDelay <= 0 ||
		c.Worker.ApplicationLockDuration <= 0 || c.Worker.MaxParallelDeployments < 1 {
		return errors.New("worker deployment runtime configuration is invalid")
	}
	if c.Notifications.Retention <= 0 || c.Notifications.ProjectionInterval <= 0 {
		return errors.New("notification configuration is invalid")
	}
	if c.Observability.Tracing.SampleRatio < 0 || c.Observability.Tracing.SampleRatio > 1 {
		return errors.New("observability.tracing.sample_ratio must be between 0 and 1")
	}
	if c.Observability.Tracing.Enabled && strings.TrimSpace(c.Observability.Tracing.OTLPEndpoint) == "" {
		return errors.New("observability.tracing.otlp_endpoint is required when tracing is enabled")
	}
	if c.OIDC.AccessTokenMaxTTL <= 0 {
		return errors.New("oidc.access_token_max_ttl must be greater than 0")
	}
	if c.Session.IdleTimeout <= 0 || c.Session.AbsoluteTTL <= 0 || c.Session.IdleTimeout > c.Session.AbsoluteTTL {
		return errors.New("session timeout configuration is invalid")
	}
	if c.Session.LoginStateTTL <= 0 {
		return errors.New("session.login_state_ttl must be greater than 0")
	}
	if c.Tenancy.Mode != "single" && c.Tenancy.Mode != "multi" {
		return errors.New("tenancy.mode must be single or multi")
	}
	if c.Tenancy.Mode == "single" {
		if _, err := uuid.Parse(c.Tenancy.DefaultOrganizationID); err != nil {
			return errors.New("tenancy.default_organization_id must be a UUID in single-tenant mode")
		}
	}
	return nil
}

// String returns a configuration summary that is safe to write to logs.
func (c Config) String() string {
	redacted := c
	redacted.ManagerPassword = redact(c.ManagerPassword)
	redacted.Database.Password = redact(c.Database.Password)
	redacted.Redis.Password = redact(c.Redis.Password)
	redacted.OIDC.ClientSecret = redact(c.OIDC.ClientSecret)
	redacted.ArgoCD.Token = redact(c.ArgoCD.Token)
	redacted.Session.EncryptionKey = redact(c.Session.EncryptionKey)
	data, err := json.Marshal(redacted)
	if err != nil {
		return "{\"error\":\"unable to marshal configuration summary\"}"
	}
	return string(data)
}

func redact(value string) string {
	if value == "" {
		return ""
	}
	return "[REDACTED]"
}

func applyOverrides(cfg *Config, overrides Overrides) {
	if overrides.APIAddress != nil {
		cfg.API.Address = *overrides.APIAddress
	}
	if overrides.LogLevel != nil {
		cfg.Log.Level = *overrides.LogLevel
	}
}

func setDefaults(v *viper.Viper, cfg Config) {
	v.SetDefault("runtime.shutdown_timeout", cfg.Runtime.ShutdownTimeout)
	v.SetDefault("api.address", cfg.API.Address)
	v.SetDefault("api.read_timeout", cfg.API.ReadTimeout)
	v.SetDefault("api.write_timeout", cfg.API.WriteTimeout)
	v.SetDefault("api.idle_timeout", cfg.API.IdleTimeout)
	v.SetDefault("worker.reconcile_interval", cfg.Worker.ReconcileInterval)
	v.SetDefault("worker.deployment_poll_interval", cfg.Worker.DeploymentPollInterval)
	v.SetDefault("worker.job_lease_duration", cfg.Worker.JobLeaseDuration)
	v.SetDefault("worker.job_retry_delay", cfg.Worker.JobRetryDelay)
	v.SetDefault("worker.application_lock_duration", cfg.Worker.ApplicationLockDuration)
	v.SetDefault("worker.max_parallel_deployments", cfg.Worker.MaxParallelDeployments)
	v.SetDefault("notifications.retention", cfg.Notifications.Retention)
	v.SetDefault("notifications.projection_interval", cfg.Notifications.ProjectionInterval)
	v.SetDefault("database.port", cfg.Database.Port)
	v.SetDefault("database.ssl_mode", cfg.Database.SSLMode)
	v.SetDefault("database.timezone", cfg.Database.Timezone)
	setPoolDefaults(v, "database.pools.api", cfg.Database.Pools.API)
	setPoolDefaults(v, "database.pools.worker", cfg.Database.Pools.Worker)
	setPoolDefaults(v, "database.pools.migrate", cfg.Database.Pools.Migrate)
	v.SetDefault("redis.database", cfg.Redis.Database)
	v.SetDefault("redis.pool_size", cfg.Redis.PoolSize)
	v.SetDefault("redis.min_idle_connections", cfg.Redis.MinIdleConns)
	v.SetDefault("log.level", cfg.Log.Level)
	v.SetDefault("log.format", cfg.Log.Format)
	v.SetDefault("log.outputs", cfg.Log.Outputs)
	v.SetDefault("log.add_caller", cfg.Log.AddCaller)
	v.SetDefault("log.add_stacktrace", cfg.Log.AddStacktrace)
	v.SetDefault("log.development", cfg.Log.Development)
	v.SetDefault("log.color_enabled", cfg.Log.ColorEnabled)
	v.SetDefault("observability.metrics_path", cfg.Observability.MetricsPath)
	v.SetDefault("observability.tracing.enabled", cfg.Observability.Tracing.Enabled)
	v.SetDefault("observability.tracing.otlp_endpoint", cfg.Observability.Tracing.OTLPEndpoint)
	v.SetDefault("observability.tracing.insecure", cfg.Observability.Tracing.Insecure)
	v.SetDefault("observability.tracing.sample_ratio", cfg.Observability.Tracing.SampleRatio)
	v.SetDefault("oidc.access_token_max_ttl", cfg.OIDC.AccessTokenMaxTTL)
	v.SetDefault("argocd.plaintext", cfg.ArgoCD.Plaintext)
	v.SetDefault("argocd.insecure", cfg.ArgoCD.Insecure)
	v.SetDefault("argocd.request_timeout", cfg.ArgoCD.RequestTimeout)
	v.SetDefault("argocd.application_namespace", cfg.ArgoCD.ApplicationNamespace)
	v.SetDefault("aws.ecr_repositories", cfg.AWS.ECRRepositories)
	v.SetDefault("session.idle_timeout", cfg.Session.IdleTimeout)
	v.SetDefault("session.absolute_ttl", cfg.Session.AbsoluteTTL)
	v.SetDefault("session.login_state_ttl", cfg.Session.LoginStateTTL)
	v.SetDefault("session.cookie_secure", cfg.Session.CookieSecure)
	v.SetDefault("identity.sync_interval", cfg.Identity.SyncInterval)
	v.SetDefault("tenancy.mode", cfg.Tenancy.Mode)
	v.SetDefault("tenancy.default_organization_id", cfg.Tenancy.DefaultOrganizationID)
}

func setPoolDefaults(v *viper.Viper, prefix string, cfg PoolConfig) {
	v.SetDefault(prefix+".max_open_connections", cfg.MaxOpenConnections)
	v.SetDefault(prefix+".max_idle_connections", cfg.MaxIdleConnections)
	v.SetDefault(prefix+".connection_lifetime", cfg.ConnectionLifetime)
}

func bindEnvironment(v *viper.Viper) error {
	for _, key := range []string{
		"manager_password",
		"api.address", "log.level", "worker.deployment_poll_interval", "worker.job_lease_duration",
		"worker.job_retry_delay", "worker.application_lock_duration", "worker.max_parallel_deployments",
		"notifications.retention", "notifications.projection_interval",
		"database.server", "database.port", "database.user",
		"database.password", "database.database", "redis.address", "redis.username",
		"redis.password", "oidc.issuer", "oidc.client_id", "oidc.client_secret", "oidc.redirect_url",
		"oidc.web_redirect_url", "oidc.logout_url", "oidc.post_logout_redirect_url",
		"oidc.access_token_max_ttl",
		"argocd.address", "argocd.token", "argocd.plaintext", "argocd.insecure", "argocd.request_timeout", "argocd.application_namespace",
		"aws.account_id", "aws.region", "aws.ecr_repositories",
		"session.encryption_key", "observability.tracing.otlp_endpoint",
		"tenancy.mode", "tenancy.default_organization_id",
	} {
		if err := v.BindEnv(key); err != nil {
			return fmt.Errorf("bind environment variable %q: %w", key, err)
		}
	}
	return nil
}
