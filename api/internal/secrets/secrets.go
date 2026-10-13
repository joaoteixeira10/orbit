// Package secrets resolves sensitive configuration values from a pluggable
// backend selected by SECRETS_BACKEND: env (default), file (Docker/K8s-style
// *_FILE), or vault (HashiCorp Vault KV v2). Non-secret config stays in the
// environment; only credentials/keys flow through a Provider.
package secrets

import (
	"fmt"
	"os"
	"strings"
)

// Provider resolves a logical config key (e.g. "ORBIT_MASTER_KEY") to its value.
// A missing value returns ("", nil) so callers apply their own defaults /
// required-ness; a non-nil error means the backend itself failed (e.g. Vault
// unreachable) and startup should abort.
type Provider interface {
	Get(key string) (string, error)
}

// New builds the Provider for the given backend ("" = env).
func New(backend string) (Provider, error) {
	switch backend {
	case "", "env":
		return envProvider{}, nil
	case "file":
		return fileProvider{}, nil
	case "vault":
		return newVaultProvider()
	default:
		return nil, fmt.Errorf("unsupported SECRETS_BACKEND %q (want env|file|vault)", backend)
	}
}

// envProvider reads values straight from the environment.
type envProvider struct{}

func (envProvider) Get(key string) (string, error) { return os.Getenv(key), nil }

// fileProvider reads a value from the file named by "<KEY>_FILE" (the
// Docker/Kubernetes secrets convention), falling back to the plain env var.
type fileProvider struct{}

func (fileProvider) Get(key string) (string, error) {
	if path := os.Getenv(key + "_FILE"); path != "" {
		b, err := os.ReadFile(path) //nolint:gosec // operator-set secret file path (Docker/K8s secrets), not user input
		if err != nil {
			return "", fmt.Errorf("read %s_FILE (%s): %w", key, path, err)
		}
		return strings.TrimSpace(string(b)), nil
	}
	return os.Getenv(key), nil
}
