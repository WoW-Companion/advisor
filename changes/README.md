# Change fragments

A change adds `changes/<MISSION-ID>.md` with its player-facing bullets (`- ` lines, one change each, within
the limit in `docs/releasing.md`). It never edits `VERSION`, `CHANGELOG.md` or the overlay version fields.
`make release-prep` bundles the fragments into one new CHANGELOG section and bumps the version.

`make verify` checks every fragment line against the same writing rule as the CHANGELOG (100 characters at most, no mission ids, file names or developer words), and `make release-prep` refuses a line that breaks it, naming the fragment.
