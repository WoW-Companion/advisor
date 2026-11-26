-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testpriest"] = {
      schema = "1.10",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testpriest",
      sections = {
        { id = "next_quests", lines = {
          { text = "You have 2 unspent talent points: Penance", kind = "talent", points = 2 },
          { text = "Class quest: pick up The Skybreaker's Bulwark from Seena Sk...", quest = 900, kind = "pickup", title = "The Skybreaker's Bulwark", level = 10, class = true, npc = "Seena Skyward", zone = "Dunemark" },
          { text = "Turn in \"Blood\" Tithe: Part 2 (Dunemark)", quest = 901, kind = "turnin", title = "\"Blood\" Tithe: Part 2", level = 9, elite = true, npc = "Aamelia O'Windfield", zone = "Dunemark" },
          { text = "0/6 Windsong Crawler Meat; +2 more (Dunemark)", quest = 902, kind = "progress", title = "Crawler Season: A Very Long Quest Title That Is Never Truncated In The New Fields", level = 12, zone = "Dunemark", objectives = { { text = "Windsong Crawler Meat", have = 0, need = 6 }, { text = "Find Aamelia Windfield", have = 0, need = 1 }, { text = "Optional: Bring Tea" } } },
          { text = "Pick up: Wolves at the Door (Dunemark)", quest = 903, kind = "pickup", title = "Wolves at the Door", level = 11, zone = "Dunemark" },
          { text = "Game data not downloaded yet; see Setup", kind = "note" },
        } },
        { id = "upgrades", summary = "1 upgrade you can get", lines = {
          { text = "Head: Cloth Hood. From level 14, big upgrade, fills an empty slot", item = 1, slot = "Head", ilvl = 20, atLevel = 14, wear = "From level 14", gain = "Big upgrade", replaces = "fills an empty slot", how = "Drops sometimes from Near Wolf in Wolf Den (levels 10 to 14)", pathKind = "dungeon" },
        } },
      },
    },
  },
}
