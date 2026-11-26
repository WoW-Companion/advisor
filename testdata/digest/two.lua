-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testchar"] = {
      schema = "1.10",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testchar",
      sections = {
        { id = "next_quests", lines = {
          { text = "Spend 2 talent points: Penance (Discipline)", kind = "talent", points = 2, details = { "2 talent points to spend", "Next pick: Penance (rank 1)" }, asks = { "Why does the Penance talent suit my play style?", "What changes in play once I take the Penance talent?" }, ref = "talents", why = "Your plan's next pick: Penance" },
          { text = "Turn in Wolves at the Door (Dunemark)", quest = 100, kind = "turnin", title = "Wolves at the Door", level = 11, zone = "Dunemark", details = { "In Dunemark", "Quest status: ready to turn in now", "A level below you" }, asks = { "What comes after I hand in Wolves at the Door?", "Should I finish anything nearby before handing in Wolves at the Door?" }, ref = "quest:100", why = "Done — hand it in on your way" },
          { text = "Finish Gather Peacebloom (Dunemark)", quest = 101, kind = "progress", title = "Gather Peacebloom", level = 12, zone = "Dunemark", details = { "In Dunemark", "Quest status: not ready to turn in yet", "Right for your level" }, asks = { "What should I do alongside Gather Peacebloom to save time?", "What does Gather Peacebloom lead to?" }, ref = "quest:101" },
        } },
        { id = "upgrades", summary = "1 upgrade you can get", lines = {
          { text = "Cloth Hood of the \"Sage\"", item = 1, slot = "Head", ilvl = 20, wear = "Wear now", gain = "Small upgrade", how = "Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14)", pathKind = "dungeon", details = { "Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14)" }, asks = { "How do I get Cloth Hood of the \"Sage\" from Near Wolf in Wolf Den?", "How much does Cloth Hood of the \"Sage\" help my Discipline priest?", "Will Cloth Hood of the \"Sage\" stay my best pick for the head slot?" }, ref = "item:1" },
        } },
        { id = "keep_materials", lines = {
          { text = "Copper Ore x20", item = 2770, asks = { "Will I need Copper Ore for a craft soon?", "Is Copper Ore worth selling instead?" }, ref = "item:2770" },
          { text = "Peacebloom x12", item = 2447, asks = { "Will I need Peacebloom for a craft soon?", "Is Peacebloom worth selling instead?" }, ref = "item:2447" },
          { text = "Copper Bar x5", item = 2840, asks = { "Will I need Copper Bar for a craft soon?", "Is Copper Bar worth selling instead?" }, ref = "item:2840" },
        } },
        { id = "notes", lines = {
          { text = "1 failed quest: abandon", kind = "note", asks = { "Should I abandon my failed quests?" }, ref = "note:failed-quests" },
          { text = "Bank not captured: open your bank once", kind = "note", asks = { "Why does my bank need opening?" }, ref = "note:bank" },
          { text = "Dataset is build 1.59.0.63000, game is 64000", kind = "note", asks = { "What does the dataset mismatch mean?" }, ref = "note:dataset-build" },
        } },
      },
      dungeons = {
        { name = "Wolf Den", line = "lvl 10-14: Cloth Hood of the \"Sage\"", item_id = 1, details = { "For levels 10 to 14, right for your level", "Near Wolf drops Cloth Hood of the \"Sage\"", "It fits your head slot" }, asks = { "What should I bring to Wolf Den?", "Should I run Wolf Den solo or wait for a group?" }, ref = "dungeon:1", why = "Drops Cloth Hood of the \"Sage\" for your head slot" },
      },
      talents = {
        { line = "Penance rank 1", spell_id = 1001, details = { "Next pick: Penance, rank 1" }, asks = { "Why does the Penance talent suit my play style?", "What changes in play once I take the Penance talent?" }, ref = "talents", why = "Core heal" },
        { line = "Power Word: Shield rank 1", details = { "Next pick: Power Word: Shield, rank 1" }, asks = { "Why does the Power Word: Shield talent suit my play style?", "What changes in play once I take the Power Word: Shield talent?" }, ref = "talents", why = "Shield" },
        { line = "Power Word: Shield rank 2", details = { "Next pick: Power Word: Shield, rank 2" }, asks = { "Why does the Power Word: Shield talent suit my play style?", "What changes in play once I take the Power Word: Shield talent?" }, ref = "talents", why = "Shield" },
      },
    },
    ["Otherchar"] = {
      schema = "1.10",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Otherchar",
      sections = {
        { id = "notes", lines = {
          { text = "Dataset is build 1.59.0.63000, game is 64000", kind = "note", asks = { "What does the dataset mismatch mean?" }, ref = "note:dataset-build" },
        } },
      },
      talents = {
        { line = "Deadly Poison rank 1", spell_id = 2001, details = { "Next pick: Deadly Poison, rank 1" }, asks = { "Why does the Deadly Poison talent suit my play style?", "What changes in play once I take the Deadly Poison talent?" }, ref = "talents", why = "Next pick in your build" },
      },
    },
  },
}
