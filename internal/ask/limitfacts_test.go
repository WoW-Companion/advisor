package ask

import (
	"strings"
	"testing"
)

func TestLimitFactsPvP(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"what level can I join Warsong Gulch?", "Battlegrounds:"},
		{"which battleground bracket am I in at level 37?", "Battlegrounds:"},
		{"how many honor points do I need for rank 10?", "Honor and PvP ranks:"},
		{"what does the honor vendor sell for a Knight-Captain?", "Honor and PvP ranks:"},
		{"how many Marks of Honor for the Arathi Basin reward?", "Honor and PvP ranks:"},
		{"what do I need to become Grand Marshal?", "Honor and PvP ranks:"},
		{"how much honor does a kill give?", "Honor and PvP ranks:"},
		{"what is the best pvp gear for my class?", "Honor and PvP ranks:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"where is the weapon master in the Valley of Honor?", "how do I get to the Valley of Honor, how much does the trainer charge?", "what does rank 3 Frostbolt cost?", "I am honored with Orgrimmar, what next?", "what quest should I do next?"} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}

func TestLimitFactsTravelTimes(t *testing.T) {
	for _, q := range []string{
		"what is the hearthstone cooldown?",
		"how long is the cooldown on my hearthstone?",
		"how often does the zeppelin to Orgrimmar run?",
		"what is the boat schedule from Menethil Harbor to Auberdine?",
		"how long is the boat ride from Ratchet to Booty Bay?",
		"how much does a flight path cost from Tarren Mill?",
	} {
		if got := LimitFacts(q); !strings.Contains(got, "Travel times:") || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want the travel times fact", q, got)
		}
	}
	for _, q := range []string{"where do I take the zeppelin to Undercity?", "bind my hearthstone in Goldshire, is that a good inn?", "which flight path should I take to Tarren Mill?", "how long to farm 20 boar meat?"} {
		if got := LimitFacts(q); strings.Contains(got, "Travel times:") {
			t.Errorf("LimitFacts(%q) = %q, want no travel times fact", q, got)
		}
	}
}

func TestLimitFactsRidingAndClassTrainer(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how much does Apprentice Riding cost?", "Riding and mounts:"},
		{"where is the riding trainer for orcs?", "Riding and mounts:"},
		{"do I need reputation for my racial mount?", "Riding and mounts:"},
		{"how much gold is the Black War Steed?", "Riding and mounts:"},
		{"how much does my mage trainer charge?", "Class trainer prices:"},
		{"what does the class trainer charge for my new spells?", "Class trainer prices:"},
		{"how much does it cost to train my new spells?", "Class trainer prices:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"how much does the alchemy trainer charge?", "what does rank 3 Frostbolt cost?", "how do I get to Blackrock Mountain?", "where is the weapon master in Orgrimmar?", "which spells does my mage learn at level 20?"} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}

func TestLimitFactsEventsWeatherZoneLevels(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"when does Brewfest start and what does it give?", "Holiday and world events:"},
		{"what is sold by the Winter Veil vendor?", "Holiday and world events:"},
		{"is the Darkmoon Faire in town this month?", "Holiday and world events:"},
		{"which holiday is on now?", "Holiday and world events:"},
		{"does it rain in Tanaris? what is the weather like?", "Weather and time of day:"},
		{"does the mob only spawn at night?", "Weather and time of day:"},
		{"is there a day and night cycle?", "Weather and time of day:"},
		{"what level is Duskwood for?", "Zone levels:"},
		{"which zone is right for level 20, what level range?", "Zone levels:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"how do I cast Rain of Fire?", "what do night elf hunters get?", "I am level 30 in Duskwood, what quests are there?", "what level do I learn Frostbolt?", "where do I get the Darkmoon Card trinket?", "how do I get to Winterspring?", "where is the Harvest Golem?"} {
		if got := LimitFacts(q); strings.Contains(got, "Holiday and world events:") || strings.Contains(got, "Weather and time of day:") || strings.Contains(got, "Zone levels:") {
			t.Errorf("LimitFacts(%q) = %q, want none of the event, weather or zone level facts", q, got)
		}
	}
}

func TestLimitFactsBossLockoutDifficultyRoles(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"what abilities does Ragnaros use?", "Boss abilities and strategy:"},
		{"how do I beat Onyxia, what are her phases?", "Boss abilities and strategy:"},
		{"what does Edwin VanCleef do in the Deadmines, any adds?", "Boss abilities and strategy:"},
		{"what is Nefarian's enrage timer?", "Boss abilities and strategy:"},
		{"what should I interrupt on the last boss?", "Boss abilities and strategy:"},
		{"how much health does Kel'Thuzad have?", "Boss abilities and strategy:"},
		{"when does the Molten Core lockout reset?", "Raid lockouts and resets:"},
		{"how many instances can I enter per hour?", "Raid lockouts and resets:"},
		{"am I saved to Blackwing Lair, what is my raid ID?", "Raid lockouts and resets:"},
		{"does Stratholme have a heroic mode?", "Dungeon difficulty and modes:"},
		{"how difficult is Blackrock Depths?", "Dungeon difficulty and modes:"},
		{"is Scarlet Monastery too hard for a level 38 group?", "Dungeon difficulty and modes:"},
		{"how many tanks and healers do I need for Zul'Gurub?", "Group roles and threat:"},
		{"what group composition works for Stratholme?", "Group roles and threat:"},
		{"do I need a tank and a healer for Wailing Caverns?", "Group roles and threat:"},
		{"how much threat does a warrior generate per second?", "Group roles and threat:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{
		"which boss drops Thunderfury?", "what is the last boss in the Deadmines?", "what spells does a mage learn at level 20?", "how do I kill the boar for the quest?",
		"my talents reset, what do I do?", "how long until my cooldown resets?", "which priest spec heals best?", "what level is Blackrock Depths for?",
		"what is the best tank spec for a druid?", "is a dungeon good for a level 30 priest?", "what abilities does a warrior get?", "how many people for Onyxia's Lair?",
	} {
		got := LimitFacts(q)
		for _, f := range []string{"Boss abilities and strategy:", "Raid lockouts and resets:", "Dungeon difficulty and modes:", "Group roles and threat:"} {
			if strings.Contains(got, f) {
				t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, f)
			}
		}
	}
}

func TestLimitFactsVanityTitlesAndTools(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how many companion pets can I keep?", "Companion pets and vanity items:"},
		{"does the Argent Dawn tabard give reputation?", "Companion pets and vanity items:"},
		{"what achievements can I earn?", "Achievements and titles:"},
		{"how do I get the Scarab Lord title?", "Achievements and titles:"},
		{"how much skill does the Big Iron Fishing Pole give?", "Profession tool effects:"},
		{"how much faster is a skinning knife than a dagger?", "Profession tool effects:"},
		{"how many slots does the key ring have?", "Bag and bank size:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{
		"where do I buy the Cat Carrier (Bombay)?", "what is the title of the quest in Goldshire?",
		"where do I buy a skinning knife?", "do I need a mining pick to mine copper?", "how do I lure the guards away?",
	} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}

func TestLimitFactsQuiverMailAndClassQuests(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how many slots does the Heavy Quiver have?", "Bag and bank size:"},
		{"what is the biggest ammo pouch?", "Bag and bank size:"},
		{"how many slots does a soul bag have?", "Bag and bank size:"},
		{"how long does mail stay in my mailbox?", "Mail:"},
		{"can I send gold through the mail?", "Mail:"},
		{"can I mail a soulbound item to my alt?", "Mail:"},
		{"what is the warlock class quest chain?", "Class quest chains:"},
		{"which class quests does a paladin have?", "Class quest chains:"},
		{"what is the hunter pet quest chain?", "Class quest chains:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{
		"what mail armor should my shaman buy, how much does it cost?", "can a priest wear mail?", "what is the best leather or mail for a hunter?",
		"where do I buy a quiver?", "which class can use a quiver?", "what quests are in the Barrens?", "what comes after Murloc Gills in the Toxic Soil quest line?",
	} {
		got := LimitFacts(q)
		for _, f := range []string{"Mail:", "Class quest chains:", "Bag and bank size:"} {
			if strings.Contains(got, f) {
				t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, f)
			}
		}
	}
}

func TestLimitFactsReagentsShardsTotemsRituals(t *testing.T) {
	const topic = "Class reagents, soul shards, totems and rituals:"
	for _, q := range []string{
		"how many soul shards does Summon Voidwalker use?", "how do I get more soul shards?", "what reagent does Rebirth need?",
		"how many Maple Seeds do I need for Rebirth?", "which reagent do I need for Portal: Orgrimmar?", "what is Symbol of Kings used for?",
		"how many Wild Thornroot does Gift of the Wild use?", "what does Windfury Totem do and how long does it last?", "what is the range of Mana Spring Totem?",
		"how many people does Ritual of Summoning need?", "what does the Ritual of Doom require?", "does Ritual of Summoning cost a soul shard?",
		"what is the cooldown of Grounding Totem?", "how many Ankhs should I carry for Reincarnation?",
	} {
		if got := LimitFacts(q); !strings.Contains(got, topic) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", q, got, topic)
		}
	}
	for _, q := range []string{
		"which totem should my shaman equip?", "where do I buy flash powder?", "what level do I learn Searing Totem?", "which spells does a warlock learn at level 20?",
		"where do I get the Totem of Life?", "my bags are full, what should I sell?", "can I summon my party to the dungeon?", "what is the best mage spell at level 40?",
	} {
		if got := LimitFacts(q); strings.Contains(got, topic) {
			t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, topic)
		}
	}
}

func TestLimitFactsHunterPets(t *testing.T) {
	const want = "Hunter pets, diet, abilities and taming:"
	for _, q := range []string{
		"what does my wolf eat?", "what food does my cat like?", "can my pet eat bread?", "what is the diet of a boar?", "how do I tame a pet?", "which beasts can I tame?",
		"what does Growl do for my pet?", "what rank of Bite should my pet have?", "how do I train my pet's skills?", "what abilities does my hunter's pet learn?",
		"what is my pet's happiness?", "how many stable slots does my hunter pet have?", "which family is exotic?", "which pets are best for a hunter?",
	} {
		if got := LimitFacts(q); !strings.Contains(got, want) || !strings.Contains(got, "Say so") || strings.Contains(got, "Companion pets") {
			t.Errorf("LimitFacts(%q) = %q, want %q", q, got, want)
		}
	}
	for _, q := range []string{
		"at what level do I learn Feed Pet?", "what level is Mend Pet rank 3?", "when do I get Call Pet?", "which spell does Eyes of the Beast use?",
		"what abilities does my imp learn?", "what level is Claw for a druid?", "how many companion pets can I keep?", "what is a pet battle?",
	} {
		if got := LimitFacts(q); strings.Contains(got, want) {
			t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, want)
		}
	}
}
