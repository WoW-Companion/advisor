package provider

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
)

// turnAdapter plays scripted turns: each streams its text, and the first asks for a tool.
type turnAdapter struct{ turns [][]string }

func (a *turnAdapter) Chat(_ context.Context, req Request, onText func(string)) (Reply, error) {
	i := len(req.Messages) / 2
	var text strings.Builder
	for _, t := range a.turns[i] {
		onText(t)
		text.WriteString(t)
	}
	r := Reply{Text: text.String()}
	if i < len(a.turns)-1 {
		r.Calls = []Call{{ID: "c", Name: "server_info", Args: json.RawMessage(`{}`)}}
	}
	return r, nil
}

// Only the turn that calls no tool streams: the model's words before a tool call never reach OnText.
func TestInterimTextIsNotStreamed(t *testing.T) {
	ad := &turnAdapter{turns: [][]string{{"Sources are ", "unknown here."}, {"Final ", "answer."}}}
	var shown strings.Builder
	text, _, err := Loop{Adapter: ad, Model: "m", Prompt: "q", Tools: &fakeTools{result: "x"}, Stream: true, OnText: func(s string) { shown.WriteString(s) }}.Run(context.Background())
	if err != nil || text != "Final answer." || shown.String() != "Final answer." {
		t.Errorf("text %q, shown %q, err %v", text, shown.String(), err)
	}
}
