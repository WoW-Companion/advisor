-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testrogue"] = {
      schema = "1.8",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testrogue",
      sections = {
        { id = "notes", lines = {
          { text = "Dataset is build 1.59.0.63000, game is 64000", kind = "note", asks = { "What does the dataset mismatch mean?" }, ref = "note:dataset-build" },
        } },
      },
      talents = {
        { line = "Deadly Poison rank 1", spell_id = 2001, details = { "Next pick: Deadly Poison, rank 1" }, asks = { "Where should my talent point go?", "What does Deadly Poison do?" }, ref = "talents", why = "Next pick in your plan" },
      },
    },
  },
}
