// The part of the shell that works when the page, the sidecar or the AI connection does not: the tray
// menu's update items, and one small window (`recovery.html`, static, no scripts of the main UI) that
// shows the update check, the go-back confirmation and the safe screen after a bad upgrade. Everything
// here runs in Rust over `update.rs`'s verified install path.
use std::sync::atomic::{AtomicBool, Ordering};
use std::sync::Mutex;

use serde::Serialize;
use tauri::menu::MenuItem;
use tauri::{AppHandle, Manager, State, WebviewUrl, WebviewWindowBuilder, Wry};

use crate::update::{self, Check};

pub const LABEL: &str = "recovery";

#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct Button {
    pub id: String,
    pub label: String,
}

/// What the recovery window shows: a title, a few plain sentences and the buttons.
#[derive(Clone, Debug, Default, PartialEq, Serialize)]
pub struct View {
    pub title: String,
    pub text: String,
    pub buttons: Vec<Button>,
    /// Working: the buttons are off and `text` follows the installer's progress.
    pub busy: bool,
}

fn button(id: &str, label: &str) -> Button {
    Button { id: id.into(), label: label.into() }
}

/// The safe screen after a bad upgrade: what went wrong and the three ways out. "Go back" is left
/// out when no earlier installer is saved.
pub fn safe_view(previous: Option<&str>) -> View {
    let mut buttons = Vec::new();
    if let Some(p) = previous {
        buttons.push(button("go_back", &format!("Go back to {p}")));
    }
    buttons.push(button("keep_trying", "Keep trying"));
    buttons.push(button("logs", "Open logs"));
    View {
        title: "WoW Companion".into(),
        text: "This update isn't working on your PC.".into(),
        buttons,
        busy: false,
    }
}

/// The answer of the tray's "Check for updates".
pub fn check_view(c: &Check, running: &str) -> View {
    let (text, buttons) = match c {
        Check::Available(v) => (
            format!("Version {v} is available. You have {running}."),
            vec![button("install", &format!("Install {v}")), button("close", "Later")],
        ),
        Check::Settling(v) => (
            format!("Version {v} came out less than a day ago. It is offered after a day, so a bad release can be pulled first. You have {running}."),
            vec![button("close", "OK")],
        ),
        Check::UpToDate => (format!("You are up to date ({running})."), vec![button("close", "OK")]),
    };
    View { title: "Check for updates".into(), text, buttons, busy: false }
}

pub fn confirm_back_view(previous: &str) -> View {
    View {
        title: "Go back".into(),
        text: format!("Go back to {previous}? Your settings from that version are restored if a backup exists; otherwise they stay as they are."),
        buttons: vec![button("go_back", &format!("Go back to {previous}")), button("close", "Cancel")],
        busy: false,
    }
}

pub fn busy_view(title: &str, text: &str) -> View {
    View { title: title.into(), text: text.into(), buttons: Vec::new(), busy: true }
}

pub fn error_view(title: &str, text: &str) -> View {
    View { title: title.into(), text: text.into(), buttons: vec![button("close", "OK")], busy: false }
}

/// Which tray items are on and what the go-back item says.
#[derive(Clone, Debug, PartialEq)]
pub struct TrayState {
    pub check_enabled: bool,
    pub go_back_enabled: bool,
    pub go_back_text: String,
    pub logs_enabled: bool,
}

/// `previous` is the version a cached installer exists for; `busy` is an install under way (the items
/// that start one are off, so nothing runs twice). The logs folder can always be opened.
pub fn tray_state(previous: Option<&str>, busy: bool) -> TrayState {
    TrayState {
        check_enabled: !busy,
        go_back_enabled: !busy && previous.is_some(),
        go_back_text: match previous {
            Some(p) => format!("Go back to {p}"),
            None => "Go back (no earlier version saved)".into(),
        },
        logs_enabled: true,
    }
}

/// The tray's update items, kept so their state can follow the cache and the busy flag.
pub struct TrayItems {
    pub check: MenuItem<Wry>,
    pub go_back: MenuItem<Wry>,
    pub logs: MenuItem<Wry>,
}

/// The recovery window's current view, the version "Install" would install and whether an install is
/// under way.
#[derive(Default)]
pub struct Recovery {
    view: Mutex<View>,
    target: Mutex<String>,
    busy: AtomicBool,
}

pub fn refresh_tray(app: &AppHandle) {
    let Some(items) = app.try_state::<TrayItems>() else { return };
    let busy = app.state::<Recovery>().busy.load(Ordering::SeqCst);
    let st = tray_state(update::previous_version(app).as_deref(), busy);
    let _ = items.check.set_enabled(st.check_enabled);
    let _ = items.go_back.set_enabled(st.go_back_enabled);
    let _ = items.go_back.set_text(&st.go_back_text);
    let _ = items.logs.set_enabled(st.logs_enabled);
}

fn set_view(app: &AppHandle, v: View) {
    *app.state::<Recovery>().view.lock().unwrap() = v;
}

fn set_busy(app: &AppHandle, busy: bool) {
    app.state::<Recovery>().busy.store(busy, Ordering::SeqCst);
    refresh_tray(app);
}

fn log_dir(app: &AppHandle) -> Option<std::path::PathBuf> {
    app.path().app_log_dir().ok()
}

pub fn log(app: &AppHandle, text: &str) {
    if let Some(d) = log_dir(app) {
        update::log_line(&d, text);
    }
}

/// Shows the recovery window (building it the first time). Called off the main thread: building a
/// window from a menu callback can stall the event loop.
pub fn show_window(app: &AppHandle) {
    let app = app.clone();
    std::thread::spawn(move || {
        if let Some(w) = app.get_webview_window(LABEL) {
            let _ = w.show();
            let _ = w.set_focus();
            return;
        }
        let built = WebviewWindowBuilder::new(&app, LABEL, WebviewUrl::App("recovery.html".into()))
            .title("WoW Companion")
            .inner_size(460.0, 300.0)
            .resizable(false)
            .center()
            .always_on_top(true)
            .build();
        if let Err(e) = built {
            log(&app, &format!("could not open the recovery window: {e}"));
        }
    });
}

fn close_window(app: &AppHandle) {
    if let Some(w) = app.get_webview_window(LABEL) {
        let _ = w.close();
    }
}

fn open_logs(app: &AppHandle) {
    match log_dir(app).ok_or_else(|| "no log folder".to_string()).and_then(|d| update::open_folder(&d)) {
        Ok(()) => {}
        Err(e) => log(app, &format!("could not open the logs folder: {e}")),
    }
}

/// The tray's "Check for updates": asks GitHub from the shell and shows the answer.
pub fn tray_check(app: &AppHandle) {
    set_view(app, busy_view("Check for updates", "Checking for updates…"));
    show_window(app);
    let app = app.clone();
    std::thread::spawn(move || {
        let running = env!("CARGO_PKG_VERSION");
        match update::check_latest(&app) {
            Ok(c) => {
                log(&app, &format!("update check: {c:?}"));
                *app.state::<Recovery>().target.lock().unwrap() = match &c {
                    Check::Available(v) => v.clone(),
                    _ => String::new(),
                };
                set_view(&app, check_view(&c, running));
            }
            Err(e) => {
                log(&app, &format!("update check failed: {e}"));
                set_view(&app, error_view("Check for updates", &format!("Could not check for updates: {e}")));
            }
        }
    });
}

/// The tray's "Go back to <previous>": asks first, then `go_back` does it.
pub fn tray_go_back(app: &AppHandle) {
    match update::previous_version(app) {
        Some(p) => {
            set_view(app, confirm_back_view(&p));
            show_window(app);
        }
        None => {
            set_view(app, error_view("Go back", "There is no earlier version saved on this PC to go back to."));
            show_window(app);
        }
    }
}

pub fn tray_logs(app: &AppHandle) {
    open_logs(app);
}

/// The safe screen.
pub fn show_safe(app: &AppHandle, previous: Option<&str>) {
    set_view(app, safe_view(previous));
    show_window(app);
}

/// Hands restored page settings to the page that is running (it keeps them in `localStorage`), so
/// the settings the older version last used are what it shows. The page is on its way out, so this
/// is a last write, not a sync.
pub fn give_settings_to_page(app: &AppHandle, json: &str) {
    let Some(w) = app.get_webview_window("main") else { return };
    let Ok(quoted) = serde_json::to_string(json) else { return };
    let _ = w.eval(&format!("try{{localStorage.setItem('wowc',{quoted})}}catch(e){{}}"));
}

/// Settings of `target` restored when the install is a go-back (an older version than the one
/// running); the page's own settings go to the page. Any problem is logged and never stops the install.
pub fn restore_for_go_back(app: &AppHandle, target: &str) {
    let running = env!("CARGO_PKG_VERSION");
    if !(update::version_key(target) < update::version_key(running)) || update::version_key(target).is_none() {
        return;
    }
    match update::restore_backup(app, target) {
        Ok(Some(r)) => {
            log(app, &format!("restored the settings backup of {target}"));
            if let Some(j) = r.settings_json {
                give_settings_to_page(app, &j);
            }
        }
        Ok(None) => log(app, &format!("no settings backup of {target}; current settings kept")),
        Err(e) => log(app, &format!("could not restore the settings of {target}: {e}")),
    }
}

/// Goes back: the verified cached installer of `target`, its settings backup, then the installer.
fn go_back(app: &AppHandle, target: &str) -> Result<(), String> {
    let installer = update::prepare_back(app, target)?;
    restore_for_go_back(app, target);
    update::run_and_exit(app, &installer)
}

fn install(app: &AppHandle, target: &str) -> Result<(), String> {
    let installer = update::prepare(app, target, "")?;
    update::run_and_exit(app, &installer)
}

fn work(app: &AppHandle, title: &str, first: &str, job: impl FnOnce(&AppHandle) -> Result<(), String> + Send + 'static) {
    set_view(app, busy_view(title, first));
    set_busy(app, true);
    let app = app.clone();
    let title = title.to_string();
    std::thread::spawn(move || {
        if let Err(e) = job(&app) {
            log(&app, &format!("{title}: {e}"));
            set_view(&app, error_view(&title, &e));
            set_busy(&app, false);
        }
    });
}

#[tauri::command]
pub fn recovery_state(window: tauri::Window, rec: State<Recovery>) -> Result<View, String> {
    if window.label() != LABEL {
        return Err("not allowed".into());
    }
    let mut v = rec.view.lock().unwrap().clone();
    if v.busy && rec.busy.load(Ordering::SeqCst) {
        let p = update::progress();
        if !p.is_empty() {
            v.text = p;
        }
    }
    Ok(v)
}

#[tauri::command]
pub fn recovery_do(window: tauri::Window, app: AppHandle, action: String) -> Result<(), String> {
    if window.label() != LABEL {
        return Err("not allowed".into());
    }
    match action.as_str() {
        "close" => close_window(&app),
        "logs" => open_logs(&app),
        "keep_trying" => {
            crate::keep_trying(&app);
            close_window(&app);
        }
        "install" => {
            let target = app.state::<Recovery>().target.lock().unwrap().clone();
            if !update::valid_version(&target) {
                return Err("There is nothing to install.".into());
            }
            work(&app, "Install", "Getting the update…", move |a| install(a, &target));
        }
        "go_back" => {
            let Some(target) = update::previous_version(&app) else {
                return Err("There is no earlier version saved on this PC.".into());
            };
            work(&app, "Go back", "Going back…", move |a| go_back(a, &target));
        }
        _ => return Err("unknown action".into()),
    }
    Ok(())
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn the_safe_screen_says_what_the_mission_says() {
        let v = safe_view(Some("0.7.54"));
        assert_eq!(v.text, "This update isn't working on your PC.");
        let labels: Vec<&str> = v.buttons.iter().map(|b| b.label.as_str()).collect();
        assert_eq!(labels, vec!["Go back to 0.7.54", "Keep trying", "Open logs"]);
        let ids: Vec<&str> = v.buttons.iter().map(|b| b.id.as_str()).collect();
        assert_eq!(ids, vec!["go_back", "keep_trying", "logs"]);
    }

    #[test]
    fn the_safe_screen_without_an_earlier_installer_cannot_offer_going_back() {
        let ids: Vec<String> = safe_view(None).buttons.into_iter().map(|b| b.id).collect();
        assert_eq!(ids, vec!["keep_trying".to_string(), "logs".to_string()]);
    }

    #[test]
    fn the_tray_menu_follows_the_cache_and_the_busy_flag() {
        let idle = tray_state(Some("0.7.54"), false);
        assert!(idle.check_enabled && idle.go_back_enabled && idle.logs_enabled);
        assert_eq!(idle.go_back_text, "Go back to 0.7.54");
        let none = tray_state(None, false);
        assert!(none.check_enabled && !none.go_back_enabled && none.logs_enabled);
        assert_eq!(none.go_back_text, "Go back (no earlier version saved)");
        let busy = tray_state(Some("0.7.54"), true);
        assert!(!busy.check_enabled && !busy.go_back_enabled, "an install under way is not started twice");
        assert!(busy.logs_enabled, "the logs can always be opened");
    }

    #[test]
    fn the_update_check_view_offers_install_only_when_there_is_one() {
        let ids = |c: Check| check_view(&c, "0.7.54").buttons.into_iter().map(|b| b.id).collect::<Vec<_>>();
        assert_eq!(ids(Check::Available("0.7.55".into())), vec!["install".to_string(), "close".to_string()]);
        assert_eq!(ids(Check::UpToDate), vec!["close".to_string()]);
        assert_eq!(ids(Check::Settling("0.7.55".into())), vec!["close".to_string()]);
        assert!(check_view(&Check::UpToDate, "0.7.54").text.contains("up to date"));
    }

    #[test]
    fn a_working_view_has_no_buttons_and_a_failure_can_be_closed() {
        assert!(busy_view("Go back", "Going back…").buttons.is_empty());
        assert!(busy_view("Go back", "x").busy);
        assert_eq!(error_view("Go back", "no").buttons.len(), 1);
        assert!(confirm_back_view("0.7.54").buttons.iter().any(|b| b.id == "go_back"));
    }
}
