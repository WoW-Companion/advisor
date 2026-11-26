// Why the app ended: the shell's lifecycle record. Everything that is not tied to the window system
// lives here as plain functions over paths and numbers, so it is testable: local-time stamps, the
// size-capped rotating logs, the exit reason, the panic hook's text, the heartbeat file, the
// "previous session ended unexpectedly" line, the helper's restart backoff, redaction, and the
// diagnostics zip. Nothing here logs a key, a token or a prompt: the helper's handshake line is never
// written, and any line that looks like a secret is replaced before it reaches a file or a zip.
use std::fs;
use std::path::{Path, PathBuf};
use std::sync::{Mutex, OnceLock};

use serde::{Deserialize, Serialize};

/// Each log is capped at about this size...
pub const LOG_CAP: u64 = 1024 * 1024;
/// ...and this many older files are kept next to it (`wowc-shell.1.log` is the newest of them).
pub const LOG_KEEP: u32 = 3;
/// The shell's own log and the helper's.
pub const SHELL_LOG: &str = "wowc-shell.log";
pub const HELPER_LOG: &str = "wowc-advisor.log";
/// The heartbeat state file (in the app data folder, not the log folder).
pub const HEARTBEAT_FILE: &str = "heartbeat.json";
/// How often the heartbeat is written.
pub const HEARTBEAT_SECS: u64 = 60;
/// The helper may die this many times inside `RESTART_WINDOW_SECS` before the shell gives up.
pub const RESTART_TRIES: usize = 3;
pub const RESTART_WINDOW_SECS: u64 = 5 * 60;
/// Seconds to wait before each restart try.
const BACKOFF: [u64; RESTART_TRIES] = [2, 10, 30];
/// What the player is told when the helper cannot stay up.
pub const PLANNER_STOPPED: &str = "The planner stopped. Restart the app or open the logs.";

// --- local time -----------------------------------------------------------------------------------

/// Seconds east of UTC for the machine's time zone now (0 where it cannot be asked).
#[cfg(windows)]
pub fn offset_secs() -> i64 {
    #[repr(C)]
    struct Tzi {
        bias: i32,
        std_name: [u16; 32],
        std_date: [u16; 8],
        std_bias: i32,
        dst_name: [u16; 32],
        dst_date: [u16; 8],
        dst_bias: i32,
    }
    #[link(name = "kernel32")]
    extern "system" {
        fn GetTimeZoneInformation(tzi: *mut Tzi) -> u32;
    }
    let mut t: Tzi = unsafe { std::mem::zeroed() };
    let r = unsafe { GetTimeZoneInformation(&mut t) };
    let extra = match r {
        1 => t.std_bias,
        2 => t.dst_bias,
        _ => 0,
    };
    -((t.bias + extra) as i64) * 60
}

#[cfg(not(windows))]
pub fn offset_secs() -> i64 {
    0
}

pub fn now_secs() -> u64 {
    std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0)
}

/// Days since 1970-01-01 to (year, month, day), proleptic Gregorian.
fn civil(days: i64) -> (i64, i64, i64) {
    let z = days + 719_468;
    let era = z.div_euclid(146_097);
    let doe = z.rem_euclid(146_097);
    let yoe = (doe - doe / 1460 + doe / 36_524 - doe / 146_096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    (yoe + era * 400 + if m <= 2 { 1 } else { 0 }, m, d)
}

/// "2026-11-25T15:52:07-07:00": ISO 8601 for `epoch` seen from `offset` seconds east of UTC.
pub fn iso(epoch: u64, offset: i64) -> String {
    let local = epoch as i64 + offset;
    let (y, m, d) = civil(local.div_euclid(86_400));
    let sod = local.rem_euclid(86_400);
    let sign = if offset < 0 { '-' } else { '+' };
    let o = offset.abs();
    format!("{y:04}-{m:02}-{d:02}T{:02}:{:02}:{:02}{sign}{:02}:{:02}", sod / 3600, sod % 3600 / 60, sod % 60, o / 3600, o % 3600 / 60)
}

/// "15:52": the clock time the player sees.
pub fn clock(epoch: u64, offset: i64) -> String {
    let sod = (epoch as i64 + offset).rem_euclid(86_400);
    format!("{:02}:{:02}", sod / 3600, sod % 3600 / 60)
}

/// "<epoch> <iso local time>": every log line starts with this.
fn stamp(epoch: u64, offset: i64) -> String {
    format!("{epoch} {}", iso(epoch, offset))
}

// --- rotating logs --------------------------------------------------------------------------------

/// `wowc-shell.log` -> `wowc-shell.2.log`.
fn rotated(name: &str, n: u32) -> String {
    match name.rsplit_once('.') {
        Some((stem, ext)) => format!("{stem}.{n}.{ext}"),
        None => format!("{name}.{n}"),
    }
}

/// Appends `line` (one line, newline added) to `dir/name`. When that would take the file past `cap`
/// bytes it is first rotated: the newest older file is `name.1`, and only `keep` of them are kept.
pub fn append_rotating(dir: &Path, name: &str, line: &str, cap: u64, keep: u32) {
    let _ = fs::create_dir_all(dir);
    let file = dir.join(name);
    let text = format!("{}\n", line.replace(['\n', '\r'], " "));
    let size = fs::metadata(&file).map(|m| m.len()).unwrap_or(0);
    if size > 0 && size + text.len() as u64 > cap {
        let _ = fs::remove_file(dir.join(rotated(name, keep)));
        for n in (1..keep).rev() {
            let _ = fs::rename(dir.join(rotated(name, n)), dir.join(rotated(name, n + 1)));
        }
        let _ = fs::rename(&file, dir.join(rotated(name, 1)));
    }
    if let Ok(mut f) = fs::OpenOptions::new().create(true).append(true).open(&file) {
        use std::io::Write;
        let _ = f.write_all(text.as_bytes());
    }
}

/// One line of the shell's own log.
pub fn log_line(dir: &Path, text: &str) {
    let line = format!("{} {}", stamp(now_secs(), offset_secs()), text);
    append_rotating(dir, SHELL_LOG, &line, LOG_CAP, LOG_KEEP);
}

/// One line the helper printed, kept in its own log (secret-looking lines are replaced).
pub fn helper_line(dir: &Path, stream: &str, text: &str) {
    let t = text.trim_end();
    if t.is_empty() {
        return;
    }
    let line = format!("{} {stream} {}", stamp(now_secs(), offset_secs()), redact_line(t));
    append_rotating(dir, HELPER_LOG, &line, LOG_CAP, LOG_KEEP);
}

static LOG_DIR: OnceLock<PathBuf> = OnceLock::new();
static REASON: Mutex<String> = Mutex::new(String::new());

/// Remembers where the logs go, for code that has no app handle (the panic hook, the OS shutdown
/// handler). Set once at start.
pub fn set_log_dir(dir: PathBuf) {
    let _ = LOG_DIR.set(dir);
}

/// Writes to the shell log from anywhere.
pub fn log(text: &str) {
    if let Some(d) = LOG_DIR.get() {
        log_line(d, text);
    }
}

// --- the exit reason ------------------------------------------------------------------------------

/// Says why the app is about to end. The first reason given wins: a quit that closes the window is a
/// quit, not a window close.
pub fn set_reason(why: &str) {
    let mut r = REASON.lock().unwrap_or_else(|e| e.into_inner());
    if r.is_empty() {
        *r = why.to_string();
    }
}

pub fn reason() -> String {
    REASON.lock().unwrap_or_else(|e| e.into_inner()).clone()
}

/// The line logged when the app ends.
pub fn exit_text(reason: &str, code: Option<i32>) -> String {
    let why = if reason.is_empty() { "unknown (nothing said why)" } else { reason };
    match code {
        Some(c) => format!("exit: {why} (code {c})"),
        None => format!("exit: {why}"),
    }
}

// --- panics ---------------------------------------------------------------------------------------

/// The text of a panic: its message and where it happened.
pub fn panic_text(msg: &str, loc: Option<(&str, u32)>) -> String {
    match loc {
        Some((f, l)) => format!("PANIC: {msg} at {f}:{l}"),
        None => format!("PANIC: {msg}"),
    }
}

/// Writes a panic to the shell log in `dir`.
pub fn log_panic(dir: &Path, msg: &str, loc: Option<(&str, u32)>) {
    log_line(dir, &panic_text(msg, loc));
}

/// Installs the panic hook: the message and location go to the log, then the usual handler runs.
pub fn install_panic_hook() {
    let prev = std::panic::take_hook();
    std::panic::set_hook(Box::new(move |info| {
        let msg = if let Some(s) = info.payload().downcast_ref::<&str>() {
            s.to_string()
        } else if let Some(s) = info.payload().downcast_ref::<String>() {
            s.clone()
        } else {
            "(no message)".to_string()
        };
        let loc = info.location().map(|l| (l.file().to_string(), l.line()));
        if let Some(d) = LOG_DIR.get() {
            log_panic(d, &msg, loc.as_ref().map(|(f, l)| (f.as_str(), *l)));
        }
        prev(info);
    }));
}

/// Logs a Windows logoff or shutdown (the console control handler is told of both, even for a
/// program with no console), then lets the default handling go on.
#[cfg(windows)]
pub fn install_shutdown_handler() {
    #[link(name = "kernel32")]
    extern "system" {
        fn SetConsoleCtrlHandler(handler: Option<unsafe extern "system" fn(u32) -> i32>, add: i32) -> i32;
    }
    unsafe extern "system" fn on_ctrl(kind: u32) -> i32 {
        let why = match kind {
            2 => "console close",
            5 => "OS logoff",
            6 => "OS shutdown",
            _ => return 0,
        };
        set_reason(why);
        log(&exit_text(why, None));
        0
    }
    unsafe {
        SetConsoleCtrlHandler(Some(on_ctrl), 1);
    }
}

#[cfg(not(windows))]
pub fn install_shutdown_handler() {}

// --- the heartbeat --------------------------------------------------------------------------------

/// "Alive" every minute: a tiny file, not the log.
#[derive(Clone, Debug, Default, PartialEq, Serialize, Deserialize)]
#[serde(default)]
pub struct Heartbeat {
    pub alive_at: u64,
    pub alive_iso: String,
    pub version: String,
    pub session_at: u64,
}

pub fn heartbeat_for(version: &str, session_at: u64, now: u64, offset: i64) -> Heartbeat {
    Heartbeat { alive_at: now, alive_iso: iso(now, offset), version: version.to_string(), session_at }
}

pub fn write_heartbeat(path: &Path, hb: &Heartbeat) -> Result<(), String> {
    if let Some(dir) = path.parent() {
        fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    }
    let text = serde_json::to_string(hb).map_err(|e| e.to_string())?;
    let tmp = path.with_extension("json.tmp");
    fs::write(&tmp, text).map_err(|e| e.to_string())?;
    fs::rename(&tmp, path).map_err(|e| e.to_string())
}

pub fn read_heartbeat(path: &Path) -> Option<Heartbeat> {
    fs::read_to_string(path).ok().and_then(|t| serde_json::from_str(&t).ok())
}

/// What the start of the app says about the session before it. `open` is the crash check's "no clean
/// exit recorded". Returns (the log line, the player's notice) when that session ended unexpectedly.
pub fn previous_session(open: bool, hb: Option<&Heartbeat>, offset: i64) -> Option<(String, String)> {
    if !open {
        return None;
    }
    let (line, at) = match hb {
        Some(h) if h.alive_at > 0 => {
            (format!("previous session ended unexpectedly; last alive at {}, version {}", iso(h.alive_at, offset), h.version), clock(h.alive_at, offset))
        }
        _ => ("previous session ended unexpectedly; no heartbeat was recorded".to_string(), String::new()),
    };
    let note = if at.is_empty() { "WoW Companion closed unexpectedly".to_string() } else { format!("WoW Companion closed unexpectedly at {at}") };
    Some((line, note))
}

// --- the helper's restarts ------------------------------------------------------------------------

/// The helper has just died. Returns (try number, seconds to wait) for another start, or `None` when
/// it has already been started `RESTART_TRIES` times inside the last five minutes. `history` holds the
/// times of the earlier tries.
pub fn restart_delay(history: &mut Vec<u64>, now: u64) -> Option<(usize, u64)> {
    history.retain(|t| now.saturating_sub(*t) < RESTART_WINDOW_SECS);
    if history.len() >= RESTART_TRIES {
        return None;
    }
    let delay = BACKOFF[history.len()];
    history.push(now);
    Some((history.len(), delay))
}

// --- redaction ------------------------------------------------------------------------------------

const SECRET_WORDS: [&str; 10] = ["api_key", "apikey", "api-key", "token", "secret", "authorization", "bearer ", "password", "sk-", "prompt"];

/// A line that could carry a key, a token or a prompt is replaced whole.
pub fn redact_line(line: &str) -> String {
    let low = line.to_ascii_lowercase();
    if SECRET_WORDS.iter().any(|w| low.contains(w)) {
        "[redacted: this line may hold a secret]".to_string()
    } else {
        line.to_string()
    }
}

pub fn redact_text(text: &str) -> String {
    text.lines().map(redact_line).collect::<Vec<_>>().join("\n")
}

// --- the diagnostics text -------------------------------------------------------------------------

/// What the page needs from disk for its "Copy diagnostics" text: the shell log's tail and the
/// crash-check record. The advisor strips names, paths and keys before anything is shown.
#[derive(Serialize)]
pub struct DiagnosticsFiles {
    pub shell_log: String,
    pub guard: String,
}

/// Lines in the shell log that record how the app started, ended or crashed.
fn is_record(line: &str) -> bool {
    let l = line.to_lowercase();
    ["exit", "crash", "panic", "unexpected", "giving up", "restart try", "silent", "start "].iter().any(|k| l.contains(k))
}

/// The last 20 lines of the shell log, then up to 10 earlier crash and exit records.
pub fn shell_log_tail(text: &str) -> String {
    let lines: Vec<&str> = text.lines().collect();
    let split = lines.len().saturating_sub(20);
    let mut older: Vec<&str> = lines[..split].iter().copied().filter(|l| is_record(l)).collect();
    let skip = older.len().saturating_sub(10);
    older.drain(..skip);
    let mut out = String::new();
    if !older.is_empty() {
        out.push_str("-- earlier records --\n");
        for l in older {
            out.push_str(l);
            out.push('\n');
        }
        out.push_str("-- last 20 lines --\n");
    }
    for l in &lines[split..] {
        out.push_str(l);
        out.push('\n');
    }
    out
}

/// Reads the two files; a missing one is an empty string.
pub fn diagnostics_files(log_dir: &Path, data_dir: &Path) -> DiagnosticsFiles {
    let read = |p: PathBuf| fs::read(p).map(|b| String::from_utf8_lossy(&b).to_string()).unwrap_or_default();
    DiagnosticsFiles { shell_log: redact_text(&shell_log_tail(&read(log_dir.join(SHELL_LOG)))), guard: redact_text(&read(data_dir.join("launch-guard.json"))) }
}

#[cfg(test)]
mod tests {
    use super::*;

    fn tmp(tag: &str) -> PathBuf {
        let d = std::env::temp_dir().join(format!("wowc-life-{tag}-{}", std::process::id()));
        let _ = fs::remove_dir_all(&d);
        fs::create_dir_all(&d).unwrap();
        d
    }

    #[test]
    fn times_are_iso_with_the_offset() {
        // 2026-11-25 22:52:07 UTC is 15:52:07 in Mountain time (-07:00).
        let t = 1_795_647_127;
        assert_eq!(iso(t, 0), "2026-11-25T22:52:07+00:00");
        assert_eq!(iso(t, -7 * 3600), "2026-11-25T15:52:07-07:00");
        assert_eq!(iso(t, 5 * 3600 + 1800), "2026-11-26T04:22:07+05:30");
        assert_eq!(clock(t, -7 * 3600), "15:52");
        assert_eq!(iso(0, 0), "1970-01-01T00:00:00+00:00");
    }

    #[test]
    fn exit_reasons_are_written_with_both_stamps() {
        let d = tmp("exit");
        set_reason("tray quit");
        set_reason("window close"); // the first reason wins
        assert_eq!(reason(), "tray quit");
        log_line(&d, &exit_text(&reason(), Some(0)));
        let text = fs::read_to_string(d.join(SHELL_LOG)).unwrap();
        let line = text.lines().next().unwrap();
        let mut parts = line.splitn(3, ' ');
        assert!(parts.next().unwrap().parse::<u64>().is_ok(), "epoch first: {line}");
        assert!(parts.next().unwrap().contains('T'), "ISO next: {line}");
        assert_eq!(parts.next().unwrap(), "exit: tray quit (code 0)");
        assert_eq!(exit_text("", None), "exit: unknown (nothing said why)");
        let _ = fs::remove_dir_all(&d);
    }

    #[test]
    fn the_panic_hook_text_has_message_and_location() {
        let d = tmp("panic");
        log_panic(&d, "boom", Some(("src/main.rs", 42)));
        let text = fs::read_to_string(d.join(SHELL_LOG)).unwrap();
        assert!(text.trim_end().ends_with("PANIC: boom at src/main.rs:42"), "{text}");
        assert_eq!(panic_text("x", None), "PANIC: x");
        let _ = fs::remove_dir_all(&d);
    }

    #[test]
    fn a_heartbeat_with_no_clean_exit_gives_the_unexpected_line() {
        let d = tmp("hb");
        let p = d.join(HEARTBEAT_FILE);
        let off = -7 * 3600;
        write_heartbeat(&p, &heartbeat_for("0.7.74", 1_795_645_000, 1_795_647_127, off)).unwrap();
        let hb = read_heartbeat(&p).unwrap();
        let (line, note) = previous_session(true, Some(&hb), off).unwrap();
        assert_eq!(line, "previous session ended unexpectedly; last alive at 2026-11-25T15:52:07-07:00, version 0.7.74");
        assert_eq!(note, "WoW Companion closed unexpectedly at 15:52");
        // A clean exit says nothing; no heartbeat still says it ended unexpectedly.
        assert!(previous_session(false, Some(&hb), off).is_none());
        assert!(previous_session(true, None, off).unwrap().1.ends_with("unexpectedly"));
        assert!(read_heartbeat(&d.join("missing.json")).is_none());
        let _ = fs::remove_dir_all(&d);
    }

    #[test]
    fn a_helper_that_keeps_dying_is_restarted_with_backoff_then_given_up_on() {
        let mut h = Vec::new();
        assert_eq!(restart_delay(&mut h, 1000), Some((1, 2)));
        assert_eq!(restart_delay(&mut h, 1010), Some((2, 10)));
        assert_eq!(restart_delay(&mut h, 1050), Some((3, 30)));
        assert_eq!(restart_delay(&mut h, 1100), None, "a fourth death inside five minutes: the notice");
        // Once the earlier tries are over five minutes old it may be restarted again.
        assert_eq!(restart_delay(&mut h, 1000 + RESTART_WINDOW_SECS + 60), Some((1, 2)));
    }

    #[test]
    fn the_shell_log_tail_keeps_the_last_lines_and_earlier_records() {
        let mut text = String::from("1 exit requested: tray quit\n");
        for i in 0..40 {
            text.push_str(&format!("{} plain line {i}\n", i + 2));
        }
        let out = shell_log_tail(&text);
        assert!(out.contains("exit requested") && out.contains("plain line 39") && !out.contains("plain line 5\n"));
        assert_eq!(out.lines().filter(|l| l.contains("plain line")).count(), 20);
        assert_eq!(shell_log_tail(""), "");
    }

    #[test]
    fn logs_rotate_at_the_cap_and_keep_three() {
        let d = tmp("rot");
        for i in 0..40 {
            append_rotating(&d, "x.log", &format!("line {i:02}"), 30, 3);
        }
        let mut names: Vec<String> = fs::read_dir(&d).unwrap().flatten().map(|e| e.file_name().to_string_lossy().to_string()).collect();
        names.sort();
        assert_eq!(names, ["x.1.log", "x.2.log", "x.3.log", "x.log"]);
        assert!(fs::metadata(d.join("x.log")).unwrap().len() <= 30);
        // The newest older file holds later lines than the oldest.
        let a = fs::read_to_string(d.join("x.1.log")).unwrap();
        let b = fs::read_to_string(d.join("x.3.log")).unwrap();
        assert!(a > b);
        let _ = fs::remove_dir_all(&d);
    }

    #[test]
    fn helper_output_is_logged_with_secrets_replaced() {
        let d = tmp("helper");
        helper_line(&d, "err", "digest written");
        helper_line(&d, "out", "{\"url\":\"x\",\"token\":\"abc\"}");
        helper_line(&d, "err", "   ");
        let text = fs::read_to_string(d.join(HELPER_LOG)).unwrap();
        assert!(text.contains("err digest written") && !text.contains("abc"));
        assert_eq!(text.lines().count(), 2);
        let _ = fs::remove_dir_all(&d);
    }
}
