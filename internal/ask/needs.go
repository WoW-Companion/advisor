package ask

import (
	"regexp"
	"sort"
	"strings"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/profile"
	"github.com/WoW-Companion/advisor/internal/talents"
)

// NeedsPlayStyle is the Needs value of a question that cannot be answered before the character has a
// play style.
const NeedsPlayStyle = "play_style"

// NeedsSpec is the Needs value of a talent question for a character that has a role and focus but no
// spec: the card has one button per talent tree and nothing else.
const NeedsSpec = "spec"

// SpecNote is the line under the spec buttons.
const SpecNote = "Saved for this character. Change it any time in Settings → Play Style."

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
	Hook       string   `json:"hook,omitempty"` // for a stat question: "Choose a play style for a stat check"
	Roles      []Option `json:"roles"`
	Activities []Option `json:"activities"`
	Specs      []string `json:"specs,omitempty"` // the class's trees, optional
	Note       string   `json:"note,omitempty"`  // NeedsSpec: the line under the buttons
	Role       string   `json:"role,omitempty"`  // the saved role and focus the choice is saved with; an empty one is still to be asked
	Focus      string   `json:"focus,omitempty"`
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
	return TalentQuestion(q.Text) || q.Ref == "talents" || StatQuestion(q)
}

var statWords = regexp.MustCompile(`(?i)\b(stats?|strength|agility|stamina|intellect|spirit)\b`)

// StatQuestion reports whether a question is about the character's stats (the stat check's subject).
func StatQuestion(q Question) bool {
	return q.Ref == "stats" || statWords.MatchString(q.Text)
}

// PlayStyleNeeded is the choices to put to the player instead of an AI call, when q needs a play style
// the character has not saved; ok is false when it does not, or the character cannot be read (the
// agent then works as before).
func PlayStyleNeeded(cfg Config, q Question) (Needs, bool) {
	if !needsStyle(q) {
		return Needs{}, false
	}
	cc, ok := load(cfg, q.Character)
	if !ok {
		return Needs{}, false
	}
	p := cc.c.Profile
	if p != nil && !profile.Incomplete(p) {
		return specNeeded(cc, q, p)
	}
	n := NeedsFor(cc.c, q)
	if p != nil { // one of role and focus is "Not set": ask for only that one
		n.Role, n.Focus = p.Role, p.Focus
		if p.Role != "" {
			n.Roles = nil
		}
		if p.Focus != "" {
			n.Activities = nil
		}
	}
	if specs := talents.Specs(cc.ds, cc.c); len(specs) > 0 {
		n.Specs = specs
	}
	return n, true
}

// NeedsFor builds the choices for c.
func NeedsFor(c character.Character, q Question) Needs {
	n := Needs{Needs: NeedsPlayStyle, Character: q.Character, Question: q.Text, Ref: q.Ref,
		Roles:      options(profile.Roles, nil),
		Activities: options(profile.Foci, activityLabels)}
	if StatQuestion(q) && !TalentQuestion(q.Text) && q.Ref != "talents" {
		n.Hook = "Choose a play style for a stat check"
	}
	if c.Talents != nil {
		for _, t := range c.Talents.Trees {
			if !t.IsProfession() {
				n.Specs = append(n.Specs, t.Name)
			}
		}
	}
	return n
}

// specNeeded is the spec-only card: a talent question, a saved role and focus, no spec that names one of
// the class's trees. No AI call is made for it.
func specNeeded(cc loaded, q Question, p *character.Profile) (Needs, bool) {
	if !(TalentQuestion(q.Text) || q.Ref == "talents") {
		return Needs{}, false
	}
	specs := talents.Specs(cc.ds, cc.c)
	if len(specs) == 0 || talents.MatchSpec(specs, p.Spec) != "" {
		return Needs{}, false
	}
	return Needs{Needs: NeedsSpec, Character: q.Character, Question: q.Text, Ref: q.Ref, Specs: OrderSpecs(specs, p.Role),
		Note: SpecNote, Role: p.Role, Focus: p.Focus}, true
}

var tankTrees = map[string]bool{"protection": true, "feral combat": true, "feral": true, "blood": true}

// damageTankTrees are tank trees that are also a damage spec: a Feral Combat druid tanks as a bear and deals damage as a cat.
var damageTankTrees = map[string]bool{"feral combat": true, "feral": true}
var healTrees = map[string]bool{"holy": true, "restoration": true, "discipline": true}

// OrderSpecs puts the trees that fit the role first: tank trees for a tank, healing trees for a healer,
// the rest, and a hybrid tree such as Feral Combat, for damage. Every tree is still offered; the order is otherwise kept.
func OrderSpecs(specs []string, role string) []string {
	rank := func(s string) int {
		l := strings.ToLower(s)
		switch role {
		case "tank":
			if tankTrees[l] {
				return 0
			}
		case "healer":
			if healTrees[l] {
				return 0
			}
		default:
			if (!tankTrees[l] || damageTankTrees[l]) && !healTrees[l] {
				return 0
			}
		}
		return 1
	}
	out := append([]string(nil), specs...)
	sort.SliceStable(out, func(i, j int) bool { return rank(out[i]) < rank(out[j]) })
	return out
}
