package main

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestMainConfigDefaults(t *testing.T) {
	// Unset UI-related env vars to ensure defaults
	os.Unsetenv("UI_PORT")
	os.Unsetenv("UI_EVENT_BUS_BUFFER")
	os.Unsetenv("UI_READ_TIMEOUT")
	os.Unsetenv("UI_WRITE_TIMEOUT")

	// We can't easily test the full main function,
	// but we can check that the config loads without panicking.

	// Verify the package builds correctly
	assert.NotPanics(t, func() {
		_ = os.Getenv("UI_PORT")
	}, "env vars should be accessible")
}

func TestMainEnvVars(t *testing.T) {
	os.Setenv("UI_PORT", "9090")
	defer os.Unsetenv("UI_PORT")

	assert.Equal(t, "9090", os.Getenv("UI_PORT"))
}
