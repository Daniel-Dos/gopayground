package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Daniel-Dos/gopayground/internal/models"
	"github.com/Daniel-Dos/gopayground/internal/producer"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseFlags_Defaults(t *testing.T) {
	f, err := parsePublishFlags(nil)
	require.NoError(t, err)

	assert.Equal(t, "confirmed", f.status)
	assert.Equal(t, 100.00, f.amount)
	assert.Equal(t, "BRL", f.currency)
	assert.Equal(t, "payment.events", f.topic)
	assert.Equal(t, "localhost:9092", f.brokers)
	assert.Equal(t, 1, f.count)
	assert.Equal(t, 0, f.rate)
	assert.False(t, f.dryRun)
	assert.False(t, f.jsonOutput)
}

func TestParseFlags_Custom(t *testing.T) {
	f, err := parsePublishFlags([]string{
		"--payment-id", "abc-123",
		"--status", "failed",
		"--amount", "50.00",
		"--currency", "USD",
		"--description", "test",
		"--topic", "custom-topic",
		"--brokers", "kafka:9092,kafka:9093",
		"--count", "10",
		"--rate", "5",
		"--dry-run",
		"--json-output",
	})
	require.NoError(t, err)

	assert.Equal(t, "abc-123", f.paymentID)
	assert.Equal(t, "failed", f.status)
	assert.Equal(t, 50.00, f.amount)
	assert.Equal(t, "USD", f.currency)
	assert.Equal(t, "test", f.description)
	assert.Equal(t, "custom-topic", f.topic)
	assert.Equal(t, "kafka:9092,kafka:9093", f.brokers)
	assert.Equal(t, 10, f.count)
	assert.Equal(t, 5, f.rate)
	assert.True(t, f.dryRun)
	assert.True(t, f.jsonOutput)
}

func TestParseFlags_WithPublishSubcommand(t *testing.T) {
	// Simula exatamente como o docker-compose chama: command: ["publish", ...]
	// parsePublishFlags recebe apenas os args após o subcomando
	f, err := parsePublishFlags([]string{
		"--count", "10",
		"--rate", "2",
		"--brokers", "kafka:9092",
	})
	require.NoError(t, err)

	assert.Equal(t, "kafka:9092", f.brokers, "--brokers deve vir do arg, não do default localhost")
	assert.Equal(t, 10, f.count)
	assert.Equal(t, 2, f.rate)
	assert.Equal(t, "confirmed", f.status) // default
}

func TestGetEvents_Default(t *testing.T) {
	f := flags{count: 1, status: "confirmed", amount: 100, currency: "BRL"}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.NotEmpty(t, events[0].PaymentID)
	assert.Equal(t, "confirmed", events[0].Status)
	assert.Equal(t, 100.00, events[0].Amount)
}

func TestGetEvents_Payload(t *testing.T) {
	f := flags{payload: `{"payment_id":"abc-123","status":"confirmed","amount":50,"currency":"BRL","timestamp":"2026-05-24T10:00:00Z"}`}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "abc-123", events[0].PaymentID)
	assert.Equal(t, 50.00, events[0].Amount)
}

func TestGetEvents_PayloadArray(t *testing.T) {
	f := flags{payload: `[
		{"payment_id":"abc-123","status":"confirmed","amount":50,"currency":"BRL","timestamp":"2026-05-24T10:00:00Z"},
		{"payment_id":"def-456","status":"failed","amount":30,"currency":"USD","timestamp":"2026-05-24T10:00:00Z"}
	]`}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 2)
}

func TestGetEvents_PayloadInvalid(t *testing.T) {
	f := flags{payload: `invalid json`}
	_, err := getEvents(f)
	assert.Error(t, err)
}

func TestGetEvents_File(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "events.json")
	err := os.WriteFile(path, []byte(`[
		{"payment_id":"abc-123","status":"confirmed","amount":50,"currency":"BRL","timestamp":"2026-05-24T10:00:00Z"}
	]`), 0644)
	require.NoError(t, err)

	f := flags{file: path, count: 1}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestGetEvents_FileObject(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "event.json")
	err := os.WriteFile(path, []byte(`{"payment_id":"abc-123","status":"confirmed","amount":50,"currency":"BRL","timestamp":"2026-05-24T10:00:00Z"}`), 0644)
	require.NoError(t, err)

	f := flags{file: path, count: 1}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestGetEvents_FileNotFound(t *testing.T) {
	f := flags{file: "/nonexistent/events.json"}
	_, err := getEvents(f)
	assert.Error(t, err)
}

func TestGetEvents_FileTooLarge(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "large.json")
	largeData := make([]byte, maxFileSize+1)
	err := os.WriteFile(path, largeData, 0644)
	require.NoError(t, err)

	f := flags{file: path}
	_, err = getEvents(f)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "file too large")
}

func TestGetEvents_Bulk(t *testing.T) {
	f := flags{count: 50}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 50)
}

func TestGetEvents_BulkSingle(t *testing.T) {
	f := flags{count: 1}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 1)
}

func TestGetEvents_PayloadPriority(t *testing.T) {
	// --payload should take priority over --file and --count
	f := flags{
		payload: `{"payment_id":"p1","status":"confirmed","amount":50,"currency":"BRL","timestamp":"2026-05-24T10:00:00Z"}`,
		file:    "/some/file.json",
		count:   100,
	}
	events, err := getEvents(f)
	require.NoError(t, err)
	require.Len(t, events, 1)
	assert.Equal(t, "p1", events[0].PaymentID)
}

func TestPrintResults_NoError(t *testing.T) {
	results := []producer.Result{
		{
			Event:     &models.PaymentEvent{PaymentID: "abc-123"},
			Partition: 0,
			Offset:    42,
		},
	}

	// Should not panic or exit
	printResults(results, false)
	printResults(results, true)
}

func TestPrintEvents(t *testing.T) {
	events := []*models.PaymentEvent{
		{PaymentID: "abc-123", Status: "confirmed", Amount: 100, Currency: "BRL", Timestamp: "2026-05-24T10:00:00Z"},
	}

	printEvents(events, false)
	printEvents(events, true)
}
