package ask

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/provider"
)

type memKeys map[string]string

func (m memKeys) Get(p string) (string, error) {
	if k, ok := m[p]; ok {
		return k, nil
	}
	return "", provider.ErrNoKey
}
func (m memKeys) Set(p, k string) error { m[p] = k; return nil }
func (m memKeys) Delete(p string) error { delete(m, p); return nil }

type noIn struct{}

func fakeAdvisor() *mcp.Server {
	s := mcp.NewServer(&mcp.Implementation{Name: "wowc-advisor", Version: "t"}, nil)
	mcp.AddTool(s, &mcp.Tool{Name: "server_info", Description: "info"}, func(context.Context, *mcp.CallToolRequest, noIn) (*mcp.CallToolResult, map[string]string, error) {
		return nil, map[string]string{"fact": "42"}, nil
	})
	return s
}

// chatServer asks for server_info once, then answers with whatever the tool said.
func chatServer(t *testing.T, seen *[]string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var b struct {
			Messages []map[string]any `json:"messages"`
			Tools    []any            `json:"tools"`
			Stream   bool             `json:"stream"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		last := b.Messages[len(b.Messages)-1]
		var msg, usage string
		if last["role"] == "tool" {
			c, _ := json.Marshal("Answer from " + fmt.Sprint(last["content"]))
			msg, usage = `{"content":`+string(c)+`}`, `{"prompt_tokens":2000,"completion_tokens":30,"prompt_tokens_details":{"cached_tokens":1500}}`
		} else {
			*seen = append(*seen, fmt.Sprint(b.Messages[0]["content"], "|", last["content"], "|tools=", len(b.Tools)))
			msg, usage = `{"tool_calls":[{"index":0,"id":"c","type":"function","function":{"name":"server_info","arguments":"{}"}}]}`, `{"prompt_tokens":500,"completion_tokens":5}`
		}
		if b.Stream {
			fmt.Fprintf(w, "data: {\"choices\":[{\"delta\":%s}]}\n\ndata: {\"choices\":[],\"usage\":%s}\n\ndata: [DONE]\n\n", msg, usage)
			return
		}
		fmt.Fprintf(w, `{"choices":[{"message":%s}],"usage":%s}`, msg, usage)
	}))
}

func TestAPIAnswersWithTheAdvisorTools(t *testing.T) {
	var seen []string
	srv := chatServer(t, &seen)
	defer srv.Close()
	a, err := APIFor(provider.ClientRef{Kind: "api", Provider: "custom", BaseURL: srv.URL, Model: "m"}, Options{}, memKeys{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	cfg := Config{API: a, Server: fakeAdvisor}
	q := Question{Character: "R:New", Text: "what now?"}
	var chunks []string
	text, streamed, usage, err := RespondUsage(context.Background(), cfg, q, cfg.Prompt(q), false, func(s string) { chunks = append(chunks, s) })
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(text, `"fact":"42"`) || !streamed {
		t.Errorf("text %q streamed %v", text, streamed)
	}
	if usage.Line() != "≈1.0k in · 35 out · 1.5k cached · m" || usage.In != 1000 {
		t.Errorf("usage %+v line %q", usage, usage.Line())
	}
	sys, user, _ := strings.Cut(seen[0], "|Question: ")
	if len(seen) != 1 || !strings.Contains(sys, "Character key: R:New") || strings.Contains(sys, "what now?") || user != "what now?|tools=1" {
		t.Errorf("request %v", seen)
	}
}

func TestFallbackWhenAClientCannotStart(t *testing.T) {
	var seen []string
	srv := chatServer(t, &seen)
	defer srv.Close()
	good, _ := APIFor(provider.ClientRef{Kind: "api", Provider: "custom", BaseURL: srv.URL, Model: "m"}, Options{}, memKeys{}, nil)
	noKey, _ := APIFor(provider.ClientRef{Kind: "api", Provider: "anthropic"}, Options{}, memKeys{}, nil)
	q := Question{Character: "R:New", Text: "hi"}
	for name, first := range map[string]Config{
		"no key":     {API: noKey},
		"no program": {Command: []string{"wowc-no-such-agent-program"}},
		"no client":  {},
	} {
		t.Run(name, func(t *testing.T) {
			cfg := first
			cfg.Server = fakeAdvisor
			cfg.Fallback = []Config{{API: good, Server: fakeAdvisor}}
			text, _, usage, err := RespondUsage(context.Background(), cfg, q, cfg.Prompt(q), false, nil)
			if err != nil || !strings.HasPrefix(text, "Answer from") || !strings.Contains(usage.Notice, "didn't start") {
				t.Errorf("%q %v notice %q", text, err, usage.Notice)
			}
		})
	}
	// An answer-time failure does not fall back.
	bad := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { http.Error(w, `{"error":{"message":"overloaded"}}`, 500) }))
	defer bad.Close()
	b, _ := APIFor(provider.ClientRef{Kind: "api", Provider: "custom", BaseURL: bad.URL, Model: "m"}, Options{}, memKeys{}, nil)
	cfg := Config{API: b, Server: fakeAdvisor, Fallback: []Config{{API: good, Server: fakeAdvisor}}}
	if _, _, _, err := RespondUsage(context.Background(), cfg, q, cfg.Prompt(q), false, nil); err == nil || !strings.Contains(err.Error(), "overloaded") {
		t.Errorf("got %v", err)
	}
}

func TestWithClients(t *testing.T) {
	base := Config{Command: []string{"claude"}, Options: Options{Tier: TierBalanced}}
	if got := base.WithClients(provider.Settings{}, nil, nil); got.API != nil || len(got.Command) != 1 {
		t.Errorf("empty settings changed the CLI setup: %+v", got)
	}
	st := provider.Settings{Clients: []provider.ClientRef{{Kind: "api", Provider: "anthropic"}, {Kind: "cli", Agent: "codex"}}}
	got := base.WithClients(st, memKeys{"anthropic": "k"}, nil)
	if got.API == nil || got.API.Model != "claude-sonnet-5-5" || !got.API.HasKey || len(got.Fallback) != 1 || got.Fallback[0].Command[0] != "codex" {
		t.Errorf("%+v %+v", got.API, got.Fallback)
	}
}
