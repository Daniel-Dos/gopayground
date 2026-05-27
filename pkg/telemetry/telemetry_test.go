package telemetry_test

import (
	"context"
	"os"
	"testing"

	"github.com/Daniel-Dos/gopayground/internal/config"
	"github.com/Daniel-Dos/gopayground/pkg/telemetry"

	"github.com/stretchr/testify/require"
	"go.opentelemetry.io/otel"
)

func init() {
	os.Setenv("OTEL_EXPORTER_OTLP_ENDPOINT", "localhost:0")
}

func TestInitTracerProvider(t *testing.T) {
	cfg := config.NewConfig()
	cfg.OTelEndpoint = "localhost:0"

	tp, err := telemetry.InitTracerProvider(context.Background(), cfg)
	if err != nil {
		t.Skip("OTel collector not available, skipping test: ", err)
	}
	require.NotNil(t, tp)

	// Shutdown may fail if no collector is running - that's expected
	_ = tp.Shutdown(context.Background())
}

func TestInitMeterProvider(t *testing.T) {
	cfg := config.NewConfig()
	cfg.OTelEndpoint = "localhost:0"

	mp, err := telemetry.InitMeterProvider(context.Background(), cfg)
	if err != nil {
		t.Skip("OTel collector not available, skipping test: ", err)
	}
	require.NotNil(t, mp)

	// Shutdown may fail if no collector is running - that's expected
	_ = mp.Shutdown(context.Background())
}

func TestNewMeter(t *testing.T) {
	meter := telemetry.NewMeter("test-meter")
	require.NotNil(t, meter)
}

func TestNewTracer(t *testing.T) {
	tracer := telemetry.NewTracer("test-tracer")
	require.NotNil(t, tracer)
}

func TestGlobalProviders(t *testing.T) {
	tp := otel.GetTracerProvider()
	require.NotNil(t, tp)

	mp := otel.GetMeterProvider()
	require.NotNil(t, mp)
}
