// Package assistant is Orbit's built-in assistant: a model — Claude, or one
// behind an OpenAI-style API (OpenAI, Ollama, LM Studio, vLLM…) — that
// answers with Orbit's own tools (the MCP server's), each run as the person
// asking. The package knows the models; the tools and who runs them are the
// caller's (package api).
package assistant

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Tool is one of Orbit's tools as a model sees it: its name, what it does
// and its JSON Schema.
type Tool struct {
	Name        string
	Description string
	Schema      map[string]any
}

// Turn is a line of the conversation as shown: the person's or the
// assistant's text (earlier turns are sent as text only, so a conversation
// carries over from one model to another).
type Turn struct {
	Role string `json:"role"` // user | assistant
	Text string `json:"text"`
	// Steps: the tools the assistant used for this answer, in order.
	Steps []Step `json:"steps,omitempty"`
	At    string `json:"at,omitempty"`
}

// Step is a tool the assistant used: its name and the gist of what for.
type Step struct {
	Tool string `json:"tool"`
	Hint string `json:"hint,omitempty"`
}

// Request is one answer to work out.
type Request struct {
	// System is what the model is told first: Orbit, the person, the rules.
	System string
	// Context is what changes with every message (now, what's on screen),
	// kept apart so the rest can be cached.
	Context string
	History []Turn // ends with the person's message
	Tools   []Tool
	// Call runs a tool as the person; the text goes back to the model.
	Call func(ctx context.Context, name string, args map[string]any) (string, bool)
	// Step says a tool is about to run (for the panel).
	Step func(Step)
	// Delta passes on the answer's words as the model writes them. Words
	// written before a tool runs (Step) aren't the answer: the panel drops them.
	Delta func(string)
}

// Provider answers a Request, running tools as the model asks.
type Provider interface {
	Answer(ctx context.Context, r Request) (string, error)
}

// MaxRounds caps the tool calls one answer may make (a model going round in
// circles stops here and says so).
const MaxRounds = 12

// maxResult is how much of a tool's answer the model gets.
const maxResult = 24000

var (
	ErrRefused  = errors.New("the model declined to answer this")
	ErrTooLong  = errors.New("the answer was cut off: it ran too long")
	ErrTooMany  = errors.New("it took too many steps; ask for something narrower")
	ErrNoConfig = errors.New("the assistant isn't set up")
)

// Kinds of provider.
const (
	Anthropic = "anthropic"
	OpenAI    = "openai" // OpenAI, or any server speaking its API
	Ollama    = "ollama"
)

// New makes the provider a Config describes; client makes its HTTP calls
// (the owner's addresses may be on the local network, never loopback).
func (c Config) New(client *http.Client) (Provider, error) {
	switch c.Provider {
	case Anthropic:
		if c.Key == "" {
			return nil, ErrNoConfig
		}
		return newClaude(c, client), nil
	case OpenAI, Ollama:
		if c.URL == "" || c.Model == "" {
			return nil, ErrNoConfig
		}
		base := strings.TrimRight(c.URL, "/")
		if c.Provider == Ollama {
			base = OllamaBase(base)
		}
		return &openAI{base: base, key: c.Key, model: c.Model, http: client}, nil
	}
	return nil, ErrNoConfig
}

// OllamaBase gives Ollama's OpenAI-style API for its bare address:
// http://host:11434 → http://host:11434/v1. An address with a path is kept.
func OllamaBase(addr string) string {
	addr = strings.TrimRight(addr, "/")
	if u, err := url.Parse(addr); err == nil && u.Host != "" && u.Path == "" {
		return addr + "/v1"
	}
	return addr
}

// Timeout is how long one call to a model may take (thinking included).
const Timeout = 5 * time.Minute

func clipResult(s string) string {
	if r := []rune(s); len(r) > maxResult {
		return string(r[:maxResult]) + "\n…(cut here; ask for less)"
	}
	return s
}

// hint is the gist of a tool call for the panel: what it searched, read or
// looked at, if it says.
func hint(args map[string]any) string {
	for _, k := range []string{"query", "title", "name", "date", "from"} {
		if s, ok := args[k].(string); ok && s != "" {
			if r := []rune(s); len(r) > 60 {
				s = string(r[:60]) + "…"
			}
			return s
		}
	}
	return ""
}
