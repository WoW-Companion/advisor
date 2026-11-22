# talent_plan

`talent_plan(character, spec?, limit=5)` compares the points a character has spent with the
curated build for their spec and level band and lists the next points to spend.

- **Builds** come from the dataset's `talent_builds` table (class, spec, band, order, node_id,
  points, rationale). They are advisor defaults, not authoritative. Node names come from
  `talent_nodes`.
- **Spec** is the talent group with the most points spent. A tie, or no points, needs the `spec`
  argument; a spec argument always wins. Group names are matched to build specs by name.
- **Band** is the level band (for example `10-19`) holding the character's level. The plan covers
  the build's steps from every band up to it, in order.
- **Diff**: captured ranks (matched by node id, else by name) are credited against the build's
  steps in order; what is left is the plan, one point per line with the rank it reaches and the
  step's rationale. `limit` caps the list.
- **Off-build** points (a node with more ranks than the build gives it over all its bands) are
  listed with both numbers. They are a choice, not an error.
- **No build** for the spec or level (or no `talent_builds` table): the plan says so and lists the
  builds the dataset has. Nothing is invented.
- The digest's unspent-points note names the next node when a plan exists.

Operator check: run `talent_plan` against a real ForeverScribe capture and a built dataset.

# Talent advisor: play style, plan and next picks

Talent advice follows what the player wants from the character, and it advances as they level.

## The flow

1. **Ask once.** "What talents should I get?" starts with `get_character_profile`. With no saved
   play style the AI puts one short multiple-choice question to the player (role: damage, tank,
   healer or balanced; focus: leveling, dungeons, PvP or mixed; optionally the spec tree and a
   guide URL) and saves the answer with `set_character_profile`. The overlay's **Play style**
   settings group (role, focus, spec for the chosen character) changes it later.
2. **Build a plan, once.** With no plan saved (or on "refresh my talent plan", or when the spec
   changed) the AI calls `talent_tree` for the captured node names, reads the player's chosen guide
   (default: the Icy Veins WoW: Forever guide, found from
   `https://www.icy-veins.com/wow-forever/<class>-class-overview`) and saves a **talent plan** with
   `set_character_profile`: an ordered list of `{node, rank}`, the guide URL and the date. This is
   a one-off, larger call; the usage line shows its size.
3. **Advise each point, deterministically.** From then on the picks are computed from the saved
   plan, the captured ranks and the unspent points. No model call, no tokens. They appear in the
   Next list (ranked just below the class-quest milestone), the in-game digest and the prompt
   facts, for example: `You have 2 unspent talent points: take Whirlwind (ranks 1-2/2) in Fury`
   with the reason `plan: <guide URL> Fury build`. Spend a point, `/reload`, and the next pick
   advances.

## What is stored, and where

`profiles.json` in the user config directory (`%AppData%\wowc-advisor`, `~/Library/Application
Support/wowc-advisor`, `~/.config/wowc-advisor`; override with `WOWC_PROFILES`), mode 0600, one
entry per character key. A profile is role, focus, spec and an optional guide URL. A plan is the
spec, guide URL, date and `{node, rank}` pairs. Guide text is never stored, quoted at length or
sent anywhere, and nothing from a guide enters a repo, release or dataset. Answers credit the guide
with its link.

## Checks

- `set_character_profile` checks every plan node against the captured class tree (matched by name,
  written back with the captured spelling) and the node's max rank. Unknown nodes and bad ranks are
  dropped and listed. If none is left the plan is refused. At most 120 picks.
- A node the plan names that the tree lacks (a guide that differs from Forever's tree) shows in the
  Next reason, with the request to adapt the plan once.
- A profile whose spec differs from the plan's marks the plan **stale**: no picks until it is
  refreshed.
- A node the capture says cannot be bought yet is skipped; the Next reason says so.

## The guide fetch

Only the player's own AI client fetches, only for a talent question, and only where the client can
be limited to one site. Confirmed against `claude --help` (2.1.323): `--tools WebFetch` names the one
built-in tool (the default `--tools ""` removes it) and `--allowedTools mcp__wowc-advisor
"WebFetch(domain:icy-veins.com)"` pre-approves fetches of that domain only. Codex, Antigravity and
Gemini CLI have no equivalent narrow option, so they get no web tool: the answer shows the guide
link and the player may paste a plan.

Operator check: on a level 10 warrior, ask "What talents should I get?", answer the style question,
see a plan citing the guide and Next show "N unspent talent points: take ..."; spend a point,
`/reload`, and the next pick advances.

Not done: spell descriptions. The dataset has no spell text table, so `talent_tree` lists names,
branches, ranks and spell ids only.
