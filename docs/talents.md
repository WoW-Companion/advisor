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
