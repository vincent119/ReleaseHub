package infrastructure

import (
	"context"
	"errors"
	"io"
	"reflect"
	"strconv"
	"strings"
	"testing"
	"time"

	applicationpkg "github.com/argoproj/argo-cd/v3/pkg/apiclient/application"

	argodomain "github.com/vincent119/ReleaseHub/Server/internal/argocd/domain"
)

func TestReceiveRuntimeLogsBoundaries(t *testing.T) {
	entry := logEntryForTest("中文\nstack", false)
	timestamp, pod := "2026-01-01T00:00:00Z", "pod"
	entry.TimeStampStr, entry.PodName = &timestamp, &pod
	expected := argodomain.RuntimeLogEntry{Content: entry.GetContent(), Timestamp: timestamp, PodName: pod}
	failure := errors.New("stream failed")
	large := logEntryForTest(strings.Repeat("a", maxRuntimePayload), false)
	tests := []struct {
		name      string
		entries   []*applicationpkg.LogEntry
		err       error
		want      []argodomain.RuntimeLogEntry
		wantCalls int
	}{
		{name: "empty EOF", want: []argodomain.RuntimeLogEntry{}, wantCalls: 1},
		{name: "EOF preserves multiline and metadata", entries: []*applicationpkg.LogEntry{entry}, want: []argodomain.RuntimeLogEntry{expected}, wantCalls: 2},
		{name: "Last stops before next record", entries: []*applicationpkg.LogEntry{logEntryForTest("last", true), entry}, want: []argodomain.RuntimeLogEntry{{Content: "last"}}, wantCalls: 1},
		{name: "blank record is retained", entries: []*applicationpkg.LogEntry{{}}, want: []argodomain.RuntimeLogEntry{{}}, wantCalls: 2},
		{name: "failure before first record", err: failure, wantCalls: 1},
		{name: "midstream failure does not return successful partial logs", entries: []*applicationpkg.LogEntry{entry}, err: failure, wantCalls: 2},
		{name: "exact byte bound stops reading", entries: []*applicationpkg.LogEntry{large, entry}, want: []argodomain.RuntimeLogEntry{{Content: large.GetContent()}}, wantCalls: 1},
		{name: "oversize first record is not split", entries: []*applicationpkg.LogEntry{logEntryForTest(large.GetContent()+"a", false)}, want: []argodomain.RuntimeLogEntry{}, wantCalls: 1},
		{name: "cumulative UTF8 byte bound", entries: []*applicationpkg.LogEntry{logEntryForTest(strings.Repeat("a", maxRuntimePayload-2), false), logEntryForTest("中", false), entry}, want: []argodomain.RuntimeLogEntry{{Content: strings.Repeat("a", maxRuntimePayload-2)}}, wantCalls: 2},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stream := &sequenceLogStream{entries: tt.entries, err: tt.err}
			got, err := receiveRuntimeLogs(stream)
			if !errors.Is(err, tt.err) || !reflect.DeepEqual(got, tt.want) || stream.calls != tt.wantCalls {
				t.Fatalf("records=%d, error=%v, calls=%d; want records=%d, error=%v, calls=%d", len(got), err, stream.calls, len(tt.want), tt.err, tt.wantCalls)
			}
		})
	}
}

func TestReceiveRuntimeLogsStopsAtRecordLimit(t *testing.T) {
	entries := make([]*applicationpkg.LogEntry, maxRuntimeLogLines+1)
	for i := range entries {
		entries[i] = logEntryForTest("record", false)
	}
	stream := &sequenceLogStream{entries: entries}
	got, err := receiveRuntimeLogs(stream)
	if err != nil || int64(len(got)) != maxRuntimeLogLines || int64(stream.calls) != maxRuntimeLogLines {
		t.Fatalf("records=%d, calls=%d, error=%v", len(got), stream.calls, err)
	}
}

func TestRuntimePodLogsQueryAndOpenFailure(t *testing.T) {
	for _, tail := range []int64{-1, 0, 1, 500, 501} {
		t.Run(strconv.FormatInt(tail, 10), func(t *testing.T) {
			failure := errors.New("open failed")
			manager := &runtimeManagerStub{}
			manager.openLogs = func(_ context.Context, query *applicationpkg.ApplicationPodLogsQuery) (applicationpkg.ApplicationService_PodLogsClient, error) {
				wantTail := tail
				if wantTail <= 0 || wantTail > 500 {
					wantTail = 500
				}
				if query.GetFollow() || query.GetTailLines() != wantTail || query.GetContainer() != "worker" || query.GetPodName() != "pod" || query.GetNamespace() != "test" || query.GetName() != "app" || query.GetAppNamespace() != "argocd" || query.GetProject() != "project" {
					t.Fatalf("unexpected query: %v", query)
				}
				return nil, failure
			}
			client := &Client{runtime: manager, requestTimeout: time.Minute}
			query := logQueryForTest()
			query.TailLines = tail
			got, err := client.GetRuntimePodLogs(t.Context(), argodomain.ApplicationIdentity{Namespace: "argocd", Name: "app"}, "project", query)
			if !errors.Is(err, failure) || got != nil || !errors.Is(manager.context.Err(), context.Canceled) {
				t.Fatalf("records=%d, error=%v, context=%v", len(got), err, manager.context.Err())
			}
		})
	}
}

func TestRuntimePodLogsBlockedReceiveEndsWithContext(t *testing.T) {
	for _, mode := range []string{"parent cancellation", "parent deadline", "shorter request deadline"} {
		t.Run(mode, func(t *testing.T) {
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if mode == "parent deadline" {
				var deadlineCancel context.CancelFunc
				ctx, deadlineCancel = context.WithTimeout(ctx, 100*time.Millisecond)
				defer deadlineCancel()
			}
			entered := make(chan struct{})
			manager := &runtimeManagerStub{}
			manager.openLogs = func(ctx context.Context, _ *applicationpkg.ApplicationPodLogsQuery) (applicationpkg.ApplicationService_PodLogsClient, error) {
				return &blockingLogStream{runtimePodLogsStream: runtimePodLogsStream{context: ctx}, entered: entered}, nil
			}
			timeout := time.Minute
			if mode == "shorter request deadline" {
				timeout = 100 * time.Millisecond
			}
			client := &Client{runtime: manager, requestTimeout: timeout}
			done := make(chan error, 1)
			finished := make(chan struct{})
			go func() {
				defer close(finished)
				_, err := client.GetRuntimePodLogs(ctx, argodomain.ApplicationIdentity{Name: "app"}, "project", logQueryForTest())
				done <- err
			}()
			t.Cleanup(func() {
				cancel()
				select {
				case <-finished:
				case <-time.After(5 * time.Second):
					t.Error("log reader did not exit")
				}
			})
			select {
			case <-entered:
			case <-time.After(5 * time.Second):
				t.Fatal("Recv was not reached")
			}
			deadline, ok := manager.context.Deadline()
			if !ok || time.Until(deadline) > runtimeLogsTimeout {
				t.Fatal("missing bounded stream deadline")
			}
			want := context.DeadlineExceeded
			if mode == "parent cancellation" {
				cancel()
				want = context.Canceled
			}
			select {
			case err := <-done:
				if !errors.Is(err, want) {
					t.Fatalf("error=%v, want %v", err, want)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("blocked Recv did not stop")
			}
		})
	}
}

func logQueryForTest() argodomain.RuntimeLogQuery {
	return argodomain.RuntimeLogQuery{Resource: argodomain.RuntimeResourceRef{Version: "v1", Kind: "Pod", Namespace: "test", Name: "pod"}, Container: "worker"}
}

func logEntryForTest(content string, last bool) *applicationpkg.LogEntry {
	return &applicationpkg.LogEntry{Content: &content, Last: &last}
}

type sequenceLogStream struct {
	entries []*applicationpkg.LogEntry
	err     error
	calls   int
}

func (s *sequenceLogStream) Recv() (*applicationpkg.LogEntry, error) {
	s.calls++
	if s.calls <= len(s.entries) {
		return s.entries[s.calls-1], nil
	}
	if s.err != nil {
		return nil, s.err
	}
	return nil, io.EOF
}

type blockingLogStream struct {
	runtimePodLogsStream
	entered chan struct{}
}

func (s *blockingLogStream) Recv() (*applicationpkg.LogEntry, error) {
	close(s.entered)
	<-s.context.Done()
	return nil, s.context.Err()
}
