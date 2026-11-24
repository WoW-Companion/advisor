package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
)

// Anthropic talks to the Messages API (https://platform.claude.com/docs/en/api/messages). The system
// prompt and the tool list carry cache-control breakpoints, so repeat asks read them from the cache.
type Anthropic struct{ Endpoint }

const anthropicVersion = "2023-06-01"

func (a Anthropic) headers() map[string]string {
	return map[string]string{"x-api-key": a.Key, "anthropic-version": anthropicVersion}
}

// Models lists the models the key can use.
func (a Anthropic) Models(ctx context.Context) ([]string, error) {
	return a.ModelList(ctx, a.headers())
}

type aBlock struct {
	Type         string          `json:"type"`
	Text         string          `json:"text,omitempty"`
	ID           string          `json:"id,omitempty"`
	Name         string          `json:"name,omitempty"`
	Input        json.RawMessage `json:"input,omitempty"`
	ToolUseID    string          `json:"tool_use_id,omitempty"`
	Content      string          `json:"content,omitempty"`
	IsError      bool            `json:"is_error,omitempty"`
	CacheControl map[string]any  `json:"cache_control,omitempty"`
}

func (a Anthropic) body(req Request) map[string]any {
	ephemeral := map[string]any{"type": "ephemeral"}
	var msgs []map[string]any
	for _, m := range req.Messages {
		var blocks []aBlock
		if m.Text != "" {
			blocks = append(blocks, aBlock{Type: "text", Text: m.Text})
		}
		for _, c := range m.Calls {
			in := c.Args
			if len(in) == 0 {
				in = json.RawMessage("{}")
			}
			blocks = append(blocks, aBlock{Type: "tool_use", ID: c.ID, Name: c.Name, Input: in})
		}
		for _, r := range m.Results {
			blocks = append(blocks, aBlock{Type: "tool_result", ToolUseID: r.CallID, Content: r.Text, IsError: r.IsError})
		}
		msgs = append(msgs, map[string]any{"role": m.Role, "content": blocks})
	}
	body := map[string]any{
		"model":      req.Model,
		"max_tokens": 4096,
		"messages":   msgs,
		"stream":     req.Stream,
	}
	if req.System != "" {
		body["system"] = []aBlock{{Type: "text", Text: req.System, CacheControl: ephemeral}}
	}
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for i, t := range req.Tools {
			sc := t.Schema
			if len(sc) == 0 {
				sc = json.RawMessage(`{"type":"object"}`)
			}
			m := map[string]any{"name": t.Name, "description": t.Description, "input_schema": sc}
			if i == len(req.Tools)-1 {
				m["cache_control"] = ephemeral
			}
			tools = append(tools, m)
		}
		body["tools"] = tools
	}
	return a.merge(body)
}

type aUsage struct {
	In        int `json:"input_tokens"`
	Out       int `json:"output_tokens"`
	CacheNew  int `json:"cache_creation_input_tokens"`
	CacheRead int `json:"cache_read_input_tokens"`
}

func (u aUsage) usage() Usage { return Usage{In: u.In + u.CacheNew, Out: u.Out, Cached: u.CacheRead} }

// Chat makes one Messages call.
func (a Anthropic) Chat(ctx context.Context, req Request, onText func(string)) (Reply, error) {
	if a.Key == "" {
		return Reply{}, &StartError{"No API key is saved for Anthropic."}
	}
	resp, err := a.do(ctx, http.MethodPost, "/messages", a.body(req), a.headers())
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	if !req.Stream {
		var out struct {
			Content []aBlock `json:"content"`
			Usage   aUsage   `json:"usage"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil {
			return Reply{}, errors.New("the provider's answer wasn't in the expected form")
		}
		r := Reply{Usage: out.Usage.usage()}
		for _, b := range out.Content {
			switch b.Type {
			case "text":
				r.Text += b.Text
				if onText != nil && b.Text != "" {
					onText(b.Text)
				}
			case "tool_use":
				r.Calls = append(r.Calls, Call{ID: b.ID, Name: b.Name, Args: b.Input})
			}
		}
		return r, nil
	}
	var r Reply
	type partial struct {
		call Call
		json []byte
	}
	var cur *partial
	err = readSSE(resp.Body, func(_, data string) error {
		var ev struct {
			Type  string `json:"type"`
			Index int    `json:"index"`
			Block struct {
				Type string `json:"type"`
				ID   string `json:"id"`
				Name string `json:"name"`
			} `json:"content_block"`
			Delta struct {
				Type    string `json:"type"`
				Text    string `json:"text"`
				Partial string `json:"partial_json"`
			} `json:"delta"`
			Message struct {
				Usage aUsage `json:"usage"`
			} `json:"message"`
			Usage aUsage `json:"usage"`
			Error struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			return nil
		}
		switch ev.Type {
		case "message_start":
			r.Usage = ev.Message.Usage.usage()
		case "content_block_start":
			cur = nil
			if ev.Block.Type == "tool_use" {
				cur = &partial{call: Call{ID: ev.Block.ID, Name: ev.Block.Name}}
			}
		case "content_block_delta":
			switch ev.Delta.Type {
			case "text_delta":
				r.Text += ev.Delta.Text
				if onText != nil && ev.Delta.Text != "" {
					onText(ev.Delta.Text)
				}
			case "input_json_delta":
				if cur != nil {
					cur.json = append(cur.json, ev.Delta.Partial...)
				}
			}
		case "content_block_stop":
			if cur != nil {
				cur.call.Args = json.RawMessage(cur.json)
				r.Calls = append(r.Calls, cur.call)
				cur = nil
			}
		case "message_delta":
			r.Usage.Out = ev.Usage.Out
			if ev.Usage.In+ev.Usage.CacheNew+ev.Usage.CacheRead > 0 {
				r.Usage = ev.Usage.usage()
			}
		case "error":
			return httpError(400, []byte(`{"error":{"message":`+quote(ev.Error.Message)+`}}`), a.Key)
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, err
	}
	return r, nil
}

func quote(s string) string { b, _ := json.Marshal(s); return string(b) }
