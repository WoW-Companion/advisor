// Installing updates by itself, in the shell so it works when the page is broken. With "Install updates
// automatically" on, the shell looks for a release on its own schedule (the same rules as the banner:
// a day old unless "Get updates right away" is on, never pulled, never skipped) and installs it at
// the start of the app or after the player has been idle for five minutes, never while an answer is
// being written. A pulled running version goes back to its `back_to` the same way. Everything that
// decides is pure code over `AutoState` and `Activity`; the thread at the bottom only calls it.
use std::collections::BTreeMap;
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::Mutex;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::{AppHandle, Emitter, Manager, State};

use crate::recover;
use crate::update::{self, Check, GoBack};

/// How long with no ask, no streaming answer and no input focus counts as idle.
pub const IDLE_SECS: u64 = 5 * 60;
/// Tries per version (a failed download or checksum is one); then the banner is all that is left.
pub const MAX_TRIES: u32 = 3;
/// How long "Updating to <v>…" shows before the install starts.
pub const NOTE_SECS: u64 = 5;
/// The first lookup after start, and how long after the start counts as "at start".
pub const FIRST_CHECK_SECS: u64 = 20;
pub const START_WINDOW_SECS: u64 = 3 * 60;
/// The regular lookup, as in the page.
pub const CHECK_SECS: u64 = 4 * 60 * 60;
const TICK_SECS: u64 = 30;
/// A page that has not reported for this long is not trusted to still be busy.
pub const STALE_SECS: u64 = 2 * 60;

const FILE: &str = "auto-update.json";

/// A go-back the shell did by itself, kept for the "✓ Went back to <v>: <reason>" note.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
pub struct WentBack {
    pub version: String,
    pub reason: String,
}

/// The setting and what the shell needs to apply the banner's rules without the page. Fields are only
/// ever added, so an older shell can read a newer file.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct AutoState {
    pub enabled: bool,
    /// "Get updates right away".
    pub update_now: bool,
    /// Versions the player skipped.
    pub skipped: Vec<String>,
    /// Versions the shell must not install by itself: ones that were gone back from.
    pub blocked: Vec<String>,
    /// Failed tries per version.
    pub tries: BTreeMap<String, u32>,
    pub went_back: Option<WentBack>,
}

/// What the page last said about the player: whether an ask or a streaming answer is under way, whether
/// an input has focus, and when.
#[derive(Clone, Debug, Default, PartialEq)]
pub struct Activity {
    pub asking: bool,
    pub focused: bool,
    pub reported: u64,
    pub last_active: u64,
}

impl Activity {
    pub fn report(&mut self, asking: bool, focused: bool, now: u64) {
        if asking || focused || self.asking || self.focused {
            self.last_active = now;
        }
        self.asking = asking;
        self.focused = focused;
        self.reported = now;
    }

    fn fresh(&self, now: u64) -> bool {
        now < self.reported.saturating_add(STALE_SECS)
    }

    /// An answer is being written (or asked for) right now.
    pub fn asking_now(&self, now: u64) -> bool {
        self.asking && self.fresh(now)
    }

    /// Seconds with no ask, no streaming answer and no input focus; 0 while any of them holds.
    pub fn quiet_secs(&self, now: u64) -> u64 {
        if self.asking_now(now) || (self.focused && self.fresh(now)) {
            0
        } else {
            now.saturating_sub(self.last_active)
        }
    }
}

/// What a lookup found.
#[derive(Clone, Debug, PartialEq)]
pub struct Found {
    pub check: Check,
    pub back: Option<GoBack>,
}

#[derive(Clone, Debug, PartialEq)]
pub enum Action {
    Install(String),
    GoBack { version: String, reason: String },
}

fn tries(st: &AutoState, v: &str) -> u32 {
    st.tries.get(v).copied().unwrap_or(0)
}

fn older(a: &str, b: &str) -> bool {
    matches!((update::version_key(a), update::version_key(b)), (Some(x), Some(y)) if x < y)
}

/// What to do now, if anything. `at_start` is true in the first minutes after the app started, when
/// the idle wait is skipped (an answer being written still stops everything).
pub fn decide(st: &AutoState, found: &Found, running: &str, now: u64, act: &Activity, at_start: bool) -> Option<Action> {
    if !st.enabled || act.asking_now(now) {
        return None;
    }
    if !at_start && act.quiet_secs(now) < IDLE_SECS {
        return None;
    }
    if let Some(b) = &found.back {
        if update::valid_version(&b.back_to) && older(&b.back_to, running) && tries(st, &b.back_to) < MAX_TRIES {
            let reason = if b.reason.is_empty() { "it was pulled".to_string() } else { b.reason.clone() };
            return Some(Action::GoBack { version: b.back_to.clone(), reason });
        }
    }
    let v = match &found.check {
        Check::Available(v) => v,
        Check::Settling(v) if st.update_now => v,
        _ => return None,
    };
    let listed = |l: &Vec<String>| l.iter().any(|x| x.trim().trim_start_matches('v') == v.as_str());
    if listed(&st.skipped) || listed(&st.blocked) || tries(st, v) >= MAX_TRIES || !older(running, v) {
        return None;
    }
    Some(Action::Install(v.clone()))
}

/// Counts one failed try of `v`.
pub fn add_try(st: &mut AutoState, v: &str) {
    *st.tries.entry(v.to_string()).or_insert(0) += 1;
}

// --- the file ---------------------------------------------------------------------------------------

pub fn load(path: &Path) -> AutoState {
    fs::read_to_string(path).ok().and_then(|t| serde_json::from_str(&t).ok()).unwrap_or_default()
}

pub fn save(path: &Path, st: &AutoState) -> Result<(), String> {
    if let Some(d) = path.parent() {
        fs::create_dir_all(d).map_err(|e| e.to_string())?;
    }
    let text = serde_json::to_string_pretty(st).map_err(|e| e.to_string())?;
    fs::write(path, text).map_err(|e| e.to_string())
}

pub fn file_path(app: &AppHandle) -> Option<PathBuf> {
    app.path().app_config_dir().ok().map(|d| d.join(FILE))
}

fn now_secs() -> u64 {
    std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0)
}

// --- the shell's state and the page's commands --------------------------------------------------------

#[derive(Default)]
pub struct Auto {
    st: Mutex<AutoState>,
    act: Mutex<Activity>,
    /// The page's settings as it last sent them, saved in the backup an automatic install makes.
    settings: Mutex<String>,
}

fn persist(app: &AppHandle, st: &AutoState) {
    if let Some(p) = file_path(app) {
        if let Err(e) = save(&p, st) {
            recover::log(app, &format!("could not save the automatic update record: {e}"));
        }
    }
}

/// Reads the saved record into the shell's state (at start).
pub fn init(app: &AppHandle) {
    if let Some(p) = file_path(app) {
        *app.state::<Auto>().st.lock().unwrap() = load(&p);
    }
}

/// A version the shell must not install by itself any more (it was gone back from, or showed the safe
/// screen), so a go-back is not undone at the next lookup.
pub fn block(app: &AppHandle, version: &str) {
    let Some(a) = app.try_state::<Auto>() else { return };
    let mut st = a.st.lock().unwrap();
    if !st.blocked.iter().any(|b| b == version) {
        st.blocked.push(version.to_string());
        persist(app, &st);
    }
}

fn main_only(window: &tauri::Window) -> Result<(), String> {
    if window.label() == "main" {
        Ok(())
    } else {
        Err("not allowed".into())
    }
}

/// The page's update settings: the switch, "Get updates right away", the skipped versions, and its own
/// settings for the backup. Sent on load and after every change.
#[tauri::command]
pub fn auto_update_config(window: tauri::Window, app: AppHandle, auto: State<Auto>, enabled: bool, update_now: bool, skipped: Vec<String>, settings_json: String) -> Result<(), String> {
    main_only(&window)?;
    let mut st = auto.st.lock().unwrap();
    let next = AutoState { enabled, update_now, skipped, ..st.clone() };
    if next != *st {
        *st = next;
        persist(&app, &st);
    }
    *auto.settings.lock().unwrap() = settings_json;
    Ok(())
}

/// Whether an ask or a streaming answer is under way and whether an input has focus; sent on every
/// change and every half minute.
#[tauri::command]
pub fn report_activity(window: tauri::Window, auto: State<Auto>, asking: bool, focused: bool) -> Result<(), String> {
    main_only(&window)?;
    auto.act.lock().unwrap().report(asking, focused, now_secs());
    Ok(())
}

/// The reason of an automatic go-back to `version`, once ("" when there is none).
#[tauri::command]
pub fn take_went_back(window: tauri::Window, app: AppHandle, auto: State<Auto>, version: String) -> Result<String, String> {
    main_only(&window)?;
    let mut st = auto.st.lock().unwrap();
    let Some(w) = st.went_back.take() else { return Ok(String::new()) };
    persist(&app, &st);
    Ok(if w.version == version { w.reason } else { String::new() })
}

// --- the thread -------------------------------------------------------------------------------------

fn note(app: &AppHandle, text: &str) {
    let _ = app.emit("auto-update-note", text);
}

enum Outcome {
    Deferred,
    Failed(String),
}

fn run(app: &AppHandle, action: &Action, found: &Found, started: u64) -> Outcome {
    let running = env!("CARGO_PKG_VERSION");
    let (version, text) = match action {
        Action::Install(v) => (v.clone(), format!("Updating to {v}…")),
        Action::GoBack { version, .. } => (version.clone(), format!("Going back to {version}…")),
    };
    note(app, &text);
    std::thread::sleep(Duration::from_secs(NOTE_SECS));
    // The player may have asked something in these seconds: look again before anything starts.
    let still = {
        let a = app.state::<Auto>();
        let st = a.st.lock().unwrap().clone();
        let act = a.act.lock().unwrap().clone();
        let now = now_secs();
        decide(&st, found, running, now, &act, now.saturating_sub(started) < START_WINDOW_SECS)
    };
    if still.as_ref() != Some(action) {
        note(app, "");
        return Outcome::Deferred;
    }
    let settings = app.state::<Auto>().settings.lock().unwrap().clone();
    let prepared = match action {
        Action::Install(v) => update::prepare(app, v, &settings),
        Action::GoBack { .. } => update::prepare_back(app, &version),
    };
    let installer = match prepared {
        Ok(p) => p,
        Err(e) => return Outcome::Failed(e),
    };
    if let Action::GoBack { version, reason } = action {
        let a = app.state::<Auto>();
        let mut st = a.st.lock().unwrap();
        st.went_back = Some(WentBack { version: version.clone(), reason: reason.clone() });
        persist(app, &st);
    }
    recover::restore_for_go_back(app, &version);
    match update::run_and_exit(app, &installer) {
        Ok(()) => Outcome::Deferred,
        Err(e) => Outcome::Failed(e),
    }
}

/// Starts the thread that looks, waits for a quiet moment and installs. It does nothing while the
/// setting is off.
pub fn start(app: &AppHandle) {
    let app = app.clone();
    let started = now_secs();
    std::thread::spawn(move || {
        std::thread::sleep(Duration::from_secs(FIRST_CHECK_SECS));
        let running = env!("CARGO_PKG_VERSION");
        let mut found: Option<Found> = None;
        let mut next_check = 0u64;
        loop {
            let now = now_secs();
            let enabled = app.state::<Auto>().st.lock().unwrap().enabled;
            if enabled && found.is_none() && now >= next_check {
                next_check = now + CHECK_SECS;
                match update::check_full(&app) {
                    Ok((check, back)) => found = Some(Found { check, back }),
                    Err(e) => recover::log(&app, &format!("automatic update check failed: {e}")),
                }
            }
            if let Some(f) = found.clone() {
                let (st, act) = {
                    let a = app.state::<Auto>();
                    let r = (a.st.lock().unwrap().clone(), a.act.lock().unwrap().clone());
                    r
                };
                if let Some(action) = decide(&st, &f, running, now, &act, now.saturating_sub(started) < START_WINDOW_SECS) {
                    recover::log(&app, &format!("automatic update: {action:?}"));
                    match run(&app, &action, &f, started) {
                        Outcome::Deferred => {}
                        Outcome::Failed(e) => {
                            let v = match &action {
                                Action::Install(v) => v.clone(),
                                Action::GoBack { version, .. } => version.clone(),
                            };
                            let a = app.state::<Auto>();
                            let mut s = a.st.lock().unwrap();
                            add_try(&mut s, &v);
                            persist(&app, &s);
                            recover::log(&app, &format!("automatic update of {v} failed (try {} of {MAX_TRIES}): {e}", tries(&s, &v)));
                            drop(s);
                            note(&app, "");
                            // The next lookup is the retry; after the last try only the banner is left.
                            found = None;
                        }
                    }
                } else if !enabled {
                    found = None;
                }
            }
            std::thread::sleep(Duration::from_secs(TICK_SECS));
        }
    });
}

#[cfg(test)]
mod tests {
    use super::*;

    const NOW: u64 = 1_800_000_000;

    fn on() -> AutoState {
        AutoState { enabled: true, ..Default::default() }
    }

    fn avail(v: &str) -> Found {
        Found { check: Check::Available(v.into()), back: None }
    }

    fn idle_act() -> Activity {
        Activity { asking: false, focused: false, reported: NOW - 10, last_active: NOW - 2 * IDLE_SECS }
    }

    #[test]
    fn it_does_nothing_while_the_setting_is_off() {
        assert_eq!(decide(&AutoState::default(), &avail("0.7.62"), "0.7.61", NOW, &idle_act(), true), None);
    }

    #[test]
    fn idle_means_five_quiet_minutes_with_no_ask_and_no_focus() {
        let mut a = idle_act();
        let f = avail("0.7.62");
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &a, false), Some(Action::Install("0.7.62".into())));
        a.last_active = NOW - IDLE_SECS + 1;
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &a, false), None, "four minutes fifty-nine is not idle");
        a.last_active = NOW - IDLE_SECS;
        assert!(decide(&on(), &f, "0.7.61", NOW, &a, false).is_some());
        // Input focus is activity however long ago the last keystroke was.
        a = Activity { focused: true, reported: NOW - 5, last_active: NOW - 3600, ..Default::default() };
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &a, false), None);
        // Releasing focus starts the wait from that moment.
        a.report(false, false, NOW);
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &a, false), None);
        assert!(decide(&on(), &f, "0.7.61", NOW + IDLE_SECS, &Activity { reported: NOW + IDLE_SECS - 1, ..a.clone() }, false).is_some());
    }

    #[test]
    fn nothing_installs_while_an_answer_is_streaming_even_at_start() {
        let a = Activity { asking: true, focused: true, reported: NOW - 5, last_active: NOW - 3600 };
        assert_eq!(decide(&on(), &avail("0.7.62"), "0.7.61", NOW, &a, true), None);
        assert_eq!(decide(&on(), &avail("0.7.62"), "0.7.61", NOW, &a, false), None);
        // A page that went quiet (a crash) does not hold an update off forever.
        let stale = Activity { reported: NOW - STALE_SECS - 1, ..a.clone() };
        assert!(decide(&on(), &avail("0.7.62"), "0.7.61", NOW, &stale, true).is_some());
    }

    #[test]
    fn at_start_the_idle_wait_is_skipped_but_focus_alone_does_not_block() {
        let a = Activity { asking: false, focused: true, reported: NOW - 1, last_active: NOW };
        assert_eq!(decide(&on(), &avail("0.7.62"), "0.7.61", NOW, &a, false), None);
        assert!(decide(&on(), &avail("0.7.62"), "0.7.61", NOW, &a, true).is_some());
    }

    #[test]
    fn a_release_is_installed_by_the_banners_rules() {
        let a = idle_act();
        let settling = Found { check: Check::Settling("0.7.62".into()), back: None };
        assert_eq!(decide(&on(), &settling, "0.7.61", NOW, &a, true), None, "the day's wait holds");
        let now_on = AutoState { update_now: true, ..on() };
        assert!(decide(&now_on, &settling, "0.7.61", NOW, &a, true).is_some(), "get updates right away skips it");
        let up = Found { check: Check::UpToDate, back: None };
        assert_eq!(decide(&on(), &up, "0.7.61", NOW, &a, true), None, "a pulled or older release is UpToDate");
        let skipped = AutoState { skipped: vec!["v0.7.62".into()], ..on() };
        assert_eq!(decide(&skipped, &avail("0.7.62"), "0.7.61", NOW, &a, true), None);
        let blocked = AutoState { blocked: vec!["0.7.62".into()], ..on() };
        assert_eq!(decide(&blocked, &avail("0.7.62"), "0.7.61", NOW, &a, true), None);
        assert_eq!(decide(&on(), &avail("0.7.61"), "0.7.61", NOW, &a, true), None, "never the running version");
        assert_eq!(decide(&on(), &avail("0.7.60"), "0.7.61", NOW, &a, true), None);
    }

    #[test]
    fn three_failed_tries_leave_only_the_banner() {
        let mut st = on();
        let f = avail("0.7.62");
        for n in 0..MAX_TRIES {
            assert!(decide(&st, &f, "0.7.61", NOW, &idle_act(), true).is_some(), "try {}", n + 1);
            add_try(&mut st, "0.7.62");
        }
        assert_eq!(decide(&st, &f, "0.7.61", NOW, &idle_act(), true), None);
        // Another version starts fresh.
        assert!(decide(&st, &avail("0.7.63"), "0.7.61", NOW, &idle_act(), true).is_some());
    }

    #[test]
    fn a_pulled_running_version_goes_back_on_the_same_idle_rule() {
        let back = GoBack { back_to: "0.7.60".into(), reason: "it loses saved plans".into() };
        let f = Found { check: Check::UpToDate, back: Some(back) };
        let want = Some(Action::GoBack { version: "0.7.60".into(), reason: "it loses saved plans".into() });
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &idle_act(), false), want);
        let busy = Activity { last_active: NOW - 10, ..idle_act() };
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &busy, false), None);
        let streaming = Activity { asking: true, ..idle_act() };
        assert_eq!(decide(&on(), &f, "0.7.61", NOW, &streaming, true), None);
        assert_eq!(decide(&AutoState::default(), &f, "0.7.61", NOW, &idle_act(), true), None);
        // A back_to that is not older, not a version, or tried out falls back to the banner.
        for bad in ["0.7.62", "", "../x"] {
            let g = Found { check: Check::UpToDate, back: Some(GoBack { back_to: bad.into(), reason: String::new() }) };
            assert_eq!(decide(&on(), &g, "0.7.61", NOW, &idle_act(), true), None, "{bad:?}");
        }
        let mut tried = on();
        for _ in 0..MAX_TRIES {
            add_try(&mut tried, "0.7.60");
        }
        assert_eq!(decide(&tried, &f, "0.7.61", NOW, &idle_act(), true), None);
        // No reason given: a plain one.
        let g = Found { check: Check::UpToDate, back: Some(GoBack { back_to: "0.7.60".into(), reason: String::new() }) };
        assert_eq!(decide(&on(), &g, "0.7.61", NOW, &idle_act(), true), Some(Action::GoBack { version: "0.7.60".into(), reason: "it was pulled".into() }));
    }

    #[test]
    fn the_record_survives_a_save_and_a_bad_file_is_the_default() {
        let dir = std::env::temp_dir().join(format!("wowc-auto-test-{}", std::process::id()));
        let p = dir.join(FILE);
        let mut st = AutoState { enabled: true, skipped: vec!["0.7.9".into()], ..Default::default() };
        add_try(&mut st, "0.7.62");
        st.went_back = Some(WentBack { version: "0.7.60".into(), reason: "r".into() });
        save(&p, &st).unwrap();
        assert_eq!(load(&p), st);
        fs::write(&p, "{ not json").unwrap();
        assert_eq!(load(&p), AutoState::default());
        assert!(!load(&dir.join("missing.json")).enabled, "off by default");
        let _ = fs::remove_dir_all(&dir);
    }
}
