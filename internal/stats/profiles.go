// Package stats is the stat compass: per class, spec and role stat priorities (our own curated
// table), a plain-words stat check of a character against them, and the weights every other feature
// (upgrade ranking, Auction House suffixes) shares, so the advice agrees everywhere.
//
// THE WEIGHTS ARE BALLPARK. They say how much a stat type matters to a play style, from 0 to 1, as a
// guide for a leveling or dungeon character, not a simulation. Players differ; the advice says so.
package stats

import "strings"

// Stat names, as the profile tables and the gear weights spell them.
const (
	Strength    = "strength"
	Agility     = "agility"
	Stamina     = "stamina"
	Intellect   = "intellect"
	Spirit      = "spirit"
	Armor       = "armor"
	Dodge       = "dodge"
	Parry       = "parry"
	Block       = "block"
	AttackPower = "attack_power"
	Crit        = "crit"
)

// Primary are the stats an item carries and a total can be compared across.
var Primary = []string{Strength, Agility, Stamina, Intellect, Spirit}

// Roles.
const (
	RoleDamage = "damage"
	RoleTank   = "tank"
	RoleHealer = "healer"
)

// Stat preferences, stored with the play style.
const (
	PrefBalanced = "balanced"
	PrefDamage   = "damage"
	PrefSurvival = "survival"
)

// Prefs are the allowed stat preferences with the words the player sees.
var Prefs = []struct{ Value, Label string }{
	{PrefBalanced, "Balanced"}, {PrefDamage, "More damage"}, {PrefSurvival, "More survivability"},
}

// Profile is one stat priority list.
type Profile struct {
	Class   string // class token, "WARRIOR"
	Spec    string // tree name; empty for the class's fallback
	Role    string
	Arch    string // which reasons apply
	Weights map[string]float64
}

func w(kv ...any) map[string]float64 {
	m := map[string]float64{}
	for i := 0; i+1 < len(kv); i += 2 {
		switch v := kv[i+1].(type) {
		case float64:
			m[kv[i].(string)] = v
		case int:
			m[kv[i].(string)] = float64(v)
		}
	}
	return m
}

// Archetypes pick the reasons.
const (
	archStr    = "melee-strength"
	archAgi    = "melee-agility"
	archRanged = "ranged-agility"
	archCaster = "caster"
	archHealer = "healer"
	archTank   = "tank"
	archFeral  = "feral"
)

// table is the curated fact table. The first profile of a class and role is that role's default; the
// class's balanced fallback is the first profile of the class.
var table = []Profile{
	{"WARRIOR", "Fury", RoleDamage, archStr, w(Strength, 1, Agility, 0.8, Stamina, 0.4)},
	{"WARRIOR", "Arms", RoleDamage, archStr, w(Strength, 1, Agility, 0.6, Stamina, 0.5)},
	{"WARRIOR", "Protection", RoleTank, archTank, w(Stamina, 1, Strength, 0.6, Agility, 0.5, Armor, 0.8, Dodge, 0.5, Parry, 0.5, Block, 0.5)},

	{"PALADIN", "Retribution", RoleDamage, archStr, w(Strength, 1, Agility, 0.4, Stamina, 0.5, Intellect, 0.3)},
	{"PALADIN", "Protection", RoleTank, archTank, w(Stamina, 1, Strength, 0.5, Agility, 0.3, Intellect, 0.3, Armor, 0.8, Dodge, 0.4, Parry, 0.4, Block, 0.6)},
	{"PALADIN", "Holy", RoleHealer, archHealer, w(Intellect, 1, Stamina, 0.4, Spirit, 0.3)},

	{"HUNTER", "Marksmanship", RoleDamage, archRanged, w(Agility, 1, Stamina, 0.4, Intellect, 0.3, Strength, 0.1)},
	{"HUNTER", "Beast Mastery", RoleDamage, archRanged, w(Agility, 1, Stamina, 0.5, Intellect, 0.3, Strength, 0.1)},
	{"HUNTER", "Survival", RoleDamage, archRanged, w(Agility, 1, Stamina, 0.5, Intellect, 0.3, Strength, 0.1)},

	{"ROGUE", "Combat", RoleDamage, archAgi, w(Agility, 1, Strength, 0.6, Stamina, 0.4)},
	{"ROGUE", "Assassination", RoleDamage, archAgi, w(Agility, 1, Strength, 0.5, Stamina, 0.4)},
	{"ROGUE", "Subtlety", RoleDamage, archAgi, w(Agility, 1, Strength, 0.5, Stamina, 0.4)},

	{"PRIEST", "Holy", RoleHealer, archHealer, w(Intellect, 1, Spirit, 0.8, Stamina, 0.4)},
	{"PRIEST", "Discipline", RoleHealer, archHealer, w(Intellect, 1, Spirit, 0.7, Stamina, 0.4)},
	{"PRIEST", "Shadow", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.5, Spirit, 0.4)},

	{"SHAMAN", "Enhancement", RoleDamage, archStr, w(Strength, 1, Agility, 0.6, Stamina, 0.5, Intellect, 0.3)},
	{"SHAMAN", "Elemental", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.5, Spirit, 0.2)},
	{"SHAMAN", "Restoration", RoleHealer, archHealer, w(Intellect, 1, Spirit, 0.4, Stamina, 0.5)},

	{"MAGE", "Fire", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.5, Spirit, 0.3)},
	{"MAGE", "Frost", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.5, Spirit, 0.3)},
	{"MAGE", "Arcane", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.4, Spirit, 0.4)},

	{"WARLOCK", "Affliction", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.7, Spirit, 0.3)},
	{"WARLOCK", "Destruction", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.6, Spirit, 0.2)},
	{"WARLOCK", "Demonology", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.7, Spirit, 0.3)},

	{"DRUID", "Balance", RoleDamage, archCaster, w(Intellect, 1, Stamina, 0.5, Spirit, 0.5)},
	{"DRUID", "Feral Combat", RoleDamage, archFeral, w(Agility, 1, Strength, 0.9, Stamina, 0.5)},
	{"DRUID", "Feral Combat", RoleTank, archTank, w(Stamina, 1, Agility, 0.8, Strength, 0.4, Armor, 0.8, Dodge, 0.6)},
	{"DRUID", "Restoration", RoleHealer, archHealer, w(Intellect, 1, Spirit, 0.8, Stamina, 0.4)},
}

// Classes are the class tokens the table covers.
func Classes() []string {
	var out []string
	for _, p := range table {
		if len(out) == 0 || out[len(out)-1] != p.Class {
			out = append(out, p.Class)
		}
	}
	return out
}

// reasons: plain one-line reasons, per archetype and stat, in our own words.
var reasons = map[string]map[string]string{
	archStr: {
		Strength:  "more attack power, so every hit lands harder",
		Agility:   "more critical hits, which feed your rage",
		Stamina:   "more health, safer pulls",
		Intellect: "a little more mana for your utility spells",
		Spirit:    "it does almost nothing for you",
	},
	archAgi: {
		Agility:   "more attack power and critical hits",
		Strength:  "more attack power, so every hit lands harder",
		Stamina:   "more health, safer pulls",
		Intellect: "it does little for your energy",
		Spirit:    "it does almost nothing for you",
	},
	archRanged: {
		Agility:   "more ranged attack power and critical hits",
		Stamina:   "more health, safer pulls",
		Intellect: "more mana for longer fights",
		Strength:  "it does little for a ranged attacker",
		Spirit:    "it does almost nothing for you",
	},
	archFeral: {
		Agility:   "more attack power and critical hits in form",
		Strength:  "more attack power, so every hit lands harder",
		Stamina:   "more health, safer pulls",
		Intellect: "it does little in form",
		Spirit:    "it does almost nothing in form",
	},
	archCaster: {
		Intellect: "more mana and spell crits, so you cast longer and harder",
		Stamina:   "more health, safer pulls",
		Spirit:    "faster mana back between fights",
		Strength:  "it does nothing for spells",
		Agility:   "it does little for spells",
	},
	archHealer: {
		Intellect: "more mana, so you can keep healing",
		Spirit:    "mana back faster while you heal",
		Stamina:   "more health, so you survive a stray hit",
		Strength:  "it does nothing for healing",
		Agility:   "it does little for healing",
	},
	archTank: {
		Stamina:   "more health, safer pulls",
		Strength:  "more block and attack power for holding enemies",
		Agility:   "more armor and dodge, so fewer hits land",
		Armor:     "soaks up part of every hit",
		Dodge:     "some hits miss you entirely",
		Parry:     "some hits are turned aside",
		Block:     "each block trims a hit",
		Intellect: "a little more mana for your abilities",
		Spirit:    "it does almost nothing for you",
	},
}

// Names are the words for stats in player text.
var names = map[string]string{
	Strength: "Strength", Agility: "Agility", Stamina: "Stamina", Intellect: "Intellect", Spirit: "Spirit",
	Armor: "armor", Dodge: "dodge", Parry: "parry", Block: "block",
}

// Name is a stat as a player says it.
func Name(stat string) string { return names[stat] }

// Reason is the one-line plain reason a stat matters (or not) to a profile.
func (p Profile) Reason(stat string) string { return reasons[p.Arch][stat] }

// Label is the profile's short name: the spec, or the class for a fallback.
func (p Profile) Label() string {
	if p.Spec != "" {
		return p.Spec
	}
	return ClassWord(p.Class)
}

// ClassWord is "Warrior" for "WARRIOR".
func ClassWord(class string) string {
	if class == "" {
		return ""
	}
	l := strings.ToLower(class)
	return strings.ToUpper(l[:1]) + l[1:]
}

// Lookup picks the profile for a class, a spec the player chose (a tree name, or a talent group name
// that contains one) and a role. The spec wins when it names a profile of the class; else the role's
// first profile; else the class's balanced fallback (its first profile). ok is false for a class
// the table does not know.
func Lookup(class, spec, role string) (Profile, bool) {
	class = strings.ToUpper(strings.TrimSpace(class))
	spec = strings.ToLower(strings.TrimSpace(spec))
	role = strings.ToLower(strings.TrimSpace(role))
	var first, byRole *Profile
	var bySpec *Profile
	for i := range table {
		p := &table[i]
		if p.Class != class {
			continue
		}
		if first == nil {
			first = p
		}
		if spec != "" && specMatches(spec, strings.ToLower(p.Spec)) && (role == "" || role == "balanced" || p.Role == role) && bySpec == nil {
			bySpec = p
		}
		if byRole == nil && p.Role == role {
			byRole = p
		}
	}
	switch {
	case bySpec != nil:
		return *bySpec, true
	case byRole != nil:
		out := *byRole
		out.Spec = "" // the player named no matching tree: a role profile, labelled by the class
		return out, true
	case first != nil:
		out := *first
		out.Spec = ""
		return out, true
	}
	return Profile{}, false
}

// ForClass is the balanced fallback of a class.
func ForClass(class string) (Profile, bool) { return Lookup(class, "", "") }

// Adjust returns the weights with a stat preference applied: "damage" lifts the damage stats and
// eases the survival ones, "survival" the reverse. Weights stay within 0 to 1, and the primary stat
// (the one the profile weights highest) always keeps the lead: a preference moves the other stats
// up or down next to it but never past it.
func Adjust(weights map[string]float64, pref string) map[string]float64 {
	lead, lw := "", -1.0
	for _, k := range append(append([]string{}, Primary...), Armor, Dodge, Parry, Block) {
		if v, ok := weights[k]; ok && v > lw {
			lead, lw = k, v
		}
	}
	out := make(map[string]float64, len(weights))
	for k, v := range weights {
		f := 1.0
		def := k == Stamina || k == Armor || k == Dodge || k == Parry || k == Block
		switch pref {
		case PrefDamage:
			if def {
				f = 0.7
			} else {
				f = 1.15
			}
		case PrefSurvival:
			if def {
				f = 1.5
			} else {
				f = 0.85
			}
		}
		out[k] = min(v*f, 1)
	}
	if lead != "" {
		out[lead] = weights[lead]
		for k, v := range out {
			if k != lead {
				out[k] = min(v, weights[lead]*leadMargin)
			}
		}
	}
	return out
}

// leadMargin keeps every other weight at most this share of the primary stat's.
const leadMargin = 0.95

// specMatches: "test discipline" and "feral" both match their trees.
func specMatches(spec, tree string) bool {
	return strings.Contains(spec, tree) || strings.Contains(tree, spec)
}
