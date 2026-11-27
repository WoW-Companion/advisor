package ask

import (
	"strings"
	"testing"

	"github.com/WoW-Companion/advisor/internal/store"
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

func TestLimitFactsSecondarySkills(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how much does a Heavy Silk Bandage heal?", "First Aid bandages:"},
		{"how long does bandaging take and what is Recently Bandaged?", "First Aid bandages:"},
		{"what does Strong Anti-Venom cure?", "First Aid bandages:"},
		{"can I bandage in combat?", "First Aid bandages:"},
		{"what lockpicking skill do I need for a Large Iron Lockbox?", "Lockpicking:"},
		{"when does a rogue learn Pick Lock?", "Lockpicking:"},
		{"what is in a Battered Junkbox?", "Lockpicking:"},
		{"do I need a cooking fire to cook?", "Cooking fires and food effects:"},
		{"what does Well Fed give?", "Cooking fires and food effects:"},
		{"how much health does Smoked Bear Meat restore?", "Cooking fires and food effects:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{
		"which bandage should I make to level First Aid?", "how much does a Silk Bandage cost?", "what skill do I need to make Heavy Silk Bandage?", "where do I learn Expert First Aid?",
		"how long will it take to level First Aid?", "which food should I cook to reach 150 Cooking?", "where is the Cooking trainer?",
	} {
		if got := LimitFacts(q); strings.Contains(got, "First Aid bandages:") || strings.Contains(got, "Lockpicking:") || strings.Contains(got, "Cooking fires and food effects:") {
			t.Errorf("LimitFacts(%q) = %q, want no secondary skill line", q, got)
		}
	}
}

func TestLimitFactsQuestDifficulty(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how much XP does this quest give?", "Quest XP rewards:"},
		{"how much experience do I get from that quest?", "Quest XP rewards:"},
		{"what XP will the quest reward at level 30?", "Quest XP rewards:"},
		{"is this an elite quest?", "Elite and group quests:"},
		{"how many players does this quest need?", "Elite and group quests:"},
		{"what is the suggested group size for the quest?", "Elite and group quests:"},
		{"can I solo this quest?", "Elite and group quests:"},
		{"what is the minimum level for this quest?", "Quest minimum level:"},
		{"what level do I need to pick up the quest?", "Quest minimum level:"},
		{"how low a level can I be to accept that quest?", "Quest minimum level:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"what quest should I do next?", "which quests are grey for me?", "how much XP do I need for level 30?", "is Westfall Stew worth it?", "what color is a level 30 quest for me?", "which quests should I do for XP?", "what level is the quest Tharil'zun?", "which dungeon quests can I turn in?"} {
		if got := LimitFacts(q); got != "" {
			t.Errorf("LimitFacts(%q) = %q, want none", q, got)
		}
	}
}

func TestLimitFactsRareDropAndSetBonus(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"where does the rare elite Gruff Swiftbite spawn?", "Rare spawns and named elites:"},
		{"how long is the respawn timer of Hogger?", "Rare spawns and named elites:"},
		{"what zone has the named elite with the best drop?", "Rare spawns and named elites:"},
		{"what is the set bonus of Dreadmist Raiment?", "Item set bonuses:"},
		{"what does the 8 piece Battlegear of Might give?", "Item set bonuses:"},
		{"is Silk Cloth a world drop?", "Drop chances and world drops:"},
		{"what is the drop chance of Skullsplitter Tusk?", "Drop chances and world drops:"},
		{"what zone is the rare Gruff Swiftbite in?", "Rare spawns and named elites:"},
		{"where do I find rares in Westfall?", "Rare spawns and named elites:"},
		{"how many people do I need for the elite Hogger?", "Rare spawns and named elites:"},
		{"what level is the boss Edwin VanCleef?", "Rare spawns and named elites:"},
		{"what triggers the spawn of Mor'Ladim?", "Rare spawns and named elites:"},
		{"is the rare always up?", "Rare spawns and named elites:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"what is a rare quality item worth?", "which quest should I do next?", "where do I buy Silk Cloth?", "is this an elite quest?", "what gear should I wear at level 40?", "where do I find rare herbs?", "where can I find elite gear?", "what level for the elite quests in Stonetalon?"} {
		got := LimitFacts(q)
		for _, topic := range []string{"Rare spawns and named elites:", "Item set bonuses:", "Drop chances and world drops:"} {
			if strings.Contains(got, topic) {
				t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, topic)
			}
		}
	}
}

func TestLimitFactsEscortTimedItemStartAndRaceQuests(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"how do I do the escort quest in the Barrens?", "Escort quests:"},
		{"is there a timer on the quest in Wetlands?", "Timed quests:"},
		{"which timed quests can I do at level 30?", "Timed quests:"},
		{"which quest does Heart of Zeal start?", "Quests that start from an item:"},
		{"which drops start quests at level 30?", "Quests that start from an item:"},
		{"which quests are Horde only in Ashenvale?", "Race, class and faction limits on quests:"},
		{"are there quests only orcs can take?", "Race, class and faction limits on quests:"},
		{"is this quest race-specific?", "Race, class and faction limits on quests:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"what quests are in the Barrens?", "where does the Skullsplitter Tusk drop?", "which Alliance races can be a paladin?", "when does the Darkmoon Faire start?"} {
		got := LimitFacts(q)
		for _, topic := range []string{"Escort quests:", "Timed quests:", "Quests that start from an item:", "Race, class and faction limits on quests:"} {
			if strings.Contains(got, topic) {
				t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, topic)
			}
		}
	}
}

func TestLimitFactsRidingFlightHearthstone(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"where do I learn to ride as a gnome?", "Riding and mounts:"},
		{"which races can ride a kodo?", "Riding and mounts:"},
		{"what does it cost to fly from Stormwind to Ironforge?", "Travel times:"},
		{"how much is the tram from Ironforge to Stormwind?", "Travel times:"},
		{"how often can I use my hearthstone?", "Travel times:"},
		{"what is my hearthstone's CD?", "Travel times:"},
		{"where is the flight master in Stormwind?", "Flight paths:"},
		{"how do I unlock the Thelsamar flight path?", "Flight paths:"},
		{"where do I get a new hearthstone if I lost mine?", "Hearthstone:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"how do I ride the zeppelin to Undercity?", "how long is the boat ride to Auberdine?", "which quest do I do next in Westfall?"} {
		if got := LimitFacts(q); strings.Contains(got, "Riding and mounts:") || strings.Contains(got, "Flight paths:") || strings.Contains(got, "Hearthstone:") {
			t.Errorf("LimitFacts(%q) = %q, want no riding, flight path or hearthstone fact", q, got)
		}
	}
}

func TestLimitFactsClassReagentSpellPhrasings(t *testing.T) {
	const topic = "Class reagents, soul shards, totems and rituals:"
	for _, q := range []string{
		"do I need reagents for Rebirth?", "what do I need to cast Vanish?", "what does a paladin need for Divine Intervention?", "what do shamans need for Reincarnation?",
		"do I need an ankh to resurrect myself?", "what do I need for Gift of the Wild?", "do mages need reagents for Arcane Brilliance?", "what do I need to summon a felhunter?",
	} {
		if got := LimitFacts(q); !strings.Contains(got, topic) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", q, got, topic)
		}
	}
	for _, q := range []string{"where do I buy flash powder?", "what do I need to level Alchemy?", "what do I need to craft Elixir of Fortitude?", "what do I need to summon my party to the dungeon?"} {
		if got := LimitFacts(q); strings.Contains(got, topic) {
			t.Errorf("LimitFacts(%q) = %q, want no %s", q, got, topic)
		}
	}
	for _, q := range []string{"what does a hunter need to start the pet quest?", "where is the bear form quest?", "who gives the totem quests?"} {
		if got := LimitFacts(q); !strings.Contains(got, "Class quest chains:") {
			t.Errorf("LimitFacts(%q) = %q, want the class quest line", q, got)
		}
	}
}

func TestLimitFactsWhereToLevelTerritoryCities(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"where should I level at level 25?", "Zone levels:"},
		{"what zone should I quest in at level 30 as a horde?", "Zone levels:"},
		{"where is a good place to grind at level 40?", "Zone levels:"},
		{"which zones are good for a level 18 alliance character?", "Zone levels:"},
		{"where should I go next, I'm level 35", "Zone levels:"},
		{"what is the best zone for level 12 horde?", "Zone levels:"},
		{"is Ashenvale contested territory for the Horde?", "Zone territory:"},
		{"can I quest in Hillsbrad Foothills as Alliance?", "Zone territory:"},
		{"is Stranglethorn Vale hostile for me?", "Zone territory:"},
		{"which zones are Horde territory?", "Zone territory:"},
		{"is Redridge Mountains an Alliance zone?", "Zone territory:"},
		{"will I get attacked by guards in The Barrens?", "Zone territory:"},
		{"where is the nearest capital city for me?", "Capital city services:"},
		{"which city is closest to The Barrens for the Horde?", "Capital city services:"},
		{"is there a neutral auction house?", "Capital city services:"},
		{"where's the post office in Undercity?", "Capital city services:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"where do I level my mining?", "where do I level up my first aid to 150?", "what quests should I do at level 20?", "where can I mine copper in The Barrens as a horde?", "I am level 30 in Duskwood, what quests are there?"} {
		if got := LimitFacts(q); strings.Contains(got, "Zone levels:") || strings.Contains(got, "Zone territory:") {
			t.Errorf("LimitFacts(%q) = %q, want no zone level or territory fact", q, got)
		}
	}
}

func TestLimitFactsGearLevelOfAnInstance(t *testing.T) {
	for _, q := range []string{
		"what is the recommended gear level for Blackrock Depths?",
		"what item level do I need for Molten Core?",
		"what ilvl for Scholomance?",
		"what gear score for BWL?",
		"is a good gear score enough for Naxxramas?",
	} {
		if got := LimitFacts(q); !strings.Contains(got, "Dungeon difficulty and modes:") || !strings.Contains(got, "Say so") {
			t.Errorf("LimitFacts(%q) = %q, want the dungeon difficulty fact", q, got)
		}
	}
	for _, q := range []string{"what item level is the helm from Deadmines?", "what is the item level of the Deadmines drops?", "can I use an item level 30 helm in Deadmines?"} {
		if got := LimitFacts(q); strings.Contains(got, "Dungeon difficulty and modes:") {
			t.Errorf("LimitFacts(%q) = %q, want no dungeon difficulty fact: an item's level is in the data", q, got)
		}
	}
}

func TestLimitFactsQuestGiverTurnInPrerequisite(t *testing.T) {
	for _, tc := range []struct{ q, want string }{
		{"who gives the quest Tharil'zun?", "Quest givers and where a quest starts:"},
		{"where do I pick up the quest Beren's Peril?", "Quest givers and where a quest starts:"},
		{"where does the quest The Essence of Aku'Mai start?", "Quest givers and where a quest starts:"},
		{"where is the quest giver in Goldshire?", "Quest givers and where a quest starts:"},
		{"where do I turn in Vital Intelligence?", "Quest turn-in:"},
		{"who do I hand in Fields of Grief to?", "Quest turn-in:"},
		{"which NPC takes the quest Deliveries to Sven?", "Quest turn-in:"},
		{"what is the prerequisite quest for Elixir of Suffering?", "Quest prerequisites, follow-ups and breadcrumbs:"},
		{"what do I need to finish before Arugal Must Die?", "Quest prerequisites, follow-ups and breadcrumbs:"},
		{"is there a breadcrumb quest to Razorfen Kraul?", "Quest prerequisites, follow-ups and breadcrumbs:"},
		{"what quest leads to Battle of Hillsbrad?", "Quest prerequisites, follow-ups and breadcrumbs:"},
		{"what is the follow-up to Fields of Grief?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	} {
		if got := LimitFacts(tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") || !strings.Contains(got, "from memory") {
			t.Errorf("LimitFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"what quests are in the Barrens?", "what quest should I do next?", "which quest do I do next in Westfall?", "which dungeon quests can I turn in?", "where do I buy Silk Cloth?",
		"which quest does Heart of Zeal start?", "what comes after Murloc Gills in the Toxic Soil quest line?", "what level is the quest Tharil'zun?"} {
		got := LimitFacts(q)
		for _, h := range []string{"Quest givers and where", "Quest turn-in:", "Quest prerequisites,"} {
			if strings.Contains(got, h) {
				t.Errorf("LimitFacts(%q) has %q", q, h)
			}
		}
	}
}

func TestNamedQuestFacts(t *testing.T) {
	ds := store.NewDir("../../testdata/places_snapshot")
	for _, tc := range []struct{ q, want string }{
		{"what is the minimum level for Dusky Crab Cakes?", "Quest minimum level:"},
		{"what level do I need to start Tharil'zun?", "Quest minimum level:"},
		{"who gives Beren's Peril?", "Quest givers and where a quest starts:"},
		{"who takes Vital Intelligence?", "Quest turn-in:"},
		{"what do I need before Arugal Must Die?", "Quest prerequisites, follow-ups and breadcrumbs:"},
	} {
		if got := NamedQuestFacts(ds, tc.q); !strings.Contains(got, tc.want) || !strings.Contains(got, "Say so") {
			t.Errorf("NamedQuestFacts(%q) = %q, want %q", tc.q, got, tc.want)
		}
	}
	for _, q := range []string{"what is the minimum level for Scarlet Monastery?", "what level is the quest Tharil'zun?", "what comes after Murloc Gills in the Toxic Soil quest line?", "who gives the quest Tharil'zun?", "what quests are in the Barrens?"} {
		if got := NamedQuestFacts(ds, q); got != "" {
			t.Errorf("NamedQuestFacts(%q) = %q, want none (LimitFacts or the quest line answers it, or no quest is named)", q, got)
		}
	}
	if got := NamedQuestFacts(nil, "what is the minimum level for Dusky Crab Cakes?"); got != "" {
		t.Errorf("NamedQuestFacts(nil store) = %q, want none", got)
	}
}
