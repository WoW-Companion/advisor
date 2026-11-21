package ask

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
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
	if !stream {
		var out bytes.Buffer
		if err := execAgent(ctx, cfg, q, prompt, nil, &capped{w: &out, left: MaxOutput}); err != nil {
			return "", false, err
		}
		if strings.TrimSpace(out.String()) == "" {
			return "", false, errors.New("the agent gave no answer")
		}
		return Clean(out.String()), false, nil
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
			Result  string `json:"result"`
			IsError bool   `json:"is_error"`
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
		}
	}}
	if err := execAgent(ctx, cfg, q, prompt, StreamArgs, lw); err != nil {
		return "", false, err
	}
	lw.flush()
	if failed {
		return "", false, errors.New("the agent reported an error")
	}
	text = strings.TrimSpace(result)
	if text == "" {
		text = strings.TrimSpace(acc.String())
	}
	if text == "" {
		return "", false, errors.New("the agent gave no answer")
	}
	return strings.ToValidUTF8(text, ""), acc.Len() > 0, nil
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
