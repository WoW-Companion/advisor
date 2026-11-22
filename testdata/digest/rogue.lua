-- Data only: written by wowc-advisor; overwritten on every update. See docs/digest-format.md in the addon repo.
WoWCompanionDigest = {
  schema = "2.0",
  generated = 1800000000,
  characters = {
    ["Testrogue"] = {
      schema = "1.4",
      generated = 1800000000,
      build = "1.60.0.64000",
      character = "Testrogue",
      sections = {
        { id = "upgrades", lines = {
          { text = "finger1: Ring of Mind (ilvl 18) from object 900", item = 6, slot = "finger1", ilvl = 18, details = { "finger1: Ring of Mind, ilvl 18", "Your slot is empty", "From object 900 (object)" }, prompt = "I'm a level 12 rogue. A possible upgrade is Ring of Mind for my finger1 slot (item level 18), from object 900. How do I get it, and is it worth the effort for my build?" },
        } },
        { id = "notes", lines = {
          { text = "Dataset is build 1.59.0.63000, game is 64000" },
        } },
      },
      talents = {
        { line = "Deadly Poison rank 1", spell_id = 2001, details = { "Next pick: Deadly Poison, rank 1", "Why: Poison" }, prompt = "I'm a level 12 rogue. The next talent point in my plan is Deadly Poison to rank 1. The plan says: Poison. Is this the right pick for me now, and what does it change?" },
      },
    },
  },
}
