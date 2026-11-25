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
- `addon_pulled` and `notice` are reserved and ignored for now.

## How an install works

The Rust shell (`overlay/src-tauri/src/update.rs`) downloads the installer and `SHA256SUMS` of the chosen
release over HTTPS, checks the SHA-256 (a mismatch deletes the file and installs nothing), keeps the last
two installers in the app cache folder (the installed one and the one it replaced, so going back works offline
afterwards), copies the overlay settings and profile files to a dated backup folder (the last three are kept),
runs the installer in passive mode and exits. The installer allows downgrades (`allowDowngrades` in
`tauri.conf.json`).
