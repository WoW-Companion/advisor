//go:build integration

package integration

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/store"
)

// charSpec is one synthetic character. Every id in its capture comes from the dataset; the name is
// invented and no real capture is involved.
type charSpec struct {
	name, class string
	level       int
	armor       int64 // item subclass of the armor it wears: 1 cloth, 2 leather
	mask        int64 // allowable_class_mask bit of its class
	specs       []string
	tree        bool // carries the Forever-shaped tree: talent nodes by id
}

var specs = []charSpec{
	{"Priestlow", "PRIEST", 12, 1, 1 << 4, []string{"Discipline", "Holy", "Shadow Magic"}, false},
	{"Priestmid", "PRIEST", 24, 1, 1 << 4, []string{"Discipline", "Holy", "Shadow Magic"}, true},
	{"Priesthigh", "PRIEST", 40, 1, 1 << 4, []string{"Discipline", "Holy", "Shadow Magic"}, false},
	{"Roguelow", "ROGUE", 12, 2, 1 << 3, []string{"Assassination", "Combat", "Subtlety"}, false},
	{"Roguemid", "ROGUE", 24, 2, 1 << 3, []string{"Assassination", "Combat", "Subtlety"}, false},
	{"Roguehigh", "ROGUE", 40, 2, 1 << 3, []string{"Assassination", "Combat", "Subtlety"}, false},
}

// armorSlots are the inventory types worn as armor, with the capture's slot name.
var armorSlots = []struct {
	slot int64
	name string
}{{1, "head"}, {3, "shoulder"}, {5, "chest"}, {7, "legs"}, {8, "feet"}, {10, "hands"}}

func num(v any) int64 { n, _ := v.(int64); return n }
func text(v any) string {
	s, _ := v.(string)
	return s
}

// pick returns up to n rows spread evenly through rows, so a capture does not take only the first ids.
func pick(rows []store.Row, n int) []store.Row {
	if len(rows) <= n {
		return rows
	}
	out := make([]store.Row, 0, n)
	for i := 0; i < n; i++ {
		out = append(out, rows[i*len(rows)/n])
	}
	return out
}

func lua(b *strings.Builder, v any, depth int) {
	pad := strings.Repeat("\t", depth)
	switch x := v.(type) {
	case map[string]any:
		keys := make([]string, 0, len(x))
		for k := range x {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		b.WriteString("{\n")
		for _, k := range keys {
			fmt.Fprintf(b, "%s\t[%q] = ", pad, k)
			lua(b, x[k], depth+1)
			b.WriteString(",\n")
		}
		b.WriteString(pad + "}")
	case []any:
		b.WriteString("{\n")
		for _, e := range x {
			b.WriteString(pad + "\t")
			lua(b, e, depth+1)
			b.WriteString(",\n")
		}
		b.WriteString(pad + "}")
	case string:
		fmt.Fprintf(b, "%q", x)
	case int:
		fmt.Fprint(b, x)
	case int64:
		fmt.Fprint(b, x)
	case bool:
		fmt.Fprint(b, x)
	default:
		panic(fmt.Sprintf("lua: %T", v))
	}
}

// capture builds the character's ForeverScribe file from the dataset's own rows.
func (s charSpec) capture(ds store.DataStore, build string) (string, error) {
	lvl := int64(s.level)
	items, err := ds.Filter("items", store.Range{Column: "required_level", Min: lvl - 8, Max: lvl - 3},
		store.Range{Column: "quality", Min: int64(1), Max: int64(3)})
	if err != nil {
		return "", err
	}
	usable := func(r store.Row) bool {
		m := num(r["allowable_class_mask"])
		return m == -1 || m&s.mask != 0
	}
	equip := map[string]any{}
	for _, a := range armorSlots {
		var rows []store.Row
		for _, r := range items {
			if num(r["slot"]) == a.slot && num(r["class_id"]) == 4 && num(r["subclass_id"]) == s.armor && usable(r) {
				rows = append(rows, r)
			}
		}
		if len(rows) > 0 {
			r := pick(rows, 1)[0]
			equip[a.name] = map[string]any{"itemID": num(r["id"]), "name": text(r["name"]), "ilvl": int(num(r["item_level"])), "quality": int(num(r["quality"]))}
		}
	}
	var weapons []store.Row
	for _, r := range items {
		if num(r["class_id"]) == 2 && usable(r) && (num(r["slot"]) == 13 || num(r["slot"]) == 21) {
			weapons = append(weapons, r)
		}
	}
	if len(weapons) > 0 {
		r := pick(weapons, 1)[0]
		equip["mainhand"] = map[string]any{"itemID": num(r["id"]), "name": text(r["name"]), "ilvl": int(num(r["item_level"])), "quality": int(num(r["quality"]))}
	}

	// Bags: grey junk, upgrade candidates, a recipe's reagents and an item the dataset does not know.
	var bags []any
	stack := func(r store.Row, n int) {
		bags = append(bags, map[string]any{"itemID": num(r["id"]), "name": text(r["name"]), "count": n})
	}
	grey, err := ds.Filter("items", store.Range{Column: "quality", Min: int64(0), Max: int64(0)}, store.Range{Column: "required_level", Min: int64(0), Max: lvl})
	if err != nil {
		return "", err
	}
	for _, r := range pick(grey, 4) {
		stack(r, 3)
	}
	for _, r := range pick(items, 6) {
		stack(r, 1)
	}
	bags = append(bags, map[string]any{"itemID": int64(99999999), "name": "Unlisted Thing", "count": 1})

	// Professions and the reagents of one of their recipes.
	prof := "Tailoring"
	if s.class == "ROGUE" {
		prof = "Leatherworking"
	}
	recipes, err := ds.Filter("recipes", store.Range{Column: "learn_skill", Min: int64(1), Max: int64(75)})
	if err != nil {
		return "", err
	}
	skills := []any{map[string]any{"name": prof, "skillID": 100, "rank": 40, "max": 75, "group": "Professions"}}
	for _, r := range recipes {
		if text(r["skill_line_name"]) != prof {
			continue
		}
		rs, _ := ds.Filter("recipe_reagents", store.Range{Column: "spell_id", Min: num(r["spell_id"]), Max: num(r["spell_id"])})
		for _, g := range rs {
			bags = append(bags, map[string]any{"itemID": num(g["reagent_item_id"]), "name": fmt.Sprintf("Reagent %d", num(g["reagent_item_id"])), "count": 4})
		}
		if len(rs) > 0 {
			break
		}
	}

	// Quests: real ids with a quest line and a zone, titles invented (the dataset holds none).
	qrows, err := ds.Filter("quests", store.Range{Column: "quest_line_id", Min: int64(1)}, store.Range{Column: "zone_id", Min: int64(1)})
	if err != nil {
		return "", err
	}
	qs := pick(qrows, 8)
	var quests []any
	for i, r := range qs {
		q := map[string]any{"questID": num(r["id"]), "title": fmt.Sprintf("Synthetic quest %d", num(r["id"])), "complete": i < 2, "failed": i == 7}
		if z := text(r["zone_name"]); z != "" {
			q["zone"] = z
		}
		switch {
		case i == 5:
			q["level"] = s.level - 9 // grey
		case i >= 2:
			q["level"] = s.level + i%3
		}
		quests = append(quests, q)
	}

	var groups []any
	spent := max(s.level-9, 0)
	for i, g := range s.specs {
		sp := 0
		if i == len(s.specs)-1 {
			sp = spent
		}
		groups = append(groups, map[string]any{"name": g, "order": i + 1, "spent": sp})
	}
	tree := map[string]any{"name": s.class, "points": spent, "currency": map[string]any{"spent": spent, "unspent": 1}, "groups": groups}
	if s.tree {
		// "Shadow" (the Icy Veins builds, one row per point) or "Shadow Magic" (the older banded builds).
		rows, err := ds.Filter("talent_builds", store.Range{Column: "class", Min: "Priest", Max: "Priest"}, store.Range{Column: "spec", Min: "Shadow", Max: "Shadow Magic"})
		if err != nil {
			return "", err
		}
		var ts []any
		at, total := map[int64]int{}, 0
		for _, r := range rows {
			nid := num(r["node_id"])
			n, err := ds.Filter("talent_nodes", store.Range{Column: "node_id", Min: nid, Max: nid})
			if err != nil || len(n) == 0 {
				continue
			}
			rank, gain := int(num(r["points"])), int(num(r["points"]))
			if _, perPoint := r["rank_after"]; perPoint {
				rank, gain = int(num(r["rank_after"])), 1
			}
			if i, seen := at[nid]; seen {
				ts[i].(map[string]any)["rank"] = rank
			} else {
				at[nid] = len(ts)
				ts = append(ts, map[string]any{"nodeID": nid, "spellID": num(n[0]["spell_id"]), "name": text(n[0]["name"]), "rank": rank, "max": int(num(n[0]["max_rank"])), "group": "Shadow Magic"})
			}
			if total += gain; total >= spent {
				break
			}
		}
		tree["talents"] = ts
	}
	snap := map[string]any{
		"identity": map[string]any{"name": s.name, "realm": "Testrealm", "class": s.class, "race": "Human", "faction": "Alliance",
			"client": map[string]any{"version": build, "build": build, "toc": build}},
		"level": s.level, "xp": 1000, "xpMax": 9000, "money": 54321, "capturedAt": time.Now().Unix(), "addonVersion": "0.10.1",
		"equipment": equip, "bags": map[string]any{"items": bags}, "quests": quests, "skills": skills,
		"talents": map[string]any{"system": "traits", "unspent": 1, "trees": []any{tree}},
	}
	key := "Testrealm:" + s.name
	var b strings.Builder
	b.WriteString("-- Synthetic capture built from dataset ids. Not a real character.\nForeverScribeDB = ")
	lua(&b, map[string]any{"schema": 1, "addonVersion": "0.10.1", "settings": map[string]any{}, "chars": map[string]any{key: map[string]any{"key": key, "snapshot": snap}}}, 0)
	b.WriteString("\n")
	return b.String(), nil
}

func writeCaptures(dir string, ds store.DataStore, build string) (map[string]string, error) {
	out := map[string]string{}
	for _, s := range specs {
		body, err := s.capture(ds, build)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", s.name, err)
		}
		p := filepath.Join(dir, s.name+".lua")
		if err := os.WriteFile(p, []byte(body), 0o644); err != nil {
			return nil, err
		}
		out[s.name] = p
	}
	return out, nil
}
