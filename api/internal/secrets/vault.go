package secrets

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	vault "github.com/hashicorp/vault/api"
)

// vaultProvider serves secrets from a HashiCorp Vault KV v2 secret read once at
// startup. Keys present in the secret win; anything else falls back to the
// environment, so non-secret config can still come from env under this backend.
type vaultProvider struct {
	data map[string]string
}

func (v *vaultProvider) Get(key string) (string, error) {
	if val, ok := v.data[key]; ok {
		return val, nil
	}
	return os.Getenv(key), nil
}

// newVaultProvider connects to Vault (VAULT_ADDR), authenticates by token or
// AppRole, and reads the KV v2 secret at VAULT_KV_MOUNT/VAULT_KV_PATH.
func newVaultProvider() (Provider, error) {
	addr := os.Getenv("VAULT_ADDR")
	if addr == "" {
		return nil, errors.New("SECRETS_BACKEND=vault requires VAULT_ADDR")
	}
	cfg := vault.DefaultConfig()
	cfg.Address = addr
	client, err := vault.NewClient(cfg)
	if err != nil {
		return nil, fmt.Errorf("vault client: %w", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	switch {
	case os.Getenv("VAULT_TOKEN") != "":
		client.SetToken(os.Getenv("VAULT_TOKEN"))
	case os.Getenv("VAULT_ROLE_ID") != "":
		login, err := client.Logical().WriteWithContext(ctx, "auth/approle/login", map[string]any{
			"role_id":   os.Getenv("VAULT_ROLE_ID"),
			"secret_id": os.Getenv("VAULT_SECRET_ID"),
		})
		if err != nil {
			return nil, fmt.Errorf("vault AppRole login: %w", err)
		}
		if login == nil || login.Auth == nil || login.Auth.ClientToken == "" {
			return nil, errors.New("vault AppRole login returned no token")
		}
		client.SetToken(login.Auth.ClientToken)
	default:
		return nil, errors.New("vault: set VAULT_TOKEN or VAULT_ROLE_ID (+ VAULT_SECRET_ID)")
	}

	mount := os.Getenv("VAULT_KV_MOUNT")
	if mount == "" {
		mount = "secret"
	}
	path := os.Getenv("VAULT_KV_PATH")
	if path == "" {
		return nil, errors.New("vault: set VAULT_KV_PATH (KV v2 path holding the secrets)")
	}

	sec, err := client.KVv2(mount).Get(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("vault read %s/%s: %w", mount, path, err)
	}
	data := make(map[string]string, len(sec.Data))
	for k, val := range sec.Data {
		if s, ok := val.(string); ok {
			data[k] = s
		}
	}
	return &vaultProvider{data: data}, nil
}
