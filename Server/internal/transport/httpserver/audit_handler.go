package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	auditapp "github.com/vincent119/ReleaseHub/Server/internal/audit/application"
	auditdomain "github.com/vincent119/ReleaseHub/Server/internal/audit/domain"
	authz "github.com/vincent119/ReleaseHub/Server/internal/authorization/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type auditQueryService interface {
	Capabilities(context.Context, auditapp.Principal) (auditapp.Capabilities, error)
	ResolveScope(context.Context, authz.ScopeKind, *uuid.UUID) (authz.Scope, error)
	List(context.Context, auditapp.Principal, auditdomain.QueryFilter, string) (auditapp.QueryPage, error)
	FilterOptions(context.Context, auditapp.Principal, auditdomain.FilterOptionFilter) ([]auditapp.FilterOption, error)
	Detail(context.Context, auditapp.Principal, uuid.UUID) (auditdomain.EventDetail, error)
}

type auditHandler struct {
	authn   *authHandler
	queries auditQueryService
}

var auditAPIErrorDefinitions = []apiErrorDefinition{
	{Code: "AUDIT_INVALID_QUERY", Category: apiErrorValidation, Status: http.StatusBadRequest, Message: "Audit Trail query is invalid"},
	{Code: "AUDIT_NOT_FOUND", Category: apiErrorNotFound, Status: http.StatusNotFound, Message: "Audit Trail data was not found"},
	{Code: "AUDIT_UNAVAILABLE", Category: apiErrorDependency, Status: http.StatusServiceUnavailable, Message: "Audit Trail is unavailable", Retryable: true},
}

// GetAuditCapabilities returns server-owned Audit Trail presentation capabilities.
func (h *auditHandler) GetAuditCapabilities(c *gin.Context) {
	principal, ok := h.principal(c)
	if !ok {
		return
	}
	value, err := h.queries.Capabilities(c.Request.Context(), principal)
	if !respondAuditError(c, err) {
		return
	}
	c.JSON(http.StatusOK, contract.AuditCapabilitiesResponse{Data: auditCapabilities(value), Meta: responseMeta(c)})
}

// ListAuditEvents returns one stable metadata-free Audit Trail page.
func (h *auditHandler) ListAuditEvents(c *gin.Context, params contract.ListAuditEventsParams) {
	principal, ok := h.principal(c)
	if !ok {
		return
	}
	scopeID := auditScopeID(params.ScopeId)
	scope, err := h.queries.ResolveScope(c.Request.Context(), authz.ScopeKind(params.ScopeKind), scopeID)
	if err != nil {
		respondAuditError(c, err)
		return
	}
	page, err := h.queries.List(c.Request.Context(), principal, auditFilter(scope, params), auditCursor(params.Cursor))
	if !respondAuditError(c, err) {
		return
	}
	meta := responseMeta(c)
	c.JSON(http.StatusOK, contract.AuditEventListResponse{Data: auditEventSummaries(page.Items), Meta: contract.CursorPageMeta{
		RequestId: meta.RequestId, Timestamp: meta.Timestamp, NextCursor: optionalCursor(page.NextCursor), HasMore: page.HasMore,
	}})
}

// ListAuditFilterOptions returns bounded autocomplete values from authorized events only.
func (h *auditHandler) ListAuditFilterOptions(c *gin.Context, params contract.ListAuditFilterOptionsParams) {
	principal, ok := h.principal(c)
	if !ok {
		return
	}
	scope, err := h.queries.ResolveScope(c.Request.Context(), authz.ScopeKind(params.ScopeKind), auditScopeID(params.ScopeId))
	if err != nil {
		respondAuditError(c, err)
		return
	}
	filter := auditFilterOptionFilter(scope, params)
	options, err := h.queries.FilterOptions(c.Request.Context(), principal, filter)
	if !respondAuditError(c, err) {
		return
	}
	response := make([]contract.AuditFilterOption, 0, len(options))
	for _, option := range options {
		response = append(response, contract.AuditFilterOption{Value: option.Value, Label: option.Label})
	}
	c.JSON(http.StatusOK, contract.AuditFilterOptionListResponse{Data: response, Meta: responseMeta(c)})
}

// GetAuditEvent returns one authorized Audit Trail detail with safe metadata.
func (h *auditHandler) GetAuditEvent(c *gin.Context, eventID uuid.UUID) {
	principal, ok := h.principal(c)
	if !ok {
		return
	}
	value, err := h.queries.Detail(c.Request.Context(), principal, eventID)
	if !respondAuditError(c, err) {
		return
	}
	c.JSON(http.StatusOK, contract.AuditEventDetailResponse{Data: auditEventDetail(value), Meta: responseMeta(c)})
}

func (h *auditHandler) principal(c *gin.Context) (auditapp.Principal, bool) {
	if h.queries == nil {
		_, _, ok := h.authn.authenticate(c)
		if ok {
			respondError(c, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Audit Trail is unavailable")
		}
		return auditapp.Principal{}, false
	}
	_, user, ok := h.authn.authenticate(c)
	return auditapp.Principal{UserID: user.ID, Disabled: user.Disabled}, ok
}

func auditScopeID(value *uuid.UUID) *uuid.UUID {
	if value == nil {
		return nil
	}
	copy := *value
	return &copy
}

func auditFilter(scope authz.Scope, params contract.ListAuditEventsParams) auditdomain.QueryFilter {
	filter := auditdomain.QueryFilter{Scope: scope, Limit: auditLimit(params.Limit)}
	if params.OccurredFrom != nil {
		filter.OccurredFrom = *params.OccurredFrom
	}
	if params.OccurredTo != nil {
		filter.OccurredTo = *params.OccurredTo
	}
	applyAuditTextFilters(&filter, params)
	return filter
}

func auditFilterOptionFilter(scope authz.Scope, params contract.ListAuditFilterOptionsParams) auditdomain.FilterOptionFilter {
	filter := auditdomain.FilterOptionFilter{Scope: scope, Field: auditdomain.FilterOptionField(params.Field)}
	if params.OccurredFrom != nil {
		filter.OccurredFrom = *params.OccurredFrom
	}
	if params.OccurredTo != nil {
		filter.OccurredTo = *params.OccurredTo
	}
	if params.Search != nil {
		filter.Search = *params.Search
	}
	if params.Limit != nil {
		filter.Limit = *params.Limit
	}
	return filter
}

func applyAuditTextFilters(filter *auditdomain.QueryFilter, params contract.ListAuditEventsParams) {
	if params.Action != nil {
		filter.Action = *params.Action
	}
	if params.ResourceType != nil {
		filter.ResourceType = *params.ResourceType
	}
	if params.Actor != nil {
		filter.Actor = *params.Actor
	}
	if params.RequestId != nil {
		filter.RequestID = *params.RequestId
	}
}

func auditLimit(value *contract.Limit) int {
	if value == nil {
		return auditdomain.DefaultLimit
	}
	return int(*value)
}

func auditCursor(value *contract.Cursor) string {
	if value == nil {
		return ""
	}
	return string(*value)
}

func auditCapabilities(value auditapp.Capabilities) contract.AuditCapabilities {
	roots := make([]contract.AuditScopeRoot, 0, len(value.ScopeRoots))
	for _, root := range value.ScopeRoots {
		id := scopeRootID(root.Scope)
		roots = append(roots, contract.AuditScopeRoot{Kind: contract.AuditScopeKind(root.Scope.Kind), Id: id, Label: root.Label})
	}
	return contract.AuditCapabilities{Visible: value.Visible, ScopeRoots: roots}
}

func scopeRootID(scope authz.Scope) *uuid.UUID {
	var value uuid.UUID
	switch scope.Kind {
	case authz.ScopeProject:
		value = scope.ProjectID
	case authz.ScopeEnvironment:
		value = scope.EnvironmentID
	case authz.ScopeApplication:
		value = scope.ApplicationID
	default:
		return nil
	}
	return &value
}

func auditEventSummaries(values []auditdomain.EventSummary) []contract.AuditEventSummary {
	result := make([]contract.AuditEventSummary, 0, len(values))
	for _, value := range values {
		result = append(result, auditEventSummary(value))
	}
	return result
}

func auditEventSummary(value auditdomain.EventSummary) contract.AuditEventSummary {
	return contract.AuditEventSummary{Id: value.ID, OccurredAt: value.OccurredAt, Actor: auditActor(value),
		Action: value.Action, Resource: contract.AuditResourceSummary{Type: value.ResourceType, Id: value.ResourceID},
		Scope: auditScope(value.Scope), RequestId: auditOptionalString(value.RequestID), HasMetadata: value.HasMetadata}
}

func auditEventDetail(value auditdomain.EventDetail) contract.AuditEventDetail {
	summary := auditEventSummary(value.EventSummary)
	return contract.AuditEventDetail{Id: summary.Id, OccurredAt: summary.OccurredAt, Actor: summary.Actor,
		Action: summary.Action, Resource: summary.Resource, Scope: summary.Scope, RequestId: summary.RequestId,
		HasMetadata: summary.HasMetadata, Metadata: value.Metadata, MetadataTruncated: value.MetadataTruncated}
}

func auditActor(value auditdomain.EventSummary) contract.AuditActorSummary {
	if value.ActorID == nil {
		return contract.AuditActorSummary{Kind: contract.System}
	}
	if value.ActorDisplayName == "" {
		return contract.AuditActorSummary{Kind: contract.Unknown, Id: value.ActorID}
	}
	return contract.AuditActorSummary{Kind: contract.User, Id: value.ActorID, DisplayName: auditOptionalString(value.ActorDisplayName)}
}

func auditScope(value auditdomain.ScopeAncestry) contract.AuditScopeSummary {
	kind, id := auditScopeIdentity(value)
	return contract.AuditScopeSummary{Kind: contract.AuditScopeKind(kind), Id: id, OrganizationId: value.OrganizationID,
		ProjectId: value.ProjectID, EnvironmentId: value.EnvironmentID, ApplicationId: value.ApplicationID,
		Resolution: contract.AuditScopeSummaryResolution(value.Resolution)}
}

func auditScopeIdentity(value auditdomain.ScopeAncestry) (authz.ScopeKind, *uuid.UUID) {
	if value.ApplicationID != nil {
		return authz.ScopeApplication, value.ApplicationID
	}
	if value.EnvironmentID != nil {
		return authz.ScopeEnvironment, value.EnvironmentID
	}
	if value.ProjectID != nil {
		return authz.ScopeProject, value.ProjectID
	}
	return authz.ScopePlatform, nil
}

func auditOptionalString(value string) *string {
	if value == "" {
		return nil
	}
	return &value
}

func respondAuditError(c *gin.Context, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) {
		respondRequestCanceled(c)
		return false
	}
	if errors.Is(err, auditdomain.ErrInvalidFilter) || errors.Is(err, auditdomain.ErrInvalidCursor) {
		respondError(c, http.StatusBadRequest, "AUDIT_INVALID_QUERY", "Audit Trail query is invalid")
	} else if errors.Is(err, auditapp.ErrForbidden) || errors.Is(err, auditapp.ErrNotFound) {
		respondError(c, http.StatusNotFound, "AUDIT_NOT_FOUND", "Audit Trail data was not found")
	} else {
		respondError(c, http.StatusServiceUnavailable, "AUDIT_UNAVAILABLE", "Audit Trail is unavailable")
	}
	return false
}
