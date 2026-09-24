package infrastructure

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"slices"
	"strings"
	"time"

	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	corev1 "k8s.io/api/core/v1"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

const (
	maxRuntimeLogLines = int64(500)
	maxRuntimePayload  = 1 << 20
	runtimeLogsTimeout = 15 * time.Second
)

// GetRuntimeTree reads one live Application resource tree without requesting a refresh or mutation.
func (c *Client) GetRuntimeTree(ctx context.Context, identity argodomain.ApplicationIdentity, project string) (argodomain.RuntimeTree, error) {
	if c.runtime == nil {
		return argodomain.RuntimeTree{}, fmt.Errorf("argo CD runtime client is unavailable")
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	tree, err := c.runtime.ResourceTree(requestCtx, runtimeResourcesQuery(identity, project))
	if err != nil {
		return argodomain.RuntimeTree{}, fmt.Errorf("get Argo CD Application resource tree: %w", err)
	}
	return mapRuntimeTree(tree, time.Now().UTC()), nil
}

// GetRuntimeResource reads the live manifest for one resource.
func (c *Client) GetRuntimeResource(ctx context.Context, identity argodomain.ApplicationIdentity, project string, resource argodomain.RuntimeResourceRef) (string, error) {
	if c.runtime == nil {
		return "", fmt.Errorf("argo CD runtime client is unavailable")
	}
	if err := resource.Validate(); err != nil {
		return "", err
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	response, err := c.runtime.GetResource(requestCtx, runtimeResourceRequest(identity, project, resource))
	if err != nil {
		return "", fmt.Errorf("get Argo CD Application resource: %w", err)
	}
	manifest := response.GetManifest()
	if len(manifest) > maxRuntimePayload {
		return "", fmt.Errorf("runtime resource manifest exceeds size limit")
	}
	return manifest, nil
}

// ListRuntimeEvents reads a bounded set of events for one authorized resource.
func (c *Client) ListRuntimeEvents(ctx context.Context, identity argodomain.ApplicationIdentity, project string, resource argodomain.RuntimeResourceRef) ([]argodomain.RuntimeEvent, error) {
	if c.runtime == nil {
		return nil, fmt.Errorf("argo CD runtime client is unavailable")
	}
	if err := resource.Validate(); err != nil {
		return nil, err
	}
	requestCtx, cancel := c.requestContext(ctx)
	defer cancel()
	response, err := c.runtime.ListResourceEvents(requestCtx, runtimeEventsQuery(identity, project, resource))
	if err != nil {
		return nil, fmt.Errorf("list Argo CD Application resource events: %w", err)
	}
	return mapRuntimeEvents(response.Items), nil
}

// GetRuntimePodLogs consumes a bounded Argo CD PodLogs stream.
func (c *Client) GetRuntimePodLogs(ctx context.Context, identity argodomain.ApplicationIdentity, project string, query argodomain.RuntimeLogQuery) ([]argodomain.RuntimeLogEntry, error) {
	if c.runtime == nil {
		return nil, fmt.Errorf("argo CD runtime client is unavailable")
	}
	if err := query.Resource.Validate(); err != nil {
		return nil, err
	}
	if query.Resource.Kind != "Pod" {
		return nil, fmt.Errorf("runtime logs are available only for Pods")
	}
	tailLines := query.TailLines
	if tailLines <= 0 || tailLines > maxRuntimeLogLines {
		tailLines = maxRuntimeLogLines
	}
	boundedCtx, boundedCancel := context.WithTimeout(ctx, runtimeLogsTimeout)
	defer boundedCancel()
	requestCtx, cancel := c.requestContext(boundedCtx)
	defer cancel()
	stream, err := c.runtime.PodLogs(requestCtx, runtimePodLogsQuery(identity, project, query, tailLines))
	if err != nil {
		return nil, fmt.Errorf("open Argo CD Pod logs stream: %w", err)
	}
	return receiveRuntimeLogs(stream)
}

func runtimeResourcesQuery(identity argodomain.ApplicationIdentity, project string) *applicationpkg.ResourcesQuery {
	return &applicationpkg.ResourcesQuery{ApplicationName: &identity.Name, AppNamespace: &identity.Namespace, Project: &project}
}

func runtimeResourceRequest(identity argodomain.ApplicationIdentity, project string, resource argodomain.RuntimeResourceRef) *applicationpkg.ApplicationResourceRequest {
	return &applicationpkg.ApplicationResourceRequest{
		Name: &identity.Name, AppNamespace: &identity.Namespace, Project: &project,
		Namespace: &resource.Namespace, ResourceName: &resource.Name, Version: &resource.Version,
		Group: &resource.Group, Kind: &resource.Kind,
	}
}

func runtimeEventsQuery(identity argodomain.ApplicationIdentity, project string, resource argodomain.RuntimeResourceRef) *applicationpkg.ApplicationResourceEventsQuery {
	return &applicationpkg.ApplicationResourceEventsQuery{
		Name: &identity.Name, AppNamespace: &identity.Namespace, Project: &project,
		ResourceNamespace: &resource.Namespace, ResourceName: &resource.Name, ResourceUID: &resource.UID,
	}
}

func runtimePodLogsQuery(identity argodomain.ApplicationIdentity, project string, query argodomain.RuntimeLogQuery, tailLines int64) *applicationpkg.ApplicationPodLogsQuery {
	follow := false
	return &applicationpkg.ApplicationPodLogsQuery{
		Name: &identity.Name, AppNamespace: &identity.Namespace, Project: &project,
		Namespace: &query.Resource.Namespace, PodName: &query.Resource.Name,
		Container: &query.Container, TailLines: &tailLines, Follow: &follow,
	}
}

func mapRuntimeTree(tree *argov1alpha1.ApplicationTree, observedAt time.Time) argodomain.RuntimeTree {
	if tree == nil {
		return argodomain.RuntimeTree{Resources: []argodomain.RuntimeResource{}, ObservedAt: observedAt.UTC()}
	}
	resources := make([]argodomain.RuntimeResource, 0, len(tree.Nodes)+len(tree.OrphanedNodes))
	for i := range tree.Nodes {
		resources = append(resources, mapRuntimeNode(tree.Nodes[i], false))
	}
	for i := range tree.OrphanedNodes {
		resources = append(resources, mapRuntimeNode(tree.OrphanedNodes[i], true))
	}
	slices.SortFunc(resources, func(left, right argodomain.RuntimeResource) int {
		return strings.Compare(left.Ref.Key(), right.Ref.Key())
	})
	return argodomain.RuntimeTree{Resources: resources, ObservedAt: observedAt.UTC()}
}

func mapRuntimeNode(node argov1alpha1.ResourceNode, orphaned bool) argodomain.RuntimeResource {
	value := argodomain.RuntimeResource{
		Ref: mapRuntimeRef(node.ResourceRef), ParentRefs: mapRuntimeRefs(node.ParentRefs),
		Info: mapRuntimeInfo(node.Info), Images: slices.Clone(node.Images), Orphaned: orphaned,
	}
	if node.Health != nil {
		value.HealthStatus = string(node.Health.Status)
		value.HealthMessage = node.Health.Message
	}
	if node.CreatedAt != nil {
		createdAt := node.CreatedAt.Time.UTC()
		value.CreatedAt = &createdAt
	}
	value.Networking = mapRuntimeNetworking(node.NetworkingInfo)
	return value
}

func mapRuntimeNetworking(value *argov1alpha1.ResourceNetworkingInfo) argodomain.RuntimeNetworking {
	if value == nil {
		return argodomain.RuntimeNetworking{}
	}
	networking := argodomain.RuntimeNetworking{
		TargetRefs: mapRuntimeRefs(value.TargetRefs), TargetLabels: maps.Clone(value.TargetLabels),
		Labels: maps.Clone(value.Labels), ExternalURLs: slices.Clone(value.ExternalURLs),
	}
	for _, ingress := range value.Ingress {
		if ingress.Hostname != "" {
			networking.Ingress = append(networking.Ingress, ingress.Hostname)
		} else if ingress.IP != "" {
			networking.Ingress = append(networking.Ingress, ingress.IP)
		}
	}
	return networking
}

func mapRuntimeRef(value argov1alpha1.ResourceRef) argodomain.RuntimeResourceRef {
	return argodomain.RuntimeResourceRef{
		Group: value.Group, Version: value.Version, Kind: value.Kind,
		Namespace: value.Namespace, Name: value.Name, UID: value.UID,
	}
}

func mapRuntimeRefs(values []argov1alpha1.ResourceRef) []argodomain.RuntimeResourceRef {
	result := make([]argodomain.RuntimeResourceRef, 0, len(values))
	for _, value := range values {
		result = append(result, mapRuntimeRef(value))
	}
	return result
}

func mapRuntimeInfo(values []argov1alpha1.InfoItem) []argodomain.RuntimeInfo {
	result := make([]argodomain.RuntimeInfo, 0, len(values))
	for _, value := range values {
		result = append(result, argodomain.RuntimeInfo{Name: value.Name, Value: value.Value})
	}
	return result
}

func mapRuntimeEvents(values []corev1.Event) []argodomain.RuntimeEvent {
	result := make([]argodomain.RuntimeEvent, 0, len(values))
	for _, value := range values {
		result = append(result, argodomain.RuntimeEvent{
			Type: value.Type, Reason: value.Reason, Message: value.Message, Count: value.Count,
			FirstObserved: runtimeEventTime(value.FirstTimestamp.Time), LastObserved: runtimeEventTime(value.LastTimestamp.Time),
		})
	}
	slices.SortFunc(result, func(left, right argodomain.RuntimeEvent) int {
		return strings.Compare(runtimeEventSortValue(right), runtimeEventSortValue(left))
	})
	return result
}

func runtimeEventTime(value time.Time) *time.Time {
	if value.IsZero() {
		return nil
	}
	result := value.UTC()
	return &result
}

func runtimeEventSortValue(value argodomain.RuntimeEvent) string {
	if value.LastObserved != nil {
		return value.LastObserved.Format(time.RFC3339Nano)
	}
	if value.FirstObserved != nil {
		return value.FirstObserved.Format(time.RFC3339Nano)
	}
	return ""
}

type runtimeLogStream interface {
	Recv() (*applicationpkg.LogEntry, error)
}

func receiveRuntimeLogs(stream runtimeLogStream) ([]argodomain.RuntimeLogEntry, error) {
	entries := make([]argodomain.RuntimeLogEntry, 0)
	bytesRead := 0
	for int64(len(entries)) < maxRuntimeLogLines && bytesRead < maxRuntimePayload {
		entry, err := stream.Recv()
		if errors.Is(err, io.EOF) {
			return entries, nil
		}
		if err != nil {
			return nil, fmt.Errorf("receive Argo CD Pod logs: %w", err)
		}
		content := entry.GetContent()
		bytesRead += len(content)
		if bytesRead > maxRuntimePayload {
			break
		}
		entries = append(entries, argodomain.RuntimeLogEntry{
			Content: content, Timestamp: entry.GetTimeStampStr(), PodName: entry.GetPodName(),
		})
		if entry.GetLast() {
			return entries, nil
		}
	}
	return entries, nil
}
