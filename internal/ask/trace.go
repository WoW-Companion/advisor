package ask

import (
	"encoding/json"
	"strings"
	"sync"
)

// Entity is something an answer's tools touched: a quest, NPC, item, zone or recipe, with its display name.
type Entity struct {
	Kind string `json:"kind"`
	ID   int64  `json:"id,omitempty"`
	Name string `json:"name"`
}

// maxPerResult bounds how many entities one tool result may add, so a long list does not flood the card.
const maxPerResult = 3

// Trace collects the entities seen in one ask's tool calls and results, oldest first.
type Trace struct {
	mu    sync.Mutex
	tools map[string]string // tool_use id -> tool name
	ents  []Entity
	saved string // the last "Saved: ..." line a successful set_character_profile returned
}

// Use records a tool call (stream-json "tool_use") so its result can be matched to the tool.
func (t *Trace) Use(id, tool string) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if t.tools == nil {
		t.tools = map[string]string{}
	}
	t.tools[id] = tool
}

// Result records the text a tool returned for the call id.
func (t *Trace) Result(id, text string) {
	t.mu.Lock()
	tool := t.tools[id]
	t.mu.Unlock()
	t.Add(tool, text)
}

// Add records the text a named tool returned.
func (t *Trace) Add(tool, text string) {
	es := EntitiesIn(tool, text)
	note := ""
	if strings.Contains(tool, "set_character_profile") {
		note = SavedNote(text)
	}
	t.mu.Lock()
	t.ents = append(t.ents, es...)
	if note != "" {
		t.saved = note
	}
	t.mu.Unlock()
}

// Saved is the line a successful set_character_profile call returned in this ask ("" when none did).
func (t *Trace) Saved() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.saved
}

// SavedNote is the saved_note field of a set_character_profile result, "" when it is not there.
func SavedNote(text string) string {
	i := strings.Index(text, "{")
	if i < 0 {
		return ""
	}
	var v struct {
		Note string `json:"saved_note"`
	}
	if json.Unmarshal([]byte(text[i:]), &v) != nil {
		return ""
	}
	return v.Note
}

// WithSaved ends the answer with the saved note, once; the note comes from the tool, not the model.
func WithSaved(answer, note string) string {
	if note == "" || strings.Contains(answer, note) {
		return answer
	}
	return strings.TrimRight(answer, " \n") + "\n\n" + note
}

// Entities is what was seen, oldest first.
func (t *Trace) Entities() []Entity {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]Entity(nil), t.ents...)
}

func toolKind(tool string) string {
	tool = strings.ToLower(tool)
	switch {
	case strings.Contains(tool, "quest"):
		return "quest"
	case strings.Contains(tool, "npc"):
		return "npc"
	case strings.Contains(tool, "recipe"):
		return "recipe"
	case strings.Contains(tool, "dungeon"):
		return "zone"
	case strings.Contains(tool, "item"), strings.Contains(tool, "drops"):
		return "item"
	}
	return ""
}

var (
	nameKeys = []string{"name", "title", "quest_name", "npc_name", "item_name"}
	idKeys   = []struct{ key, kind string }{
		{"quest_id", "quest"}, {"questId", "quest"}, {"npc_id", "npc"}, {"npcId", "npc"},
		{"item_id", "item"}, {"itemId", "item"}, {"spell_id", "recipe"}, {"id", ""}, {"entry", ""},
	}
)

// EntitiesIn finds the entities in one tool result: JSON objects with a display name and an id (or a
// tool that names its kind), and a "zone" name. At most maxPerResult, in the order they appear.
func EntitiesIn(tool, text string) []Entity {
	i := strings.IndexAny(text, "{[")
	if i < 0 {
		return nil
	}
	var v any
	if json.Unmarshal([]byte(text[i:]), &v) != nil {
		return nil
	}
	var out []Entity
	base := toolKind(tool)
	var walk func(v any)
	walk = func(v any) {
		if len(out) >= maxPerResult {
			return
		}
		switch x := v.(type) {
		case []any:
			for _, e := range x {
				walk(e)
			}
		case map[string]any:
			var name string
			for _, k := range nameKeys {
				if s, ok := x[k].(string); ok && strings.TrimSpace(s) != "" {
					name = strings.TrimSpace(s)
					break
				}
			}
			var id int64
			kind := base
			for _, k := range idKeys {
				if n, ok := x[k.key].(float64); ok && n > 0 {
					id = int64(n)
					if k.kind != "" {
						kind = k.kind
					}
					break
				}
			}
			if name != "" && kind != "" && (id > 0 || base != "") && len(out) < maxPerResult {
				out = append(out, Entity{Kind: kind, ID: id, Name: name})
			}
			if z, ok := x["zone"].(string); ok && strings.TrimSpace(z) != "" && len(out) < maxPerResult {
				out = append(out, Entity{Kind: "zone", Name: strings.TrimSpace(z)})
			}
			for _, k := range sortedKeys(x) {
				switch x[k].(type) {
				case []any, map[string]any:
					walk(x[k])
				}
			}
		}
	}
	walk(v)
	return out
}

func sortedKeys(m map[string]any) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	for i := 1; i < len(ks); i++ {
		for j := i; j > 0 && ks[j] < ks[j-1]; j-- {
			ks[j], ks[j-1] = ks[j-1], ks[j]
		}
	}
	return ks
}
