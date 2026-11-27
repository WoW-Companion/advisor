-- Data only: written by wowc-advisor; overwritten on every update. See docs/plan-format.md in the advisor repo.
WoWCompanionPlan = {
  schema = "1.7",
  generated = 1795363809,
  characters = {
    ["Hero"] = {
      start = { map = 1458, x = 50.0, y = 50.0, at = 0 },
      basis = { captured = 0, inLog = { 100, 101, 102, 601, 999 } },
      zone = "Undercity",
      distance = 0.0,
      steps = {
        { n = 1, kind = "errand", questID = 0, quest = "", label = "Train now with Rogue Master", map = 1458, x = 82.0, y = 14.0, done = false, after = {  }, place = "Undercity", errand = "train" },
        { n = 2, kind = "errand", questID = 0, quest = "", label = "Open your bank once", map = 1458, x = -1.0, y = -1.0, done = false, after = {  }, place = "Undercity", errand = "bank" },
      },
      exit = { label = "Walk or ride to Silverpine Forest: turn in Quest 102", map = 7102, x = -1.0, y = -1.0, via = "walk" },
      trips = {
        { order = 1, map = 7102, zone = "Silverpine Forest", ready = 1, todo = 0, pickups = 0, questIDs = { 102 }, reason = "1 ready to turn in", travel = "Walk or ride to Silverpine Forest: turn in Quest 102", eta = 10 },
        { order = 2, map = 7100, zone = "Stranglethorn Vale", ready = 1, todo = 0, pickups = 0, questIDs = { 100 }, reason = "1 ready to turn in", travel = "Walk or ride to Undercity, then zeppelin from Undercity to Grom'gol, then ride to Stranglethorn Vale: turn in Quest 100", eta = 20 },
        { order = 3, map = 7101, zone = "The Barrens", ready = 1, todo = 0, pickups = 0, questIDs = { 101 }, reason = "1 ready to turn in", travel = "Boat from Booty Bay to Ratchet, then ride to The Barrens", eta = 8 },
      },
      parked = {
        { questID = 601, quest = "Guano Quest", why = "needs a group: Razorfen Kraul (The Barrens)", kind = "dungeon", level = 30, diff = "yellow" },
      },
      later = {
        { n = 1, kind = "objective", questID = 601, quest = "Guano Quest", label = "Kill Kraul Boar (0/1)", map = 7099, x = 30.0, y = 30.0, done = false, after = {  }, place = "Razorfen Kraul", level = 30, diff = "yellow" },
        { n = 2, kind = "turnin", questID = 100, quest = "Quest 100", label = "Turn in Quest 100 to Clerk Stranglethorn Vale", map = 7100, x = 50.0, y = 50.0, done = false, after = {  }, place = "Stranglethorn Vale", level = 30, diff = "yellow" },
        { n = 3, kind = "turnin", questID = 101, quest = "Quest 101", label = "Turn in Quest 101 to Clerk The Barrens", map = 7101, x = 50.0, y = 50.0, done = false, after = {  }, place = "The Barrens", level = 30, diff = "yellow" },
        { n = 4, kind = "turnin", questID = 102, quest = "Quest 102", label = "Turn in Quest 102 to Clerk Silverpine Forest", map = 7102, x = 50.0, y = 50.0, done = false, after = {  }, place = "Silverpine Forest", level = 30, diff = "yellow" },
      },
      find = {
        { n = 1, questID = 999, quest = "Lost Quest", hint = "Find who takes Lost Quest", done = false },
      },
      errands = {
        { n = 1, kind = "train", label = "Train now with Rogue Master", map = 1458, x = 82.0, y = 14.0, why = "2 new spells or ranks to train at this level" },
        { n = 2, kind = "bank", label = "Open your bank once", map = 1458, x = -1.0, y = -1.0, why = "open your bank once so the advisor can see it" },
        { n = 0, kind = "restock", label = "buy 4 Empty Vial from Sly Garrett", map = 1458, x = 83.0, y = 14.0, why = "short of reagents for a batch of 5", attach = 1, npc = "Sly Garrett" },
      },
      flights = {
      },
      guidance = {
        [100] = {
          questID = 100, quest = "Quest 100", class = false, steps = {  },
          objectives = {},
          turnin = { text = "Turn in Quest 100 to Clerk Stranglethorn Vale", map = 7100, x = 50.0, y = 50.0, zone = "Stranglethorn Vale", place = "Stranglethorn Vale" },
          after = "After this: nothing else is planned here.",
        },
        [101] = {
          questID = 101, quest = "Quest 101", class = false, steps = {  },
          objectives = {},
          turnin = { text = "Turn in Quest 101 to Clerk The Barrens", map = 7101, x = 50.0, y = 50.0, zone = "The Barrens", place = "The Barrens" },
          after = "After this: nothing else is planned here.",
        },
        [102] = {
          questID = 102, quest = "Quest 102", class = false, steps = {  },
          objectives = {},
          turnin = { text = "Turn in Quest 102 to Clerk Silverpine Forest", map = 7102, x = 50.0, y = 50.0, zone = "Silverpine Forest", place = "Silverpine Forest" },
          after = "After this: nothing else is planned here.",
        },
        [601] = {
          questID = 601, quest = "Guano Quest", class = false, steps = {  },
          objectives = {
            { text = "Kill Kraul Boar (0/1)", map = 7099, x = 30.0, y = 30.0, zone = "Razorfen Kraul", place = "Razorfen Kraul" },
          },
          after = "After this: nothing else is planned here.",
        },
      },
      here = {
      },
      finishers = {
        { questID = 100, quest = "Quest 100", npc = "Clerk Stranglethorn Vale", map = 7100, x = 50.0, y = 50.0, place = "Stranglethorn Vale" },
        { questID = 102, quest = "Quest 102", npc = "Clerk Silverpine Forest", map = 7102, x = 50.0, y = 50.0, place = "Silverpine Forest" },
      },
      zones = {
        [7100] = { zone = "Stranglethorn Vale", travel = "Zeppelin to Grom'gol", arrival = { kind = "walk" }, order = { 2 } },
        [7101] = { zone = "The Barrens", travel = "Zeppelin to Grom'gol, then boat to Ratchet", arrival = { kind = "walk" }, order = { 3 } },
        [7102] = { zone = "Silverpine Forest", travel = "Walk or ride to Silverpine Forest", arrival = { kind = "walk" }, order = { 4 } },
      },
    },
  },
}
