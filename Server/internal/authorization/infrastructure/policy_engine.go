package infrastructure

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"github.com/casbin/casbin/v3"
	"github.com/casbin/casbin/v3/model"
	"github.com/casbin/casbin/v3/persist"

	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
)

const policyModel = `
[request_definition]
r = sub, tenant, obj, act

[policy_definition]
p = sub, tenant, obj, act, eft

[policy_effect]
e = some(where (p.eft == allow)) && !some(where (p.eft == deny))

[matchers]
m = r.sub == p.sub && r.tenant == p.tenant && scopeMatch(r.obj, p.obj) && r.act == p.act
`

// PolicySource is the read-only contract required by the cached Casbin engine.
type PolicySource interface {
	persist.Adapter
	persist.ContextAdapter
	CurrentRevision(context.Context) (uint64, error)
}

// PolicyEngine applies local disable, explicit deny, role union, and default deny precedence.
type PolicyEngine struct {
	mu       sync.RWMutex
	enforcer casbin.IEnforcerContext
	source   PolicySource
	revision uint64
}

// NewPolicyEngine loads the initial policy snapshot.
func NewPolicyEngine(source PolicySource) (*PolicyEngine, error) {
	if source == nil {
		return nil, fmt.Errorf("policy source is required")
	}
	policy, err := model.NewModelFromString(policyModel)
	if err != nil {
		return nil, fmt.Errorf("parse Casbin policy model: %w", err)
	}
	enforcer, err := casbin.NewContextEnforcer(policy, source)
	if err != nil {
		return nil, fmt.Errorf("create Casbin policy engine: %w", err)
	}
	enforcer.AddFunction("scopeMatch", scopeMatch)
	enforcer.EnableAutoSave(false)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	revision, err := source.CurrentRevision(ctx)
	if err != nil {
		return nil, err
	}
	return &PolicyEngine{enforcer: enforcer, source: source, revision: revision}, nil
}

// Authorize evaluates the currently cached policy and defaults to denial.
func (e *PolicyEngine) Authorize(_ context.Context, request authz.AuthorizationRequest) (bool, error) {
	if err := request.Validate(); err != nil {
		return false, fmt.Errorf("validate authorization request: %w", err)
	}
	if request.Disabled {
		return false, nil
	}
	e.mu.RLock()
	defer e.mu.RUnlock()
	allowed, err := e.enforcer.Enforce(
		request.UserID.String(), request.Scope.Tenant(), request.Scope.Path(), string(request.Permission),
	)
	if err != nil {
		return false, fmt.Errorf("enforce authorization policy: %w", err)
	}
	return allowed, nil
}

// Reload replaces the cached policy atomically with the latest committed snapshot.
func (e *PolicyEngine) Reload(ctx context.Context) error {
	e.mu.Lock()
	defer e.mu.Unlock()
	if err := e.enforcer.LoadPolicyCtx(ctx); err != nil {
		return fmt.Errorf("reload authorization policy: %w", err)
	}
	revision, err := e.source.CurrentRevision(ctx)
	if err != nil {
		return err
	}
	e.revision = revision
	return nil
}

// ReloadIfStale repairs missed notifications before policy evaluation continues.
func (e *PolicyEngine) ReloadIfStale(ctx context.Context) error {
	current, err := e.source.CurrentRevision(ctx)
	if err != nil {
		return err
	}
	e.mu.RLock()
	loaded := e.revision
	e.mu.RUnlock()
	if current <= loaded {
		return nil
	}
	return e.Reload(ctx)
}

// AuthorizeFresh revalidates cache revision before a sensitive operation.
func (e *PolicyEngine) AuthorizeFresh(ctx context.Context, request authz.AuthorizationRequest) (bool, error) {
	if err := e.ReloadIfStale(ctx); err != nil {
		return false, err
	}
	return e.Authorize(ctx, request)
}

// Revision returns the latest policy revision loaded by this process.
func (e *PolicyEngine) Revision() uint64 {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.revision
}

func scopeMatch(arguments ...interface{}) (interface{}, error) {
	if len(arguments) != 2 {
		return false, fmt.Errorf("scopeMatch expects resource and policy paths")
	}
	resource, resourceOK := arguments[0].(string)
	policy, policyOK := arguments[1].(string)
	if !resourceOK || !policyOK || resource == "" || policy == "" {
		return false, nil
	}
	return resource == policy || strings.HasPrefix(resource, policy+"/"), nil
}
