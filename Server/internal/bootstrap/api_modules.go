package bootstrap

import (
	"context"
	"strings"
	"time"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argoinfra "github.com/vincent119/ReleaseHub/Server/internal/argocd/infrastructure"
	authzapp "github.com/vincent119/ReleaseHub/Server/internal/authorization/application"
	authzinfra "github.com/vincent119/ReleaseHub/Server/internal/authorization/infrastructure"
	catalogapp "github.com/vincent119/ReleaseHub/Server/internal/catalog/application"
	cataloginfra "github.com/vincent119/ReleaseHub/Server/internal/catalog/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/config"
	deployapp "github.com/vincent119/ReleaseHub/Server/internal/deployment/application"
	deployinfra "github.com/vincent119/ReleaseHub/Server/internal/deployment/infrastructure"
	identityapp "github.com/vincent119/ReleaseHub/Server/internal/identity/application"
	identityinfra "github.com/vincent119/ReleaseHub/Server/internal/identity/infrastructure"
	"github.com/vincent119/ReleaseHub/Server/internal/infrastructure/database"
	"github.com/vincent119/ReleaseHub/Server/internal/transport/httpserver"
	"gorm.io/gorm"
)

type policyModule struct {
	engine   *authzinfra.PolicyEngine
	listener *authzinfra.PolicyRevisionListener
}

type identityServices struct {
	identity *identityapp.IdentityService
	sessions *identityapp.SessionService
}

type identitySecurity struct {
	states    *identityapp.LoginStateCodec
	protector *identityapp.TokenProtector
}

type apiModules struct {
	handler        httpserver.APIOptions
	policyListener *authzinfra.PolicyRevisionListener
}

type apiHandlerDependencies struct {
	cfg       config.Config
	version   string
	resources *processResources
	policy    *authzinfra.PolicyEngine
	groups    *authzapp.OIDCGroupService
	local     *identityapp.LocalAuthService
}

type authComposition struct {
	cfg      config.Config
	services *identityServices
	security *identitySecurity
	provider identityapp.OIDCProvider
	groups   *authzapp.OIDCGroupService
	local    *identityapp.LocalAuthService
}

type apiHandlerParts struct {
	auth        httpserver.AuthHandlerOptions
	oidcEnabled bool
	catalog     httpserver.CatalogHandlerOptions
	argoCD      httpserver.ArgoCDHandlerOptions
	access      httpserver.AccessHandlerOptions
	workflow    httpserver.WorkflowHandlerOptions
	plan        httpserver.PlanHandlerOptions
	deployment  httpserver.DeploymentHandlerOptions
}

func newAPIModulesWithLocal(cfg config.Config, version string, resources *processResources, local *identityapp.LocalAuthService) (*apiModules, error) {
	policy, err := newPolicyModule(resources.db, cfg.Database)
	if err != nil {
		return nil, err
	}
	groups, err := newOIDCGroupService(resources.db)
	if err != nil {
		return nil, err
	}
	handler, err := newAPIHandlerOptions(apiHandlerDependencies{
		cfg: cfg, version: version, resources: resources,
		policy: policy.engine, groups: groups, local: local,
	})
	if err != nil {
		return nil, err
	}
	return &apiModules{handler: handler, policyListener: policy.listener}, nil
}

func newPolicyModule(db *gorm.DB, cfg config.DatabaseConfig) (*policyModule, error) {
	adapter, err := authzinfra.NewPolicyAdapter(db)
	if err != nil {
		return nil, err
	}
	engine, err := authzinfra.NewPolicyEngine(adapter)
	if err != nil {
		return nil, err
	}
	listener, err := authzinfra.NewPolicyRevisionListener(database.PostgreSQLURL(cfg), engine)
	if err != nil {
		return nil, err
	}
	return &policyModule{engine: engine, listener: listener}, nil
}

func newOIDCGroupService(db *gorm.DB) (*authzapp.OIDCGroupService, error) {
	repository, err := authzinfra.NewOIDCGroupRepository(db)
	if err != nil {
		return nil, err
	}
	return authzapp.NewOIDCGroupService(repository)
}

func newAPIHandlerOptions(dependencies apiHandlerDependencies) (httpserver.APIOptions, error) {
	parts, err := buildCoreHandlerParts(dependencies)
	if err != nil {
		return httpserver.APIOptions{}, err
	}
	parts.access, err = newAccessHandlerOptions(dependencies.resources.db, dependencies.policy, dependencies.local)
	if err != nil {
		return httpserver.APIOptions{}, err
	}
	parts.workflow, err = newWorkflowHandlerOptions(dependencies.resources.db, dependencies.policy)
	if err != nil {
		return httpserver.APIOptions{}, err
	}
	parts.plan, err = newPlanHandlerOptions(dependencies.resources.db, dependencies.policy)
	if err != nil {
		return httpserver.APIOptions{}, err
	}
	parts.deployment, err = newDeploymentHandlerOptions(dependencies.resources.db, dependencies.policy, dependencies.resources.argoClient)
	return apiHandlerOptions(dependencies, parts), err
}

func buildCoreHandlerParts(dependencies apiHandlerDependencies) (apiHandlerParts, error) {
	auth, oidcEnabled, err := newAuthHandlerOptions(dependencies.cfg, dependencies.resources.db, dependencies.groups, dependencies.local)
	if err != nil {
		return apiHandlerParts{}, err
	}
	catalog, err := newCatalogHandlerOptions(dependencies.resources.db, dependencies.policy)
	if err != nil {
		return apiHandlerParts{}, err
	}
	argoCD, err := newArgoCDHandlerOptions(dependencies.resources.db, dependencies.resources.argoClient, dependencies.policy)
	return apiHandlerParts{auth: auth, oidcEnabled: oidcEnabled, catalog: catalog, argoCD: argoCD}, err
}

func apiHandlerOptions(dependencies apiHandlerDependencies, parts apiHandlerParts) httpserver.APIOptions {
	return httpserver.APIOptions{
		System: httpserver.SystemHandlerOptions{
			Version: dependencies.version, TenancyMode: dependencies.cfg.Tenancy.Mode,
			OIDCEnabled: parts.oidcEnabled,
		},
		Auth: parts.auth, Catalog: parts.catalog,
		ArgoCD: parts.argoCD, Access: parts.access, Workflow: parts.workflow, Plan: parts.plan,
		Deployment: parts.deployment,
	}
}

func newPlanHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine) (httpserver.PlanHandlerOptions, error) {
	repository, err := deployinfra.NewDeploymentPlanRepository(db)
	if err != nil {
		return httpserver.PlanHandlerOptions{}, err
	}
	service, err := deployapp.NewPlanDefinitionService(deployapp.PlanDefinitionServiceOptions{
		Repository: repository, Authorizer: policy, Scopes: repository,
		Clock: deployapp.SystemWorkflowClock{},
	})
	if err != nil {
		return httpserver.PlanHandlerOptions{}, err
	}
	bindings, err := newDeploymentBindingService(db, policy)
	return httpserver.PlanHandlerOptions{Definitions: service, Bindings: bindings}, err
}

func newDeploymentBindingService(db *gorm.DB, policy *authzinfra.PolicyEngine) (*deployapp.DeploymentBindingService, error) {
	repository, err := deployinfra.NewDeploymentBindingRepository(db)
	if err != nil {
		return nil, err
	}
	return deployapp.NewDeploymentBindingService(repository, policy, deployapp.SystemWorkflowClock{})
}

func newWorkflowHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine) (httpserver.WorkflowHandlerOptions, error) {
	repository, err := deployinfra.NewWorkflowDefinitionRepository(db)
	if err != nil {
		return httpserver.WorkflowHandlerOptions{}, err
	}
	service, err := deployapp.NewWorkflowDefinitionService(repository, policy, deployapp.SystemWorkflowClock{})
	return httpserver.WorkflowHandlerOptions{Definitions: service}, err
}

func newCatalogHandlerOptions(db *gorm.DB, policy *authzinfra.PolicyEngine) (httpserver.CatalogHandlerOptions, error) {
	repository, err := cataloginfra.NewCatalogRepository(db)
	if err != nil {
		return httpserver.CatalogHandlerOptions{}, err
	}
	service, err := catalogapp.NewService(repository, policy)
	if err != nil {
		return httpserver.CatalogHandlerOptions{}, err
	}
	status, err := argoinfra.NewStatusRepository(db)
	return httpserver.CatalogHandlerOptions{Service: service, StatusReader: status}, err
}

func newArgoCDHandlerOptions(db *gorm.DB, client *argoinfra.Client, policy *authzinfra.PolicyEngine) (httpserver.ArgoCDHandlerOptions, error) {
	candidateRepository, err := argoinfra.NewCandidateRepository(db)
	if err != nil {
		return httpserver.ArgoCDHandlerOptions{}, err
	}
	candidates, err := argoapp.NewCandidateService(candidateRepository, policy)
	if err != nil {
		return httpserver.ArgoCDHandlerOptions{}, err
	}
	options := httpserver.ArgoCDHandlerOptions{Candidates: candidates}
	if client == nil {
		return options, nil
	}
	onboardingRepository, err := argoinfra.NewOnboardingRepository(db)
	if err != nil {
		return httpserver.ArgoCDHandlerOptions{}, err
	}
	options.Onboarding, err = argoapp.NewOnboardingService(onboardingRepository, client, policy)
	return options, err
}

func newAuthHandlerOptions(cfg config.Config, db *gorm.DB, groups *authzapp.OIDCGroupService, local *identityapp.LocalAuthService) (httpserver.AuthHandlerOptions, bool, error) {
	services, err := newIdentityServices(cfg, db)
	if err != nil {
		return httpserver.AuthHandlerOptions{}, false, err
	}
	security, err := newIdentitySecurity(cfg)
	if err != nil {
		return httpserver.AuthHandlerOptions{}, false, err
	}
	provider, err := discoverOIDCProvider(cfg)
	if err != nil {
		return httpserver.AuthHandlerOptions{}, false, err
	}
	options, err := composeAuthHandlerOptions(authComposition{
		cfg: cfg, services: services, security: security,
		provider: provider, groups: groups, local: local,
	})
	return options, provider != nil, err
}

func newIdentityServices(cfg config.Config, db *gorm.DB) (*identityServices, error) {
	identities, err := identityinfra.NewIdentityRepository(db)
	if err != nil {
		return nil, err
	}
	sessions, err := identityinfra.NewSessionRepository(db)
	if err != nil {
		return nil, err
	}
	identityService, err := identityapp.NewIdentityService(identities)
	if err != nil {
		return nil, err
	}
	sessionService, err := identityapp.NewSessionService(sessions, identityapp.SystemClock{}, cfg.Session.IdleTimeout, cfg.Session.AbsoluteTTL)
	return &identityServices{identity: identityService, sessions: sessionService}, err
}

func newIdentitySecurity(cfg config.Config) (*identitySecurity, error) {
	clock := identityapp.SystemClock{}
	states, err := identityapp.NewLoginStateCodec([]byte(cfg.Session.EncryptionKey), clock, cfg.Session.LoginStateTTL)
	if err != nil {
		return nil, err
	}
	protector, err := identityapp.NewTokenProtector([]byte(cfg.Session.EncryptionKey))
	return &identitySecurity{states: states, protector: protector}, err
}

func discoverOIDCProvider(cfg config.Config) (identityapp.OIDCProvider, error) {
	if strings.TrimSpace(cfg.OIDC.Issuer) == "" && strings.TrimSpace(cfg.OIDC.ClientID) == "" && strings.TrimSpace(cfg.OIDC.RedirectURL) == "" {
		return nil, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return identityinfra.NewOIDCProvider(ctx, cfg.OIDC)
}

func composeAuthHandlerOptions(composition authComposition) (httpserver.AuthHandlerOptions, error) {
	flow, err := identityapp.NewAuthFlow(
		composition.provider, composition.security.states,
		composition.services.identity, composition.services.sessions,
		composition.security.protector, composition.cfg.Identity.SyncInterval,
		identityapp.WithOIDCGroupSynchronizer(composition.groups),
	)
	return httpserver.AuthHandlerOptions{
		Flow: flow, Local: composition.local, WebRedirectURL: composition.cfg.OIDC.WebRedirectURL,
		CookieSecure:  composition.cfg.Session.CookieSecure,
		LoginStateTTL: composition.cfg.Session.LoginStateTTL,
		SessionTTL:    composition.cfg.Session.AbsoluteTTL,
	}, err
}
