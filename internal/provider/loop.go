package provider

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"
)

// Tools is the set of tools the model may call.
type Tools interface {
	List(ctx context.Context) ([]ToolDef, error)
	// Call runs one tool; isErr marks a tool-level failure whose text is shown to the model.
	Call(ctx context.Context, name string, args json.RawMessage) (text string, isErr bool, err error)
}

// Loop limits and inputs.
type Loop struct {
	Adapter   Adapter
	Model     string
	System    string
	Prompt    string
	Tools     Tools
	MaxTurns  int           // at most this many model calls (default 10)
	Timeout   time.Duration // for the whole loop (default 2 minutes)
	MaxResult int           // bytes kept of one tool result (default 16 KiB)
	Stream    bool
	OnText    func(string)
}

// Defaults.
const (
	DefaultMaxTurns  = 10
	DefaultTimeout   = 2 * time.Minute
	DefaultMaxResult = 16 << 10
)

// Run asks the model, runs the tools it calls and feeds the results back until it answers. It
// returns the final text and the usage summed over every call.
func (l Loop) Run(ctx context.Context) (string, Usage, error) {
	var total Usage
	if l.MaxTurns <= 0 {
		l.MaxTurns = DefaultMaxTurns
	}
	if l.Timeout <= 0 {
		l.Timeout = DefaultTimeout
	}
	if l.MaxResult <= 0 {
		l.MaxResult = DefaultMaxResult
	}
	ctx, cancel := context.WithTimeout(ctx, l.Timeout)
	defer cancel()
	var defs []ToolDef
	if l.Tools != nil {
		var err error
		if defs, err = l.Tools.List(ctx); err != nil {
			return "", total, fmt.Errorf("couldn't list the advisor's tools: %w", err)
		}
	}
	msgs := []Message{{Role: "user", Text: l.Prompt}}
	for turn := 0; turn < l.MaxTurns; turn++ {
		// A turn's text is held until the turn ends: only the turn that calls no tool is the answer,
		// and the model's words before or between tool calls are never shown.
		var held []string
		rep, err := l.Adapter.Chat(ctx, Request{Model: l.Model, System: l.System, Messages: msgs, Tools: defs, Stream: l.Stream}, func(t string) {
			held = append(held, t)
		})
		total = total.Add(rep.Usage)
		if err != nil {
			return "", total, l.explain(ctx, err)
		}
		if len(rep.Calls) == 0 {
			if l.OnText != nil {
				for _, t := range held {
					l.OnText(t)
				}
			}
			if strings.TrimSpace(rep.Text) == "" {
				return "", total, errors.New("the model gave no answer")
			}
			return rep.Text, total, nil
		}
		msgs = append(msgs, Message{Role: "assistant", Text: rep.Text, Calls: rep.Calls})
		var res []Result
		for _, c := range rep.Calls {
			text, isErr, err := l.Tools.Call(ctx, c.Name, c.Args)
			if err != nil {
				if ctx.Err() != nil {
					return "", total, l.explain(ctx, ctx.Err())
				}
				text, isErr = "tool failed: "+err.Error(), true
			}
			res = append(res, Result{CallID: c.ID, Name: c.Name, Text: capText(text, l.MaxResult), IsError: isErr})
		}
		msgs = append(msgs, Message{Role: "user", Results: res})
	}
	return "", total, fmt.Errorf("the model used all %d steps without answering", l.MaxTurns)
}

func (l Loop) explain(ctx context.Context, err error) error {
	switch {
	case errors.Is(ctx.Err(), context.DeadlineExceeded):
		return fmt.Errorf("the model took longer than %s", l.Timeout)
	case ctx.Err() != nil:
		return ctx.Err()
	}
	return err
}

// capText cuts s to max bytes at a rune boundary, saying so.
func capText(s string, max int) string {
	if len(s) <= max {
		return s
	}
	cut := s[:max]
	for len(cut) > 0 && !utf8.ValidString(cut) {
		cut = cut[:len(cut)-1]
	}
	return cut + "\n…[result cut]"
}
