package config

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadSettings_Defaults(t *testing.T) {
	t.Setenv("HOME", t.TempDir())

	s, err := LoadSettings()
	require.NoError(t, err)

	assert.Equal(t, "https://api.ruptor.dev", s.CloudURL)
	assert.Equal(t, "./reports/", s.OutputPath)
	assert.Equal(t, "info", s.LogLevel)
	assert.False(t, s.TelemetryEnabled)
}

func TestLoadSettings_EnvOverride(t *testing.T) {
	t.Setenv("HOME", t.TempDir())
	t.Setenv("RUPTOR_LOG_LEVEL", "debug")
	t.Setenv("RUPTOR_OUTPUT_PATH", "/tmp/ruptor-reports/")
	t.Setenv("RUPTOR_TELEMETRY_ENABLED", "true")

	s, err := LoadSettings()
	require.NoError(t, err)

	assert.Equal(t, "debug", s.LogLevel)
	assert.Equal(t, "/tmp/ruptor-reports/", s.OutputPath)
	assert.True(t, s.TelemetryEnabled)
}

func TestApplyChaosEnvOverrides_ProxyPort(t *testing.T) {
	t.Setenv("RUPTOR_PROXY_PORT", "9999")
	cfg := &ChaosConfig{}
	applyChaosEnvOverrides(cfg)
	assert.Equal(t, 9999, cfg.Proxy.Port)
}

func TestApplyChaosEnvOverrides_IgnoresInvalidInt(t *testing.T) {
	t.Setenv("RUPTOR_PROXY_PORT", "not-a-port")
	cfg := &ChaosConfig{Proxy: ProxyConfig{Port: 8080}}
	applyChaosEnvOverrides(cfg)
	assert.Equal(t, 8080, cfg.Proxy.Port, "invalid env value must not clobber the YAML port")
}

func TestApplyChaosEnvOverrides_OutputPath(t *testing.T) {
	t.Setenv("RUPTOR_OUTPUT_PATH", "/custom/out/")
	cfg := &ChaosConfig{}
	applyChaosEnvOverrides(cfg)
	assert.Equal(t, "/custom/out/", cfg.Output.Path)
}
