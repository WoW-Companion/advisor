-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testrogue"] = {
      schema = "1.6",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testrogue",
      sections = {
        { id = "upgrades", lines = {
          { text = "finger1: Ring of Mind (ilvl 18)", item = 6, slot = "finger1", ilvl = 18, details = { "Ring: Ring of Mind, item level 18", "Your slot is empty" }, prompt = "I'm a level 12 rogue. A possible upgrade is Ring of Mind for my ring slot (item level 18). Is it worth getting for how I play, and how do I get it?", asks = { "Is Ring of Mind worth getting now?", "Where do I get Ring of Mind?" }, ref = "item:6", why = "Fills your empty ring slot" },
        } },
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
