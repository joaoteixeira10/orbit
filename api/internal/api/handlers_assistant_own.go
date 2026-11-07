package api

import (
	"context"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/joaoteixeira10/orbit/api/internal/assistant"
	"github.com/joaoteixeira10/orbit/api/internal/auth"
	"github.com/joaoteixeira10/orbit/api/internal/models"
	"github.com/joaoteixeira10/orbit/api/internal/safedial"
)

// A person's own model (Settings → Assistant): when the owner lets people
// use their own, each may set up theirs — their Claude or OpenAI key, their
// Ollama — and use it rather than the instance's. Theirs has no daily limit.
// Its address must be public and https, unless it's one the operator named
// in AI_SERVERS (a server on Orbit's own network).

// assistantPublicHTTP calls a person's own model: public addresses only.
// Tests swap it.
var assistantPublicHTTP = func() *http.Client {
	c := safedial.PublicClient(assistant.Timeout)
	c.Transport.(*http.Transport).ResponseHeaderTimeout = assistant.Timeout
	return c
}

// assistantFor is the model a person asks: their own when the owner lets
// people use their own and they chose theirs, else the instance's when it's
// on; ok is false when there's neither.
func (h *Handlers) assistantFor(uid string) (cfg assistant.Config, own, ok bool) {
	inst := assistant.Load(h.db, h.sealer)
	if inst.AllowOwn {
		var o models.AssistantOwn
		if h.db.First(&o, "user_id = ?", uid).Error == nil && o.InUse {
			mine := h.ownConfig(o)
			if _, err := mine.New(http.DefaultClient); err == nil {
				return mine, true, true
			}
		}
	}
	_, err := inst.New(http.DefaultClient)
	return inst, false, inst.Enabled && err == nil
}

func (h *Handlers) ownConfig(o models.AssistantOwn) assistant.Config {
	cfg := assistant.Config{Enabled: true, Provider: o.Provider, Model: o.Model, URL: o.URL, Context: o.Context}
	if len(o.KeySealed) > 0 {
		if plain, err := h.sealer.Unseal(o.KeySealed); err == nil {
			cfg.Key = string(plain)
		}
	}
	if cfg.Model == "" {
		cfg.Model = assistant.DefaultModel(cfg.Provider)
	}
	return cfg
}

// assistantClient calls the model: the instance's may be on the local
// network; a person's own only public, or a server in AI_SERVERS.
func (h *Handlers) assistantClient(cfg assistant.Config, own bool) *http.Client {
	switch {
	case !own:
		return assistantHTTP()
	case h.aiServer(cfg.URL):
		return &http.Client{Timeout: assistant.Timeout, CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	}
	return assistantPublicHTTP()
}

// origin is an address's scheme://host[:port], lower-case.
func origin(raw string) string {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Host == "" {
		return ""
	}
	return strings.ToLower(u.Scheme + "://" + u.Host)
}

// aiServer tells whether an address is on a server named in AI_SERVERS.
func (h *Handlers) aiServer(raw string) bool {
	o := origin(raw)
	for _, s := range h.cfg.AIServers {
		if o != "" && origin(s) == o {
			return true
		}
	}
	return false
}

// publicAddress tells why a person's own address can't be used, if it
// can't: plain http, or a private or local address.
func publicAddress(raw string) string {
	u, _ := url.Parse(raw)
	if u.Scheme != "https" {
		return "the address must use https (or be one Orbit's operator allows, in AI_SERVERS)"
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	ips, err := net.DefaultResolver.LookupIP(ctx, "ip", u.Hostname())
	if err != nil {
		return "there's no server at that address"
	}
	for _, ip := range ips {
		if safedial.Blocked(ip) || safedial.Private(ip) {
			return "that address is on a private network; Orbit's operator can allow it (AI_SERVERS)"
		}
	}
	return ""
}

type assistantOwnResponse struct {
	Allowed  bool   `json:"allowed"`  // the owner lets people use their own
	Instance string `json:"instance"` // the instance's model, if it has one on
	Use      bool   `json:"use"`      // theirs, rather than the instance's
	Provider string `json:"provider"`
	Model    string `json:"model"`
	URL      string `json:"url"`
	HasKey   bool   `json:"has_key"` // the key itself is never sent back
	Context  int    `json:"context"`
	// Servers: the ones they may use on Orbit's network (AI_SERVERS).
	Servers []string `json:"servers"`
}

type assistantOwnRequest struct {
	Use      bool   `json:"use"`
	Provider string `json:"provider" binding:"omitempty,oneof=anthropic openai ollama"`
	Model    string `json:"model" binding:"max=120"`
	URL      string `json:"url" binding:"max=300"`
	Key      string `json:"key" binding:"max=500"` // "" keeps the one there is
	ClearKey bool   `json:"clear_key"`
	Context  int    `json:"context" binding:"omitempty,min=2048,max=1048576"`
}

// getAssistantOwn returns my own model for the assistant.
//
//	@Summary	My own model for the assistant
//	@Tags		assistant
//	@Produce	json
//	@Security	BearerAuth
//	@Success	200	{object}	assistantOwnResponse
//	@Router		/me/assistant [get]
func (h *Handlers) getAssistantOwn(c *gin.Context) {
	inst := assistant.Load(h.db, h.sealer)
	var o models.AssistantOwn
	h.db.First(&o, "user_id = ?", auth.UserID(c))
	out := assistantOwnResponse{
		Allowed: inst.AllowOwn, Use: o.InUse, Provider: o.Provider, Model: o.Model, URL: o.URL,
		HasKey: len(o.KeySealed) > 0, Context: o.Context, Servers: append([]string{}, h.cfg.AIServers...),
	}
	if _, err := inst.New(http.DefaultClient); inst.Enabled && err == nil {
		out.Instance = inst.Model
	}
	c.JSON(http.StatusOK, out)
}

// setAssistantOwn sets up my own model, and whether I use it (when the
// owner lets people use their own).
//
//	@Summary	Set up my own model for the assistant
//	@Tags		assistant
//	@Accept		json
//	@Produce	json
//	@Security	BearerAuth
//	@Param		body	body		assistantOwnRequest	true	"My model"
//	@Success	200		{object}	assistantOwnResponse
//	@Router		/me/assistant [put]
func (h *Handlers) setAssistantOwn(c *gin.Context) {
	var req assistantOwnRequest
	if !bindJSON(c, &req) {
		return
	}
	if !assistant.Load(h.db, h.sealer).AllowOwn {
		c.JSON(http.StatusForbidden, gin.H{"error": "this Orbit's owner hasn't let people use their own model"})
		return
	}
	uid := auth.UserID(c)
	var o models.AssistantOwn
	h.db.First(&o, "user_id = ?", uid)
	o.UserID = uid
	if req.Provider == "" {
		req.Provider = assistant.Anthropic
	}
	if req.Provider != o.Provider && !req.ClearKey && strings.TrimSpace(req.Key) == "" {
		o.KeySealed = nil // a key is for the provider it was given for
	}
	o.InUse, o.Provider, o.Model, o.Context = req.Use, req.Provider, strings.TrimSpace(req.Model), req.Context
	o.URL = strings.TrimRight(strings.TrimSpace(req.URL), "/")
	if o.URL == "" && o.Provider == assistant.OpenAI {
		o.URL = "https://api.openai.com/v1"
	}
	if o.Provider == assistant.Anthropic {
		o.URL = ""
	}
	if o.URL != "" {
		if u, err := url.Parse(o.URL); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.RawQuery != "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "the address must be an http(s) address, with no query"})
			return
		}
		if !h.aiServer(o.URL) {
			if why := publicAddress(o.URL); why != "" {
				c.JSON(http.StatusBadRequest, gin.H{"error": why})
				return
			}
		}
		if o.Provider == assistant.Ollama {
			o.URL = assistant.OllamaRoot(o.URL)
		}
	}
	if o.Model == "" {
		o.Model = assistant.DefaultModel(o.Provider)
	}
	switch {
	case req.ClearKey:
		o.KeySealed = nil
	case strings.TrimSpace(req.Key) != "":
		sealed, err := h.sealer.Seal([]byte(strings.TrimSpace(req.Key)))
		if err != nil {
			h.internalError(c, err, "could not keep the key")
			return
		}
		o.KeySealed = sealed
	}
	if o.InUse {
		switch {
		case o.Provider == assistant.Anthropic && len(o.KeySealed) == 0:
			c.JSON(http.StatusBadRequest, gin.H{"error": "Claude needs an API key"})
			return
		case o.Provider != assistant.Anthropic && o.URL == "":
			c.JSON(http.StatusBadRequest, gin.H{"error": "give the server's address"})
			return
		case o.Model == "":
			c.JSON(http.StatusBadRequest, gin.H{"error": "name the model"})
			return
		}
	}
	o.UpdatedAt = time.Now()
	if err := h.db.Save(&o).Error; err != nil {
		h.internalError(c, err, "could not save it")
		return
	}
	h.recordCtx(c, "assistant.own", "user", uid, gin.H{"use": o.InUse, "provider": o.Provider, "model": o.Model, "key_changed": strings.TrimSpace(req.Key) != "" || req.ClearKey})
	h.getAssistantOwn(c)
}
