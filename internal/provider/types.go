// Package provider asks model provider APIs (Anthropic, and any OpenAI-compatible server) with a key
// the player saved, and runs the tool loop in process. It uses only the standard library for HTTP and SSE.
package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// Message is one turn of the conversation. An assistant message carries text and tool calls; a user
// message carries text, or the results of the assistant's calls.
type Message struct {
	Role    string // "user" or "assistant"
	Text    string
	Calls   []Call
	Results []Result
}

// Call is a tool call the model asked for.
type Call struct {
	ID, Name string
	Args     json.RawMessage
}

// Result is the answer to a Call.
type Result struct {
	CallID, Name, Text string
	IsError            bool
}

// ToolDef is a tool offered to the model.
type ToolDef struct {
	Name, Description string
	Schema            json.RawMessage // JSON Schema of the arguments
}

// Request is one model call.
type Request struct {
	Model    string
	System   string
	Messages []Message
	Tools    []ToolDef
	Stream   bool
}

// Usage counts tokens: In is fresh input (including tokens written to a cache), Cached the input read
// from a cache, Out the output.
type Usage struct{ In, Out, Cached int }

// Add sums two usages.
func (u Usage) Add(o Usage) Usage { return Usage{u.In + o.In, u.Out + o.Out, u.Cached + o.Cached} }

// Reply is what one model call returned.
type Reply struct {
	Text  string
	Calls []Call
	Usage Usage
}

// Adapter makes one model call; onText (may be nil) is told text as it streams.
type Adapter interface {
	Chat(ctx context.Context, req Request, onText func(string)) (Reply, error)
}

// Error kinds. StartError means the provider could not be used at all (no key, not reachable, key
// refused), so the next client in a fallback list may be tried. NoToolsError means the model cannot use tools.
type StartError struct{ Msg string }

func (e *StartError) Error() string { return e.Msg }

// ErrNoTools is the plain message shown when a model can't call tools.
const ErrNoTools = "This model can't use tools; pick another."

// NoToolsError is returned when the provider says the model does not support tools.
type NoToolsError struct{}

func (NoToolsError) Error() string { return ErrNoTools }

// IsStart reports whether err is a StartError.
func IsStart(err error) bool { var s *StartError; return errors.As(err, &s) }

// looksNoTools reports whether a provider's error message means the model has no tool support.
func looksNoTools(msg string) bool {
	m := strings.ToLower(msg)
	if !strings.Contains(m, "tool") {
		return false
	}
	for _, w := range []string{"not support", "does not support", "doesn't support", "unsupported", "no endpoints found", "not capable"} {
		if strings.Contains(m, w) {
			return true
		}
	}
	return false
}

// httpError turns a failed response into an error fit to show the player. The message is the
// provider's own; it never contains the request, so never the key. key, when non-empty, is scrubbed anyway.
func httpError(status int, body []byte, key string) error {
	msg := providerMessage(body)
	if key != "" {
		msg = strings.ReplaceAll(msg, key, "…")
	}
	switch {
	case looksNoTools(msg) && (status == 400 || status == 404 || status == 422):
		return NoToolsError{}
	case status == 401 || status == 403:
		return &StartError{fmt.Sprintf("The provider refused the key (%d): %s", status, msg)}
	}
	return fmt.Errorf("the provider answered %d: %s", status, msg)
}

// providerMessage digs the message out of the usual error shapes, else the first 200 bytes.
func providerMessage(body []byte) string {
	var e struct {
		Error json.RawMessage `json:"error"`
		Msg   string          `json:"message"`
	}
	if json.Unmarshal(body, &e) == nil {
		var o struct {
			Message string `json:"message"`
		}
		var s string
		switch {
		case json.Unmarshal(e.Error, &o) == nil && o.Message != "":
			return o.Message
		case json.Unmarshal(e.Error, &s) == nil && s != "":
			return s
		case e.Msg != "":
			return e.Msg
		}
	}
	s := strings.TrimSpace(string(body))
	if len(s) > 200 {
		s = s[:200] + "…"
	}
	if s == "" {
		return "no details"
	}
	return s
}
