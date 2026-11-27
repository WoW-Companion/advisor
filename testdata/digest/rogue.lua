-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testrogue"] = {
      schema = "1.10",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testrogue",
      sections = {
        { id = "notes", lines = {
          { text = "No data for game build 64000 yet: using 63000", kind = "note", asks = { "Why is there no data for my game build yet?" }, ref = "note:dataset-build" },
        } },
      },
      talents = {
        { line = "Deadly Poison rank 1", spell_id = 2001, details = { "Next pick: Deadly Poison, rank 1" }, asks = { "Why does the Deadly Poison talent suit my play style?", "What changes in play once I take the Deadly Poison talent?" }, ref = "talents", why = "Next pick in your build" },
      },
    },
  },
}
