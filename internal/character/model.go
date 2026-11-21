// Package character reads characters captured by ForeverScribe
// (https://github.com/MattMaximo/foreverscribe, MIT) into a typed model.
//
// Fields ForeverScribe did not capture are nil (or an empty string for text), never invented.
package character

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

type Talent struct {
	Name    string
	Group   string
	Rank    int
	MaxRank *int
	NodeID  *int
	SpellID *int
}

// TalentGroup is one named branch of a tree (for example "Discipline") and its spent points.
type TalentGroup struct {
	Name  string
	Order *int
	Spent int
}

type TalentTree struct {
	Name      string
	Points    *int // points spent in the tree
	Unspent   *int // currency.unspent
	MaxPoints *int // currency.max
	Groups    []TalentGroup
	Talents   []Talent
}

type Talents struct {
	System  string
	Loadout string
	Unspent *int // across the whole capture (tree currencies, else the top-level field)
	Trees   []TalentTree
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

	Equipment    map[string]EquippedItem // nil when not captured
	Bags         []ItemStack
	BagsCaptured bool
	Bank         []ItemStack
	BankCaptured bool // false until the player has opened the bank with ForeverScribe running

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
