package auth

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// Store is the on-disk representation of ~/.ruptor/config.yaml. It
// is a superset of the fields internal/config.Settings reads, but
// this package only cares about the token. A future PR may merge
// the two readers; for v1 they coexist because the auth code needs
// atomic token rotation and viper is not well-suited to partial
// rewrites.
type Store struct {
	Token string `yaml:"token"`
	// Preserve any other top-level keys so Save doesn't clobber
	// settings the user has customised manually.
	extras map[string]any
}

// DefaultConfigPath returns ~/.ruptor/config.yaml. Tests override by
// passing an explicit path to LoadFrom / SaveTo.
func DefaultConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("auth: home dir: %w", err)
	}
	return filepath.Join(home, ".ruptor", "config.yaml"), nil
}

// Load reads the default config path. A missing file returns
// ErrNotAuthenticated so the CLI can show a "run `ruptor auth login`"
// hint without inventing a generic not-found error.
func Load() (*Store, error) {
	path, err := DefaultConfigPath()
	if err != nil {
		return nil, err
	}
	return LoadFrom(path)
}

// LoadFrom reads a specific path. The 0600 permissions check is
// enforced here — a world-readable config is refused with
// ErrBadConfigPerms so secrets never linger in group-readable files.
func LoadFrom(path string) (*Store, error) {
	info, err := os.Stat(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, ErrNotAuthenticated
		}
		return nil, fmt.Errorf("auth: stat config: %w", err)
	}
	if err := checkPerms(info.Mode()); err != nil {
		return nil, err
	}

	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("auth: read config: %w", err)
	}
	return decodeStore(raw)
}

// Save writes to the default config path with 0600 perms. Uses a
// temp-file + rename so a crash mid-write cannot truncate the
// existing token.
func (s *Store) Save() error {
	path, err := DefaultConfigPath()
	if err != nil {
		return err
	}
	return s.SaveTo(path)
}

// SaveTo writes to an explicit path. Creates the parent directory
// with 0700 perms if missing.
func (s *Store) SaveTo(path string) error {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("auth: creating %s: %w", dir, err)
	}

	doc := s.toDocument()
	raw, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("auth: marshal config: %w", err)
	}

	tmp, err := os.CreateTemp(dir, ".config.*.yaml")
	if err != nil {
		return fmt.Errorf("auth: temp file: %w", err)
	}
	tmpPath := tmp.Name()
	if err := os.Chmod(tmpPath, 0o600); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("auth: chmod: %w", err)
	}
	if _, err := tmp.Write(raw); err != nil {
		tmp.Close()
		_ = os.Remove(tmpPath)
		return fmt.Errorf("auth: write: %w", err)
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("auth: close: %w", err)
	}
	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("auth: rename: %w", err)
	}
	return nil
}

// Clear removes the config file. Used by `ruptor auth logout`.
// Missing file is not an error — logout should be idempotent.
func Clear() error {
	path, err := DefaultConfigPath()
	if err != nil {
		return err
	}
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("auth: remove config: %w", err)
	}
	return nil
}

func decodeStore(raw []byte) (*Store, error) {
	var doc map[string]any
	if err := yaml.Unmarshal(raw, &doc); err != nil {
		return nil, fmt.Errorf("auth: unmarshal config: %w", err)
	}
	s := &Store{extras: make(map[string]any)}
	for k, v := range doc {
		if k == "token" {
			if token, ok := v.(string); ok {
				s.Token = token
			}
			continue
		}
		s.extras[k] = v
	}
	return s, nil
}

func (s *Store) toDocument() map[string]any {
	doc := make(map[string]any, len(s.extras)+1)
	for k, v := range s.extras {
		doc[k] = v
	}
	if s.Token != "" {
		doc["token"] = s.Token
	}
	return doc
}

// checkPerms rejects files more permissive than 0600.
func checkPerms(mode os.FileMode) error {
	if mode.Perm()&0o077 != 0 {
		return ErrBadConfigPerms
	}
	return nil
}
