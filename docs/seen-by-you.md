# Your own capture and "seen by you" facts

The advisor reads the WoW Companion addon's own files, so ForeverScribe is optional.

- **Characters:** `WoWCompanionDB.characters` in `WTF/Account/<account>/SavedVariables/WoWCompanion.lua`
  (the addon's `docs/capture-format.md`). When ForeverScribe's file is beside it, a character in both is
  merged: the newer capture (by capture time) leads, and a part it lacks (a bank, quests, talents) comes
  from the other. A character in only one file is used as it is. The doctor and the setup checklist say which
  source each character came from.
- **What you saw:** `WoWCompanionDB.observed` (the addon's `docs/observed-format.md`). These facts come
  before the dataset and Questie for your own answers, and are marked "seen by you":
  - trainer offers (trainer, cost, required skill, position) fill `where_to_learn`, the skill-up planner's
    learn step and the class spell costs;
  - vendor stock and prices fill the vendor ways of `where_to_learn`;
  - gathering and fishing rows fill `where_to_gather` and the planner's "gather here" lines (zones are named
    from what the capture knows; otherwise the answer says "map N");
  - quest givers fill a giver the dataset lacks.
- The footer under a know-how answer adds "Trainers, vendors, gathering and quest givers: seen by you"
  when such a fact grounded it.
