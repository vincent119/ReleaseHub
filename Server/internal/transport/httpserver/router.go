// Package httpserver builds the Gin HTTP boundary for the ReleaseHub API.
package httpserver

import (
	"net/http"
	"sync/atomic"

	"github.com/gin-gonic/gin"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/vincent119/zlogger"
	"go.opentelemetry.io/otel/trace"

	"github.com/vincent119/ReleaseHub/Server/internal/observability"
	contract "github.com/vincent119/ReleaseHub/Server/internal/transport/openapi"
)

const requestIDHeader = "X-Request-ID"

const (
	sessionCookieName = "releasehub_session"
	csrfCookieName    = "releasehub_csrf"
	loginCookieName   = "releasehub_login"
)

// RouterOptions contains transport infrastructure supplied by the composition root.
type RouterOptions struct {
	Logger         *zlogger.Logger
	Registry       *prometheus.Registry
	HTTPMetrics    *observability.HTTPMetrics
	TracerProvider trace.TracerProvider
	Readiness      *Readiness
	MetricsPath    string
}

// Readiness tracks whether the process accepts traffic.
type Readiness struct {
	ready atomic.Bool
}

func NewReadiness() *Readiness {
	readiness := &Readiness{}
	readiness.ready.Store(true)
	return readiness
}

// BeginShutdown changes readiness before the process stops accepting new traffic.
func (r *Readiness) BeginShutdown() {
	r.ready.Store(false)
}

func (r *Readiness) isReady() bool {
	return r.ready.Load()
}

// NewRouter composes transport infrastructure with an injected API contract handler.
func NewRouter(options RouterOptions, handler contract.ServerInterface) http.Handler {
	router := newEngine(options)
	registerProbes(router, options)
	registerContract(router, handler)
	return router
}

func newEngine(options RouterOptions) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	router := gin.New()
	router.Use(requestObserver(options), gin.Recovery())
	return router
}

func registerProbes(router *gin.Engine, options RouterOptions) {
	router.GET("/healthz", healthHandler)
	router.GET("/readyz", readinessHandler(options.Readiness))
	router.GET(options.MetricsPath, gin.WrapH(promhttp.HandlerFor(options.Registry, promhttp.HandlerOpts{})))
}

func healthHandler(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}

func readinessHandler(readiness *Readiness) gin.HandlerFunc {
	return func(c *gin.Context) {
		if readiness.isReady() {
			c.JSON(http.StatusOK, gin.H{"status": "ready"})
			return
		}
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "not_ready"})
	}
}

func registerContract(router *gin.Engine, handler contract.ServerInterface) {
	contract.RegisterHandlersWithOptions(router, handler, contract.GinServerOptions{
		ErrorHandler: contractErrorHandler,
	})
}

func contractErrorHandler(c *gin.Context, _ error, status int) {
	respondError(c, status, "INVALID_REQUEST", "Request parameters are invalid")
}
