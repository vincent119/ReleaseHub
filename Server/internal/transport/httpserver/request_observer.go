package httpserver

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/vincent119/zlogger"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/codes"
	"go.opentelemetry.io/otel/trace"
)

type httpObserver struct {
	options    RouterOptions
	probePaths map[string]struct{}
	tracer     trace.Tracer
}

type requestObservation struct {
	ctx       context.Context
	startedAt time.Time
	requestID string
	isProbe   bool
	span      trace.Span
}

func requestObserver(options RouterOptions) gin.HandlerFunc {
	observer := &httpObserver{
		options: options,
		probePaths: map[string]struct{}{
			"/healthz": {}, "/readyz": {}, options.MetricsPath: {},
		},
		tracer: options.TracerProvider.Tracer("releasehub/http"),
	}
	return observer.observe
}

func (o *httpObserver) observe(c *gin.Context) {
	observation := o.begin(c)
	c.Next()
	if observation.skipSuccessfulProbe(c) {
		return
	}
	observation.ensureSpan(c, o.tracer)
	observation.recordSpan(c)
	o.recordMetrics(c, observation)
	o.writeLog(c, observation)
}

func (o *httpObserver) begin(c *gin.Context) *requestObservation {
	startedAt := time.Now()
	requestID := normalizedRequestID(c.GetHeader(requestIDHeader))
	c.Header(requestIDHeader, requestID)
	c.Request.Header.Set(requestIDHeader, requestID)
	ctx := zlogger.WithRequestID(c.Request.Context(), requestID)
	_, isProbe := o.probePaths[c.Request.URL.Path]
	observation := &requestObservation{
		ctx: ctx, startedAt: startedAt, requestID: requestID, isProbe: isProbe,
	}
	if !isProbe {
		ctx, observation.span = o.tracer.Start(ctx, "HTTP request")
		c.Request = c.Request.WithContext(ctx)
	}
	return observation
}

func (o *requestObservation) skipSuccessfulProbe(c *gin.Context) bool {
	return o.isProbe && c.Writer.Status() < http.StatusBadRequest
}

func (o *requestObservation) ensureSpan(c *gin.Context, tracer trace.Tracer) {
	if !o.isProbe {
		return
	}
	_, o.span = tracer.Start(
		o.ctx, c.Request.Method+" "+normalizedRoute(c),
		trace.WithTimestamp(o.startedAt),
	)
}

func (o *requestObservation) recordSpan(c *gin.Context) {
	status := c.Writer.Status()
	route := normalizedRoute(c)
	o.span.SetName(c.Request.Method + " " + route)
	o.span.SetAttributes(
		attribute.String("http.request.method", c.Request.Method),
		attribute.String("http.route", route),
		attribute.Int("http.response.status_code", status),
	)
	if status >= http.StatusBadRequest {
		o.span.SetStatus(codes.Error, http.StatusText(status))
	}
	o.span.End(trace.WithTimestamp(time.Now()))
}

func (o *httpObserver) recordMetrics(c *gin.Context, observation *requestObservation) {
	o.options.HTTPMetrics.Observe(
		c.Request.Method, normalizedRoute(c), c.Writer.Status(),
		time.Since(observation.startedAt),
	)
}

func (o *httpObserver) writeLog(c *gin.Context, observation *requestObservation) {
	fields := observation.logFields(c)
	if c.Writer.Status() >= http.StatusBadRequest || len(c.Errors) > 0 {
		o.options.Logger.Error("HTTP request failed", fields...)
		return
	}
	o.options.Logger.Info("HTTP request", fields...)
}

func (o *requestObservation) logFields(c *gin.Context) []zlogger.Field {
	fields := []zlogger.Field{
		zlogger.String("request_id", o.requestID),
		zlogger.String("method", c.Request.Method),
		zlogger.String("route", normalizedRoute(c)),
		zlogger.Int("status", c.Writer.Status()),
		zlogger.Duration("latency", time.Since(o.startedAt)),
	}
	if o.span.SpanContext().HasTraceID() {
		fields = append(fields, traceLogFields(o.span.SpanContext())...)
	}
	return fields
}

func traceLogFields(span trace.SpanContext) []zlogger.Field {
	return []zlogger.Field{
		zlogger.String("trace_id", span.TraceID().String()),
		zlogger.String("span_id", span.SpanID().String()),
	}
}

func normalizedRoute(c *gin.Context) string {
	if c.FullPath() == "" {
		return "unmatched"
	}
	return c.FullPath()
}

func normalizedRequestID(value string) string {
	if value == "" || len(value) > 128 {
		return newRequestID()
	}
	return value
}

func newRequestID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err == nil {
		return hex.EncodeToString(bytes)
	}
	return strconv.FormatInt(time.Now().UnixNano(), 10)
}
