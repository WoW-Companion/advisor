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
}

type ItemStack struct {
	ItemID int
	Count  int
	Name   string
	Link   string
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

	Equipment    map[string]EquippedItem // nil when not captured
	Bags         []ItemStack
	BagsCaptured bool
	Bank         []ItemStack
	BankCaptured bool // false until the player has opened the bank with ForeverScribe running

	NPCs            []NPC // nil when the capture holds no npcs table
	Quests          []Quest
	QuestsCaptured  bool
	CompletedQuests []int
	Talents         *Talents
	Skills          []Skill
	SkillsCaptured  bool
	Professions     []Skill // each profession once
	Reputations     []Reputation
}

// File is everything one SavedVariables file holds.
type File struct {
	Addon      string
	Schema     *int
	Client     Client
	Characters []Character
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
