# Change fragments

A change adds `changes/<MISSION-ID>.md` with its player-facing bullets (`- ` lines, one change each, within
the limit in `docs/releasing.md`). It never edits `VERSION`, `CHANGELOG.md` or the overlay version fields.
`make release-prep` bundles the fragments into one new CHANGELOG section and bumps the version.
