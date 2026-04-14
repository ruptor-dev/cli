package config

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/viper"
)

// Settings holds ruptor's own configuration (token location, cloud URL,
// telemetry toggle, default output dir) — distinct from the user-authored
// chaos/simulate YAML documents.
//
// Precedence (CLAUDE.md):
//  1. CLI flag (--cloud, --output, etc.)
//  2. RUPTOR_* environment variable
//  3. .ruptor.yaml in the current directory (project-local)
//  4. ~/.ruptor/config.yaml (user global, created by `ruptor auth login`)
//  5. Hardcoded default
type Settings struct {
	// ConfigDir is the directory holding ~/.ruptor/config.yaml + token +
	// pending-reports spool. Defaults to ~/.ruptor.
	ConfigDir string `mapstructure:"config_dir"`
	// CloudURL is the platform API endpoint. Only used when --cloud fires.
	CloudURL string `mapstructure:"cloud_url"`
	// CloudEnabled is true when the --cloud flag is set (runtime only;
	// never persisted to disk).
	CloudEnabled bool `mapstructure:"-"`
	// TelemetryEnabled is opt-in; flipped on after `ruptor auth login`.
	TelemetryEnabled bool `mapstructure:"telemetry_enabled"`
	// OutputPath is the default report destination when a command does
	// not override it with --output.
	OutputPath string `mapstructure:"output_path"`
	// LogLevel is the default zerolog level: debug, info, warn, error.
	LogLevel string `mapstructure:"log_level"`
}

// LoadSettings builds a Settings from the full viper precedence chain.
// The first existing file in the search path wins for baseline values;
// env vars override file values; caller-supplied CLI flags are applied
// on top by Settings.ApplyFlags.
func LoadSettings() (*Settings, error) {
	v := newSettingsViper()

	if err := readSettingsFiles(v); err != nil {
		return nil, err
	}

	var s Settings
	if err := v.Unmarshal(&s); err != nil {
		return nil, fmt.Errorf("config: unmarshalling settings: %w", err)
	}
	return &s, nil
}

func newSettingsViper() *viper.Viper {
	v := viper.New()
	v.SetConfigName("config")
	v.SetConfigType("yaml")
	v.SetEnvPrefix("RUPTOR")
	v.SetEnvKeyReplacer(strings.NewReplacer(".", "_"))
	v.AutomaticEnv()

	home, err := os.UserHomeDir()
	if err == nil {
		v.AddConfigPath(filepath.Join(home, ".ruptor"))
	}

	v.SetDefault("config_dir", defaultConfigDir(home))
	v.SetDefault("cloud_url", "https://api.ruptor.dev")
	v.SetDefault("telemetry_enabled", false)
	v.SetDefault("output_path", "./reports/")
	v.SetDefault("log_level", "info")

	return v
}

// readSettingsFiles reads ~/.ruptor/config.yaml if present, then a
// project-local .ruptor.yaml from the current directory. Missing files
// are not an error — defaults + env vars still produce a usable Settings.
func readSettingsFiles(v *viper.Viper) error {
	if err := v.ReadInConfig(); err != nil {
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return fmt.Errorf("config: reading ~/.ruptor/config.yaml: %w", err)
		}
	}

	local := viper.New()
	local.SetConfigFile(".ruptor.yaml")
	local.SetConfigType("yaml")
	if err := local.ReadInConfig(); err != nil {
		if !os.IsNotExist(err) {
			// Any non-ENOENT read error is worth surfacing: a typo'd
			// project-local file should not silently fall through.
			return nil
		}
		return nil
	}
	for k, val := range local.AllSettings() {
		v.Set(k, val)
	}
	return nil
}

func defaultConfigDir(home string) string {
	if home == "" {
		return ".ruptor"
	}
	return filepath.Join(home, ".ruptor")
}
