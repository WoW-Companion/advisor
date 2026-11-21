package companion

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
)

func TestSettingsApplyToTheNextAsk(t *testing.T) {
	s := New(Config{Ask: ask.Config{Command: []string{"claude"}}})
	if got := s.settings(); got.Length != "standard" || got.Model != "efficient" || got.Reasoning != "low" || got.Client != "claude" || len(got.Choices.Models) != 4 || !got.Usage || got.ModelNote == "" {
		t.Fatalf("defaults %+v", got)
	}
	w := httptest.NewRecorder()
	s.setSettings(w, httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"length":"brief","model":"custom","custom_model":"--bad","usage":false}`)))
	if o := s.askCfg().Options; o.Length != "brief" || o.Tier != "efficient" || s.settings().Usage {
		t.Errorf("a custom name that is not a model name must fall back: %+v", o)
	}
	s.setSettings(w, httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"length":"detailed","model":"best","reasoning":"high","usage":true}`)))
	if o := s.askCfg().Options; o.Length != "detailed" || o.Tier != "best" || o.Reasoning != "high" || !s.settings().Usage {
		t.Errorf("%+v", o)
	}
}

// The choices follow the AI client, so the overlay can offer its models and hide what it lacks.
func TestSettingsChoicesFollowTheClient(t *testing.T) {
	for cmd, want := range map[string][2]int{"claude": {4, 3}, "codex": {2, 3}, "gemini": {4, 0}, "my-agent": {0, 0}} {
		got := New(Config{Ask: ask.Config{Command: []string{cmd}}}).settings()
		if len(got.Choices.Models) != want[0] || len(got.Choices.Reasoning) != want[1] {
			t.Errorf("%s: %+v", cmd, got.Choices)
		}
	}
}
