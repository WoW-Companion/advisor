-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testchar"] = {
      schema = "1.2",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testchar",
      sections = {
        { id = "next_quests", lines = {
          { text = "Turn in Wolves at the Door (Dunemark)", quest = 100 },
          { text = "Gather Peacebloom (Dunemark)", quest = 101 },
        } },
        { id = "upgrades", lines = {
          { text = "head: Cloth Hood of the \"Sage\" (ilvl 20) from Near Wolf", item = 1 },
          { text = "finger1: Ring of Mind (ilvl 18) from object 900", item = 6 },
        } },
        { id = "keep_materials", lines = {
          { text = "Copper Ore x20", item = 2770 },
          { text = "Peacebloom x12", item = 2447 },
          { text = "Copper Bar x5", item = 2840 },
        } },
        { id = "notes", lines = {
          { text = "Spend 2 unspent talent points: Penance" },
          { text = "1 failed quest: abandon" },
          { text = "Bank not captured: open your bank once" },
          { text = "Dataset is build 1.59.0.63000, game is 64000" },
        } },
      },
      dungeons = {
        { name = "Wolf Den", line = "lvl 10-14: Cloth Hood of the \"Sage\"", item_id = 1 },
      },
      talents = {
        { line = "Penance rank 1", spell_id = 1001 },
        { line = "Power Word: Shield rank 1" },
        { line = "Power Word: Shield rank 2" },
      },
    },
    ["Otherchar"] = {
      schema = "1.2",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Otherchar",
      sections = {
        { id = "upgrades", lines = {
          { text = "finger1: Ring of Mind (ilvl 18) from object 900", item = 6 },
        } },
        { id = "notes", lines = {
          { text = "Dataset is build 1.59.0.63000, game is 64000" },
        } },
      },
      talents = {
        { line = "Deadly Poison rank 1", spell_id = 2001 },
      },
    },
  },
}
