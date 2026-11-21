// The overlay shell: a transparent window, a tray, two global shortcuts, and the Go advisor as a
// sidecar. It holds no game logic; it never reads or touches the game process.
#![cfg_attr(not(debug_assertions), windows_subsystem = "windows")]

use std::sync::atomic::{AtomicBool, AtomicU64, Ordering};
use std::sync::Mutex;

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
    }
    let _ = ui.click_item.set_checked(on);
    let _ = app.emit("clickthrough", on);
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
        Err(e) => return sidecar_down(app, e.to_string()),
    };
    let (mut rx, child) = match cmd.spawn() {
        Ok(p) => p,
        Err(e) => return sidecar_down(app, e.to_string()),
    };
    *sc.child.lock().unwrap() = Some(child);
    let app = app.clone();
    tauri::async_runtime::spawn(async move {
        let mut last_err = String::new();
        while let Some(ev) = rx.recv().await {
            match ev {
                CommandEvent::Stdout(line) => {
                    if let Ok(c) = serde_json::from_slice::<Conn>(&line) {
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

/// The official install pages of the AI clients and of ForeverScribe, and the Getting Started guide; the page may open these and nothing else.
const INSTALL_PAGES: [&str; 5] = [
    "https://github.com/MattMaximo/foreverscribe",
    "https://docs.claude.com/en/docs/claude-code/setup",
    "https://github.com/openai/codex",
    "https://github.com/google-gemini/gemini-cli",
    "https://github.com/WoW-Companion/advisor/blob/main/docs/getting-started.md",
];

#[tauri::command]
#[allow(deprecated)]
fn open_install_page(app: AppHandle, url: String) -> Result<(), String> {
    if !INSTALL_PAGES.contains(&url.as_str()) {
        return Err("not an install page".into());
    }
    app.shell().open(url, None).map_err(|e| e.to_string())
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
    let menu = Menu::with_items(app, &[&toggle, &overlay_item, &click_item, &sep, &quit])?;
    app.manage(Ui {
        overlay: AtomicBool::new(true),
        click_through: AtomicBool::new(false),
        overlay_item,
        click_item,
    });
    let mut tray = TrayIconBuilder::new()
        .tooltip(concat!("WoW Companion ", env!("CARGO_PKG_VERSION")))
        .menu(&menu)
        .show_menu_on_left_click(false)
        .on_menu_event(|app, ev| {
            let ui = app.state::<Ui>();
            match ev.id().as_ref() {
                "toggle" => toggle_visible(app),
                "overlay" => apply_mode(app, ui.overlay_item.is_checked().unwrap_or(true)),
                "click" => apply_click_through(app, ui.click_item.is_checked().unwrap_or(false)),
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
        .invoke_handler(tauri::generate_handler![
            get_status,
            restart_sidecar,
            get_agent,
            set_agent,
            open_install_page,
            get_ui,
            set_mode,
            set_click_through,
            hide_window,
            quit,
            set_shortcuts
        ])
        .setup(|app| {
            build_tray(app)?;
            start_sidecar(app.handle());
            Ok(())
        })
        .build(tauri::generate_context!())
        .expect("error while building the WoW Companion app")
        .run(|app, ev| {
            if let RunEvent::Exit = ev {
                stop_sidecar(app);
            }
        });
}
