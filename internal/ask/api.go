package ask

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/WoW-Companion/advisor/internal/provider"
)

// API is a model provider API client (client kind "api"): the loop runs in this process, and the
// advisor's tools are served to it over an in-memory MCP session, the same server a CLI starts.
type API struct {
	Name    string           // short name for messages, e.g. "Anthropic (Claude)"
	Adapter provider.Adapter // sends the calls; holds the key
	Model   string           // the model id to ask
	NeedKey bool
	HasKey  bool
	// MaxTurns, Timeout and MaxResult override the loop's defaults (zero: MaxTurns, Config.Timeout, the loop's).
	MaxResult int
}

// mcpTools serves the advisor's tools to the loop through an MCP client session.
type mcpTools struct{ cs *mcp.ClientSession }

func (t mcpTools) List(ctx context.Context) ([]provider.ToolDef, error) {
	var out []provider.ToolDef
	for tool, err := range t.cs.Tools(ctx, nil) {
		if err != nil {
			return nil, err
		}
		sc, _ := json.Marshal(tool.InputSchema)
		out = append(out, provider.ToolDef{Name: tool.Name, Description: tool.Description, Schema: sc})
	}
	return out, nil
}

func (t mcpTools) Call(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	var a any
	if len(args) > 0 {
		if err := json.Unmarshal(args, &a); err != nil {
			return "the arguments weren't valid JSON", true, nil
		}
	}
	res, err := t.cs.CallTool(ctx, &mcp.CallToolParams{Name: name, Arguments: a})
	if err != nil {
		return "", false, err
	}
	var parts []string
	for _, c := range res.Content {
		if tc, ok := c.(*mcp.TextContent); ok {
			parts = append(parts, tc.Text)
		}
	}
	text := strings.Join(parts, "\n")
	if text == "" && res.StructuredContent != nil {
		b, _ := json.Marshal(res.StructuredContent)
		text = string(b)
	}
	return text, res.IsError, nil
}

// connectTools starts the advisor's MCP server in process and connects a client to it.
func connectTools(ctx context.Context, newServer func() *mcp.Server) (provider.Tools, func(), error) {
	if newServer == nil {
		return nil, nil, errors.New("the advisor's tools aren't available to this client")
	}
	st, ct := mcp.NewInMemoryTransports()
	ss, err := newServer().Connect(ctx, st, nil)
	if err != nil {
		return nil, nil, err
	}
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "wowc-ask", Version: "1"}, nil).Connect(ctx, ct, nil)
	if err != nil {
		ss.Close()
		return nil, nil, err
	}
	return mcpTools{cs}, func() { cs.Close(); ss.Close() }, nil
}

// splitPrompt cuts a prompt into the system part (instructions, and the character key) and the user
// part (the pre-fetched facts and the question), so the stable part can be cached by the provider.
func splitPrompt(p string) (system, user string) {
	i := strings.Index(p, "\n\nThe character's current facts are below")
	if i < 0 {
		i = strings.Index(p, "\n\nQuestion: ")
	}
	if i < 0 {
		return "", p
	}
	return p[:i], strings.TrimSpace(p[i:])
}

// respondAPI answers through a provider API.
func respondAPI(ctx context.Context, cfg Config, prompt string, onText func(string)) (text string, streamed bool, usage Usage, err error) {
	a := cfg.API
	usage.Model = a.Model
	if a.NeedKey && !a.HasKey {
		return "", false, usage, &provider.StartError{Msg: "No API key is saved for " + a.Name + "."}
	}
	if a.Model == "" {
		return "", false, usage, &provider.StartError{Msg: "No model is chosen for " + a.Name + "."}
	}
	tools, closeTools, err := connectTools(ctx, cfg.Server)
	if err != nil {
		return "", false, usage, fmt.Errorf("couldn't start the advisor's tools: %w", err)
	}
	defer closeTools()
	sys, user := splitPrompt(prompt)
	n := 0
	loop := provider.Loop{Adapter: a.Adapter, Model: a.Model, System: sys, Prompt: user, Tools: tools,
		MaxTurns: MaxTurns, Timeout: cfg.Timeout, MaxResult: a.MaxResult, Stream: onText != nil,
		OnText: func(s string) {
			if onText != nil && n < MaxOutput {
				n += len(s)
				onText(s)
			}
		}}
	text, u, err := loop.Run(ctx)
	usage.In, usage.Out, usage.Cached = u.In, u.Out, u.Cached
	usage.Tokens = u.In + u.Out + u.Cached
	if err != nil {
		return "", n > 0, usage, err
	}
	return CleanTo(text, cfg.Options.AnswerLimit()), n > 0, usage, nil
}

// startFailure reports whether err means the client could not be started at all, so the next
// client in the list may be tried.
func startFailure(err error) bool { return provider.IsStart(err) }

// APIFor builds the API client for ref; keys supplies its key (nil: none). opts is the overlay's
// answer setting, which picks the tier when ref names none.
func APIFor(ref provider.ClientRef, opts Options, keys provider.KeyStore, hc *http.Client) (*API, error) {
	p, ok := provider.PresetByID(ref.Provider)
	if !ok {
		return nil, fmt.Errorf("unknown provider %q", ref.Provider)
	}
	var key string
	if keys != nil {
		key, _ = keys.Get(p.ID)
	}
	ad := provider.NewAdapter(p, ref.BaseURL, key)
	if hc != nil {
		switch a := ad.(type) {
		case provider.Anthropic:
			a.HTTP = hc
			ad = a
		case provider.OpenAI:
			a.HTTP = hc
			ad = a
		}
	}
	model := ref.Model
	if model == "" {
		o := opts.Normalize()
		tier := cmpOr(ref.Tier, o.Tier)
		model = p.ModelFor(tier, o.Custom)
		if model == "" {
			model = p.Models[TierEfficient]
		}
	}
	return &API{Name: p.Name, Adapter: ad, Model: model, NeedKey: p.NeedKey, HasKey: key != ""}, nil
}

func cmpOr(a, b string) string {
	if a != "" {
		return a
	}
	return b
}

// WithClients applies a saved client list to c: the first client answers, the rest follow in order
// when it cannot start. An empty list returns c unchanged (the CLI-only setup).
func (c Config) WithClients(st provider.Settings, keys provider.KeyStore, hc *http.Client) Config {
	if len(st.Clients) == 0 {
		return c
	}
	base := c
	base.Fallback = nil
	var cfgs []Config
	for _, ref := range st.Clients {
		one := base
		switch ref.Kind {
		case provider.ClientAPI:
			a, err := APIFor(ref, c.Options, keys, hc)
			if err != nil {
				continue
			}
			one.API, one.Command, one.Name, one.Source = a, nil, a.Name, SourceConfig
		case provider.ClientCLI:
			one.API, one.Command, one.Name, one.Source = nil, expand(ref.Agent), agentName(ref.Agent, expand(ref.Agent)), SourceConfig
		default:
			continue
		}
		cfgs = append(cfgs, one)
	}
	if len(cfgs) == 0 {
		return c
	}
	out := cfgs[0]
	out.Fallback = cfgs[1:]
	return out
}

// Usable reports whether the config can ask anything: an agent command or an API client.
func (c Config) Usable() bool { return len(c.Command) > 0 || c.API != nil }
