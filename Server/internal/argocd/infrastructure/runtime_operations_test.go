package infrastructure

import (
	"context"
	"io"
	"testing"
	"time"

	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"
	argov1alpha1 "github.com/argoproj/argo-cd/v3/pkg/apis/application/v1alpha1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	corev1 "k8s.io/api/core/v1"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestRuntimePodLogsUsesBoundedCanceledContext(t *testing.T) {
	manager := &runtimeManagerStub{}
	client := &Client{runtime: manager, token: "test", requestTimeout: time.Minute}
	query := argodomain.RuntimeLogQuery{Resource: argodomain.RuntimeResourceRef{
		Version: "v1", Kind: "Pod", Namespace: "payments", Name: "api-1",
	}}

	_, err := client.GetRuntimePodLogs(context.Background(), argodomain.ApplicationIdentity{Namespace: "argocd", Name: "payments"}, "payments", query)

	if err != nil {
		t.Fatalf("get runtime Pod logs: %v", err)
	}
	deadline, ok := manager.context.Deadline()
	if !ok || time.Until(deadline) > runtimeLogsTimeout {
		t.Fatalf("logs deadline = %v, present = %t", deadline, ok)
	}
	select {
	case <-manager.context.Done():
	default:
		t.Fatal("logs context remains active after bounded snapshot completed")
	}
}

func TestMapRuntimeNodePreservesNetworkSelectorEvidence(t *testing.T) {
	targetLabels := map[string]string{"app": "api"}
	labels := map[string]string{"app": "api", "revision": "v2"}
	node := argov1alpha1.ResourceNode{
		ResourceRef: argov1alpha1.ResourceRef{Version: "v1", Kind: "Service", Namespace: "payments", Name: "api"},
		NetworkingInfo: &argov1alpha1.ResourceNetworkingInfo{
			TargetLabels: targetLabels,
			Labels:       labels,
			TargetRefs:   []argov1alpha1.ResourceRef{{Kind: "Pod"}},
		},
	}

	value := mapRuntimeNode(node, false)
	targetLabels["app"] = "changed"
	labels["revision"] = "changed"

	if value.Networking.TargetLabels["app"] != "api" || value.Networking.Labels["revision"] != "v2" {
		t.Fatalf("networking evidence = %#v", value.Networking)
	}
	if len(value.Networking.TargetRefs) != 1 || value.Networking.TargetRefs[0].Kind != "Pod" {
		t.Fatalf("target refs = %#v", value.Networking.TargetRefs)
	}
}

type runtimeManagerStub struct{ context context.Context }

func (*runtimeManagerStub) ResourceTree(context.Context, *applicationpkg.ResourcesQuery, ...grpc.CallOption) (*argov1alpha1.ApplicationTree, error) {
	return &argov1alpha1.ApplicationTree{}, nil
}

func (*runtimeManagerStub) GetResource(context.Context, *applicationpkg.ApplicationResourceRequest, ...grpc.CallOption) (*applicationpkg.ApplicationResourceResponse, error) {
	return &applicationpkg.ApplicationResourceResponse{}, nil
}

func (*runtimeManagerStub) ListResourceEvents(context.Context, *applicationpkg.ApplicationResourceEventsQuery, ...grpc.CallOption) (*corev1.EventList, error) {
	return &corev1.EventList{}, nil
}

func (s *runtimeManagerStub) PodLogs(ctx context.Context, _ *applicationpkg.ApplicationPodLogsQuery, _ ...grpc.CallOption) (applicationpkg.ApplicationService_PodLogsClient, error) {
	s.context = ctx
	return &runtimePodLogsStream{context: ctx}, nil
}

type runtimePodLogsStream struct{ context context.Context }

func (*runtimePodLogsStream) Recv() (*applicationpkg.LogEntry, error) { return nil, io.EOF }
func (*runtimePodLogsStream) Header() (metadata.MD, error)            { return nil, nil }
func (*runtimePodLogsStream) Trailer() metadata.MD                    { return nil }
func (*runtimePodLogsStream) CloseSend() error                        { return nil }
func (s *runtimePodLogsStream) Context() context.Context              { return s.context }
func (*runtimePodLogsStream) SendMsg(any) error                       { return nil }
func (*runtimePodLogsStream) RecvMsg(any) error                       { return io.EOF }
