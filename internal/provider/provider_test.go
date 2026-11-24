package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

const secret = "sk-test-SECRET-123"

type fakeTools struct {
	calls  []string
	result string
}

func (f *fakeTools) List(context.Context) ([]ToolDef, error) {
	return []ToolDef{{Name: "server_info", Description: "info", Schema: json.RawMessage(`{"type":"object"}`)}}, nil
}
func (f *fakeTools) Call(_ context.Context, name string, args json.RawMessage) (string, bool, error) {
	f.calls = append(f.calls, name+string(args))
	return f.result, false, nil
}

func sse(w http.ResponseWriter, events ...string) {
	w.Header().Set("Content-Type", "text/event-stream")
	for _, e := range events {
		fmt.Fprint(w, e)
	}
}

// anthropicServer answers the first call with a tool use, the second with the final text.
func anthropicServer(t *testing.T, stream bool, bodies *[]map[string]any) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/messages" || r.Header.Get("x-api-key") != secret {
			http.Error(w, `{"error":{"message":"bad auth"}}`, 401)
			return
		}
		var b map[string]any
		json.NewDecoder(r.Body).Decode(&b)
		*bodies = append(*bodies, b)
		first := len(b["messages"].([]any)) == 1
		switch {
		case !stream && first:
			fmt.Fprint(w, `{"content":[{"type":"text","text":"Checking."},{"type":"tool_use","id":"tu1","name":"server_info","input":{"a":1}}],"usage":{"input_tokens":100,"output_tokens":10,"cache_creation_input_tokens":50,"cache_read_input_tokens":0}}`)
		case !stream:
			fmt.Fprint(w, `{"content":[{"type":"text","text":"Final answer."}],"usage":{"input_tokens":20,"output_tokens":5,"cache_read_input_tokens":150}}`)
		case first:
			sse(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":100,\"cache_creation_input_tokens\":50,\"output_tokens\":1}}}\n\n",
				"event: content_block_start\ndata: {\"type\":\"content_block_start\",\"index\":0,\"content_block\":{\"type\":\"tool_use\",\"id\":\"tu1\",\"name\":\"server_info\"}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\"{\\\"a\\\"\"}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"input_json_delta\",\"partial_json\":\":1}\"}}\n\n",
				"event: content_block_stop\ndata: {\"type\":\"content_block_stop\",\"index\":0}\n\n",
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":10}}\n\n")
		default:
			sse(w, "event: message_start\ndata: {\"type\":\"message_start\",\"message\":{\"usage\":{\"input_tokens\":20,\"cache_read_input_tokens\":150,\"output_tokens\":1}}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"Final \"}}\n\n",
				"event: content_block_delta\ndata: {\"type\":\"content_block_delta\",\"index\":0,\"delta\":{\"type\":\"text_delta\",\"text\":\"answer.\"}}\n\n",
				"event: message_delta\ndata: {\"type\":\"message_delta\",\"usage\":{\"output_tokens\":5}}\n\n")
		}
	}))
}

func openaiServer(t *testing.T, stream bool, auth string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != auth {
			http.Error(w, `{"error":{"message":"bad auth"}}`, 401)
			return
		}
		var b struct {
			Messages []map[string]any `json:"messages"`
			Tools    []any            `json:"tools"`
		}
		json.NewDecoder(r.Body).Decode(&b)
		first := b.Messages[len(b.Messages)-1]["role"] == "user"
		switch {
		case !stream && first:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"server_info","arguments":"{\"a\":1}"}}]}}],"usage":{"prompt_tokens":100,"completion_tokens":10}}`)
		case !stream:
			fmt.Fprint(w, `{"choices":[{"message":{"content":"Final answer."}}],"usage":{"prompt_tokens":170,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":150}}}`)
		case first:
			sse(w, `data: {"choices":[{"delta":{"tool_calls":[{"index":0,"id":"c1","function":{"name":"server_info","arguments":"{\"a\""}}]}}]}`+"\n\n",
				`data: {"choices":[{"delta":{"tool_calls":[{"index":0,"function":{"arguments":":1}"}}]}}]}`+"\n\n",
				`data: {"choices":[],"usage":{"prompt_tokens":100,"completion_tokens":10}}`+"\n\n", "data: [DONE]\n\n")
		default:
			sse(w, `data: {"choices":[{"delta":{"content":"Final "}}]}`+"\n\n", `data: {"choices":[{"delta":{"content":"answer."}}]}`+"\n\n",
				`data: {"choices":[],"usage":{"prompt_tokens":170,"completion_tokens":5,"prompt_tokens_details":{"cached_tokens":150}}}`+"\n\n", "data: [DONE]\n\n")
		}
	}))
}

func runLoop(t *testing.T, ad Adapter, stream bool) (string, Usage, string, *fakeTools) {
	ft := &fakeTools{result: "the facts"}
	var got strings.Builder
	text, u, err := Loop{Adapter: ad, Model: "m", System: "sys", Prompt: "q", Tools: ft, Stream: stream, OnText: func(s string) { got.WriteString(s) }}.Run(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	return text, u, got.String(), ft
}

func TestAnthropicToolLoop(t *testing.T) {
	for _, stream := range []bool{false, true} {
		t.Run(fmt.Sprint("stream=", stream), func(t *testing.T) {
			var bodies []map[string]any
			srv := anthropicServer(t, stream, &bodies)
			defer srv.Close()
			text, u, streamed, ft := runLoop(t, Anthropic{Endpoint{BaseURL: srv.URL + "/v1", Key: secret}}, stream)
			if text != "Final answer." || len(ft.calls) != 1 || ft.calls[0] != `server_info{"a":1}` {
				t.Fatalf("text %q calls %v", text, ft.calls)
			}
			if u != (Usage{In: 170, Out: 15, Cached: 150}) {
				t.Errorf("usage %+v", u)
			}
			if stream && !strings.HasSuffix(streamed, "Final answer.") {
				t.Errorf("streamed %q", streamed)
			}
			first := bodies[0]
			if !strings.Contains(fmt.Sprint(first["system"]), "ephemeral") || !strings.Contains(fmt.Sprint(first["tools"]), "ephemeral") {
				t.Errorf("no cache breakpoints: %v", first)
			}
			if !strings.Contains(fmt.Sprint(bodies[1]["messages"]), "tool_result") {
				t.Errorf("result not sent back: %v", bodies[1]["messages"])
			}
		})
	}
}

func TestOpenAIToolLoop(t *testing.T) {
	for _, tc := range []struct {
		stream bool
		key    string
	}{{false, secret}, {true, secret}, {true, ""}} {
		t.Run(fmt.Sprint(tc.stream, tc.key != ""), func(t *testing.T) {
			auth := ""
			if tc.key != "" {
				auth = "Bearer " + tc.key
			}
			srv := openaiServer(t, tc.stream, auth)
			defer srv.Close()
			text, u, _, ft := runLoop(t, OpenAI{Endpoint{BaseURL: srv.URL + "/v1", Key: tc.key}}, tc.stream)
			if text != "Final answer." || len(ft.calls) != 1 || ft.calls[0] != `server_info{"a":1}` {
				t.Fatalf("text %q calls %v", text, ft.calls)
			}
			if u != (Usage{In: 120, Out: 15, Cached: 150}) {
				t.Errorf("usage %+v", u)
			}
		})
	}
}

func TestNoToolsError(t *testing.T) {
	for name, tc := range map[string]struct{ ad func(string) Adapter }{
		"openai":    {func(u string) Adapter { return OpenAI{Endpoint{BaseURL: u}} }},
		"anthropic": {func(u string) Adapter { return Anthropic{Endpoint{BaseURL: u, Key: "k"}} }},
	} {
		t.Run(name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				http.Error(w, `{"error":{"message":"registry.ollama.ai/library/gemma does not support tools"}}`, 400)
			}))
			defer srv.Close()
			_, _, err := Loop{Adapter: tc.ad(srv.URL), Model: "m", Prompt: "q", Tools: &fakeTools{}}.Run(context.Background())
			if err == nil || err.Error() != ErrNoTools {
				t.Fatalf("got %v", err)
			}
		})
	}
}

func TestStartErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, `{"error":{"message":"invalid key `+secret+`"}}`, 401)
	}))
	_, err := OpenAI{Endpoint{BaseURL: srv.URL, Key: secret}}.Chat(context.Background(), Request{Model: "m"}, nil)
	if !IsStart(err) || strings.Contains(err.Error(), secret) {
		t.Errorf("401: %v", err)
	}
	srv.Close()
	_, err = OpenAI{Endpoint{BaseURL: srv.URL, Key: secret}}.Chat(context.Background(), Request{Model: "m"}, nil)
	if !IsStart(err) || strings.Contains(err.Error(), secret) {
		t.Errorf("closed: %v", err)
	}
	_, err = Anthropic{}.Chat(context.Background(), Request{}, nil)
	if !IsStart(err) {
		t.Errorf("no key: %v", err)
	}
}

// loopForever always asks for another tool call.
type loopForever struct{ delay time.Duration }

func (l loopForever) Chat(ctx context.Context, _ Request, _ func(string)) (Reply, error) {
	select {
	case <-time.After(l.delay):
	case <-ctx.Done():
		return Reply{}, ctx.Err()
	}
	return Reply{Calls: []Call{{ID: "x", Name: "server_info"}}, Usage: Usage{In: 1}}, nil
}

func TestLoopCaps(t *testing.T) {
	ft := &fakeTools{result: strings.Repeat("é", 100)}
	_, u, err := Loop{Adapter: loopForever{}, Model: "m", Prompt: "q", Tools: ft, MaxTurns: 3, MaxResult: 51}.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "3 steps") || len(ft.calls) != 3 || u.In != 3 {
		t.Errorf("max turns: %v calls %d usage %+v", err, len(ft.calls), u)
	}
	_, _, err = Loop{Adapter: loopForever{delay: time.Second}, Model: "m", Prompt: "q", Tools: ft, Timeout: 30 * time.Millisecond}.Run(context.Background())
	if err == nil || !strings.Contains(err.Error(), "took longer") {
		t.Errorf("timeout: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err = (Loop{Adapter: loopForever{}, Model: "m", Prompt: "q", Tools: ft}).Run(ctx); err == nil {
		t.Error("cancelled context ran")
	}
	if got := capText(strings.Repeat("é", 100), 51); !strings.HasPrefix(got, strings.Repeat("é", 25)) || !strings.Contains(got, "cut") {
		t.Errorf("capText %q", got)
	}
}

func TestModelList(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"id":"b"},{"id":"a"}]}`)
	}))
	defer srv.Close()
	ids, err := OpenAI{Endpoint{BaseURL: srv.URL + "/v1"}}.Models(context.Background())
	if err != nil || strings.Join(ids, ",") != "a,b" {
		t.Errorf("%v %v", ids, err)
	}
}

func TestFileKeyStore(t *testing.T) {
	ks := FileKeyStore{Dir: t.TempDir() + "/keys"}
	if _, err := ks.Get("openai"); err != ErrNoKey {
		t.Fatalf("empty: %v", err)
	}
	if err := ks.Set("openai", secret); err != nil {
		t.Fatal(err)
	}
	if k, err := ks.Get("openai"); err != nil || k != secret {
		t.Fatalf("get %q %v", k, err)
	}
	if err := ks.Set("../evil", "x"); err == nil {
		t.Error("path escape accepted")
	}
	if err := ks.Delete("openai"); err != nil {
		t.Fatal(err)
	}
	if err := ks.Delete("openai"); err != nil {
		t.Errorf("second delete: %v", err)
	}
	if _, err := ks.Get("openai"); err != ErrNoKey {
		t.Errorf("after delete: %v", err)
	}
}

func TestSettingsRoundTrip(t *testing.T) {
	path := t.TempDir() + "/providers.json"
	c, err := ClientRef{Kind: ClientAPI, Provider: "ollama", BaseURL: "http://localhost:11434/v1", Model: "llama"}.Validate()
	if err != nil || c.BaseURL != "" {
		t.Fatalf("%+v %v", c, err)
	}
	if err := Save(path, Settings{Clients: []ClientRef{c, {Kind: ClientCLI, Agent: "claude"}}}); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || len(s.Clients) != 2 || s.Clients[0].Model != "llama" {
		t.Errorf("%+v %v", s, err)
	}
	for _, bad := range []ClientRef{{Kind: "api", Provider: "nope"}, {Kind: "api", Provider: "custom"}, {Kind: "api", Provider: "openai", BaseURL: "ftp://x"}, {Kind: "cli"}} {
		if _, err := bad.Validate(); err == nil {
			t.Errorf("%+v accepted", bad)
		}
	}
}
