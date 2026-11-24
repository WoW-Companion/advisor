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
	In      int     // fresh input: new input plus tokens written to the cache
	Out     int     // output
	Cached  int     // tokens read from the cache
	CostUSD float64 // total_cost_usd, when reported
	Model   string  // the model that ran: reported by Claude, else the one the command line selected
	Notice  string  // a one-line notice for the player, e.g. a rejected flag that was dropped
}

// UsageTip is the tooltip of the usage line.
const UsageTip = "Cached tokens are input the AI client reused from an earlier call. They are usually cheaper, but may still count toward your plan's limits."

// Line is the small muted text shown under an answer ("≈4k in · 0.6k out · 52k cached · haiku"; only
// the total when the CLI gave no split), empty with neither tokens nor a model.
func (u Usage) Line() string {
	var parts []string
	switch {
	case u.Tokens <= 0:
	case u.In+u.Out+u.Cached == 0:
		parts = append(parts, "≈"+tokenCount(u.Tokens)+" tokens")
	default:
		parts = append(parts, "≈"+tokenCount(u.In)+" in", tokenCount(u.Out)+" out")
		if u.Cached > 0 {
			parts = append(parts, tokenCount(u.Cached)+" cached")
		}
	}
	if u.Model != "" {
		parts = append(parts, u.Model)
	}
	return strings.Join(parts, " · ")
}

// tokenCount is n as "812", "3.1k" or "52k".
func tokenCount(n int) string {
	switch {
	case n < 1000:
		return fmt.Sprint(n)
	case n < 10000:
		return fmt.Sprintf("%.1fk", float64(n)/1000)
	}
	return fmt.Sprintf("%dk", (n+500)/1000)
}

// RespondUsage is Respond that also returns the token usage Claude's stream-json result reports.
//
// With cfg.Fallback set, a client that cannot start (no key, not reachable, key refused, no such
// program) hands over to the next one, and the usage notice says so.
func RespondUsage(ctx context.Context, cfg Config, q Question, prompt string, stream bool, onText func(string)) (text string, streamed bool, usage Usage, err error) {
	chain := cfg.Fallback
	cfg.Fallback = nil
	text, streamed, usage, err = respondOne(ctx, cfg, q, prompt, stream, onText)
	for _, next := range chain {
		if err == nil || !startFailure(err) {
			break
		}
		first := err.Error()
		next.Fallback = nil
		text, streamed, usage, err = respondOne(ctx, next, q, prompt, stream, onText)
		if err == nil && usage.Notice == "" {
			usage.Notice = "The first AI client didn't start (" + first + "), so another one answered."
		}
	}
	return
}

func respondOne(ctx context.Context, cfg Config, q Question, prompt string, stream bool, onText func(string)) (text string, streamed bool, usage Usage, err error) {
	if cfg.API != nil {
		return respondAPI(ctx, cfg, prompt, onText)
	}
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
			usage.In, usage.Out, usage.Cached = ev.Usage.In+ev.Usage.CacheNew, ev.Usage.Out, ev.Usage.CacheRead
			usage.Tokens, usage.CostUSD = usage.In+usage.Out+usage.Cached, ev.Cost
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
