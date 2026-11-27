# talent_plan

`talent_plan(character, spec?, limit=3)` follows the Icy Veins WoW Forever leveling build: the next
pick is the first point in the guide's order the character has not taken, among the points whose level
they have reached.

- **Builds** come from the dataset's `talent_builds` table, one row per point (class, spec, level, order,
  node_id, talent_name, rank_after, source). The reference is Icy Veins' WoW Forever leveling guides, the
  Operator's chosen source. Node names come from `talent_nodes`. **Priorities** come from
  `talent_priorities` (class, spec, rank, node_id, talent_name, reason): the guide's key leveling talents
  with a one-line reason in our own words, shown with each pick and in the AI's facts.
- **Spec**: the `spec` argument, else the stored spec, else the spec matching the stored role, else the
  build whose tree holds most of the points spent. A tie, or no points, goes to the class's default
  leveling spec (the spec with the most build points).
  A name typed in a few letters ("shadow", "feral", "prot") is the one spec whose name starts with it; one
  that starts several, or none, is a spec the dataset has no build for. Below level 10 the plan says the build
  starts at level 10, since no talent point exists before it.
- **Next pick**: walk the build in `order`; a point is taken when the captured rank of its node (matched by
  node id, spell id, then name) is at least its `rank_after`. The next picks are the first untaken points
  with `level` at or below the character's level, up to three: "Spirit Tap 5/5 done · next: Improved Shadow
  Word: Pain 1/2". With unspent points the Next line names the first one.
- **Off-build** points (a node the build does not take, or takes fewer ranks of) are noted once, plainly:
  "You have 2 points in Shadow Focus, which the Icy Veins leveling build skips". Advice continues from the
  build and never suggests a respec unprompted.
- **No checked build**: a dataset without the `level` and `rank_after` columns, a spec whose rows are marked
  `curated (unverified)`, or a spec the dataset lacks says "I don't have a checked build for <spec> yet.",
  lists the unspent points and names no pick.
- The AI's facts carry the progress line, the priority list and the off-build note; the system prompt says
  talent picks come from them, never from general knowledge.

Operator check: run `talent_plan` against a real ForeverScribe capture and a built dataset.

# Talent advisor: play style, plan and next picks

Talent advice follows what the player wants from the character, and it advances as they level.

## The flow

1. **Ask once.** "What talents should I get?" starts with `get_character_profile`. With no saved
   play style the AI puts one short multiple-choice question to the player (role: damage, tank,
   healer or balanced; focus: leveling, dungeons, PvP or mixed; optionally the spec tree) and saves
   the answer with `set_character_profile`. The overlay's **Play style** settings group (role,
   focus, spec for the chosen character) changes it later.
2. **Advise each point, deterministically.** The picks are computed from the dataset's curated
   `talent_builds`, the captured ranks and the unspent points, with no model call and no web
   lookup. The build is the stored spec's, or the spec matching the stored role when no spec is
   set. They appear in the Next list (ranked just below the class-quest milestone), the in-game
   digest, the `talent_plan` tool and the prompt facts, for example: "Take Whirlwind (rank 1):
   the build's reason." followed by at most two bullets, the next picks in the band and why it
   fits the play style. Spend a point, `/reload`, and the next pick advances.
3. **No build yet.** For a class or spec the dataset has no build for, the answer says so in one
   line and names the talents available now (a rank open, prerequisites met) as options, not a
   recommendation.

## What is stored, and where

`profiles.json` in the user config directory (`%AppData%\wowc-advisor`, `~/Library/Application
Support/wowc-advisor`, `~/.config/wowc-advisor`; override with `WOWC_PROFILES`), mode 0600, one
entry per character key. A profile is role, focus, spec and a stat preference. Nothing is fetched
from a guide, and anything a profile saved before this change holds about a guide is ignored.

Operator check: on a level 10 warrior, ask "What talents should I get?", answer the style question,
see Next show "N unspent talent points: take ..."; spend a point,
`/reload`, and the next pick advances.

Not done: spell descriptions. The dataset has no spell text table, so `talent_tree` lists names,
branches, ranks and spell ids only.
