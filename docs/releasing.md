# Releasing and pulling a version

Every merge to `main` that changes `VERSION` cuts a release (see `.github/workflows/release.yml`). The
overlay checks GitHub for new releases at start, every 4 hours and when the PC wakes, and offers the
newest release that:

- is published (not a draft) and carries `WoW-Companion-Setup-<V>-x64.exe` (the checksum for it is in
  `SHA256SUMS`);
- is newer than the version the player is running;
- is not on the pulled list below and has not been skipped by the player;
- has been public for at least 24 hours, unless the player turned on "Get updates right away".

The 24 hours is the settle delay: a bad release can be pulled before most players are offered it.

## Pulling a version

Edit [`updates/updates.json`](../updates/updates.json) on `main`. The overlay reads it from
`https://raw.githubusercontent.com/WoW-Companion/advisor/main/updates/updates.json`:

```json
{
  "schema": 1,
  "pulled": [
    { "version": "0.7.52", "reason": "plain words the player will read", "back_to": "0.7.51" }
  ],
  "addon_pulled": [],
  "notice": null
}
```

- `version`: the release to pull, without the `v`.
- `reason`: shown in the red banner, "This version has a known problem: <reason>".
- `back_to`: the version to go back to. Leave it out and the newest older release that is not pulled is used.
- A pulled version is never offered, never listed in Settings, and players already running it see the red banner with a
  one-click "Go back".
- A pull takes effect on each player's next check (within 4 hours, or at once on "Check now"; the file is
  cached for up to 10 minutes). To un-pull, remove the entry.
- If the file cannot be read or parsed, nothing is treated as pulled and updates are never blocked.
- `addon_pulled` pulls an addon version the same way: `{ "version": "1.2.0", "reason": "...", "back_to": "1.1.0" }`.
  The addon check never offers a pulled addon version (when the newest is pulled its `back_to` is offered
  instead, following the chain), and when the installed addon is pulled it offers to reinstall `back_to`
  (or the newest addon release that is not pulled). An entry that is not an object is ignored.
- `notice` is reserved and ignored for now.

## How an install works

The Rust shell (`overlay/src-tauri/src/update.rs`) downloads the installer and `SHA256SUMS` of the chosen
release over HTTPS, checks the SHA-256 (a mismatch deletes the file and installs nothing), keeps the last
two installers in the app cache folder (the installed one and the one it replaced, so going back works offline
afterwards), copies the overlay settings and profile files to a dated backup folder (the last three are kept),
runs the installer in passive mode and exits. The installer allows downgrades (`allowDowngrades` in
`tauri.conf.json`).

## Going back and the crash check

The tray icon's menu holds **Check for updates**, **Go back to <previous>** (greyed out until an earlier
installer is cached) and **Open logs folder**. They run in the Rust shell alone, over the same verified
install path as the page's buttons, so they work when the page, the Go sidecar or the AI connection is
broken. They show their answers in a small static window (`overlay/ui/recovery.html`) that shares nothing
with the main UI.

After the app starts under a new version the shell keeps `launch-guard.json` (app data folder). If, within
10 minutes of the upgrade, the sidecar misses its health check (no handshake within 60 s of starting, or it
ends first) or the app ends without a clean exit twice, the next start shows the safe screen instead of
starting the advisor: "This update isn't working on your PC", with Go back to <previous>, Keep trying and
Open logs. Keep trying is remembered for that version, so the screen never loops. Go back installs the cached
previous installer (checked against the checksum it was downloaded with) and restores the newest settings
backup made under that version; with no such backup the current settings are kept. The same restore happens
when going back from the chat banner or Settings. A log is written to `wowc-shell.log` in the app log folder.

## Data that older versions can read

A player can go back to an older version at any time, so every file the app writes must stay readable by the
release before it:

- Settings, profiles and caches written by the overlay and the advisor carry a `schema` number
  (`profiles.json`, `providers.json`, the overlay's saved settings). Bump it only when a reader must change.
- Changes are **additive only**: add a field, never rename, retype, remove or change the meaning of one. Readers
  ignore fields they do not know, and a missing field has to mean the old behaviour.
- `internal/compat` holds the previous release's readers (frozen copies, never edited) and the fixtures in
  `testdata/compat/` that the current writers produce. Its tests fail when a writer's output changes; if the
  change is additive, regenerate with `UPDATE_COMPAT=1 go test ./internal/compat` and say so in the pull request.
  At each release, replace the frozen readers with the structs of the release just before it.

## Addon updates

The addon notice says whether the game needs `/reload` (the release only changes files that are already
installed) or a full restart (the release adds files: the game reads an addon's file list only when it starts).
