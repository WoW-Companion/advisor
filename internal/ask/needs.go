package ask

import (
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
)

// NeedsPlayStyle is the Needs value of a question that cannot be answered before the character has a
// play style.
const NeedsPlayStyle = "play_style"

// Option is one button: the value saved and the words shown.
type Option struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// Needs is the structured response to a question that needs a play style the character does not
// have. No AI call is made for it: the overlay shows the options as buttons, saves the choice with the
// profile store (POST /api/profile), and the question is then asked again once, automatically.
type Needs struct {
	Needs      string   `json:"needs"` // NeedsPlayStyle
	Character  string   `json:"character"`
	Question   string   `json:"question"` // the original question, as asked
	Ref        string   `json:"ref,omitempty"`
	Roles      []Option `json:"roles"`
	Activities []Option `json:"activities"`
	Specs      []string `json:"specs,omitempty"` // the class's trees, optional
}

var activityLabels = map[string]string{"leveling": "Leveling", "dungeons": "Dungeons", "pvp": "PvP", "mixed": "Mixed"}

func options(values []string, labels map[string]string) []Option {
	var out []Option
	for _, v := range values {
		l := labels[v]
		if l == "" {
			l = strings.ToUpper(v[:1]) + v[1:]
		}
		out = append(out, Option{v, l})
	}
	return out
}

// needsStyle reports whether the question is one that needs the character's play style: a talent
// question, or one about the talents entry.
func needsStyle(q Question) bool {
	return TalentQuestion(q.Text) || q.Ref == "talents"
}

// PlayStyleNeeded is the choices to put to the player instead of an AI call, when q needs a play style
// the character has not saved; ok is false when it does not, or the character cannot be read (the
// agent then works as before).
func PlayStyleNeeded(cfg Config, q Question) (Needs, bool) {
	if !needsStyle(q) {
		return Needs{}, false
	}
	cc, ok := load(cfg, q.Character)
	if !ok || cc.c.Profile != nil {
		return Needs{}, false
	}
	return NeedsFor(cc.c, q), true
}

// NeedsFor builds the choices for c.
func NeedsFor(c character.Character, q Question) Needs {
	n := Needs{Needs: NeedsPlayStyle, Character: q.Character, Question: q.Text, Ref: q.Ref,
		Roles:      options(profile.Roles, nil),
		Activities: options(profile.Foci, activityLabels)}
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			if !t.IsProfession() {
				n.Specs = append(n.Specs, t.Name)
			}
		}
	}
	return n
}
