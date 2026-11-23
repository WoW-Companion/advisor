-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testchar"] = {
      schema = "1.7",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testchar",
      sections = {
        { id = "next_quests", lines = {
          { text = "You have 2 unspent talent points: Penance", kind = "talent", points = 2, details = { "2 talent points to spend", "Next pick: Penance (rank 1)" }, prompt = "I'm a level 12 priest with 2 unspent talent points. My plan's next pick is Penance. Should I take it now, and why?", asks = { "Where should my talent points go?", "What does Penance do?" }, ref = "talents", why = "Your plan's next pick: Penance" },
          { text = "Turn in Wolves at the Door (Dunemark)", quest = 100, kind = "turnin", title = "Wolves at the Door", level = 11, zone = "Dunemark", details = { "In Dunemark", "A level below you" }, prompt = "I'm a level 12 priest. I can turn in the quest \"Wolves at the Door\" in Dunemark. What does turning it in unlock, and what should I do next?", asks = { "What does turning in Wolves at the Door unlock?", "What should I do after I turn it in?" }, ref = "quest:100", why = "Done — hand it in on your way" },
          { text = "Gather Peacebloom (Dunemark)", quest = 101, kind = "progress", title = "Gather Peacebloom", level = 12, zone = "Dunemark", details = { "In Dunemark", "Right for your level" }, prompt = "I'm a level 12 priest. I'm working on the quest \"Gather Peacebloom\" in Dunemark. What is the best way to finish it, and what should I watch out for?", asks = { "How do I finish Gather Peacebloom?" }, ref = "quest:101" },
        } },
        { id = "upgrades", summary = "1 upgrade you can get", lines = {
          { text = "Head: Cloth Hood of the \"Sage\". Wear now, small upgrade, instead of what you wear", item = 1, slot = "Head", ilvl = 20, wear = "Wear now", gain = "Small upgrade", replaces = "instead of what you wear", how = "Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14)", pathKind = "dungeon", details = { "Head: Cloth Hood of the \"Sage\"", "Wear now. Small upgrade, instead of what you wear", "Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14)" }, prompt = "I'm a level 12 priest. A possible upgrade is Cloth Hood of the \"Sage\" for my head slot, small upgrade (instead of what you wear). The best way to get it: Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14). Is it worth getting for how I play?", asks = { "How do I get Cloth Hood of the \"Sage\"?", "Is Cloth Hood of the \"Sage\" worth getting now?" }, ref = "item:1", why = "Small upgrade for your head" },
        } },
        { id = "keep_materials", lines = {
          { text = "Copper Ore x20", item = 2770, asks = { "Should I keep Copper Ore?", "What is Copper Ore used for?" }, ref = "item:2770" },
          { text = "Peacebloom x12", item = 2447, asks = { "Should I keep Peacebloom?", "What is Peacebloom used for?" }, ref = "item:2447" },
          { text = "Copper Bar x5", item = 2840, asks = { "Should I keep Copper Bar?", "What is Copper Bar used for?" }, ref = "item:2840" },
        } },
        { id = "notes", lines = {
          { text = "1 failed quest: abandon", kind = "note", asks = { "Should I abandon my failed quests?" }, ref = "note:failed-quests" },
          { text = "Bank not captured: open your bank once", kind = "note", asks = { "Why does my bank need opening?" }, ref = "note:bank" },
          { text = "Dataset is build 1.59.0.63000, game is 64000", kind = "note", asks = { "What does the dataset mismatch mean?" }, ref = "note:dataset-build" },
        } },
      },
      dungeons = {
        { name = "Wolf Den", line = "lvl 10-14: Cloth Hood of the \"Sage\"", item_id = 1, details = { "For levels 10 to 14, right for your level", "Near Wolf drops Cloth Hood of the \"Sage\"", "It fits your head slot" }, prompt = "I'm a level 12 priest. I'm considering the dungeon Wolf Den (levels 10 to 14). Its best upgrade for me is Cloth Hood of the \"Sage\", from Near Wolf. Is it worth running now, and how should I prepare?", asks = { "Am I ready for Wolf Den?", "What should I bring to Wolf Den?" }, ref = "dungeon:1", why = "Drops Cloth Hood of the \"Sage\" for your head slot" },
      },
      talents = {
        { line = "Penance rank 1", spell_id = 1001, details = { "Next pick: Penance, rank 1" }, prompt = "I'm a level 12 priest. My next talent pick is Penance, rank 1. Why this pick now, and what does it change?", asks = { "Where should my talent point go?", "What does Penance do?" }, ref = "talents", why = "Next pick in your plan" },
        { line = "Power Word: Shield rank 1", details = { "Next pick: Power Word: Shield, rank 1" }, prompt = "I'm a level 12 priest. My next talent pick is Power Word: Shield, rank 1. Why this pick now, and what does it change?", asks = { "Where should my talent point go?", "What does Power Word: Shield do?" }, ref = "talents", why = "Next pick in your plan" },
        { line = "Power Word: Shield rank 2", details = { "Next pick: Power Word: Shield, rank 2" }, prompt = "I'm a level 12 priest. My next talent pick is Power Word: Shield, rank 2. Why this pick now, and what does it change?", asks = { "Where should my talent point go?", "What does Power Word: Shield do?" }, ref = "talents", why = "Next pick in your plan" },
      },
    },
    ["Otherchar"] = {
      schema = "1.7",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Otherchar",
      sections = {
        { id = "notes", lines = {
          { text = "Dataset is build 1.59.0.63000, game is 64000", kind = "note", asks = { "What does the dataset mismatch mean?" }, ref = "note:dataset-build" },
        } },
      },
      talents = {
        { line = "Deadly Poison rank 1", spell_id = 2001, details = { "Next pick: Deadly Poison, rank 1" }, prompt = "I'm a level 12 rogue. My next talent pick is Deadly Poison, rank 1. Why this pick now, and what does it change?", asks = { "Where should my talent point go?", "What does Deadly Poison do?" }, ref = "talents", why = "Next pick in your plan" },
      },
    },
  },
}
