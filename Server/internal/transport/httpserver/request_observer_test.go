package httpserver

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"

	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

func TestRequestObserverClassifiesHTTPOutcomeLogLevels(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		handlerErr  bool
		wantLevel   zapcore.Level
		wantMessage string
	}{
		{name: "successful request", status: http.StatusOK, wantLevel: zap.InfoLevel, wantMessage: "HTTP request"},
		{name: "unauthorized request", status: http.StatusUnauthorized, wantLevel: zap.WarnLevel, wantMessage: "HTTP request rejected"},
		{name: "client canceled request", status: 499, wantLevel: zap.InfoLevel, wantMessage: "HTTP request canceled"},
		{name: "server failure", status: http.StatusInternalServerError, wantLevel: zap.ErrorLevel, wantMessage: "HTTP request failed"},
		{name: "handler error", status: http.StatusBadRequest, handlerErr: true, wantLevel: zap.ErrorLevel, wantMessage: "HTTP request failed"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, recorded := observer.New(zap.DebugLevel)
			registry := prometheus.NewRegistry()
			metrics, err := observability.NewHTTPMetrics(registry)
			if err != nil {
				t.Fatalf("create HTTP metrics: %v", err)
			}
			router := gin.New()
			router.Use(requestObserver(RouterOptions{
				Logger:         zap.New(core),
				HTTPMetrics:    metrics,
				TracerProvider: trace.NewNoopTracerProvider(),
				MetricsPath:    "/metrics",
			}))
			router.GET("/result", func(c *gin.Context) {
				if test.handlerErr {
					_ = c.Error(errors.New("handler failed"))
				}
				c.Status(test.status)
			})

			response := httptest.NewRecorder()
			router.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/result", nil))

			entries := recorded.All()
			if len(entries) != 1 {
				t.Fatalf("log entry count = %d, want 1", len(entries))
			}
			entry := entries[0]
			if entry.Level != test.wantLevel || entry.Message != test.wantMessage {
				t.Fatalf("log = %s %q, want %s %q", entry.Level, entry.Message, test.wantLevel, test.wantMessage)
			}
			if got := entry.ContextMap()["status"]; got != int64(test.status) {
				t.Fatalf("logged status = %#v, want %d", got, test.status)
			}
			if test.handlerErr && entry.ContextMap()["error"] != "handler failed" {
				t.Fatalf("logged error = %#v", entry.ContextMap()["error"])
			}
			if got := observedRequestStatus(t, registry); got != test.status {
				t.Fatalf("metric status = %d, want %d", got, test.status)
			}
		})
	}
}

func TestAPIErrorCorrelatesResponseHeaderBodyAndLog(t *testing.T) {
	core, recorded := observer.New(zap.DebugLevel)
	registry := prometheus.NewRegistry()
	metrics, err := observability.NewHTTPMetrics(registry)
	if err != nil {
		t.Fatalf("create HTTP metrics: %v", err)
	}
	router := gin.New()
	router.Use(requestObserver(RouterOptions{
		Logger: zap.New(core), HTTPMetrics: metrics,
		TracerProvider: trace.NewNoopTracerProvider(), MetricsPath: "/metrics",
	}))
	router.GET("/error", func(c *gin.Context) {
		respondError(c, http.StatusConflict, "TEST_CONFLICT", "Safe conflict")
	})

	request := httptest.NewRequest(http.MethodGet, "/error", nil)
	request.Header.Set(requestIDHeader, "request-correlation-1")
	response := httptest.NewRecorder()
	router.ServeHTTP(response, request)

	var body contract.ErrorResponse
	if err := json.Unmarshal(response.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if got := response.Header().Get(requestIDHeader); got != body.RequestId || got != "request-correlation-1" {
		t.Fatalf("request IDs header=%q body=%q", got, body.RequestId)
	}
	entries := recorded.All()
	if len(entries) != 1 || entries[0].ContextMap()["request_id"] != body.RequestId {
		t.Fatalf("request log = %#v", entries)
	}
}

func observedRequestStatus(t *testing.T, registry *prometheus.Registry) int {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatalf("gather metrics: %v", err)
	}
	for _, family := range families {
		if family.GetName() != "releasehub_http_requests_total" {
			continue
		}
		for _, metric := range family.Metric {
			for _, label := range metric.Label {
				if label.GetName() == "status" {
					status, parseErr := strconv.Atoi(label.GetValue())
					if parseErr != nil {
						t.Fatalf("parse metric status: %v", parseErr)
					}
					return status
				}
			}
		}
	}
	t.Fatal("HTTP request metric status not found")
	return 0
}
