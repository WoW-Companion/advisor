// The crash check after an upgrade. After the app starts under a new version the shell keeps a small
// record (`launch-guard.json` in the app data folder). If, within 10 minutes of the upgrade, the sidecar
// fails its health check, or the app ends abnormally twice, the next start shows a safe screen that
// offers to go back, keep trying or open the logs. "Keep trying" is remembered for that version, so the
// safe screen never comes back for it. Everything here is pure state-machine code over `GuardState`;
// `load` and `save` are the only parts that touch the disk.
use std::fs;
use std::path::Path;

use serde::{Deserialize, Serialize};

/// How long after an upgrade a failure still counts toward the safe screen.
pub const WINDOW_SECS: u64 = 10 * 60;
/// How long the sidecar has to say hello after it starts.
pub const HEALTH_SECS: u64 = 60;
/// Abnormal exits (inside the window) that trip the check.
pub const CRASH_LIMIT: u32 = 2;

/// What the shell remembers between runs. Fields are only ever added, so an older shell can read a
/// newer file.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct GuardState {
    pub schema: u32,
    /// The version that wrote this record: the one that was running at the last start.
    pub version: String,
    /// The version before it ("" when this was not an upgrade, so nothing is tracked).
    pub previous: String,
    /// When this version first started after an upgrade (seconds since the Unix epoch); 0 = not tracked.
    pub upgraded_at: u64,
    /// When the last start happened.
    pub session_at: u64,
    /// True from a start until a clean exit; still true at the next start means the app died.
    pub open: bool,
    /// Sidecar health-check failures inside the window.
    pub health_fails: u32,
    /// Abnormal exits inside the window.
    pub abnormal_exits: u32,
    /// The player chose "Keep trying" for this version: the safe screen is not shown again for it.
    pub kept: bool,
}

/// What the start of the app should do.
#[derive(Clone, Debug, PartialEq)]
pub enum Decision {
    /// Start as usual.
    Normal,
    /// Show the safe screen (nothing else starts).
    SafeScreen,
}

fn valid(v: &str) -> bool {
    crate::update::valid_version(v)
}

/// Whether `a` is an older version than `b`. Two versions that are not plain release numbers compare
/// as not older.
pub fn older(a: &str, b: &str) -> bool {
    match (crate::update::version_key(a), crate::update::version_key(b)) {
        (Some(x), Some(y)) => x < y,
        _ => false,
    }
}

impl GuardState {
    /// An upgrade is being watched: there was an earlier version and this record said so.
    pub fn tracked(&self) -> bool {
        self.upgraded_at > 0 && !self.previous.is_empty()
    }

    /// Whether the failures so far call for the safe screen.
    pub fn tripped(&self) -> bool {
        self.tracked() && (self.health_fails >= 1 || self.abnormal_exits >= CRASH_LIMIT)
    }
}

/// The record after an app start under `running`, and what to do. `cached_previous` is the newest
/// older version whose installer is in the cache: the previous version when no record exists yet (the
/// first start after upgrading from a release that kept none).
pub fn on_start(old: &GuardState, running: &str, cached_previous: Option<&str>, now: u64) -> (GuardState, Decision) {
    let mut st = old.clone();
    st.schema = 1;
    if st.version != running {
        // A version change: a new record. Only an upgrade (from an older version) is watched; a first
        // install, a go-back or an unreadable version number is not.
        let from = if valid(&old.version) { Some(old.version.as_str()) } else { cached_previous };
        let prev = from.filter(|p| valid(p) && older(p, running)).unwrap_or("");
        st = GuardState {
            schema: 1,
            version: running.to_string(),
            previous: prev.to_string(),
            upgraded_at: if prev.is_empty() { 0 } else { now },
            session_at: now,
            open: true,
            ..GuardState::default()
        };
        return (st, Decision::Normal);
    }
    if st.open && st.tracked() && st.session_at <= st.upgraded_at.saturating_add(WINDOW_SECS) {
        // The last start inside the window never exited cleanly.
        st.abnormal_exits += 1;
    }
    st.session_at = now;
    st.open = true;
    let decision = if st.tripped() && !st.kept { Decision::SafeScreen } else { Decision::Normal };
    (st, decision)
}

/// The record after the sidecar's health check: it said hello in time (`ok`) or did not (a handshake
/// that never came within `HEALTH_SECS`, or a sidecar that ended first). A failure counts only inside
/// the window; a good start clears earlier health failures, since the problem is gone.
pub fn on_health(old: &GuardState, ok: bool, now: u64) -> GuardState {
    let mut st = old.clone();
    if ok {
        st.health_fails = 0;
    } else if st.tracked() && now <= st.upgraded_at.saturating_add(WINDOW_SECS) {
        st.health_fails += 1;
    }
    st
}

/// The record after a clean exit.
pub fn on_clean_exit(old: &GuardState) -> GuardState {
    GuardState { open: false, ..old.clone() }
}

/// The record after "Keep trying": this version is not checked again.
pub fn on_keep_trying(old: &GuardState) -> GuardState {
    GuardState { kept: true, ..old.clone() }
}

/// Reads the record; a missing or unreadable file is an empty record (nothing tracked).
pub fn load(path: &Path) -> GuardState {
    fs::read_to_string(path).ok().and_then(|t| serde_json::from_str(&t).ok()).unwrap_or_default()
}

/// Writes the record; failing to write it must never stop the app, so the error is only returned.
pub fn save(path: &Path, st: &GuardState) -> Result<(), String> {
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    }
    let text = serde_json::to_string_pretty(st).map_err(|e| e.to_string())?;
    let tmp = path.with_extension("json.tmp");
    fs::write(&tmp, text).map_err(|e| e.to_string())?;
    fs::rename(&tmp, path).map_err(|e| e.to_string())
}

#[cfg(test)]
mod tests {
    use super::*;

    const T0: u64 = 1_800_000_000;

    /// A record of 0.7.54 running normally and exited cleanly.
    fn old_clean() -> GuardState {
        GuardState { schema: 1, version: "0.7.54".into(), open: false, session_at: T0 - 5000, ..GuardState::default() }
    }

    #[test]
    fn an_upgrade_starts_a_watched_record() {
        let (st, d) = on_start(&old_clean(), "0.7.55", None, T0);
        assert_eq!(d, Decision::Normal);
        assert_eq!((st.version.as_str(), st.previous.as_str(), st.upgraded_at), ("0.7.55", "0.7.54", T0));
        assert!(st.tracked() && st.open && !st.tripped());
    }

    #[test]
    fn no_record_uses_the_cached_installer_as_the_previous_version() {
        let (st, _) = on_start(&GuardState::default(), "0.7.55", Some("0.7.54"), T0);
        assert_eq!(st.previous, "0.7.54");
        assert!(st.tracked());
        // A first install has nothing cached: nothing is watched.
        let (st, d) = on_start(&GuardState::default(), "0.7.55", None, T0);
        assert!(!st.tracked());
        assert_eq!(d, Decision::Normal);
        // A cached installer that is not older does not count either.
        assert!(!on_start(&GuardState::default(), "0.7.55", Some("0.7.56"), T0).0.tracked());
    }

    #[test]
    fn going_back_is_not_watched() {
        let mut newer = old_clean();
        newer.version = "0.7.56".into();
        let (st, d) = on_start(&newer, "0.7.54", Some("0.7.50"), T0);
        assert!(!st.tracked());
        assert_eq!(d, Decision::Normal);
        assert_eq!(st.version, "0.7.54");
    }

    #[test]
    fn a_failed_health_check_inside_the_window_shows_the_safe_screen_next_start() {
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        let st = on_health(&st, false, T0 + 61);
        assert_eq!(st.health_fails, 1);
        assert!(st.tripped());
        let st = on_clean_exit(&st);
        let (st, d) = on_start(&st, "0.7.55", None, T0 + 120);
        assert_eq!(d, Decision::SafeScreen);
        assert!(st.tripped());
    }

    #[test]
    fn a_health_failure_after_the_window_does_not_count() {
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        let st = on_health(&st, false, T0 + WINDOW_SECS + 1);
        assert_eq!(st.health_fails, 0);
        assert!(!st.tripped());
        // The edge itself still counts.
        assert_eq!(on_health(&on_start(&old_clean(), "0.7.55", None, T0).0, false, T0 + WINDOW_SECS).health_fails, 1);
    }

    #[test]
    fn a_good_start_clears_health_failures() {
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        let st = on_health(&on_health(&st, false, T0 + 70), true, T0 + 200);
        assert_eq!(st.health_fails, 0);
        assert!(!st.tripped());
    }

    #[test]
    fn two_abnormal_exits_inside_the_window_trip_it_and_one_does_not() {
        // Start 1 (the upgrade) dies without a clean exit.
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        // Start 2: one abnormal exit so far.
        let (st, d) = on_start(&st, "0.7.55", None, T0 + 60);
        assert_eq!((st.abnormal_exits, d), (1, Decision::Normal));
        // It dies again. Start 3: the second abnormal exit.
        let (st, d) = on_start(&st, "0.7.55", None, T0 + 120);
        assert_eq!(st.abnormal_exits, 2);
        assert_eq!(d, Decision::SafeScreen);
    }

    #[test]
    fn a_clean_exit_between_starts_is_not_a_crash() {
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        let st = on_clean_exit(&st);
        let (st, d) = on_start(&st, "0.7.55", None, T0 + 60);
        assert_eq!((st.abnormal_exits, d), (0, Decision::Normal));
    }

    #[test]
    fn a_crash_of_a_session_that_began_after_the_window_does_not_count() {
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        let st = on_clean_exit(&st);
        // A start long after the upgrade that then dies, twice.
        let (st, _) = on_start(&st, "0.7.55", None, T0 + WINDOW_SECS + 100);
        let (st, d) = on_start(&st, "0.7.55", None, T0 + WINDOW_SECS + 200);
        let (st, d2) = on_start(&st, "0.7.55", None, T0 + WINDOW_SECS + 300);
        assert_eq!((st.abnormal_exits, d, d2), (0, Decision::Normal, Decision::Normal));
    }

    #[test]
    fn keep_trying_never_shows_the_screen_again_for_that_version() {
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        let st = on_health(&st, false, T0 + 61);
        let (st, d) = on_start(&on_clean_exit(&st), "0.7.55", None, T0 + 100);
        assert_eq!(d, Decision::SafeScreen);
        let st = on_keep_trying(&st);
        // More failures and more crashes: still no loop.
        let st = on_health(&st, false, T0 + 160);
        let (st, d) = on_start(&st, "0.7.55", None, T0 + 200);
        assert_eq!(d, Decision::Normal);
        let (st, d) = on_start(&st, "0.7.55", None, T0 + 300);
        assert_eq!(d, Decision::Normal);
        assert!(st.kept);
        // A newer version is watched afresh.
        let (st, d) = on_start(&st, "0.7.56", None, T0 + 400);
        assert!(!st.kept && st.tracked() && st.previous == "0.7.55");
        assert_eq!(d, Decision::Normal);
    }

    #[test]
    fn the_record_round_trips_and_a_broken_file_reads_as_empty() {
        let dir = std::env::temp_dir().join(format!("wowc-guard-test-{}", std::process::id()));
        let path = dir.join("launch-guard.json");
        let (st, _) = on_start(&old_clean(), "0.7.55", None, T0);
        save(&path, &st).unwrap();
        assert_eq!(load(&path), st);
        // A newer shell's extra field is ignored.
        fs::write(&path, r#"{"schema":2,"version":"0.7.55","future":[1],"kept":true}"#).unwrap();
        assert!(load(&path).kept);
        fs::write(&path, "not json").unwrap();
        assert_eq!(load(&path), GuardState::default());
        assert_eq!(load(&dir.join("missing.json")), GuardState::default());
        let _ = fs::remove_dir_all(&dir);
    }
}
