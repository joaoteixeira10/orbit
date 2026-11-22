package secrets

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// infisicalProvider serves secrets from Infisical (the cloud, or one of your
// own), read once at startup: a project's environment, at a folder, signed
// in as a machine identity (Universal Auth: a client id and secret) or with
// an access token of one. Keys there win; anything else falls back to the
// environment, as with Vault. Secrets imported into that folder count too
// (the folder's own win over imported ones).
type infisicalProvider struct {
	data map[string]string
}

func (p *infisicalProvider) Get(key string) (string, error) {
	if val, ok := p.data[key]; ok {
		return val, nil
	}
	return os.Getenv(key), nil
}

// infisicalHTTP is the client for Infisical (tests swap it).
var infisicalHTTP = &http.Client{Timeout: 15 * time.Second}

func newInfisicalProvider() (Provider, error) {
	base := strings.TrimRight(os.Getenv("INFISICAL_URL"), "/")
	if base == "" {
		base = "https://app.infisical.com"
	}
	project := os.Getenv("INFISICAL_PROJECT_ID")
	if project == "" {
		return nil, errors.New("SECRETS_BACKEND=infisical requires INFISICAL_PROJECT_ID")
	}
	env := os.Getenv("INFISICAL_ENVIRONMENT")
	if env == "" {
		env = "prod"
	}
	folder := os.Getenv("INFISICAL_SECRET_PATH")
	if folder == "" {
		folder = "/"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()

	token := fileOrEnv("INFISICAL_TOKEN")
	if token == "" {
		id, secret := fileOrEnv("INFISICAL_CLIENT_ID"), fileOrEnv("INFISICAL_CLIENT_SECRET")
		if id == "" || secret == "" {
			return nil, errors.New("infisical: set INFISICAL_CLIENT_ID and INFISICAL_CLIENT_SECRET (a machine identity's Universal Auth), or INFISICAL_TOKEN")
		}
		var login struct {
			AccessToken string `json:"accessToken"`
		}
		if err := infisicalCall(ctx, http.MethodPost, base+"/api/v1/auth/universal-auth/login", "", map[string]string{"clientId": id, "clientSecret": secret}, &login); err != nil {
			return nil, fmt.Errorf("infisical sign-in: %w", err)
		}
		if login.AccessToken == "" {
			return nil, errors.New("infisical sign-in returned no token")
		}
		token = login.AccessToken
	}

	q := url.Values{"workspaceId": {project}, "environment": {env}, "secretPath": {folder}, "include_imports": {"true"}, "expandSecretReferences": {"true"}}
	type secret struct {
		Key   string `json:"secretKey"`
		Value string `json:"secretValue"`
	}
	var got struct {
		Secrets []secret `json:"secrets"`
		Imports []struct {
			Secrets []secret `json:"secrets"`
		} `json:"imports"`
	}
	if err := infisicalCall(ctx, http.MethodGet, base+"/api/v3/secrets/raw?"+q.Encode(), token, nil, &got); err != nil {
		return nil, fmt.Errorf("infisical read %s %s%s: %w", project, env, folder, err)
	}
	data := map[string]string{}
	for _, im := range got.Imports {
		for _, s := range im.Secrets {
			data[s.Key] = s.Value
		}
	}
	for _, s := range got.Secrets {
		data[s.Key] = s.Value // the folder's own win
	}
	return &infisicalProvider{data: data}, nil
}

// infisicalCall sends one request (JSON in and out); a refusal says
// Infisical's own message.
func infisicalCall(ctx context.Context, method, addr, token string, in, out any) error {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, addr, body)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	res, err := infisicalHTTP.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = res.Body.Close() }()
	raw, _ := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if res.StatusCode < 200 || res.StatusCode > 299 {
		var e struct {
			Message string `json:"message"`
		}
		if json.Unmarshal(raw, &e) == nil && e.Message != "" {
			return fmt.Errorf("%s: %s", res.Status, e.Message)
		}
		return errors.New(res.Status)
	}
	return json.Unmarshal(raw, out)
}

// fileOrEnv is a variable's value, or the contents of the file its _FILE
// variant names (how the credentials to Infisical itself stay out of the
// environment).
func fileOrEnv(key string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	if f := os.Getenv(key + "_FILE"); f != "" {
		if b, err := os.ReadFile(f); err == nil { //nolint:gosec // the operator's own file
			return strings.TrimRight(string(b), "\r\n")
		}
	}
	return ""
}
