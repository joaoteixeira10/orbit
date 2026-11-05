package assistant

import (
	"encoding/base64"
	"strconv"
	"strings"
	"time"

	"gitea.ubonito.org/jteixeira/orbit/api/internal/keystore"
	"gitea.ubonito.org/jteixeira/orbit/api/internal/models"

	"gorm.io/gorm"
)

// Config is the instance's assistant (Owner → Assistant).
type Config struct {
	Enabled  bool
	Provider string // anthropic | openai | ollama
	Model    string
	URL      string // the OpenAI-style API's base (…/v1), Ollama's address; Claude needs none
	Key      string // the API key (sealed at rest); Ollama needs none
	// DailyLimit: messages a person may send a day (0: no limit).
	DailyLimit int
	// Context: how much an Ollama model reads of a request, in tokens
	// (0: DefaultContext).
	Context int
}

const (
	keyEnabled = "assistant_enabled"
	keyKind    = "assistant_provider"
	keyModel   = "assistant_model"
	keyURL     = "assistant_url"
	keyKey     = "assistant_key"
	keyLimit   = "assistant_daily_limit"
	keyContext = "assistant_context"
)

// DefaultModel is the model a provider starts with (Claude's current Opus;
// the others have none, the owner names theirs).
func DefaultModel(provider string) string {
	if provider == Anthropic {
		return "claude-opus-5-5"
	}
	return ""
}

// Load reads the instance's assistant.
func Load(db *gorm.DB, sealer *keystore.Sealer) Config {
	var rows []models.Setting
	for _, k := range []string{keyEnabled, keyKind, keyModel, keyURL, keyKey, keyLimit, keyContext} {
		var r models.Setting
		if db.Where(&models.Setting{Key: k}).First(&r).Error == nil {
			rows = append(rows, r)
		}
	}
	var c Config
	for _, r := range rows {
		switch r.Key {
		case keyEnabled:
			c.Enabled = r.Value == "true"
		case keyKind:
			c.Provider = r.Value
		case keyModel:
			c.Model = r.Value
		case keyURL:
			c.URL = r.Value
		case keyLimit:
			c.DailyLimit, _ = strconv.Atoi(r.Value)
		case keyContext:
			c.Context, _ = strconv.Atoi(r.Value)
		case keyKey:
			if raw, err := base64.StdEncoding.DecodeString(r.Value); err == nil {
				if plain, err := sealer.Unseal(raw); err == nil {
					c.Key = string(plain)
				}
			}
		}
	}
	if c.Model == "" {
		c.Model = DefaultModel(c.Provider)
	}
	return c
}

// Save writes it; an empty Key keeps the one there is, clearKey removes it.
func Save(db *gorm.DB, sealer *keystore.Sealer, c Config, clearKey bool) error {
	set := func(k, v string) error {
		return db.Save(&models.Setting{Key: k, Value: v, UpdatedAt: time.Now()}).Error
	}
	for k, v := range map[string]string{
		keyEnabled: strconv.FormatBool(c.Enabled),
		keyKind:    c.Provider,
		keyModel:   strings.TrimSpace(c.Model),
		keyURL:     strings.TrimRight(strings.TrimSpace(c.URL), "/"),
		keyLimit:   strconv.Itoa(c.DailyLimit),
		keyContext: strconv.Itoa(c.Context),
	} {
		if err := set(k, v); err != nil {
			return err
		}
	}
	if clearKey {
		return db.Where(&models.Setting{Key: keyKey}).Delete(&models.Setting{}).Error
	}
	if k := strings.TrimSpace(c.Key); k != "" {
		sealed, err := sealer.Seal([]byte(k))
		if err != nil {
			return err
		}
		return set(keyKey, base64.StdEncoding.EncodeToString(sealed))
	}
	return nil
}
