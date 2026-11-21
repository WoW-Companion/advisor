package companion

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/ask"
)

func TestSettingsApplyToTheNextAsk(t *testing.T) {
	s := New(Config{Ask: ask.Config{Command: []string{"claude"}}})
	if got := s.settings(); got.Length != "standard" || got.Model != "efficient" || !got.Usage || got.ModelNote == "" {
		t.Fatalf("defaults %+v", got)
	}
	w := httptest.NewRecorder()
	s.setSettings(w, httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"length":"brief","model":"custom","custom_model":"--bad","usage":false}`)))
	if o := s.askCfg().Options; o.Length != "brief" || o.Tier != "efficient" || s.settings().Usage {
		t.Errorf("a custom name that is not a model name must fall back: %+v", o)
	}
	s.setSettings(w, httptest.NewRequest("POST", "/api/settings", strings.NewReader(`{"length":"detailed","model":"best","usage":true}`)))
	if o := s.askCfg().Options; o.Length != "detailed" || o.Tier != "best" || !s.settings().Usage {
		t.Errorf("%+v", o)
	}
}
