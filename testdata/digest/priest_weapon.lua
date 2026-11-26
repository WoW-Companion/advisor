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
          { text = "Train Staves at Hanashi in Orgrimmar (10s)", kind = "training", npc = "Hanashi", zone = "Orgrimmar", x = 50.0, y = 60.0, details = { "Weapon Master Hanashi, Orgrimmar, Valley of Honor", "Unlocks Gnarled Quarterstaff, an upgrade in your bags or from a quest", "One flight from the Crossroads" }, asks = { "Where do I train Staves?" }, ref = "weapon-skill:136", why = "Unlocks Gnarled Quarterstaff" },
        } },
        { id = "upgrades", summary = "1 upgrade you can get", lines = {
          { text = "Gnarled Quarterstaff", item = 301, slot = "Two-hand", ilvl = 24, wear = "After you train", gain = "Big upgrade", how = "Needs the Staves skill: train it at Hanashi in Orgrimmar (10s)", pathKind = "bags", details = { "Two-hand: Gnarled Quarterstaff, in your bags", "Needs the Staves skill: train it at Hanashi in Orgrimmar (10s)" }, asks = { "Should I equip Gnarled Quarterstaff now?" }, ref = "item:301" },
        } },
        { id = "notes", lines = {
          { text = "Bank not captured: open your bank once", kind = "note", asks = { "Why does my bank need opening?" }, ref = "note:bank" },
          { text = "Dataset is build 1.59.0.63000, game is 64000", kind = "note", asks = { "What does the dataset mismatch mean?" }, ref = "note:dataset-build" },
        } },
      },
    },
  },
}
