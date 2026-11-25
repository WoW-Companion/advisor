package levelup

import (
	"bytes"
	"strings"
	"testing"

	"github.com/parquet-go/parquet-go"

	"github.com/WoW-Companion/advisor/internal/character"
	"github.com/WoW-Companion/advisor/internal/questie"
	"github.com/WoW-Companion/advisor/internal/store"
)

type spellRow struct {
	ClassID    int32  `parquet:"class_id"`
	SpellID    int32  `parquet:"spell_id"`
	Name       string `parquet:"name"`
	Rank       *int32 `parquet:"rank,optional"`
	Level      int32  `parquet:"level"`
	Trained    bool   `parquet:"trained"`
	Superseded *int32 `parquet:"superseded_spell_id,optional"`
}

type offerRow struct {
	NpcID  int32 `parquet:"npc_id"`
	Spell  int32 `parquet:"spell_id"`
	Copper int64 `parquet:"cost_copper"`
}

func tbl[T any](t *testing.T, rows []T) store.SizedReaderAt {
	t.Helper()
	var buf bytes.Buffer
	if err := parquet.Write(&buf, rows); err != nil {
		t.Fatal(err)
	}
	return store.SizedReaderAt{ReaderAt: bytes.NewReader(buf.Bytes()), Size: int64(buf.Len())}
}

func i32(n int32) *int32 { return &n }

// A warrior's abilities: Rend rank 2 (level 10) supersedes rank 1 (level 4); Charge is granted, not trained.
func ds(t *testing.T, withCosts bool) store.DataStore {
	src := store.ReaderSource{"class_spells": tbl(t, []spellRow{
		{1, 772, "Rend", i32(1), 4, true, nil},
		{1, 6546, "Rend", i32(2), 10, true, i32(772)},
		{1, 355, "Taunt", nil, 10, true, nil},
		{1, 100, "Charge", nil, 10, false, nil},
		{1, 72, "Shield Bash", i32(1), 12, true, nil},
		{1, 78, "Heroic Strike", i32(2), 12, true, nil},
		{5, 585, "Smite", i32(2), 12, true, nil},
	})}
	if withCosts {
		src["trainer_spells"] = tbl(t, []offerRow{{9, 72, 9000}, {9, 78, 0}, {9, 6546, 3000}})
	}
	return store.New(src)
}

func warrior(level int, money *int) character.Character {
	return character.Character{Class: "WARRIOR", Faction: "Alliance", Level: &level, Money: money}
}

func names(sp []Spell) string {
	var n []string
	for _, s := range sp {
		n = append(n, s.Label())
	}
	return strings.Join(n, ",")
}

func TestSpellsExcludeKnownAndSupersededAndGranted(t *testing.T) {
	d := ds(t, false)
	if got := names(Spells(warrior(10, nil), d, 1, 12)); got != "Rend (Rank 2),Taunt,Heroic Strike (Rank 2),Shield Bash (Rank 1)" {
		t.Errorf("window 1-12 = %s", got)
	}
	c := warrior(10, nil)
	c.KnownSpells = map[int64]bool{355: true, 6546: true}
	if got := names(Spells(c, d, 1, 12)); got != "Heroic Strike (Rank 2),Shield Bash (Rank 1)" {
		t.Errorf("known left out = %s", got)
	}
	if got := names(Spells(warrior(10, nil), d, 12, 12)); strings.Contains(got, "Smite") {
		t.Errorf("another class leaked: %s", got)
	}
}

func TestCostTotalsWithUnknownCosts(t *testing.T) {
	sp := Spells(warrior(10, nil), ds(t, true), 10, 12)
	total, unknown := Total(sp)
	// Rend 2 = 3000, Shield Bash = 9000, Taunt and Heroic Strike (a zero offer) are unknown.
	if total != 12000 || unknown != 2 {
		t.Errorf("total %d unknown %d", total, unknown)
	}
	if _, unk := Total(Spells(warrior(10, nil), ds(t, false), 10, 12)); unk != 4 {
		t.Errorf("without trainer_spells unknown = %d", unk)
	}
}

func TestMoneyShort(t *testing.T) {
	low, high := 5000, 20000
	if s := MoneyLine(warrior(10, &low), 12000); !strings.Contains(s, "70s short") || !strings.Contains(s, "Bags review") {
		t.Errorf("short line = %q", s)
	}
	if s := MoneyLine(warrior(10, &high), 12000); s != "" {
		t.Errorf("enough money: %q", s)
	}
	if s := MoneyLine(warrior(10, nil), 12000); s != "" {
		t.Errorf("unknown money: %q", s)
	}
}

func index() *questie.Index {
	return &questie.Index{DB: "t",
		Areas: map[int]questie.Area{1: {Zone: "Valanaar"}, 2: {Zone: "Far Hold"}, 3: {Zone: "Stormwind City"}},
		Npcs: map[int]questie.Entity{
			10: {Name: "Horde Drill Sergeant", Sub: "Warrior Trainer", Faction: "H", Spawns: []questie.Spawn{{Area: 1, X: 20, Y: 20}}},
			11: {Name: "Sergeant Brand", Sub: "Warrior Trainer", Faction: "A", Spawns: []questie.Spawn{{Area: 3, X: 40, Y: 50}}},
			12: {Name: "Captain Rhea", Sub: "Warrior Trainer", Faction: "A", Spawns: []questie.Spawn{{Area: 1, X: 60, Y: 61}}},
			13: {Name: "Mage Mentor", Sub: "Mage Trainer", Faction: "A", Spawns: []questie.Spawn{{Area: 1, X: 1, Y: 1}}},
			14: {Name: "Old Veteran", Sub: "Quest Giver", Spawns: []questie.Spawn{{Area: 2, X: 33, Y: 44}}},
		},
		Quests: map[int]*questie.Quest{
			500: {ID: 500, Name: "Stance of Battle", Level: 10, MinLevel: 10, Classes: 1, Starters: []questie.Ref{{ID: 14}}, Objective: "Find the stance"},
			501: {ID: 501, Name: "Berserker Bond", Level: 14, MinLevel: 14, Classes: 1, Starters: []questie.Ref{{ID: 14}}},
			502: {ID: 502, Name: "Far Future", Level: 30, MinLevel: 30, Classes: 1, Starters: []questie.Ref{{ID: 14}}},
			503: {ID: 503, Name: "Mage Rite", Level: 10, MinLevel: 10, Classes: 128, Starters: []questie.Ref{{ID: 14}}},
			504: {ID: 504, Name: "Open To All", Level: 10, MinLevel: 10, Starters: []questie.Ref{{ID: 14}}},
		}}
}

func TestNearestClassTrainerForFactionAndZone(t *testing.T) {
	x, y := 55.0, 60.0
	c := warrior(10, nil)
	c.Location = &character.Location{Zone: "Valanaar", X: &x, Y: &y}
	tr, ok := NearestTrainer(c, index())
	if !ok || tr.Name != "Captain Rhea" || tr.Zone != "Valanaar" {
		t.Errorf("trainer = %+v %v", tr, ok)
	}
	c.Location = nil
	if tr, _ := NearestTrainer(c, index()); tr.Name == "Horde Drill Sergeant" || tr.Name == "Mage Mentor" {
		t.Errorf("wrong faction or class: %+v", tr)
	}
	if _, ok := NearestTrainer(c, nil); ok {
		t.Error("trainer without a QuestieDB")
	}
}

func TestClassQuestsByMaskAndLevel(t *testing.T) {
	var got []string
	for _, q := range ClassQuests(warrior(10, nil), index(), 5) {
		got = append(got, q.Name)
		if q.Name == "Berserker Bond" && (q.OpensAt != 14 || q.Giver != "Old Veteran" || q.Zone != "Far Hold") {
			t.Errorf("soon quest = %+v", q)
		}
	}
	if strings.Join(got, ",") != "Stance of Battle,Berserker Bond" {
		t.Errorf("class quests = %v", got)
	}
}

func TestFactsCoverTotalMoneyTrainerAndQuests(t *testing.T) {
	low := 100
	c := warrior(10, &low)
	text, spells, quests := Facts(c, ds(t, true), index(), 11, 12)
	for _, want := range []string{"Shield Bash (Rank 1)", "costs 90s", "bring about 90s", "1 of these have no cost", "short", "Sergeant Brand", "Stance of Battle"} {
		if !strings.Contains(text, want) {
			t.Errorf("facts lack %q:\n%s", want, text)
		}
	}
	if !spells || !quests {
		t.Errorf("spells %v quests %v", spells, quests)
	}
}

func TestSeenTrainerCostsOverrideTheDatasetAndNameTheTrainer(t *testing.T) {
	d := ds(t, true)
	c := warrior(10, nil)
	c.Seen = &character.Observed{Trainers: []character.SeenTrainer{{NPC: 9, Name: "Seen Warrior Trainer", Map: 84, X: 20, Y: 30, Faction: "Alliance",
		Offers: []character.SeenOffer{{Name: "Taunt", Spell: 355, Cost: 100}}}}}
	var taunt Spell
	for _, s := range Spells(c, d, 10, 12) {
		if s.ID == 355 {
			taunt = s
		}
	}
	if taunt.Cost != 100 || !taunt.Seen {
		t.Errorf("taunt %+v", taunt)
	}
	if tr, ok := SeenClassTrainer(c); !ok || tr.Name != "Seen Warrior Trainer" {
		t.Errorf("trainer %+v", tr)
	}
	if got := Spells(warrior(10, nil), d, 10, 12); got[0].Seen {
		t.Errorf("a cost nobody saw was marked: %+v", got[0])
	}
}

func TestSeenOfferMatchedByNameAndRankReplacesCost(t *testing.T) {
	c := warrior(12, nil)
	c.Seen = &character.Observed{Trainers: []character.SeenTrainer{{Name: "Trainer Tam", Faction: "Alliance", T: 1,
		Offers: []character.SeenOffer{
			{Name: "Rend", Rank: "Rank 2", State: "available", Cost: 4200, Level: 10},
			{Name: "Shield Bash", Rank: "Rank 1", State: "unavailable", Cost: 9500, Level: 12},
			{Name: "Heroic Strike", Rank: "Rank 2", State: "available", Cost: 100, Level: 14},
			{Name: "Taunt", State: "used"},
		}}}}
	by := map[string]Spell{}
	for _, s := range Spells(c, ds(t, true), 1, 12) {
		by[s.Label()] = s
	}
	if r := by["Rend (Rank 2)"]; r.Cost != 4200 || !r.Seen || r.NeedLevel != 10 {
		t.Errorf("rend %+v", r)
	}
	if s := by["Shield Bash (Rank 1)"]; !s.Blocked || s.NeedLevel != 12 {
		t.Errorf("bash %+v", s)
	}
	if _, ok := by["Heroic Strike (Rank 2)"]; ok {
		t.Error("an offer above the character's level was listed")
	}
	if _, ok := by["Taunt"]; ok {
		t.Error("a spell the trainer lists as used is known")
	}
}

type questRow struct {
	SpellID   int32  `parquet:"spell_id"`
	QuestName string `parquet:"quest_name"`
}

// dsQuest is ds plus a stance spell (level 10, learned by quest) and a quest_rewards table naming it.
func dsQuest(t *testing.T) store.DataStore {
	src := store.ReaderSource{"class_spells": tbl(t, []spellRow{
		{1, 6546, "Rend", i32(2), 10, true, nil},
		{1, 355, "Taunt", nil, 12, true, nil},
		{1, 71, "Guard Stance", nil, 10, true, nil},
		{1, 200, "Surge", nil, 10, true, nil},
	}), "quest_rewards": tbl(t, []questRow{{71, "The Test Quest"}})}
	return store.New(src)
}

func withTrainer(c character.Character) character.Character {
	c.Seen = &character.Observed{Trainers: []character.SeenTrainer{{NPC: 9, Name: "Trainer Tam", Faction: "Alliance", T: 1,
		Offers: []character.SeenOffer{
			{Name: "Surge", State: "available", Cost: 700, Level: 10},
			{Name: "Rend", Rank: "Rank 2", State: "available", Cost: 700, Level: 10},
			{Name: "Taunt", State: "unavailable", Cost: 900, Level: 12},
		}}}}
	return c
}

func TestTrainerCaptureLimitsTheList(t *testing.T) {
	got := Spells(withTrainer(warrior(10, nil)), dsQuest(t), 10, 10)
	if names(got) != "Rend (Rank 2),Surge" {
		t.Errorf("list = %s", names(got))
	}
	for _, s := range got {
		if s.Usually || !s.Seen {
			t.Errorf("trainer spell marked as a guess: %+v", s)
		}
	}
	txt := TrainerSells(withTrainer(warrior(10, nil)), dsQuest(t))
	if !strings.Contains(txt, "sells exactly") || !strings.Contains(txt, "Taunt at level 12") || strings.Contains(txt, "Guard") {
		t.Errorf("sells = %q", txt)
	}
}

func TestQuestRewardSpellIsLabelledAndNotTrainable(t *testing.T) {
	c := warrior(10, nil)
	if n := names(Spells(c, dsQuest(t), 10, 10)); strings.Contains(n, "Guard") {
		t.Errorf("quest spell called trainable: %s", n)
	}
	q := QuestSpells(c, dsQuest(t), 10, 10)
	if len(q) != 1 || q[0].Line() != "Guard Stance: from the class quest The Test Quest" {
		t.Errorf("quest spells = %+v", q)
	}
	c.KnownSpells = map[int64]bool{71: true}
	if len(QuestSpells(c, dsQuest(t), 10, 10)) != 0 {
		t.Error("a known quest spell was listed")
	}
}

func TestKnownSpellsAreSubtractedFromTrainerList(t *testing.T) {
	c := withTrainer(warrior(10, nil))
	c.KnownSpells = map[int64]bool{200: true}
	if n := names(Spells(c, dsQuest(t), 10, 10)); n != "Rend (Rank 2)" {
		t.Errorf("list = %s", n)
	}
}

func TestEmptySpellsHedgesAndNoTrainerSaysUsually(t *testing.T) {
	c := warrior(10, nil)
	got := Spells(c, dsQuest(t), 10, 10)
	if names(got) != "Rend (Rank 2),Surge" || !got[0].Usually {
		t.Fatalf("fallback = %+v", got)
	}
	text, _, _ := Facts(c, dsQuest(t), nil, 10, 10)
	for _, want := range []string{"usually trains", "if you don't have it yet", "Guard Stance", "The Test Quest"} {
		if !strings.Contains(text, want) {
			t.Errorf("facts lack %q:\n%s", want, text)
		}
	}
	c.KnownSpells = map[int64]bool{1: true}
	if text, _, _ := Facts(c, dsQuest(t), nil, 10, 10); strings.Contains(text, "if you don't have it yet") {
		t.Errorf("hedged with known spells:\n%s", text)
	}
}

func TestQuestTeachingThreeSpellsListsAllThree(t *testing.T) {
	ds := store.New(store.ReaderSource{"class_spells": tbl(t, []spellRow{
		{1, 81, "Stance Alpha", nil, 10, true, nil},
		{1, 82, "Armor Break", nil, 10, true, nil},
		{1, 83, "Provoke", nil, 10, true, nil},
		{1, 84, "Trained Thing", nil, 10, true, nil},
	}), "quest_rewards": tbl(t, []questRow{{81, "The Test Bulwark"}, {82, "The Test Bulwark"}, {83, "The Test Bulwark"}})})
	c := warrior(10, nil)
	var got []string
	for _, q := range QuestSpells(c, ds, 10, 10) {
		got = append(got, q.Line())
	}
	want := "Armor Break: from the class quest The Test Bulwark|Provoke: from the class quest The Test Bulwark|Stance Alpha: from the class quest The Test Bulwark"
	if g := strings.Join(got, "|"); len(got) != 3 || g != want {
		t.Fatalf("quest spells = %q", g)
	}
	if n := names(Spells(c, ds, 10, 10)); n != "Trained Thing" {
		t.Errorf("trainer list = %s", n)
	}
}
