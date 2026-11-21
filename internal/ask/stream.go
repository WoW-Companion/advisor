package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// StreamArgs are appended to a Claude command to make it print stream-json with partial messages.
var StreamArgs = []string{"--output-format", "stream-json", "--verbose", "--include-partial-messages"}

// maxStreamOutput bounds what is read from a streaming agent; stream-json is far chattier than text.
const maxStreamOutput = 4 << 20

// Respond runs the agent for q with prompt (see Prompt) and returns the answer. With stream set the
// command is Claude's: its text deltas are passed to onText as they arrive and streamed is true when
// any did. Otherwise the output is read whole, cut like ask does, and streamed is false.
func Respond(ctx context.Context, cfg Config, q Question, prompt string, stream bool, onText func(string)) (text string, streamed bool, err error) {
	text, streamed, _, err = RespondUsage(ctx, cfg, q, prompt, stream, onText)
	return
}

// Usage is what the CLI reported for one ask. Tokens is zero when it reported nothing.
type Usage struct {
	Tokens  int     // input, output and cache tokens together
	CostUSD float64 // total_cost_usd, when reported
	Model   string  // the model that ran: reported by Claude, else the one the command line selected
	Notice  string  // a one-line notice for the player, e.g. a rejected flag that was dropped
}

// Line is the small muted text shown under an answer ("≈3.1k tokens · haiku"), empty with neither.
func (u Usage) Line() string {
	var parts []string
	switch {
	case u.Tokens <= 0:
	case u.Tokens < 1000:
		parts = append(parts, fmt.Sprintf("≈%d tokens", u.Tokens))
	case u.Tokens < 10000:
		parts = append(parts, fmt.Sprintf("≈%.1fk tokens", float64(u.Tokens)/1000))
	default:
		parts = append(parts, fmt.Sprintf("≈%dk tokens", (u.Tokens+500)/1000))
	}
	if u.Model != "" {
		parts = append(parts, u.Model)
	}
	return strings.Join(parts, " · ")
}

// RespondUsage is Respond that also returns the token usage Claude's stream-json result reports.
func RespondUsage(ctx context.Context, cfg Config, q Question, prompt string, stream bool, onText func(string)) (text string, streamed bool, usage Usage, err error) {
	usage.Model = ModelUsed(cfg.Command, cfg.Options)
	cfg.Notice = func(n string) { usage.Notice = n }
	if !stream {
		var out bytes.Buffer
		if err := execAgent(ctx, cfg, q, prompt, nil, &capped{w: &out, left: MaxOutput}); err != nil {
			return "", false, usage, err
		}
		if strings.TrimSpace(out.String()) == "" {
			return "", false, usage, errors.New("the agent gave no answer")
		}
		return CleanTo(out.String(), cfg.Options.AnswerLimit()), false, usage, nil
	}
	var acc strings.Builder
	var result string
	var failed bool
	lw := &lineWriter{max: maxStreamOutput, fn: func(line []byte) {
		var ev struct {
			Type  string `json:"type"`
			Event struct {
				Type  string `json:"type"`
				Delta struct {
					Type string `json:"type"`
					Text string `json:"text"`
				} `json:"delta"`
			} `json:"event"`
			Result     string  `json:"result"`
			IsError    bool    `json:"is_error"`
			Cost       float64 `json:"total_cost_usd"`
			ModelUsage map[string]struct {
				In  int `json:"inputTokens"`
				Out int `json:"outputTokens"`
			} `json:"modelUsage"`
			Usage struct {
				In        int `json:"input_tokens"`
				Out       int `json:"output_tokens"`
				CacheNew  int `json:"cache_creation_input_tokens"`
				CacheRead int `json:"cache_read_input_tokens"`
			} `json:"usage"`
		}
		if json.Unmarshal(line, &ev) != nil {
			return
		}
		switch {
		case ev.Type == "stream_event" && ev.Event.Type == "content_block_delta" && ev.Event.Delta.Type == "text_delta":
			if acc.Len() < MaxOutput && ev.Event.Delta.Text != "" {
				acc.WriteString(ev.Event.Delta.Text)
				onText(ev.Event.Delta.Text)
			}
		case ev.Type == "result":
			result, failed = ev.Result, ev.IsError
			usage.Tokens, usage.CostUSD = ev.Usage.In+ev.Usage.Out+ev.Usage.CacheNew+ev.Usage.CacheRead, ev.Cost
			// Claude reports every model it used (a helper model may appear); the busiest one ran the answer.
			best, chosen := -1, ""
			for m, u := range ev.ModelUsage {
				if n := u.In + u.Out; n > best || (n == best && m < chosen) {
					best, chosen = n, m
				}
			}
			if chosen != "" {
				usage.Model = chosen
			}
		}
	}}
	if err := execAgent(ctx, cfg, q, prompt, StreamArgs, lw); err != nil {
		return "", false, usage, err
	}
	lw.flush()
	if failed {
		return "", false, usage, errors.New("the agent reported an error")
	}
	text = strings.TrimSpace(result)
	if text == "" {
		text = strings.TrimSpace(acc.String())
	}
	if text == "" {
		return "", false, usage, errors.New("the agent gave no answer")
	}
	return strings.ToValidUTF8(text, ""), acc.Len() > 0, usage, nil
}

// lineWriter calls fn for each complete line written; a line longer than max is dropped.
type lineWriter struct {
	buf  []byte
	max  int
	fn   func([]byte)
	over bool
}

func (w *lineWriter) Write(p []byte) (int, error) {
	n := len(p)
	for len(p) > 0 {
		i := bytes.IndexByte(p, '\n')
		if i < 0 {
			if len(w.buf)+len(p) > w.max {
				w.over, w.buf = true, w.buf[:0]
			} else if !w.over {
				w.buf = append(w.buf, p...)
			}
			break
		}
		if !w.over && len(w.buf)+i <= w.max {
			w.fn(append(w.buf, p[:i]...))
		}
		w.buf, w.over, p = w.buf[:0], false, p[i+1:]
	}
	return n, nil
}

// flush handles a last line with no newline.
func (w *lineWriter) flush() {
	if len(w.buf) > 0 && !w.over {
		w.fn(w.buf)
	}
	w.buf = w.buf[:0]
}
