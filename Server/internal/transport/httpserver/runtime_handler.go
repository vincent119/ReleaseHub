package httpserver

import (
	"context"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	argoapp "github.com/vincent119/ReleaseHub/Server/internal/argocd/application"
	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

type runtimeService interface {
	Topology(context.Context, argoapp.RuntimePrincipal, uuid.UUID, string) (argoapp.RuntimeTopology, error)
	Resource(context.Context, argoapp.RuntimePrincipal, uuid.UUID, argodomain.RuntimeResourceRef) (argoapp.ResourceDetail, error)
	Events(context.Context, argoapp.RuntimePrincipal, uuid.UUID, argodomain.RuntimeResourceRef) ([]argodomain.RuntimeEvent, error)
	PodLogs(context.Context, argoapp.RuntimePrincipal, uuid.UUID, argodomain.RuntimeLogQuery) ([]argodomain.RuntimeLogEntry, error)
}

func (h *catalogHandler) GetCatalogApplicationRuntimeTopology(c *gin.Context, applicationID contract.ApplicationId, params contract.GetCatalogApplicationRuntimeTopologyParams) {
	principal, ok := h.runtimePrincipal(c)
	if !ok {
		return
	}
	value, err := h.runtime.Topology(c.Request.Context(), principal, applicationID, string(params.View))
	if !respondRuntimeError(c, "get Application runtime topology", err) {
		return
	}
	c.JSON(http.StatusOK, contract.RuntimeTopologyResponse{Data: runtimeTopologyResponse(value), Meta: responseMeta(c)})
}

func (h *catalogHandler) GetCatalogApplicationRuntimeResource(c *gin.Context, applicationID contract.ApplicationId, params contract.GetCatalogApplicationRuntimeResourceParams) {
	principal, ok := h.runtimePrincipal(c)
	if !ok {
		return
	}
	value, err := h.runtime.Resource(c.Request.Context(), principal, applicationID, runtimeResourceRef(runtimeResourceParameters{
		group: params.Group, version: params.Version, kind: params.Kind, namespace: params.Namespace, name: params.Name,
	}))
	if !respondRuntimeError(c, "get Application runtime resource", err) {
		return
	}
	c.JSON(http.StatusOK, contract.RuntimeResourceDetailResponse{Data: contract.RuntimeResourceDetail{Resource: runtimeResourceResponse(value.Resource), Manifest: value.Manifest}, Meta: responseMeta(c)})
}

func (h *catalogHandler) ListCatalogApplicationRuntimeEvents(c *gin.Context, applicationID contract.ApplicationId, params contract.ListCatalogApplicationRuntimeEventsParams) {
	principal, ok := h.runtimePrincipal(c)
	if !ok {
		return
	}
	values, err := h.runtime.Events(c.Request.Context(), principal, applicationID, runtimeResourceRef(runtimeResourceParameters{
		group: params.Group, version: params.Version, kind: params.Kind, namespace: params.Namespace, name: params.Name,
	}))
	if !respondRuntimeError(c, "list Application runtime events", err) {
		return
	}
	result := make([]contract.RuntimeEvent, 0, len(values))
	for _, value := range values {
		result = append(result, contract.RuntimeEvent{Type: value.Type, Reason: value.Reason, Message: value.Message, Count: value.Count, FirstObservedAt: value.FirstObserved, LastObservedAt: value.LastObserved})
	}
	c.JSON(http.StatusOK, contract.RuntimeEventListResponse{Data: result, Meta: responseMeta(c)})
}

func (h *catalogHandler) GetCatalogApplicationRuntimePodLogs(c *gin.Context, applicationID contract.ApplicationId, params contract.GetCatalogApplicationRuntimePodLogsParams) {
	principal, ok := h.runtimePrincipal(c)
	if !ok {
		return
	}
	query := runtimeLogQuery(params)
	values, err := h.runtime.PodLogs(c.Request.Context(), principal, applicationID, query)
	if !respondRuntimeError(c, "get Application runtime Pod logs", err) {
		return
	}
	result := make([]contract.RuntimeLogEntry, 0, len(values))
	for _, value := range values {
		result = append(result, contract.RuntimeLogEntry{Content: value.Content, Timestamp: value.Timestamp, PodName: value.PodName})
	}
	c.JSON(http.StatusOK, contract.RuntimeLogListResponse{Data: result, Meta: responseMeta(c)})
}

func (h *catalogHandler) runtimePrincipal(c *gin.Context) (argoapp.RuntimePrincipal, bool) {
	_, user, ok := h.authn.authenticate(c)
	if !ok {
		return argoapp.RuntimePrincipal{}, false
	}
	if h.runtime == nil {
		respondError(c, http.StatusServiceUnavailable, "RUNTIME_UNAVAILABLE", "Application runtime observation is unavailable")
		return argoapp.RuntimePrincipal{}, false
	}
	return argoapp.RuntimePrincipal{UserID: user.ID, Disabled: user.Disabled, RequestID: c.GetHeader(requestIDHeader)}, true
}

func respondRuntimeError(c *gin.Context, operation string, err error) bool {
	if err == nil {
		return true
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		respondRequestCanceled(c)
		return false
	}
	if errors.Is(err, argoapp.ErrInvalidRuntimeQuery) {
		respondError(c, http.StatusBadRequest, "RUNTIME_QUERY_INVALID", "Runtime resource query is invalid")
		return false
	}
	if errors.Is(err, argoapp.ErrRuntimeNotFound) {
		respondError(c, http.StatusNotFound, "RUNTIME_RESOURCE_NOT_FOUND", "Runtime resource was not found")
		return false
	}
	recordRequestError(c, operation, err)
	respondError(c, http.StatusServiceUnavailable, "RUNTIME_UNAVAILABLE", "Application runtime observation is unavailable")
	return false
}

type runtimeResourceParameters struct {
	group     *contract.RuntimeResourceGroup
	version   contract.RuntimeResourceVersion
	kind      contract.RuntimeResourceKind
	namespace *contract.RuntimeResourceNamespace
	name      contract.RuntimeResourceName
}

func runtimeResourceRef(params runtimeResourceParameters) argodomain.RuntimeResourceRef {
	value := argodomain.RuntimeResourceRef{Version: string(params.version), Kind: string(params.kind), Name: string(params.name)}
	if params.group != nil {
		value.Group = string(*params.group)
	}
	if params.namespace != nil {
		value.Namespace = string(*params.namespace)
	}
	return value
}

func runtimeLogQuery(params contract.GetCatalogApplicationRuntimePodLogsParams) argodomain.RuntimeLogQuery {
	query := argodomain.RuntimeLogQuery{Resource: runtimeResourceRef(runtimeResourceParameters{
		group: params.Group, version: params.Version, kind: params.Kind, namespace: params.Namespace, name: params.Name,
	})}
	if params.Container != nil {
		query.Container = *params.Container
	}
	if params.TailLines != nil {
		query.TailLines = *params.TailLines
	}
	return query
}

func runtimeTopologyResponse(value argoapp.RuntimeTopology) contract.RuntimeTopology {
	nodes := make([]contract.RuntimeResourceNode, 0, len(value.Nodes))
	for _, node := range value.Nodes {
		nodes = append(nodes, runtimeResourceResponse(node.Resource))
	}
	edges := make([]contract.RuntimeTopologyEdge, 0, len(value.Edges))
	for _, edge := range value.Edges {
		edges = append(edges, contract.RuntimeTopologyEdge{Id: edge.ID, Source: edge.Source, Target: edge.Target, Kind: contract.RuntimeTopologyEdgeKind(edge.Kind)})
	}
	return contract.RuntimeTopology{ApplicationId: value.ApplicationID, View: contract.RuntimeTopologyView(value.View), ObservedAt: value.ObservedAt, Nodes: nodes, Edges: edges, Warnings: value.Warnings, Partial: value.Partial}
}

func runtimeResourceResponse(value argodomain.RuntimeResource) contract.RuntimeResourceNode {
	info := make([]contract.RuntimeResourceInfo, 0, len(value.Info))
	for _, item := range value.Info {
		info = append(info, contract.RuntimeResourceInfo{Name: item.Name, Value: item.Value})
	}
	return contract.RuntimeResourceNode{
		Id: value.Ref.Key(), Group: value.Ref.Group, Version: value.Ref.Version, Kind: value.Ref.Kind,
		Namespace: value.Ref.Namespace, Name: value.Ref.Name, HealthStatus: value.HealthStatus,
		HealthMessage: value.HealthMessage, CreatedAt: value.CreatedAt, Images: value.Images, Info: info,
		Ingress: value.Networking.Ingress, ExternalUrls: value.Networking.ExternalURLs, Orphaned: value.Orphaned,
	}
}
