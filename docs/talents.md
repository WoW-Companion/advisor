# talent_plan

`talent_plan(character, spec?, limit=5)` compares the points a character has spent with the
curated build for their spec and level band and lists the next points to spend.

- **Builds** come from the dataset's `talent_builds` table (class, spec, band, order, node_id,
  points, rationale). They are advisor defaults, not authoritative. Node names come from
  `talent_nodes`.
- **Spec** is the `spec` argument, else the stored spec, else the spec matching the stored role,
  else the talent group with the most points spent (a tie, or no points, needs the `spec`
  argument). Group names are matched to build specs by name.
- **Band** is the level band (for example `10-19`) holding the character's level. The plan covers
  the build's steps from every band up to it, in order.
- **Diff**: captured ranks (matched by node id, else by name) are credited against the build's
  steps in order; what is left is the plan, one point per line with the rank it reaches and the
  step's rationale. `limit` caps the list.
- **Off-build** points (a node with more ranks than the build gives it over all its bands) are
  listed with both numbers. They are a choice, not an error.
- **No build** for the spec or level (or no `talent_builds` table): the plan says so and lists the
  builds the dataset has, and the talents open now as options. Nothing is invented.
- The digest's unspent-points note names the next node from the build.

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
