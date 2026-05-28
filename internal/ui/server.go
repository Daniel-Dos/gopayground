package ui

import (
	"context"
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"

	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/trace"
)

//go:embed static/*
var staticFiles embed.FS

// Server encapsula o servidor HTTP e suas dependências.
type Server struct {
	httpServer *http.Server
	eventBus   *EventBus
	handlers   *Handlers
	logger     *slog.Logger
	meter      metric.Meter

	// Métricas OTel para observabilidade da UI
	httpRequestsTotal   metric.Int64Counter
	httpRequestDuration metric.Float64Histogram
}

// NewServer cria um novo servidor UI com todas as dependências.
func NewServer(cfg config.Config, rdb *redis.Client, dynamoClient *dynamodb.Client, logger *slog.Logger, meter metric.Meter) *Server {
	eventBus := NewEventBus(rdb, "payment:events", cfg.UI.EventBusBuffer, logger)

	handlers := NewHandlers(rdb, dynamoClient, cfg.DynamoDB.Table, eventBus, cfg.UI.ProducerURL, logger)

	mux := http.NewServeMux()

	// Static files
	staticFS, err := fs.Sub(staticFiles, "static")
	if err != nil {
		logger.Error("failed to create static file system", "error", err)
		// Fallback: serve from root embed
		mux.Handle("GET /", http.FileServer(http.FS(staticFiles)))
	} else {
		mux.Handle("GET /", http.FileServer(http.FS(staticFS)))
	}

	// API routes
	mux.HandleFunc("GET /api/events", handlers.HandleSSE)
	mux.HandleFunc("GET /api/payments", handlers.HandleListPayments)
	mux.HandleFunc("GET /api/payments/{id}/history", handlers.HandlePaymentHistory)
	mux.HandleFunc("GET /api/metrics", handlers.HandleMetrics)
	mux.HandleFunc("GET /healthz", handlers.HandleHealth)

	// Producer API routes
	mux.HandleFunc("POST /api/publish", handlers.HandlePublish)
	mux.HandleFunc("POST /api/publish/bulk", handlers.HandlePublishBulk)

	// Página do produtor
	mux.HandleFunc("GET /producer", serveProducerPage)

	// Dashboard gráfico de métricas
	mux.HandleFunc("GET /dashboard", serveDashboardPage)

	s := &Server{
		httpServer: nil, // atribuído abaixo
		eventBus:   eventBus,
		handlers:   handlers,
		logger:     logger,
		meter:      meter,
	}

	s.initMetrics()

	httpServer := &http.Server{
		Addr:         ":" + cfg.UI.Port,
		Handler:      s.applyMiddleware(mux),
		ReadTimeout:  cfg.UI.ReadTimeout,
		WriteTimeout: 0, // SSE connections are long-lived; write timeout would close them prematurely
	}
	s.httpServer = httpServer

	return s
}

// initMetrics inicializa as métricas OTel para o servidor UI.
func (s *Server) initMetrics() {
	var err error

	s.httpRequestsTotal, err = s.meter.Int64Counter("payment.ui.http_requests_total",
		metric.WithDescription("Total HTTP requests processed by the UI"),
	)
	if err != nil {
		s.logger.Warn("failed to create http_requests_total counter", "error", err)
	}

	s.httpRequestDuration, err = s.meter.Float64Histogram("payment.ui.http_request_duration_ms",
		metric.WithDescription("HTTP request duration in milliseconds"),
		metric.WithExplicitBucketBoundaries(1, 5, 10, 25, 50, 100, 250, 500, 1000, 5000),
	)
	if err != nil {
		s.logger.Warn("failed to create http_request_duration histogram", "error", err)
	}
}

// Start inicia o servidor HTTP.
func (s *Server) Start() error {
	s.logger.Info("starting UI server", "port", s.httpServer.Addr)
	return s.httpServer.ListenAndServe()
}

// Shutdown desliga o servidor de forma graciosa.
func (s *Server) Shutdown(ctx context.Context) error {
	s.logger.Info("shutting down UI server")
	s.eventBus.Close()
	return s.httpServer.Shutdown(ctx)
}

// Handler retorna o handler HTTP para fins de teste.
func (s *Server) Handler() http.Handler {
	return s.httpServer.Handler
}

// ServeHTTP permite testar o servidor via httptest.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.httpServer.Handler.ServeHTTP(w, r)
}

// Stop realiza um desligamento forçado (para testes).
func (s *Server) Stop() {
	s.eventBus.Close()
	s.httpServer.Close()
}

// serveProducerPage serve a página HTML do produtor a partir dos arquivos embutidos.
func serveProducerPage(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/producer.html")
	if err != nil {
		http.Error(w, "producer page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// serveDashboardPage serve a página HTML do dashboard gráfico.
func serveDashboardPage(w http.ResponseWriter, r *http.Request) {
	data, err := staticFiles.ReadFile("static/dashboard.html")
	if err != nil {
		http.Error(w, "dashboard page not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = w.Write(data)
}

// applyMiddleware encadeia logging, headers de segurança, instrumentação OTel e recovery.
func (s *Server) applyMiddleware(next http.Handler) http.Handler {
	return recoveryMiddleware(
		securityHeadersMiddleware(
			s.otelMiddleware(
				loggingMiddleware(next, s.logger),
			),
		),
	)
}

// loggingMiddleware registra cada requisição HTTP no log.
func loggingMiddleware(next http.Handler, logger *slog.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(wrapped, r)
		logger.Info("request",
			"method", r.Method,
			"path", r.URL.Path,
			"status", wrapped.status,
			"duration", time.Since(start).String(),
		)
	})
}

// otelMiddleware instrumenta cada requisição HTTP com métricas e tracing OTel.
func (s *Server) otelMiddleware(next http.Handler) http.Handler {
	if s.meter == nil {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		wrapped := &responseWriter{ResponseWriter: w, status: http.StatusOK}

		// Create a span for this request
		spanName := r.Method + " " + r.URL.Path
		ctx, span := trace.SpanFromContext(r.Context()).TracerProvider().Tracer("payment-ui").Start(r.Context(), spanName,
			trace.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.target", r.URL.Path),
				attribute.String("http.host", r.Host),
			),
		)
		defer span.End()

		next.ServeHTTP(wrapped, r.WithContext(ctx))

		duration := time.Since(start).Milliseconds()

		// Record metrics
		s.httpRequestsTotal.Add(ctx, 1,
			metric.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
				attribute.Int("http.status_code", wrapped.status),
			),
		)

		s.httpRequestDuration.Record(ctx, float64(duration),
			metric.WithAttributes(
				attribute.String("http.method", r.Method),
				attribute.String("http.route", r.URL.Path),
				attribute.Int("http.status_code", wrapped.status),
			),
		)

		span.SetAttributes(attribute.Int("http.status_code", wrapped.status))
	})
}

// securityHeadersMiddleware adiciona headers HTTP relacionados à segurança.
func securityHeadersMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Content-Security-Policy",
			"default-src 'self'; "+
				"style-src 'self' https://fonts.googleapis.com 'unsafe-inline'; "+
				"font-src 'self' https://fonts.gstatic.com; "+
				"script-src 'self' 'unsafe-inline'; "+
				"img-src 'self' data:; "+
				"connect-src 'self'",
		)
		next.ServeHTTP(w, r)
	})
}

// recoveryMiddleware captura pânicos e retorna 500.
func recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				slog.Error("panic recovered", "error", rec, "path", r.URL.Path)
				http.Error(w, "internal server error", http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

// responseWriter wraps http.ResponseWriter to capture the status code.
type responseWriter struct {
	http.ResponseWriter
	status int
}

func (rw *responseWriter) WriteHeader(code int) {
	rw.status = code
	rw.ResponseWriter.WriteHeader(code)
}

func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
