// Package character reads characters captured by ForeverScribe
// (https://github.com/MattMaximo/foreverscribe, MIT) into a typed model.
//
// Fields ForeverScribe did not capture are nil (or an empty string for text), never invented.
package character

import "strings"

type EquippedItem struct {
	Slot          string
	ItemID        *int
	Enchant       *int
	Link          string
	Name          string
	ItemLevel     *int
	Quality       *int
	Durability    *int
	DurabilityMax *int
	// Stats is the item's own stats as the addon recorded them (agi, sta, armor, ...); nil when it did not. Enchants are not in it.
	Stats map[string]float64
}

type ItemStack struct {
	ItemID int
	Count  int
	Name   string
	Link   string
	// The addon's optional per-item fields; nil or empty when it did not record them.
	Quality   *int
	ItemLevel *int
	ReqLevel  *int
	EquipLoc  string
	Suffix    string
	// Enchant is the enchant id on the item; nil when none or not recorded.
	Enchant *int
	// Stats is the item's own stats as the addon recorded them; nil when it did not.
	Stats map[string]float64
}

type Objective struct {
	Text string
	Have *int
	Need *int
	Done *bool
	Type string
}

type Quest struct {
	QuestID    int
	Title      string
	Level      *int
	Zone       string
	Complete   *bool
	Failed     *bool
	Objectives []Objective
	// Observed marks a quest the addon saw accepted after the capture: it is in the log, but its
	// objectives are not in the capture.
	Observed bool
	// Elite is set when the addon flagged the quest elite (a group quest); nil or false when not.
	Elite *bool
}

// QuestPOI is one captured quest map point: a map, a position as percent of that map, and when
// the client gave it (0 when not recorded).
type QuestPOI struct {
	Map    int
	X, Y   float64
	SeenAt int64
}

// Spot is one place an NPC was seen (map percent) and when.
type Spot struct {
	X, Y *float64
	T    *int64
	N    *int
}

// NPC is one NPC sighting (ForeverScribe npcs["<npcID>:<uiMapID>"]). Kind is "quest" or "gossip".
type NPC struct {
	ID      int
	Name    string
	MapID   *int
	Zone    string
	Subzone string
	X, Y    *float64
	Kind    string
	First   *int64
	Seen    *int64 // t: last interaction, epoch seconds
	Count   *int   // n: interactions
	Spots   []Spot
}

// Location is where the character stood at the last save (ForeverScribe snapshot.location). X and
// Y are percent of the map (0 to 100) and nil when the client gave no position.
type Location struct {
	Zone    string
	Subzone string
	MapName string
	MapID   *int
	X, Y    *float64
}

type Talent struct {
	Name    string
	Group   string
	Rank    int
	MaxRank *int
	NodeID  *int
	SpellID *int
	Type    string // node type as captured, "" when absent
	CanBuy  *bool  // nil when the capture does not say
}

// TalentGroup is one named branch of a tree (for example "Discipline") and its spent points.
type TalentGroup struct {
	Name  string
	Order *int
	Spent int
}

type TalentTree struct {
	Name      string
	Key       string // "professions" for a profession tree; "" for a class tree
	Points    *int   // points spent in the tree
	Unspent   *int   // currency.unspent
	MaxPoints *int   // currency.max
	MaxAvail  *int   // currency.maxAvailable
	Groups    []TalentGroup
	Talents   []Talent
}

// IsProfession reports whether the tree is a profession talent tree, not a class tree.
func (t TalentTree) IsProfession() bool { return strings.EqualFold(t.Key, "professions") }

type Talents struct {
	System  string
	Loadout string
	Unspent *int // class talent points (class tree currencies, else the top-level field)
	Trees   []TalentTree
}

// ProfessionUnspent is the unspent points over the profession trees; 0 when there are none.
func (t *Talents) ProfessionUnspent() int {
	n := 0
	if t != nil {
		for _, tr := range t.Trees {
			if tr.IsProfession() && tr.Unspent != nil && *tr.Unspent > 0 {
				n += *tr.Unspent
			}
		}
	}
	return n
}

type Skill struct {
	Name    string
	SkillID *int
	Rank    *int
	Max     *int
	Group   string
	// FromRecipes marks a profession known only because the character's recipes for it were recorded;
	// its skill level is not captured.
	FromRecipes bool
}

type Reputation struct {
	FactionID int
	Name      string
	Standing  *int
	Value     *int
	Min       *int
	Max       *int
	AtWar     *bool
	Group     string
}

// Client is the game client that wrote the capture.
type Client struct {
	Version string
	Build   string
	TOC     string
}

func (c Client) Empty() bool { return c == Client{} }

type Character struct {
	Key        string // "Realm:Name"; the name alone when the realm is empty
	Name       string
	Realm      string
	Level      *int
	Class      string
	Race       string
	Faction    string
	Money      *int
	XP         *int
	XPMax      *int
	RestedXP   *int // experience points (GetXPExhaustion), not a percentage
	CapturedAt *int64
	Addon      string
	Client     Client
	Location   *Location // nil when the capture holds none
	Hearth     string    // the hearthstone's bind location name
	// HearthHave, HearthReadyIn (seconds left when captured) and HearthCooldown (seconds, the full cooldown)
	// are nil in a capture made before the addon read them; see HearthWait.
	HearthHave     *bool
	HearthReadyIn  *int
	HearthCooldown *int
	// Source says which capture the character came from: SourceWoWC, SourceFS or both joined.
	Source string

	Equipment    map[string]EquippedItem // nil when not captured
	Bags         []ItemStack
	BagsCaptured bool
	Bank         []ItemStack
	BankCaptured bool // false until the player has opened the bank with ForeverScribe running

	NPCs           []NPC // nil when the capture holds no npcs table
	Quests         []Quest
	QuestsCaptured bool
	// QuestPOI is the point the client's quest map gave for a quest in the log, by quest id: the
	// area of its current objective. Nil when the capture holds none.
	QuestPOI        map[int]QuestPOI
	CompletedQuests []int
	// PlanPrefs are the player's plan settings (WoWCompanionDB.characters[key].planPrefs); nil means defaults.
	PlanPrefs      *PlanPrefs
	Talents        *Talents
	Skills         []Skill
	SkillsCaptured bool
	Professions    []Skill // each profession once
	Reputations    []Reputation

	// Profile is the player's own, kept locally (internal/profile) and attached after the
	// capture is read; nil when the player has set none. They are never part of a capture.
	Profile *Profile
	// Seasonal is the player's holiday-event setting, kept with the profile: "" (off), "on" or
	// "on:<event keys>" (see questie.Seasonal).
	Seasonal string
	// KnownSpells are the spell ids the character knows (a recipe's spell id included), when a capture
	// lists them; nil when it does not.
	KnownSpells map[int64]bool
	// Recipes are the recipes the client's own trade skill window listed, by profession (lower-case);
	// nil when none were recorded. A recipe listed here is known.
	Recipes map[string]SeenRecipes
	// Totals are the stat totals the addon records (WoWCompanionDB.characters[key].stats), attached with AttachTotals;
	// nil when none.
	Totals *Totals
	// Seen is what the player saw in game (WoWCompanionDB.observed): trainers, vendors, gathering and
	// quest givers; nil when nothing was recorded. These "seen by you" facts come before the dataset.
	Seen *Observed
}

// Profile is a character's play style: role (tank, healer, damage or balanced), focus (leveling,
// dungeons, pvp or mixed), the spec tree the player chose.
type Profile struct {
	Role  string `json:"role"`
	Focus string `json:"focus"`
	Spec  string `json:"spec,omitempty"`
	// StatPref is the optional stat preference: balanced, damage or survival ("" is balanced).
	StatPref string `json:"stat_pref,omitempty"`
}

// File is everything one SavedVariables file holds.
type File struct {
	Addon      string
	Schema     *int
	Client     Client
	Characters []Character
	// HasWoWC and HasFS say which capture files were read (WoWCompanionDB.characters, ForeverScribeDB).
	HasWoWC, HasFS bool
}

// CapturedAt is the newest capture time over all characters.
func (f File) CapturedAt() *int64 {
	var best *int64
	for _, c := range f.Characters {
		if c.CapturedAt != nil && (best == nil || *c.CapturedAt > *best) {
			t := *c.CapturedAt
			best = &t
		}
	}
	return best
}

// ClientOf is the client block of one character, falling back to the file's.
func (f File) ClientOf(c Character) Client {
	if !c.Client.Empty() {
		return c.Client
	}
	return f.Client
}

// DefaultLevelReach is how many levels above the player an unfinished quest may be and still be planned.
const DefaultLevelReach = 3

// PlanPrefs are the player's plan settings. LevelReach is 0 to 10, or -1 for no limit; Hidden are the
// quests the player dismissed; IncludeElite plans elite (group) quests.
type PlanPrefs struct {
	LevelReach   int
	Hidden       map[int]bool
	IncludeElite bool
}

// Reach is the level reach in effect: the default when the character has no prefs, -1 for off.
func (c Character) Reach() int {
	if c.PlanPrefs == nil {
		return DefaultLevelReach
	}
	return c.PlanPrefs.LevelReach
}

// IsHidden reports whether the player hid the quest from the plan.
func (c Character) IsHidden(id int) bool { return c.PlanPrefs != nil && c.PlanPrefs.Hidden[id] }

// ElitesIncluded reports whether the player plans elite quests.
func (c Character) ElitesIncluded() bool { return c.PlanPrefs != nil && c.PlanPrefs.IncludeElite }

// HearthWait says whether the character has a hearthstone to use and, if so, the seconds until it is ready
// at time now (unix seconds): the captured wait aged by the time since the capture. A capture without the
// hearthstone fields counts as present and ready when a bind is set; no bind or no stone is not usable.
func (c Character) HearthWait(now int64) (wait int, usable bool) {
	if strings.TrimSpace(c.Hearth) == "" || (c.HearthHave != nil && !*c.HearthHave) {
		return 0, false
	}
	if c.HearthReadyIn == nil {
		return 0, true
	}
	wait = *c.HearthReadyIn
	if c.CapturedAt != nil && now > *c.CapturedAt {
		wait -= int(now - *c.CapturedAt)
	}
	return max(wait, 0), true
}
