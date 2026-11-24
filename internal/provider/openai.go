package provider

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"sort"
)

// OpenAI talks to an OpenAI-compatible Chat Completions endpoint (BaseURL ends in /v1): OpenAI,
// OpenRouter, Ollama, LM Studio and the like. Key may be empty.
type OpenAI struct{ Endpoint }

func (o OpenAI) headers() map[string]string {
	if o.Key == "" {
		return nil
	}
	return map[string]string{"Authorization": "Bearer " + o.Key}
}

// Models lists the server's models.
func (o OpenAI) Models(ctx context.Context) ([]string, error) {
	ids, err := o.ModelList(ctx, o.headers())
	sort.Strings(ids)
	return ids, err
}

type oFunc struct {
	Name string `json:"name,omitempty"`
	Args string `json:"arguments,omitempty"`
}
type oCall struct {
	Index int    `json:"index"`
	ID    string `json:"id,omitempty"`
	Type  string `json:"type,omitempty"`
	Func  oFunc  `json:"function"`
}

func (o OpenAI) body(req Request) map[string]any {
	var msgs []map[string]any
	if req.System != "" {
		msgs = append(msgs, map[string]any{"role": "system", "content": req.System})
	}
	for _, m := range req.Messages {
		for _, r := range m.Results {
			msgs = append(msgs, map[string]any{"role": "tool", "tool_call_id": r.CallID, "content": r.Text})
		}
		if m.Text == "" && len(m.Calls) == 0 {
			continue
		}
		msg := map[string]any{"role": m.Role, "content": m.Text}
		if len(m.Calls) > 0 {
			var cs []oCall
			for i, c := range m.Calls {
				args := string(c.Args)
				if args == "" {
					args = "{}"
				}
				cs = append(cs, oCall{Index: i, ID: c.ID, Type: "function", Func: oFunc{c.Name, args}})
			}
			msg["tool_calls"] = cs
		}
		msgs = append(msgs, msg)
	}
	body := map[string]any{"model": req.Model, "messages": msgs, "stream": req.Stream}
	if req.Stream {
		body["stream_options"] = map[string]any{"include_usage": true}
	}
	if len(req.Tools) > 0 {
		var tools []map[string]any
		for _, t := range req.Tools {
			sc := t.Schema
			if len(sc) == 0 {
				sc = json.RawMessage(`{"type":"object"}`)
			}
			tools = append(tools, map[string]any{"type": "function", "function": map[string]any{"name": t.Name, "description": t.Description, "parameters": sc}})
		}
		body["tools"] = tools
	}
	return o.merge(body)
}

type oUsage struct {
	Prompt  int `json:"prompt_tokens"`
	Out     int `json:"completion_tokens"`
	Details struct {
		Cached int `json:"cached_tokens"`
	} `json:"prompt_tokens_details"`
}

func (u oUsage) usage() Usage {
	return Usage{In: max(u.Prompt-u.Details.Cached, 0), Out: u.Out, Cached: u.Details.Cached}
}

// Chat makes one Chat Completions call.
func (o OpenAI) Chat(ctx context.Context, req Request, onText func(string)) (Reply, error) {
	resp, err := o.do(ctx, http.MethodPost, "/chat/completions", o.body(req), o.headers())
	if err != nil {
		return Reply{}, err
	}
	defer resp.Body.Close()
	if !req.Stream {
		var out struct {
			Choices []struct {
				Message struct {
					Content   string  `json:"content"`
					ToolCalls []oCall `json:"tool_calls"`
				} `json:"message"`
			} `json:"choices"`
			Usage oUsage `json:"usage"`
		}
		if err := json.NewDecoder(io.LimitReader(resp.Body, 8<<20)).Decode(&out); err != nil || len(out.Choices) == 0 {
			return Reply{}, errors.New("the provider's answer wasn't in the expected form")
		}
		m := out.Choices[0].Message
		r := Reply{Text: m.Content, Usage: out.Usage.usage()}
		for _, c := range m.ToolCalls {
			r.Calls = append(r.Calls, Call{ID: c.ID, Name: c.Func.Name, Args: json.RawMessage(c.Func.Args)})
		}
		if onText != nil && r.Text != "" {
			onText(r.Text)
		}
		return r, nil
	}
	var r Reply
	calls := map[int]*Call{}
	args := map[int][]byte{}
	err = readSSE(resp.Body, func(_, data string) error {
		if data == "[DONE]" {
			return nil
		}
		var ev struct {
			Choices []struct {
				Delta struct {
					Content   string  `json:"content"`
					ToolCalls []oCall `json:"tool_calls"`
				} `json:"delta"`
			} `json:"choices"`
			Usage *oUsage `json:"usage"`
			Error *struct {
				Message string `json:"message"`
			} `json:"error"`
		}
		if json.Unmarshal([]byte(data), &ev) != nil {
			return nil
		}
		if ev.Error != nil {
			return httpError(400, []byte(`{"error":{"message":`+quote(ev.Error.Message)+`}}`), o.Key)
		}
		if ev.Usage != nil {
			r.Usage = ev.Usage.usage()
		}
		for _, ch := range ev.Choices {
			if t := ch.Delta.Content; t != "" {
				r.Text += t
				if onText != nil {
					onText(t)
				}
			}
			for _, tc := range ch.Delta.ToolCalls {
				c := calls[tc.Index]
				if c == nil {
					c = &Call{}
					calls[tc.Index] = c
				}
				if tc.ID != "" {
					c.ID = tc.ID
				}
				if tc.Func.Name != "" {
					c.Name = tc.Func.Name
				}
				args[tc.Index] = append(args[tc.Index], tc.Func.Args...)
			}
		}
		return nil
	})
	if err != nil {
		if ctx.Err() != nil {
			return Reply{}, ctx.Err()
		}
		return Reply{}, err
	}
	var idx []int
	for i := range calls {
		idx = append(idx, i)
	}
	sort.Ints(idx)
	for _, i := range idx {
		c := *calls[i]
		c.Args = json.RawMessage(args[i])
		r.Calls = append(r.Calls, c)
	}
	return r, nil
}
