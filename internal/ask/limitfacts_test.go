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
		"where do I buy the Cat Carrier (Bombay)?", "what abilities does my hunter's pet learn?", "what is the title of the quest in Goldshire?",
		"where do I buy a skinning knife?", "do I need a mining pick to mine copper?", "how do I lure the guards away?",
	} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}
