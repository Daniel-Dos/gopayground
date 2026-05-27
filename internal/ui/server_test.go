package ui_test

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Daniel-Dos/gopayground/internal/config"
	"github.com/Daniel-Dos/gopayground/internal/ui"

	"github.com/alicebob/miniredis/v2"
	"github.com/aws/aws-sdk-go-v2/service/dynamodb"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func newTestServer(t *testing.T) (*ui.Server, *miniredis.Miniredis) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { client.Close() })

	logger := slog.New(slog.NewTextHandler(discardWriter{}, nil))

	cfg := config.NewConfig()
	cfg.UIPort = "0"
	cfg.UIEventBusBuffer = 64
	cfg.UIReadTimeout = 10 * time.Second
	cfg.UIWriteTimeout = 30 * time.Second
	cfg.RedisAddr = mr.Addr()

	meter := otel.Meter("test")
	server := ui.NewServer(cfg, client, &dynamodb.Client{}, nil, logger, meter)
	return server, mr
}

func TestServer_RootRoute(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Stop()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, strings.Contains(w.Body.String(), "Monitor de Pagamentos"),
		"should serve index.html")
}

func TestServer_APIEndpoints(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Stop()

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
	}{
		{"root", "GET", "/", http.StatusOK},
		{"api payments", "GET", "/api/payments", http.StatusOK},
		{"api metrics", "GET", "/api/metrics", http.StatusOK},
		{"healthz", "GET", "/healthz", http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			w := httptest.NewRecorder()
			server.ServeHTTP(w, req)
			assert.Equal(t, tt.wantStatus, w.Code, "path: %s", tt.path)
		})
	}
}

func TestServer_SecurityHeaders(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Stop()

	req := httptest.NewRequest("GET", "/api/payments", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, "nosniff", w.Header().Get("X-Content-Type-Options"))
	assert.Equal(t, "DENY", w.Header().Get("X-Frame-Options"))
	assert.Equal(t, "no-referrer", w.Header().Get("Referrer-Policy"))
}

func TestServer_NotFound(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Stop()

	req := httptest.NewRequest("GET", "/unknown-path", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServer_APINotFound(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Stop()

	req := httptest.NewRequest("GET", "/api/unknown", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusNotFound, w.Code)
}

func TestServer_ServesHTML(t *testing.T) {
	server, _ := newTestServer(t)
	defer server.Stop()

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()
	server.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	contentType := w.Header().Get("Content-Type")
	assert.True(t, strings.Contains(contentType, "text/html"),
		"should have HTML content type, got: %s", contentType)
}
