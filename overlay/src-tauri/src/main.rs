// The overlay shell: a transparent window, a tray, two global shortcuts, and the Go advisor as a
// sidecar. It holds no game logic; it never reads or touches the game process.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

mod guard;
mod recover;
mod update;

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Mutex;
use std::time::Duration;

use serde::{Deserialize, Serialize};
use tauri::menu::{CheckMenuItem, Menu, MenuItem, PredefinedMenuItem};
use tauri::tray::{MouseButton, MouseButtonState, TrayIconBuilder, TrayIconEvent};
use tauri::{AppHandle, Emitter, Manager, RunEvent, State, WebviewWindow, Wry};
use tauri_plugin_global_shortcut::{GlobalShortcutExt, ShortcutState};
use tauri_plugin_shell::process::{CommandChild, CommandEvent};
use tauri_plugin_shell::ShellExt;
use tauri_plugin_window_state::StateFlags;

/// What the sidecar prints on its first stdout line.
#[derive(Clone, Serialize, Deserialize)]
struct Conn {
    url: String,
    token: String,
    /// The AI client the sidecar will use ("" when none) and where it came from: flag, config,
    /// auto or none.
    #[serde(default)]
    agent: String,
    #[serde(default)]
    agent_source: String,
    /// The advisor's own version, for the tray tooltip.
    #[serde(default)]
    version: String,
}

/// Where the AI client choice is kept: "" (no choice), "auto", a preset name or a command.
fn agent_file(app: &AppHandle) -> Option<std::path::PathBuf> {
    app.path().app_config_dir().ok().map(|d| d.join("agent.txt"))
}

fn saved_agent(app: &AppHandle) -> String {
    agent_file(app)
        .and_then(|p| std::fs::read_to_string(p).ok())
        .map(|s| s.trim().to_string())
        .unwrap_or_default()
}

/// The sidecar's current run. `generation` tells a stale run's exit from the current one's.
#[derive(Default)]
struct Sidecar {
    conn: Mutex<Option<Conn>>,
    child: Mutex<Option<CommandChild>>,
    error: Mutex<String>,
    generation: AtomicU64,
    /// The last generation whose health check was reported to the crash check, so each start counts once.
    reported: AtomicU64,
}

/// The crash-check record (`guard.rs`) and where it lives.
#[derive(Default)]
struct Guard {
    state: Mutex<guard::GuardState>,
}

fn now_secs() -> u64 {
    std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0)
}

fn guard_path(app: &AppHandle) -> Option<std::path::PathBuf> {
    app.path().app_data_dir().ok().map(|d| d.join("launch-guard.json"))
}

/// Changes the crash-check record and saves it. A failure to save is logged, never fatal.
fn update_guard(app: &AppHandle, f: impl FnOnce(&guard::GuardState) -> guard::GuardState) {
    let g = app.state::<Guard>();
    let mut st = g.state.lock().unwrap();
    let next = f(&st);
    if let Some(p) = guard_path(app) {
        if let Err(e) = guard::save(&p, &next) {
            recover::log(app, &format!("could not save the launch record: {e}"));
        }
    }
    *st = next;
}

/// "Keep trying" on the safe screen: this version is not checked again, and the app starts as usual.
fn keep_trying(app: &AppHandle) {
    update_guard(app, guard::on_keep_trying);
    recover::log(app, "keep trying chosen: the safe screen will not show again for this version");
    if let Some(w) = main_window(app) {
        let _ = w.show();
    }
    start_sidecar(app);
}

/// Tells the crash check whether the sidecar said hello in time. Once per start.
fn report_health(app: &AppHandle, generation: u64, ok: bool) {
    let sc = app.state::<Sidecar>();
    if sc.reported.swap(generation, Ordering::SeqCst) == generation {
        return;
    }
    update_guard(app, |s| guard::on_health(s, ok, now_secs()));
    if !ok {
        recover::log(app, "the sidecar did not start in time or ended before it said hello");
    }
}

/// Window mode and the tray items that mirror it.
struct Ui {
    overlay: AtomicBool,
    click_through: AtomicBool,
    overlay_item: CheckMenuItem<Wry>,
    click_item: CheckMenuItem<Wry>,
}

#[derive(Serialize)]
struct Status {
    conn: Option<Conn>,
    error: String,
}

#[derive(Serialize)]
struct UiState {
    overlay: bool,
    click_through: bool,
}

/// "WoW Companion 0.2.0 · advisor 0.2.0": the same line the settings panel shows. The advisor
/// part is left off until the sidecar has said its version.
fn version_line(advisor: &str) -> String {
    let app = env!("CARGO_PKG_VERSION");
    if advisor.is_empty() {
        format!("WoW Companion {app}")
    } else {
        format!("WoW Companion {app} \u{b7} advisor {advisor}")
    }
}

fn set_tooltip(app: &AppHandle, advisor: &str) {
    if let Some(t) = app.tray_by_id("main") {
        let _ = t.set_tooltip(Some(version_line(advisor)));
    }
}

fn main_window(app: &AppHandle) -> Option<WebviewWindow> {
    app.get_webview_window("main")
}

fn notice(app: &AppHandle, text: &str) {
    let _ = app.emit("notice", text);
}

fn toggle_visible(app: &AppHandle) {
    if let Some(w) = main_window(app) {
        if w.is_visible().unwrap_or(true) {
            let _ = w.hide();
        } else {
            let _ = w.show();
            let _ = w.set_focus();
        }
    }
}

/// Click-through only exists in Overlay mode; asked for elsewhere it is refused with a notice.
fn apply_click_through(app: &AppHandle, want: bool) {
    let ui = app.state::<Ui>();
    let on = want && ui.overlay.load(Ordering::Relaxed);
    if want && !on {
        notice(app, "Click-through works in Overlay mode only.");
    }
    ui.click_through.store(on, Ordering::Relaxed);
    if let Some(w) = main_window(app) {
        let _ = w.set_ignore_cursor_events(on);
        restore_styles(&w, on);
    }
    let _ = ui.click_item.set_checked(on);
    let _ = app.emit("clickthrough", on);
}

// Window extended-style bits (Windows). Click-through sets WS_EX_TRANSPARENT; turning it off must
// clear it again, and WS_EX_NOACTIVATE must never stay, or the window cannot take keyboard focus.
const WS_EX_TRANSPARENT: isize = 0x20;
const WS_EX_LAYERED: isize = 0x80000;
const WS_EX_NOACTIVATE: isize = 0x0800_0000;

/// The extended style a window should have once click-through is `on` or off, from its current one.
fn click_through_styles(ex: isize, on: bool) -> isize {
    if on {
        ex | WS_EX_TRANSPARENT | WS_EX_LAYERED
    } else {
        ex & !(WS_EX_TRANSPARENT | WS_EX_NOACTIVATE)
    }
}

#[cfg(windows)]
fn ex_style(w: &WebviewWindow) -> Option<isize> {
    #[link(name = "user32")]
    extern "system" {
        fn GetWindowLongPtrW(hwnd: isize, index: i32) -> isize;
    }
    let h = w.hwnd().ok()?.0 as isize;
    Some(unsafe { GetWindowLongPtrW(h, -20) })
}

#[cfg(windows)]
fn restore_styles(w: &WebviewWindow, on: bool) {
    #[link(name = "user32")]
    extern "system" {
        fn SetWindowLongPtrW(hwnd: isize, index: i32, v: isize) -> isize;
    }
    if let (Ok(h), Some(cur)) = (w.hwnd(), ex_style(w)) {
        let want = click_through_styles(cur, on);
        if want != cur {
            unsafe { SetWindowLongPtrW(h.0 as isize, -20, want) };
        }
    }
}

#[cfg(not(windows))]
fn ex_style(_: &WebviewWindow) -> Option<isize> {
    None
}

#[cfg(not(windows))]
fn restore_styles(_: &WebviewWindow, _: bool) {}

/// Whether a global shortcut may be registered. A shortcut swallows its keys system-wide, so it
/// must carry Ctrl, Alt or Super and end in a key that is not an editing or navigation key, and a
/// bare Ctrl+letter clipboard or undo combination is never taken.
fn shortcut_is_safe(spec: &str) -> bool {
    let parts: Vec<String> = spec.split('+').map(|p| p.trim().to_ascii_lowercase()).collect();
    let Some((key, mods)) = parts.split_last() else { return false };
    let has = |names: &[&str]| mods.iter().any(|m| names.contains(&m.as_str()));
    let strong = has(&["ctrl", "control", "commandorcontrol", "cmdorctrl", "alt", "option", "super", "cmd", "command", "meta"]);
    if !strong || key.is_empty() || mods.iter().any(|m| m.is_empty()) {
        return false;
    }
    const EDITING: [&str; 16] = [
        "backspace", "delete", "del", "insert", "ins", "arrowleft", "arrowright", "arrowup", "arrowdown",
        "left", "right", "up", "down", "home", "end", "enter",
    ];
    if EDITING.contains(&key.as_str()) {
        return false;
    }
    let only_ctrl = mods.len() == 1 && has(&["ctrl", "control", "commandorcontrol", "cmdorctrl"]);
    !(only_ctrl && matches!(key.as_str(), "v" | "c" | "x" | "a" | "z" | "y"))
}

fn apply_mode(app: &AppHandle, overlay: bool) {
    let ui = app.state::<Ui>();
    ui.overlay.store(overlay, Ordering::Relaxed);
    if let Some(w) = main_window(app) {
        let _ = w.set_always_on_top(overlay);
        let _ = w.set_skip_taskbar(overlay);
    }
    if !overlay {
        apply_click_through(app, false);
    }
    let _ = ui.overlay_item.set_checked(overlay);
    let _ = app.emit("mode", if overlay { "overlay" } else { "window" });
}

fn stop_sidecar(app: &AppHandle) {
    let sc = app.state::<Sidecar>();
    sc.generation.fetch_add(1, Ordering::SeqCst);
    if let Some(child) = sc.child.lock().unwrap().take() {
        let _ = child.kill();
    }
    *sc.conn.lock().unwrap() = None;
}

fn sidecar_down(app: &AppHandle, error: String) {
    recover::log(app, &format!("sidecar down: {error}"));
    *app.state::<Sidecar>().error.lock().unwrap() = error.clone();
    let _ = app.emit("sidecar-down", error);
}

/// Starts the Go advisor and reads its handshake line; a run that ends is reported as down.
fn start_sidecar(app: &AppHandle) {
    stop_sidecar(app);
    let sc = app.state::<Sidecar>();
    sc.error.lock().unwrap().clear();
    let mine = sc.generation.load(Ordering::SeqCst);
    let cmd = match app.shell().sidecar("wowc-advisor") {
        Ok(c) => {
            let c = c.args(["companion", "--handshake"]);
            match saved_agent(app).as_str() {
                "" => c,
                a => c.args(["--agent", a]),
            }
        }
        Err(e) => {
            report_health(app, mine, false);
            return sidecar_down(app, e.to_string());
        }
    };
    let (mut rx, child) = match cmd.spawn() {
        Ok(p) => p,
        Err(e) => {
            report_health(app, mine, false);
            return sidecar_down(app, e.to_string());
        }
    };
    *sc.child.lock().unwrap() = Some(child);
    recover::log(app, "sidecar started");
    // If it has not said hello within a minute, that start counts as a failed health check.
    let watch = app.clone();
    std::thread::spawn(move || {
        std::thread::sleep(Duration::from_secs(guard::HEALTH_SECS));
        let sc = watch.state::<Sidecar>();
        if sc.generation.load(Ordering::SeqCst) == mine && sc.conn.lock().unwrap().is_none() {
            report_health(&watch, mine, false);
        }
    });
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut last_err = String::new();
        while let Some(ev) = rx.recv().await {
            match ev {
                CommandEvent::Stdout(line) => {
                    if let Ok(c) = serde_json::from_slice::<Conn>(&line) {
                        report_health(&app, mine, true);
                        set_tooltip(&app, &c.version);
                        *app.state::<Sidecar>().conn.lock().unwrap() = Some(c.clone());
                        let _ = app.emit("sidecar-up", c);
                    }
                }
                CommandEvent::Stderr(line) => {
                    let t = String::from_utf8_lossy(&line).trim().to_string();
                    if !t.is_empty() {
                        last_err = t;
                    }
                }
                CommandEvent::Error(e) => last_err = e,
                CommandEvent::Terminated(_) => break,
                _ => {}
            }
        }
        let sc = app.state::<Sidecar>();
        if sc.generation.load(Ordering::SeqCst) == mine {
            // Ended before it ever said hello: a failed health check (a no-op once it was reported).
            report_health(&app, mine, false);
            *sc.conn.lock().unwrap() = None;
            sidecar_down(&app, last_err);
        }
    });
}

#[tauri::command]
fn get_status(sc: State<Sidecar>) -> Status {
    Status {
        conn: sc.conn.lock().unwrap().clone(),
        error: sc.error.lock().unwrap().clone(),
    }
}

#[tauri::command]
fn restart_sidecar(app: AppHandle) {
    start_sidecar(&app);
}

#[tauri::command]
fn get_agent(app: AppHandle) -> String {
    saved_agent(&app)
}

/// Saves the AI client choice ("" clears it) and restarts the sidecar with it.
#[tauri::command]
fn set_agent(app: AppHandle, agent: String) -> Result<(), String> {
    let path = agent_file(&app).ok_or("no config directory")?;
    if let Some(dir) = path.parent() {
        std::fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    }
    std::fs::write(&path, agent.trim()).map_err(|e| e.to_string())?;
    start_sidecar(&app);
    Ok(())
}

/// The official install pages of the AI clients and of ForeverScribe, the Getting Started guide, and
/// this app's latest-release page; the page may open these and nothing else, and only from a click.
const INSTALL_PAGES: [&str; 13] = [
    "https://github.com/WoW-Companion/advisor/releases/latest",
    "https://github.com/WoW-Companion/advisor/releases",
    "https://github.com/MattMaximo/foreverscribe",
    "https://code.claude.com/docs/en/setup",
    "https://github.com/openai/codex",
    "https://github.com/google-gemini/gemini-cli",
    "https://antigravity.google/docs/getting-started?tab=cli",
    "https://github.com/WoW-Companion/advisor/blob/main/docs/getting-started.md",
    "https://platform.claude.com/settings/keys",
    "https://platform.openai.com/api-keys",
    "https://openrouter.ai/keys",
    "https://ollama.com/download",
    "https://lmstudio.ai/download",
];

#[tauri::command]
#[allow(deprecated)]
fn open_install_page(app: AppHandle, url: String) -> Result<(), String> {
    if !INSTALL_PAGES.contains(&url.as_str()) {
        return Err("not an install page".into());
    }
    app.shell().open(url, None).map_err(|e| e.to_string())
}

/// Downloads and verifies one release's installer (and keeps the running version's for going back),
/// backs up the settings, runs the installer and exits. The page names only a version; the shell builds
/// the address. `settings_json` is the page's own settings, saved in the backup.
#[tauri::command]
async fn install_update(window: tauri::Window, app: AppHandle, version: String, settings_json: String) -> Result<(), String> {
    if window.label() != "main" {
        return Err("not allowed".into());
    }
    let a = app.clone();
    let v = version.clone();
    let installer = tauri::async_runtime::spawn_blocking(move || update::prepare(&a, &v, &settings_json))
        .await
        .map_err(|e| e.to_string())??;
    // Going back also puts back the settings that version last ran with (when a backup exists).
    recover::restore_for_go_back(&app, &version);
    update::run_and_exit(&app, &installer)
}

#[tauri::command]
fn get_ui(ui: State<Ui>) -> UiState {
    UiState {
        overlay: ui.overlay.load(Ordering::Relaxed),
        click_through: ui.click_through.load(Ordering::Relaxed),
    }
}

#[tauri::command]
fn set_mode(app: AppHandle, overlay: bool) {
    apply_mode(&app, overlay);
}

#[tauri::command]
fn set_click_through(app: AppHandle, on: bool) {
    apply_click_through(&app, on);
}

#[tauri::command]
fn hide_window(app: AppHandle) {
    if let Some(w) = main_window(&app) {
        let _ = w.hide();
    }
}

#[tauri::command]
fn quit(app: AppHandle) {
    app.exit(0);
}

/// Registers the two global shortcuts, replacing any earlier ones. A key another program holds is
/// reported back, not fatal.
#[tauri::command]
fn set_shortcuts(app: AppHandle, visibility: String, click_through: String) -> Result<(), String> {
    let gs = app.global_shortcut();
    let _ = gs.unregister_all();
    let mut errs = Vec::new();
    for spec in [&visibility, &click_through] {
        if !shortcut_is_safe(spec) {
            return Err(format!("{spec}: a shortcut needs Ctrl, Alt or Super and must not be an editing key"));
        }
    }
    if let Err(e) = gs.on_shortcut(visibility.as_str(), |app, _, ev| {
        if ev.state() == ShortcutState::Pressed {
            toggle_visible(app);
        }
    }) {
        errs.push(format!("{visibility}: {e}"));
    }
    if let Err(e) = gs.on_shortcut(click_through.as_str(), |app, _, ev| {
        if ev.state() == ShortcutState::Pressed {
            let on = app.state::<Ui>().click_through.load(Ordering::Relaxed);
            apply_click_through(app, !on);
        }
    }) {
        errs.push(format!("{click_through}: {e}"));
    }
    if errs.is_empty() {
        Ok(())
    } else {
        Err(errs.join("; "))
    }
}

fn build_tray(app: &tauri::App) -> tauri::Result<()> {
    let toggle = MenuItem::with_id(app, "toggle", "Show / Hide", true, None::<&str>)?;
    let overlay_item = CheckMenuItem::with_id(app, "overlay", "Overlay mode (always on top)", true, true, None::<&str>)?;
    let click_item = CheckMenuItem::with_id(app, "click", "Click-through", true, false, None::<&str>)?;
    let quit = MenuItem::with_id(app, "quit", "Quit", true, None::<&str>)?;
    let sep = PredefinedMenuItem::separator(app)?;
    let sep2 = PredefinedMenuItem::separator(app)?;
    let tray_st = recover::tray_state(update::previous_version(app.handle()).as_deref(), false);
    let check_item = MenuItem::with_id(app, "check", "Check for updates", tray_st.check_enabled, None::<&str>)?;
    let back_item = MenuItem::with_id(app, "goback", &tray_st.go_back_text, tray_st.go_back_enabled, None::<&str>)?;
    let logs_item = MenuItem::with_id(app, "logs", "Open logs folder", tray_st.logs_enabled, None::<&str>)?;
    let menu = Menu::with_items(app, &[&toggle, &overlay_item, &click_item, &sep, &check_item, &back_item, &logs_item, &sep2, &quit])?;
    app.manage(recover::TrayItems { check: check_item, go_back: back_item, logs: logs_item });
    app.manage(Ui {
        overlay: AtomicBool::new(true),
        click_through: AtomicBool::new(false),
        overlay_item,
        click_item,
    });
    let mut tray = TrayIconBuilder::with_id("main")
        .tooltip(version_line(""))
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, ev| {
            let ui = app.state::<Ui>();
            match ev.id().as_ref() {
                "toggle" => toggle_visible(app),
                "overlay" => apply_mode(app, ui.overlay_item.is_checked().unwrap_or(true)),
                "click" => apply_click_through(app, ui.click_item.is_checked().unwrap_or(false)),
                "check" => recover::tray_check(app),
                "goback" => recover::tray_go_back(app),
                "logs" => recover::tray_logs(app),
                "quit" => app.exit(0),
                _ => {}
            }
        })
        .on_tray_icon_event(|tray, ev| {
            if let TrayIconEvent::Click {
                button: MouseButton::Left,
                button_state: MouseButtonState::Up,
                ..
            } = ev
            {
                toggle_visible(tray.app_handle());
            }
        });
    if let Some(icon) = app.default_window_icon() {
        tray = tray.icon(icon.clone());
    }
    tray.build(app)?;
    Ok(())
}

/// Text bytes of a `CF_TEXT` clipboard block: up to the first NUL.
fn cf_text_bytes(raw: &[u8]) -> &[u8] {
    let end = raw.iter().position(|&b| b == 0).unwrap_or(raw.len());
    &raw[..end]
}

/// `CF_TEXT` is in the system (ANSI) code page; convert it to a string. Off Windows there is no
/// code page to ask, so it is read as UTF-8.
#[cfg(windows)]
fn ansi_to_string(bytes: &[u8]) -> String {
    #[link(name = "kernel32")]
    extern "system" {
        fn MultiByteToWideChar(cp: u32, flags: u32, src: *const u8, n: i32, dst: *mut u16, m: i32) -> i32;
    }
    if bytes.is_empty() {
        return String::new();
    }
    unsafe {
        let n = MultiByteToWideChar(0, 0, bytes.as_ptr(), bytes.len() as i32, std::ptr::null_mut(), 0);
        if n <= 0 {
            return String::from_utf8_lossy(bytes).into_owned();
        }
        let mut w = vec![0u16; n as usize];
        MultiByteToWideChar(0, 0, bytes.as_ptr(), bytes.len() as i32, w.as_mut_ptr(), n);
        String::from_utf16_lossy(&w)
    }
}

#[cfg(not(windows))]
fn ansi_to_string(bytes: &[u8]) -> String {
    String::from_utf8_lossy(bytes).into_owned()
}

/// The clipboard's text: Unicode first, then `CF_TEXT` converted from the system code page. The
/// game can write only `CF_TEXT`, which Notepad converts but a web view's paste does not pick up.
#[cfg(windows)]
fn clipboard_text() -> Result<String, String> {
    use std::ffi::c_void;
    #[link(name = "user32")]
    extern "system" {
        fn OpenClipboard(owner: *mut c_void) -> i32;
        fn CloseClipboard() -> i32;
        fn GetClipboardData(format: u32) -> *mut c_void;
    }
    #[link(name = "kernel32")]
    extern "system" {
        fn GlobalLock(h: *mut c_void) -> *mut c_void;
        fn GlobalUnlock(h: *mut c_void) -> i32;
        fn GlobalSize(h: *mut c_void) -> usize;
    }
    const CF_TEXT: u32 = 1;
    const CF_UNICODETEXT: u32 = 13;
    unsafe {
        let mut opened = false;
        for _ in 0..5 {
            if OpenClipboard(std::ptr::null_mut()) != 0 {
                opened = true;
                break;
            }
            std::thread::sleep(std::time::Duration::from_millis(10));
        }
        if !opened {
            return Err("the clipboard is busy".into());
        }
        let mut out = String::new();
        for fmt in [CF_UNICODETEXT, CF_TEXT] {
            let h = GetClipboardData(fmt);
            if h.is_null() {
                continue;
            }
            let p = GlobalLock(h);
            if p.is_null() {
                continue;
            }
            let size = GlobalSize(h);
            if fmt == CF_UNICODETEXT {
                let w = std::slice::from_raw_parts(p as *const u16, size / 2);
                let end = w.iter().position(|&c| c == 0).unwrap_or(w.len());
                out = String::from_utf16_lossy(&w[..end]);
            } else {
                let b = std::slice::from_raw_parts(p as *const u8, size);
                out = ansi_to_string(cf_text_bytes(b));
            }
            GlobalUnlock(h);
            if !out.is_empty() {
                break;
            }
        }
        CloseClipboard();
        Ok(out)
    }
}

#[cfg(not(windows))]
fn clipboard_text() -> Result<String, String> {
    Err("reading the clipboard natively is only supported on Windows".into())
}

/// Reads the clipboard's text for the ask box when the web view's paste comes up empty. Only the
/// main window may ask.
#[tauri::command]
fn read_clipboard(window: tauri::Window) -> Result<String, String> {
    if window.label() != "main" {
        return Err("not allowed".into());
    }
    clipboard_text()
}

/// The native line of the keyboard test: whether the window is focused and what could be eating keys.
#[tauri::command]
fn key_native_state(app: AppHandle) -> String {
    let Some(w) = main_window(&app) else { return "native: no window".into() };
    let ui = app.state::<Ui>();
    let style = ex_style(&w).map(|s| format!("{s:#x}")).unwrap_or_else(|| "n/a".into());
    format!(
        "native: focused={} click-through={} ex-style={} (the host does not intercept keys itself)",
        w.is_focused().unwrap_or(false),
        ui.click_through.load(Ordering::Relaxed),
        style
    )
}

fn main() {
    tauri::Builder::default()
        .plugin(tauri_plugin_shell::init())
        .plugin(tauri_plugin_global_shortcut::Builder::new().build())
        .plugin(
            tauri_plugin_window_state::Builder::default()
                .with_state_flags(StateFlags::SIZE | StateFlags::POSITION)
                .build(),
        )
        .manage(Sidecar::default())
        .manage(Guard::default())
        .manage(recover::Recovery::default())
        .invoke_handler(tauri::generate_handler![
            get_status,
            restart_sidecar,
            get_agent,
            set_agent,
            open_install_page,
            install_update,
            get_ui,
            set_mode,
            set_click_through,
            hide_window,
            quit,
            set_shortcuts,
            read_clipboard,
            key_native_state,
            recover::recovery_state,
            recover::recovery_do
        ])
        .setup(|app| {
            build_tray(app)?;
            let h = app.handle().clone();
            // The crash check: what the last run left, and whether this start is the one that shows the
            // safe screen instead of starting the sidecar.
            let old = guard_path(&h).map(|p| guard::load(&p)).unwrap_or_default();
            let (st, decision) = guard::on_start(&old, env!("CARGO_PKG_VERSION"), update::previous_version(&h).as_deref(), now_secs());
            let previous = update::previous_version(&h); // the cached installer "Go back" would run
            *h.state::<Guard>().state.lock().unwrap() = st.clone();
            if let Some(p) = guard_path(&h) {
                let _ = guard::save(&p, &st);
            }
            recover::log(&h, &format!("start {} (upgraded from {:?}) -> {:?}", env!("CARGO_PKG_VERSION"), st.previous, decision));
            match decision {
                guard::Decision::SafeScreen => {
                    if let Some(w) = main_window(&h) {
                        let _ = w.hide();
                    }
                    recover::show_safe(&h, previous.as_deref());
                }
                guard::Decision::Normal => start_sidecar(&h),
            }
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building the WoW Companion app")
        .run(|app, ev| {
            if let RunEvent::Exit = ev {
                stop_sidecar(app);
                update_guard(app, guard::on_clean_exit);
            }
        });
}

#[cfg(test)]
mod tests {
    use super::{ansi_to_string, cf_text_bytes, click_through_styles, shortcut_is_safe, version_line, WS_EX_LAYERED, WS_EX_NOACTIVATE, WS_EX_TRANSPARENT};

    #[test]
    fn cf_text_stops_at_the_nul_and_converts() {
        assert_eq!(cf_text_bytes(b"How do I? \0junk"), b"How do I? ");
        assert_eq!(cf_text_bytes(b"no nul"), b"no nul");
        assert_eq!(ansi_to_string(cf_text_bytes(b"Where is Stormwind?\r\nHelp\0\0")), "Where is Stormwind?\r\nHelp");
        assert_eq!(ansi_to_string(b""), "");
    }

    #[test]
    fn version_line_names_both_parts() {
        let app = env!("CARGO_PKG_VERSION");
        assert_eq!(version_line(""), format!("WoW Companion {app}"));
        assert_eq!(version_line("0.2.0"), format!("WoW Companion {app} \u{b7} advisor 0.2.0"));
    }

    #[test]
    fn shortcuts_claim_only_their_own_keys() {
        assert!(shortcut_is_safe("Ctrl+Shift+W"));
        assert!(shortcut_is_safe("Ctrl+Alt+T"));
        for bad in ["V", "Ctrl+V", "Ctrl+C", "Shift+V", "Ctrl+Backspace", "Alt+Delete", "Ctrl+ArrowLeft", "Shift+Insert", "", "Ctrl+"] {
            assert!(!shortcut_is_safe(bad), "{bad}");
        }
    }

    #[test]
    fn click_through_off_restores_the_styles() {
        let base = WS_EX_LAYERED;
        let on = click_through_styles(base, true);
        assert_eq!(on, base | WS_EX_TRANSPARENT);
        assert_eq!(click_through_styles(on, false), base);
        assert_eq!(click_through_styles(base | WS_EX_NOACTIVATE, false), base);
        assert_eq!(click_through_styles(click_through_styles(base, true), true), on);
    }
}
