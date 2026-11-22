-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testpriest"] = {
      schema = "1.4",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testpriest",
      sections = {
        { id = "next_quests", lines = {
          { text = "You have 2 unspent talent points: Penance", kind = "talent", points = 2, details = { "You have 2 unspent talent points: Penance", "milestone; ask \"What talents should I get?\"; most points are in Discipline (5)", "2 unspent talent points" }, prompt = "I'm a level 12 priest. I have 2 unspent talent points. You have 2 unspent talent points: Penance. What should I spend them on, and why?" },
          { text = "Turn in Wolves at the Door (Dunemark)", quest = 100, kind = "turnin", title = "Wolves at the Door", level = 11, zone = "Dunemark", details = { "ready to turn in", "Zone: Dunemark", "Level 11" }, prompt = "I'm a level 12 priest. I can turn in the quest \"Wolves at the Door\" (level 11) in Dunemark. How should I approach it, and what should I know before I go?" },
          { text = "Gather Peacebloom (Dunemark)", quest = 101, kind = "progress", title = "Gather Peacebloom", level = 12, zone = "Dunemark", details = { "level 12 quest, you are 12", "Zone: Dunemark", "Level 12" }, prompt = "I'm a level 12 priest. I'm working on the quest \"Gather Peacebloom\" (level 12) in Dunemark. How should I approach it, and what should I know before I go?" },
        } },
        { id = "upgrades", lines = {
          { text = "head: Cloth Hood of the \"Sage\" (ilvl 20) from Near Wolf", item = 1, slot = "head", ilvl = 20, details = { "head: Cloth Hood of the \"Sage\", ilvl 20", "Item level +4 over your current item", "From Near Wolf (creature)" }, prompt = "I'm a level 12 priest. A possible upgrade is Cloth Hood of the \"Sage\" for my head slot (item level 20), replacing your current item (item level 16), from Near Wolf. How do I get it, and is it worth the effort for my build?" },
          { text = "finger1: Ring of Mind (ilvl 18) from object 900", item = 6, slot = "finger1", ilvl = 18, details = { "finger1: Ring of Mind, ilvl 18", "Your slot is empty", "From object 900 (object)" }, prompt = "I'm a level 12 priest. A possible upgrade is Ring of Mind for my finger1 slot (item level 18), from object 900. How do I get it, and is it worth the effort for my build?" },
        } },
        { id = "keep_materials", lines = {
          { text = "Copper Ore x20", item = 2770 },
          { text = "Peacebloom x12", item = 2447 },
          { text = "Copper Bar x5", item = 2840 },
        } },
        { id = "notes", lines = {
          { text = "1 failed quest: abandon", kind = "note" },
          { text = "Bank not captured: open your bank once" },
          { text = "Dataset is build 1.59.0.63000, game is 64000" },
        } },
      },
      dungeons = {
        { name = "Wolf Den", line = "lvl 10-14: Cloth Hood of the \"Sage\"", item_id = 1, details = { "lvl 10-14", "Near Wolf drops Cloth Hood of the \"Sage\"", "Slot: head" }, prompt = "I'm a level 12 priest. I'm considering the dungeon Wolf Den (lvl 10-14). Its top upgrade for me is Cloth Hood of the \"Sage\", from Near Wolf. Is it worth running now, and how should I prepare?" },
      },
      talents = {
        { line = "Penance rank 1", spell_id = 1001, details = { "Next pick: Penance, rank 1", "Why: Core heal" }, prompt = "I'm a level 12 priest. The next talent point in my plan is Penance to rank 1. The plan says: Core heal. Is this the right pick for me now, and what does it change?" },
        { line = "Power Word: Shield rank 1", details = { "Next pick: Power Word: Shield, rank 1", "Why: Shield" }, prompt = "I'm a level 12 priest. The next talent point in my plan is Power Word: Shield to rank 1. The plan says: Shield. Is this the right pick for me now, and what does it change?" },
        { line = "Power Word: Shield rank 2", details = { "Next pick: Power Word: Shield, rank 2", "Why: Shield" }, prompt = "I'm a level 12 priest. The next talent point in my plan is Power Word: Shield to rank 2. The plan says: Shield. Is this the right pick for me now, and what does it change?" },
      },
    },
  },
}
