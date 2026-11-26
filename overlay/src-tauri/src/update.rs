// In-app updates: download the NSIS installer and SHA256SUMS of one release over HTTPS, check the
// SHA-256, keep the last two installers and the last three settings backups, and run the installer.
// It lives in the shell, not the page or the Go sidecar, so the tray can reach it too. Nothing here
// runs the installer in a test: the functions that touch the disk or the network are thin wrappers
// around the pure ones that are tested.
use std::fs;
use std::io::Read;
use std::path::{Path, PathBuf};

use tauri::{AppHandle, Emitter, Manager};

/// The release downloads of this app; the page can name a version, never a URL.
const DOWNLOAD_BASE: &str = "https://github.com/WoW-Companion/advisor/releases/download";
/// How many installers stay in the cache: the one just installed and the one it replaced.
pub const KEEP_INSTALLERS: usize = 2;
/// How many dated settings backups stay.
pub const KEEP_BACKUPS: usize = 3;

// --- SHA-256 (FIPS 180-4), so the shell needs no new crate ---------------------------------------

const K: [u32; 64] = [
    0x428a2f98, 0x71374491, 0xb5c0fbcf, 0xe9b5dba5, 0x3956c25b, 0x59f111f1, 0x923f82a4, 0xab1c5ed5, 0xd807aa98, 0x12835b01,
    0x243185be, 0x550c7dc3, 0x72be5d74, 0x80deb1fe, 0x9bdc06a7, 0xc19bf174, 0xe49b69c1, 0xefbe4786, 0x0fc19dc6, 0x240ca1cc,
    0x2de92c6f, 0x4a7484aa, 0x5cb0a9dc, 0x76f988da, 0x983e5152, 0xa831c66d, 0xb00327c8, 0xbf597fc7, 0xc6e00bf3, 0xd5a79147,
    0x06ca6351, 0x14292967, 0x27b70a85, 0x2e1b2138, 0x4d2c6dfc, 0x53380d13, 0x650a7354, 0x766a0abb, 0x81c2c92e, 0x92722c85,
    0xa2bfe8a1, 0xa81a664b, 0xc24b8b70, 0xc76c51a3, 0xd192e819, 0xd6990624, 0xf40e3585, 0x106aa070, 0x19a4c116, 0x1e376c08,
    0x2748774c, 0x34b0bcb5, 0x391c0cb3, 0x4ed8aa4a, 0x5b9cca4f, 0x682e6ff3, 0x748f82ee, 0x78a5636f, 0x84c87814, 0x8cc70208,
    0x90befffa, 0xa4506ceb, 0xbef9a3f7, 0xc67178f2,
];

pub struct Sha256 {
    h: [u32; 8],
    buf: Vec<u8>,
    len: u64,
}

impl Sha256 {
    pub fn new() -> Self {
        Sha256 {
            h: [0x6a09e667, 0xbb67ae85, 0x3c6ef372, 0xa54ff53a, 0x510e527f, 0x9b05688c, 0x1f83d9ab, 0x5be0cd19],
            buf: Vec::with_capacity(64),
            len: 0,
        }
    }

    fn block(&mut self, b: &[u8]) {
        let mut w = [0u32; 64];
        for i in 0..16 {
            w[i] = u32::from_be_bytes([b[4 * i], b[4 * i + 1], b[4 * i + 2], b[4 * i + 3]]);
        }
        for i in 16..64 {
            let s0 = w[i - 15].rotate_right(7) ^ w[i - 15].rotate_right(18) ^ (w[i - 15] >> 3);
            let s1 = w[i - 2].rotate_right(17) ^ w[i - 2].rotate_right(19) ^ (w[i - 2] >> 10);
            w[i] = w[i - 16].wrapping_add(s0).wrapping_add(w[i - 7]).wrapping_add(s1);
        }
        let mut v = self.h;
        for i in 0..64 {
            let s1 = v[4].rotate_right(6) ^ v[4].rotate_right(11) ^ v[4].rotate_right(25);
            let ch = (v[4] & v[5]) ^ (!v[4] & v[6]);
            let t1 = v[7].wrapping_add(s1).wrapping_add(ch).wrapping_add(K[i]).wrapping_add(w[i]);
            let s0 = v[0].rotate_right(2) ^ v[0].rotate_right(13) ^ v[0].rotate_right(22);
            let maj = (v[0] & v[1]) ^ (v[0] & v[2]) ^ (v[1] & v[2]);
            let t2 = s0.wrapping_add(maj);
            v = [t1.wrapping_add(t2), v[0], v[1], v[2], v[3].wrapping_add(t1), v[4], v[5], v[6]];
        }
        for i in 0..8 {
            self.h[i] = self.h[i].wrapping_add(v[i]);
        }
    }

    pub fn update(&mut self, mut data: &[u8]) {
        self.len += data.len() as u64;
        if !self.buf.is_empty() {
            let need = 64 - self.buf.len();
            let take = need.min(data.len());
            self.buf.extend_from_slice(&data[..take]);
            data = &data[take..];
            if self.buf.len() == 64 {
                let b = std::mem::take(&mut self.buf);
                self.block(&b);
            }
        }
        while data.len() >= 64 {
            self.block(&data[..64]);
            data = &data[64..];
        }
        self.buf.extend_from_slice(data);
    }

    pub fn finish(mut self) -> String {
        let bits = self.len * 8;
        let mut pad = vec![0x80u8];
        while (self.buf.len() + pad.len()) % 64 != 56 {
            pad.push(0);
        }
        pad.extend_from_slice(&bits.to_be_bytes());
        let total = self.len;
        self.update(&pad);
        self.len = total;
        self.h.iter().map(|x| format!("{x:08x}")).collect()
    }
}

pub fn sha256_hex(data: &[u8]) -> String {
    let mut s = Sha256::new();
    s.update(data);
    s.finish()
}

/// The SHA-256 of a file, read in chunks.
pub fn sha256_file(path: &Path) -> std::io::Result<String> {
    let mut f = fs::File::open(path)?;
    let mut s = Sha256::new();
    let mut buf = vec![0u8; 1 << 16];
    loop {
        let n = f.read(&mut buf)?;
        if n == 0 {
            return Ok(s.finish());
        }
        s.update(&buf[..n]);
    }
}

// --- names and the checksum file ------------------------------------------------------------------

/// Whether `v` is a plain release number such as "0.7.53": the only text that reaches a URL or a path.
pub fn valid_version(v: &str) -> bool {
    let parts: Vec<&str> = v.split('.').collect();
    parts.len() == 3 && parts.iter().all(|p| !p.is_empty() && p.len() <= 6 && p.bytes().all(|b| b.is_ascii_digit()))
}

/// A release number as numbers to compare, or None when it is not a plain release number.
pub fn version_key(v: &str) -> Option<(u32, u32, u32)> {
    if !valid_version(v) {
        return None;
    }
    let mut it = v.split('.').map(|p| p.parse::<u32>().unwrap_or(0));
    Some((it.next()?, it.next()?, it.next()?))
}

/// The version in an installer's file name ("WoW-Companion-Setup-0.7.54-x64.exe" gives "0.7.54").
pub fn installer_version(name: &str) -> Option<String> {
    let v = name.strip_prefix("WoW-Companion-Setup-")?.strip_suffix("-x64.exe")?;
    if valid_version(v) {
        Some(v.to_string())
    } else {
        None
    }
}

/// The newest cached installer older than `running`: the version "Go back" returns to.
pub fn previous_installer(names: &[String], running: &str) -> Option<String> {
    let run = version_key(running)?;
    names
        .iter()
        .filter_map(|n| installer_version(n))
        .filter(|v| version_key(v).map_or(false, |k| k < run))
        .max_by_key(|v| version_key(v))
}

pub fn installer_name(v: &str) -> String {
    format!("WoW-Companion-Setup-{v}-x64.exe")
}

/// The expected hash of `name` in a `sha256sum` file ("<hex>  <name>" lines, "*" before the name for
/// binary mode). A hash that is not 64 hex digits is no hash.
pub fn sums_entry(text: &str, name: &str) -> Option<String> {
    for line in text.lines() {
        let Some((hash, file)) = line.trim().split_once(char::is_whitespace) else { continue };
        if file.trim().trim_start_matches('*') == name && hash.len() == 64 && hash.bytes().all(|b| b.is_ascii_hexdigit()) {
            return Some(hash.to_ascii_lowercase());
        }
    }
    None
}

/// Checks a downloaded file against its expected hash. On a mismatch (or an unreadable file) the file
/// is deleted, so a bad installer never stays where it could be run.
pub fn verify_or_delete(path: &Path, expected: &str) -> Result<(), String> {
    let ok = matches!(sha256_file(path), Ok(h) if h.eq_ignore_ascii_case(expected));
    if !ok {
        let _ = fs::remove_file(path);
        return Err("The download did not match its checksum, so it was deleted and nothing was installed. Try again in a little while.".into());
    }
    Ok(())
}

// --- the cache and the backups --------------------------------------------------------------------

/// Which cached installers to delete. `files` is (file name, modified time). The versions in `keep`
/// (the one being installed and the one running) stay first; the rest of the `limit` goes to the most
/// recently modified others. Only installer file names are ever returned, so nothing else in the
/// folder is touched.
pub fn stale_installers(files: &[(String, u64)], keep: &[&str], limit: usize) -> Vec<String> {
    let mut ours: Vec<&(String, u64)> = files
        .iter()
        .filter(|(n, _)| n.starts_with("WoW-Companion-Setup-") && n.ends_with("-x64.exe"))
        .collect();
    ours.sort_by(|a, b| {
        let ka = keep.iter().any(|v| installer_name(v) == a.0);
        let kb = keep.iter().any(|v| installer_name(v) == b.0);
        kb.cmp(&ka).then(b.1.cmp(&a.1)).then(a.0.cmp(&b.0))
    });
    ours.into_iter().skip(limit).map(|(n, _)| n.clone()).collect()
}

/// Which dated backup folders to delete so that `limit` remain; names sort by their date.
pub fn stale_backups(names: &[String], limit: usize) -> Vec<String> {
    let mut b: Vec<&String> = names.iter().filter(|n| n.starts_with("backup-")).collect();
    b.sort();
    let drop = b.len().saturating_sub(limit);
    b.into_iter().take(drop).cloned().collect()
}

/// "backup-20261125-153005" from seconds since the Unix epoch (UTC).
pub fn backup_name(secs: u64) -> String {
    let (days, rem) = (secs / 86400, secs % 86400);
    // Civil date from a day count (Howard Hinnant's algorithm).
    let z = days as i64 + 719468;
    let era = z.div_euclid(146097);
    let doe = z.rem_euclid(146097);
    let yoe = (doe - doe / 1460 + doe / 36524 - doe / 146096) / 365;
    let doy = doe - (365 * yoe + yoe / 4 - yoe / 100);
    let mp = (5 * doy + 2) / 153;
    let d = doy - (153 * mp + 2) / 5 + 1;
    let m = if mp < 10 { mp + 3 } else { mp - 9 };
    let y = yoe + era * 400 + i64::from(m <= 2);
    format!("backup-{y:04}{m:02}{d:02}-{:02}{:02}{:02}", rem / 3600, rem % 3600 / 60, rem % 60)
}

fn now_secs() -> u64 {
    std::time::SystemTime::now().duration_since(std::time::UNIX_EPOCH).map(|d| d.as_secs()).unwrap_or(0)
}

fn mtime(p: &Path) -> u64 {
    fs::metadata(p).and_then(|m| m.modified()).ok().and_then(|t| t.duration_since(std::time::UNIX_EPOCH).ok()).map(|d| d.as_secs()).unwrap_or(0)
}

fn prune_cache(dir: &Path, keep: &[&str]) {
    let Ok(rd) = fs::read_dir(dir) else { return };
    let files: Vec<(String, u64)> = rd.flatten().map(|e| (e.file_name().to_string_lossy().into_owned(), mtime(&e.path()))).collect();
    for n in stale_installers(&files, keep, KEEP_INSTALLERS) {
        let _ = fs::remove_file(hash_record(&dir.join(&n)));
        let _ = fs::remove_file(dir.join(n));
    }
}

/// The file in a backup folder that says which app version made it (the one the settings belong to).
pub const BACKUP_VERSION_FILE: &str = "version.txt";
/// The overlay's own settings, as the page sent them.
pub const BACKUP_SETTINGS_FILE: &str = "overlay-settings.json";

/// Copies the overlay's settings (sent by the page; empty when the page is not there, as from the
/// tray) and the profile files into a new dated folder under `root`, marked with the app `version` that
/// is running, then trims the older folders to the last `KEEP_BACKUPS`.
pub fn make_backup(root: &Path, version: &str, settings_json: &str, files: &[(&str, PathBuf)], secs: u64) -> Result<PathBuf, String> {
    let dir = root.join(backup_name(secs));
    fs::create_dir_all(&dir).map_err(|e| format!("could not make the backup folder: {e}"))?;
    fs::write(dir.join(BACKUP_VERSION_FILE), version).map_err(|e| format!("could not mark the backup: {e}"))?;
    if !settings_json.trim().is_empty() {
        fs::write(dir.join(BACKUP_SETTINGS_FILE), settings_json).map_err(|e| format!("could not save the settings backup: {e}"))?;
    }
    for (name, src) in files {
        if src.is_file() {
            fs::copy(src, dir.join(name)).map_err(|e| format!("could not back up {name}: {e}"))?;
        }
    }
    if let Ok(rd) = fs::read_dir(root) {
        let names: Vec<String> = rd.flatten().map(|e| e.file_name().to_string_lossy().into_owned()).collect();
        for n in stale_backups(&names, KEEP_BACKUPS) {
            let _ = fs::remove_dir_all(root.join(n));
        }
    }
    Ok(dir)
}

/// The backup to restore when going back to `target`: the newest one made while `target` was the
/// running version. `backups` is (folder name, version it was made under); a folder with no version
/// mark is never chosen, since nothing says whose settings it holds.
pub fn pick_backup(backups: &[(String, Option<String>)], target: &str) -> Option<String> {
    backups
        .iter()
        .filter(|(n, v)| n.starts_with("backup-") && v.as_deref().map(str::trim) == Some(target))
        .map(|(n, _)| n.clone())
        .max()
}

/// A backup read into memory: what to put back where, and the page's settings. Reading it first lets
/// the current state be backed up (which trims old folders) without losing the one being restored.
pub struct Restored {
    /// The page's settings from the backup, when it had them.
    pub settings_json: Option<String>,
    pub files: Vec<(String, PathBuf, Vec<u8>)>,
}

/// Reads the files of the backup folder `dir` that belong at `targets` (name, where it belongs) and
/// the page's settings. A file the backup does not hold is simply not part of it.
pub fn load_backup(dir: &Path, targets: &[(&str, PathBuf)]) -> Result<Restored, String> {
    let mut files = Vec::new();
    for (name, dest) in targets {
        let src = dir.join(name);
        if src.is_file() {
            let bytes = fs::read(&src).map_err(|e| format!("could not read the backup of {name}: {e}"))?;
            files.push((name.to_string(), dest.clone(), bytes));
        }
    }
    let settings_json = fs::read_to_string(dir.join(BACKUP_SETTINGS_FILE)).ok().filter(|t| !t.trim().is_empty());
    Ok(Restored { settings_json, files })
}

/// Writes the files of a read backup back where they belong.
pub fn apply_backup(r: &Restored) -> Result<(), String> {
    for (name, dest, bytes) in &r.files {
        if let Some(d) = dest.parent() {
            fs::create_dir_all(d).map_err(|e| format!("could not restore {name}: {e}"))?;
        }
        fs::write(dest, bytes).map_err(|e| format!("could not restore {name}: {e}"))?;
    }
    Ok(())
}

/// `load_backup` then `apply_backup`.
pub fn restore_files(dir: &Path, targets: &[(&str, PathBuf)]) -> Result<Restored, String> {
    let r = load_backup(dir, targets)?;
    apply_backup(&r)?;
    Ok(r)
}

/// The newest backup under `root` made under `target`, if any.
pub fn find_backup(root: &Path, target: &str) -> Option<PathBuf> {
    let rd = fs::read_dir(root).ok()?;
    let list: Vec<(String, Option<String>)> = rd
        .flatten()
        .map(|e| {
            let name = e.file_name().to_string_lossy().into_owned();
            let v = fs::read_to_string(e.path().join(BACKUP_VERSION_FILE)).ok();
            (name, v)
        })
        .collect();
    pick_backup(&list, target).map(|n| root.join(n))
}

// --- download and install -------------------------------------------------------------------------

#[cfg(windows)]
fn curl(url: &str, dest: &Path) -> Result<(), String> {
    use std::os::windows::process::CommandExt;
    // curl.exe ships with Windows 10 and 11; it speaks HTTPS through the system's own TLS.
    let out = std::process::Command::new("curl.exe")
        .args(["--fail", "--silent", "--show-error", "--location", "--proto", "=https", "--proto-redir", "=https", "--max-time", "600", "--output"])
        .arg(dest)
        .arg(url)
        .creation_flags(0x0800_0000) // CREATE_NO_WINDOW
        .output()
        .map_err(|e| format!("could not start the download: {e}"))?;
    if out.status.success() {
        Ok(())
    } else {
        let _ = fs::remove_file(dest);
        Err(format!("The download failed: {}", String::from_utf8_lossy(&out.stderr).trim()))
    }
}

#[cfg(not(windows))]
fn curl(_: &str, _: &Path) -> Result<(), String> {
    Err("Updating in the app works on Windows only.".into())
}

/// The last progress line, for a window that has no event listener (the recovery window polls it).
static PROGRESS: std::sync::Mutex<String> = std::sync::Mutex::new(String::new());

pub fn progress() -> String {
    PROGRESS.lock().map(|p| p.clone()).unwrap_or_default()
}

fn say(app: &AppHandle, text: &str) {
    if let Ok(mut p) = PROGRESS.lock() {
        *p = text.to_string();
    }
    let _ = app.emit("update-progress", text);
}

/// Makes sure the verified installer of `version` is in `cache`, downloading and checking it when it
/// is not. `sums` is that release's SHA256SUMS text.
fn ensure_installer(cache: &Path, version: &str, sums: &str) -> Result<PathBuf, String> {
    let name = installer_name(version);
    let want = sums_entry(sums, &name).ok_or_else(|| format!("Version {version} has no checksum to verify, so it was not installed."))?;
    let path = cache.join(&name);
    if path.is_file() && verify_or_delete(&path, &want).is_ok() {
        let _ = fs::write(hash_record(&path), &want);
        return Ok(path);
    }
    let part = cache.join(format!("{name}.part"));
    curl(&format!("{DOWNLOAD_BASE}/v{version}/{name}"), &part)?;
    verify_or_delete(&part, &want)?;
    fs::rename(&part, &path).map_err(|e| format!("could not keep the installer: {e}"))?;
    let _ = fs::write(hash_record(&path), &want);
    Ok(path)
}

/// Where the checksum an installer was verified against is kept, so a cached installer can be checked
/// again offline (going back must work with no network).
fn hash_record(installer: &Path) -> PathBuf {
    let mut n = installer.as_os_str().to_os_string();
    n.push(".sha256");
    PathBuf::from(n)
}

/// A cached installer of `version` whose bytes still match the checksum it was downloaded against.
/// A mismatch deletes both files; no record means it is not trusted offline.
pub fn cached_verified(cache: &Path, version: &str) -> Option<PathBuf> {
    if !valid_version(version) {
        return None;
    }
    let path = cache.join(installer_name(version));
    let want = fs::read_to_string(hash_record(&path)).ok()?;
    let want = want.trim();
    if want.len() != 64 || !path.is_file() {
        return None;
    }
    if verify_or_delete(&path, want).is_err() {
        let _ = fs::remove_file(hash_record(&path));
        return None;
    }
    Some(path)
}

fn cache_dir(app: &AppHandle) -> Result<PathBuf, String> {
    let cache = app.path().app_cache_dir().map_err(|e| e.to_string())?.join("updates");
    fs::create_dir_all(&cache).map_err(|e| format!("could not make the update folder: {e}"))?;
    Ok(cache)
}

/// The version "Go back" returns to: the newest older version with a cached installer.
pub fn previous_version(app: &AppHandle) -> Option<String> {
    let cache = app.path().app_cache_dir().ok()?.join("updates");
    let names: Vec<String> = fs::read_dir(cache).ok()?.flatten().map(|e| e.file_name().to_string_lossy().into_owned()).collect();
    previous_installer(&names, env!("CARGO_PKG_VERSION"))
}

fn config_files(app: &AppHandle) -> Vec<(&'static str, PathBuf)> {
    let mut files: Vec<(&'static str, PathBuf)> = Vec::new();
    if let Ok(d) = app.path().app_config_dir() {
        files.push(("agent.txt", d.join("agent.txt")));
    }
    if let Ok(d) = app.path().config_dir() {
        files.push(("profiles.json", d.join("wowc-advisor").join("profiles.json")));
        files.push(("providers.json", d.join("wowc-advisor").join("providers.json")));
    }
    files
}

fn backups_root(app: &AppHandle) -> Result<PathBuf, String> {
    Ok(app.path().app_data_dir().map_err(|e| e.to_string())?.join("backups"))
}

/// Going back to `target`: reads the newest backup made under it, saves the current settings as a
/// backup (marked with the running version) and then puts the old files back. The page's settings from
/// the backup are returned for the caller to hand to the page. `Ok(None)` when there is no such
/// backup: nothing is changed and the current settings stay.
pub fn restore_backup(app: &AppHandle, target: &str) -> Result<Option<Restored>, String> {
    if !valid_version(target) {
        return Err("That is not a version number.".into());
    }
    let root = backups_root(app)?;
    let Some(dir) = find_backup(&root, target) else { return Ok(None) };
    let files = config_files(app);
    let old = load_backup(&dir, &files)?;
    // Keep what is there now, so going forward again finds it. A failure here must not stop going back.
    let _ = backup_now(app, "");
    apply_backup(&old)?;
    Ok(Some(old))
}

/// Everything before going back runs: the verified cached installer of `target` (or, with a network,
/// a freshly downloaded and verified one). The settings are not touched here.
pub fn prepare_back(app: &AppHandle, target: &str) -> Result<PathBuf, String> {
    if !valid_version(target) {
        return Err("That is not a version number.".into());
    }
    let cache = cache_dir(app)?;
    if let Some(p) = cached_verified(&cache, target) {
        say(app, &format!("Going back to {target}…"));
        return Ok(p);
    }
    say(app, &format!("Downloading {target}…"));
    ensure_installer(&cache, target, &sums_for(&cache, target)?)
}

/// Saves the shell's files (and the page's settings when given) in a new backup marked with the
/// running version, so a later go-back to this version finds them.
pub fn backup_now(app: &AppHandle, settings_json: &str) -> Result<(), String> {
    make_backup(&backups_root(app)?, env!("CARGO_PKG_VERSION"), settings_json, &config_files(app), now_secs()).map(|_| ())
}

fn sums_for(cache: &Path, version: &str) -> Result<String, String> {
    let p = cache.join(format!("SHA256SUMS-{version}.part"));
    curl(&format!("{DOWNLOAD_BASE}/v{version}/SHA256SUMS"), &p)?;
    let text = fs::read_to_string(&p).map_err(|e| e.to_string());
    let _ = fs::remove_file(&p);
    text
}

/// Everything before the installer runs: verified installers of `version` and (best effort) the running
/// version, the settings backup, and the cache trimmed. Returns the installer to run.
pub fn prepare(app: &AppHandle, version: &str, settings_json: &str) -> Result<PathBuf, String> {
    if !valid_version(version) {
        return Err("That is not a version number.".into());
    }
    let running = env!("CARGO_PKG_VERSION");
    let cache = cache_dir(app)?;
    say(app, &format!("Downloading {version}…"));
    let installer = ensure_installer(&cache, version, &sums_for(&cache, version)?)?;
    if version != running && valid_version(running) {
        // So "go back" works offline later. A failure here must not stop the install.
        say(app, "Keeping a copy of this version for going back…");
        if let Ok(sums) = sums_for(&cache, running) {
            let _ = ensure_installer(&cache, running, &sums);
        }
    }
    prune_cache(&cache, &[version, running]);
    say(app, "Saving your settings…");
    backup_now(app, settings_json)?;
    Ok(installer)
}

/// Runs the installer in passive mode (progress bar only, relaunching the app when done) and exits
/// this app so the installer can replace it.
#[cfg(windows)]
pub fn run_and_exit(app: &AppHandle, installer: &Path) -> Result<(), String> {
    say(app, "Installing…");
    std::process::Command::new(installer).args(["/P", "/R"]).spawn().map_err(|e| format!("could not start the installer: {e}"))?;
    std::thread::sleep(std::time::Duration::from_millis(800));
    crate::life::set_reason("installer started (update or go back): the app restarts");
    app.exit(0);
    Ok(())
}

#[cfg(not(windows))]
pub fn run_and_exit(_: &AppHandle, _: &Path) -> Result<(), String> {
    Err("Updating in the app works on Windows only.".into())
}

// --- the shell's own update check (the tray's "Check for updates") --------------------------------

const LATEST_URL: &str = "https://api.github.com/repos/WoW-Companion/advisor/releases/latest";
const PULLS_URL: &str = "https://raw.githubusercontent.com/WoW-Companion/advisor/main/updates/updates.json";
/// A release is offered once it has been public this long, as in the page.
pub const SETTLE_SECS: u64 = 24 * 60 * 60;

/// What the shell's check found.
#[derive(Clone, Debug, PartialEq)]
pub enum Check {
    /// Nothing newer that may be offered (or the newest one has been pulled).
    UpToDate,
    /// A newer release exists but is not a day old yet; it will be offered later.
    Settling(String),
    /// A newer release that may be installed.
    Available(String),
}

/// Seconds since the Unix epoch of "2026-11-25T17:06:27Z" (UTC, as GitHub writes it).
pub fn parse_utc(ts: &str) -> Option<u64> {
    let b = ts.as_bytes();
    if b.len() < 19 || b[4] != b'-' || b[7] != b'-' || (b[10] != b'T' && b[10] != b' ') || b[13] != b':' || b[16] != b':' {
        return None;
    }
    let num = |a: usize, z: usize| ts.get(a..z)?.parse::<i64>().ok();
    let (y, m, d, hh, mm, ss) = (num(0, 4)?, num(5, 7)?, num(8, 10)?, num(11, 13)?, num(14, 16)?, num(17, 19)?);
    if !(1..=12).contains(&m) || !(1..=31).contains(&d) || hh > 23 || mm > 59 || ss > 60 || y < 1970 {
        return None;
    }
    // Days from the civil date (Howard Hinnant's algorithm).
    let y2 = if m <= 2 { y - 1 } else { y };
    let era = y2.div_euclid(400);
    let yoe = y2 - era * 400;
    let mp = (m + 9) % 12;
    let doy = (153 * mp + 2) / 5 + d - 1;
    let doe = yoe * 365 + yoe / 4 - yoe / 100 + doy;
    let days = era * 146097 + doe - 719468;
    Some((days * 86400 + hh * 3600 + mm * 60 + ss) as u64)
}

/// The versions listed as pulled in `updates.json`; unreadable text pulls nothing.
pub fn pulled_versions(text: &str) -> Vec<String> {
    let Ok(v) = serde_json::from_str::<serde_json::Value>(text) else { return Vec::new() };
    v.get("pulled")
        .and_then(|p| p.as_array())
        .map(|a| {
            a.iter()
                .filter_map(|e| e.get("version").and_then(|x| x.as_str()))
                .map(|x| x.trim().trim_start_matches('v').to_string())
                .collect()
        })
        .unwrap_or_default()
}

/// The version and publication time in a `releases/latest` answer.
pub fn latest_release(text: &str) -> Option<(String, u64)> {
    let v: serde_json::Value = serde_json::from_str(text).ok()?;
    let tag = v.get("tag_name")?.as_str()?.trim().trim_start_matches('v').to_string();
    let at = v.get("published_at").and_then(|x| x.as_str()).and_then(parse_utc).unwrap_or(0);
    valid_version(&tag).then_some((tag, at))
}

/// Whether a release found by the shell may be offered: newer than `running`, not pulled, and (when
/// its time is known) public for a day.
pub fn judge(latest: &str, published: u64, running: &str, pulled: &[String], now: u64) -> Check {
    match (version_key(latest), version_key(running)) {
        (Some(l), Some(r)) if l > r && !pulled.iter().any(|p| p == latest) => {
            if published > 0 && now < published.saturating_add(SETTLE_SECS) {
                Check::Settling(latest.to_string())
            } else {
                Check::Available(latest.to_string())
            }
        }
        _ => Check::UpToDate,
    }
}

fn fetch_text(cache: &Path, url: &str, name: &str) -> Result<String, String> {
    let p = cache.join(name);
    curl(url, &p)?;
    let text = fs::read_to_string(&p).map_err(|e| e.to_string());
    let _ = fs::remove_file(&p);
    text
}

/// A pulled running version: where to go back to, and why it was pulled.
#[derive(Clone, Debug, PartialEq)]
pub struct GoBack {
    pub back_to: String,
    pub reason: String,
}

/// The pull of the running version in `updates.json`, when there is one.
pub fn running_pull(text: &str, running: &str) -> Option<GoBack> {
    let v: serde_json::Value = serde_json::from_str(text).ok()?;
    let bare = |s: &str| s.trim().trim_start_matches('v').to_string();
    let field = |e: &serde_json::Value, k: &str| e.get(k).and_then(|x| x.as_str()).map(bare).unwrap_or_default();
    v.get("pulled")?
        .as_array()?
        .iter()
        .find(|e| field(e, "version") == running)
        .map(|e| GoBack { back_to: field(e, "back_to"), reason: e.get("reason").and_then(|x| x.as_str()).unwrap_or("").trim().to_string() })
}

/// Looks up the newest release and the pull file over HTTPS, without the page or the sidecar.
pub fn check_latest(app: &AppHandle) -> Result<Check, String> {
    check_full(app).map(|(c, _)| c)
}

/// `check_latest` plus the pull of the running version, which the automatic updater acts on. A failed
/// release lookup still answers when the running version is pulled.
pub fn check_full(app: &AppHandle) -> Result<(Check, Option<GoBack>), String> {
    let cache = cache_dir(app)?;
    let running = env!("CARGO_PKG_VERSION");
    // A pull file that cannot be read pulls nothing: it must never block an update.
    let pulls = fetch_text(&cache, PULLS_URL, "updates.json.part").unwrap_or_default();
    let back = running_pull(&pulls, running);
    let latest = match fetch_text(&cache, LATEST_URL, "latest.json.part") {
        Ok(t) => t,
        Err(_) if back.is_some() => return Ok((Check::UpToDate, back)),
        Err(e) => return Err(e),
    };
    let Some((tag, at)) = latest_release(&latest) else {
        return if back.is_some() { Ok((Check::UpToDate, back)) } else { Err("The release information could not be read.".into()) };
    };
    Ok((judge(&tag, at, running, &pulled_versions(&pulls), now_secs()), back))
}

/// Opens the shell's log folder in the file manager.
pub fn open_folder(dir: &Path) -> Result<(), String> {
    fs::create_dir_all(dir).map_err(|e| e.to_string())?;
    open_dir(dir)
}

#[cfg(windows)]
fn open_dir(dir: &Path) -> Result<(), String> {
    use std::os::windows::process::CommandExt;
    std::process::Command::new("explorer.exe")
        .arg(dir)
        .creation_flags(0x0800_0000)
        .spawn()
        .map(|_| ())
        .map_err(|e| format!("could not open the folder: {e}"))
}

#[cfg(not(windows))]
fn open_dir(_: &Path) -> Result<(), String> {
    Err("Opening the folder works on Windows only.".into())
}

/// Appends a line to the shell's log (local-time stamped, about 1 MB a file, three older files kept).
pub fn log_line(dir: &Path, text: &str) {
    crate::life::log_line(dir, text);
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn sha256_matches_the_published_vectors() {
        assert_eq!(sha256_hex(b""), "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855");
        assert_eq!(sha256_hex(b"abc"), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad");
        assert_eq!(
            sha256_hex(b"abcdbcdecdefdefgefghfghighijhijkijkljklmklmnlmnomnopnopq"),
            "248d6a61d20638b8e5c026930c3e6039a33ce45964ff2167f6ecedd419db06c1"
        );
        // Split across the 64-byte block edge, in uneven pieces.
        let data = vec![b'a'; 1000];
        let mut s = Sha256::new();
        for c in data.chunks(37) {
            s.update(c);
        }
        assert_eq!(s.finish(), sha256_hex(&data));
        assert_eq!(sha256_hex(&vec![b'a'; 1_000_000]), "cdc76e5c9914fb9281a1c7e284d73e67f1809a48a497200e046d39ccc7112cd0");
    }

    #[test]
    fn checksum_file_lines() {
        let h = "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad";
        let txt = format!("{} *other.zip\n{h}  WoW-Companion-Setup-0.7.54-x64.exe\r\n", "0".repeat(64));
        assert_eq!(sums_entry(&txt, "WoW-Companion-Setup-0.7.54-x64.exe").as_deref(), Some(h));
        assert_eq!(sums_entry(&txt, "other.zip").map(|s| s.len()), Some(64));
        assert_eq!(sums_entry(&txt, "WoW-Companion-Setup-0.7.55-x64.exe"), None);
        assert_eq!(sums_entry("nothex  WoW-Companion-Setup-0.7.54-x64.exe", "WoW-Companion-Setup-0.7.54-x64.exe"), None);
    }

    #[test]
    fn only_plain_versions_reach_urls_and_paths() {
        assert!(valid_version("0.7.54"));
        for bad in ["", "0.7", "0.7.54.1", "v0.7.54", "0.7.54/../x", "0.7.a", "..", "0.7.-1", "1.2.3 "] {
            assert!(!valid_version(bad), "{bad}");
        }
    }

    #[test]
    fn a_mismatch_deletes_the_file_and_a_match_keeps_it() {
        let dir = std::env::temp_dir().join(format!("wowc-update-test-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let f = dir.join("setup.exe");
        fs::write(&f, b"abc").unwrap();
        assert!(verify_or_delete(&f, "BA7816BF8F01CFEA414140DE5DAE2223B00361A396177A9CB410FF61F20015AD").is_ok());
        assert!(f.exists());
        assert!(verify_or_delete(&f, &"0".repeat(64)).is_err());
        assert!(!f.exists());
        assert!(verify_or_delete(&f, &"0".repeat(64)).is_err(), "a missing file is a failure too");
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn the_cache_keeps_the_target_and_the_running_version_and_at_most_two() {
        let f = |v: &str, t: u64| (installer_name(v), t);
        let files = vec![f("0.7.50", 10), f("0.7.51", 20), f("0.7.52", 30), f("0.7.53", 40), ("notes.txt".to_string(), 1)];
        // Going back from 0.7.53 to 0.7.50: both stay although 0.7.50 is the oldest.
        let mut gone = stale_installers(&files, &["0.7.50", "0.7.53"], 2);
        gone.sort();
        assert_eq!(gone, vec![installer_name("0.7.51"), installer_name("0.7.52")]);
        // A fresh install with no cached running version keeps the target and the newest other.
        assert_eq!(stale_installers(&files, &["0.7.53", "0.7.99"], 2).len(), 2);
        assert!(stale_installers(&files[..2], &["0.7.51", "0.7.50"], 2).is_empty());
        assert!(!stale_installers(&files, &["0.7.53"], 2).contains(&"notes.txt".to_string()));
    }

    #[test]
    fn backups_are_dated_and_trimmed_to_three() {
        assert_eq!(backup_name(0), "backup-19700101-000000");
        assert_eq!(backup_name(1_795_620_605), "backup-20261125-153005");
        let names: Vec<String> = ["backup-20261101-000000", "backup-20261125-153005", "notes", "backup-20261110-120000", "backup-20261120-010101"].iter().map(|s| s.to_string()).collect();
        assert_eq!(stale_backups(&names, 3), vec!["backup-20261101-000000".to_string()]);
        assert!(stale_backups(&names[..2], 3).is_empty());
    }

    #[test]
    fn a_backup_holds_the_settings_and_the_files_and_trims_old_folders() {
        let root = std::env::temp_dir().join(format!("wowc-backup-test-{}", std::process::id()));
        let src = root.join("src");
        fs::create_dir_all(&src).unwrap();
        fs::write(src.join("profiles.json"), "{\"p\":1}").unwrap();
        let backups = root.join("backups");
        for secs in [1_000_000u64, 2_000_000, 3_000_000, 4_000_000] {
            make_backup(&backups, "0.7.54", "{\"opacity\":50}", &[("profiles.json", src.join("profiles.json")), ("missing.json", src.join("missing.json"))], secs).unwrap();
        }
        let mut names: Vec<String> = fs::read_dir(&backups).unwrap().flatten().map(|e| e.file_name().to_string_lossy().into_owned()).collect();
        names.sort();
        assert_eq!(names.len(), 3);
        assert!(!names.contains(&backup_name(1_000_000)));
        let last = backups.join(backup_name(4_000_000));
        assert_eq!(fs::read_to_string(last.join("overlay-settings.json")).unwrap(), "{\"opacity\":50}");
        assert_eq!(fs::read_to_string(last.join("profiles.json")).unwrap(), "{\"p\":1}");
        assert!(!last.join("missing.json").exists());
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn versions_are_compared_as_numbers() {
        assert!(version_key("0.7.9") < version_key("0.7.10"));
        assert_eq!(version_key("0.7"), None);
        assert_eq!(installer_version("WoW-Companion-Setup-0.7.54-x64.exe").as_deref(), Some("0.7.54"));
        assert_eq!(installer_version("WoW-Companion-Setup-0.7.54-x64.exe.sha256"), None);
        assert_eq!(installer_version("notes.txt"), None);
    }

    #[test]
    fn the_previous_version_is_the_newest_older_cached_installer() {
        let names: Vec<String> = ["0.7.9", "0.7.10", "0.7.54", "0.7.56"].iter().map(|v| installer_name(v)).chain(["x.part".to_string(), format!("{}.sha256", installer_name("0.7.53"))]).collect();
        assert_eq!(previous_installer(&names, "0.7.54").as_deref(), Some("0.7.10"));
        assert_eq!(previous_installer(&names, "0.7.56").as_deref(), Some("0.7.54"));
        assert_eq!(previous_installer(&names, "0.7.9"), None);
        assert_eq!(previous_installer(&names, "dev"), None);
        assert_eq!(previous_installer(&[], "0.7.54"), None);
    }

    #[test]
    fn the_backup_to_restore_is_the_newest_one_made_under_the_target_version() {
        let b = |n: &str, v: Option<&str>| (n.to_string(), v.map(|s| s.to_string()));
        let list = vec![
            b("backup-20261101-000000", Some("0.7.53")),
            b("backup-20261110-120000", Some("0.7.54")),
            b("backup-20261120-010101", Some("0.7.54\n")),
            b("backup-20261125-153005", Some("0.7.55")),
            b("backup-20261126-000000", None),
            b("notes", Some("0.7.54")),
        ];
        assert_eq!(pick_backup(&list, "0.7.54").as_deref(), Some("backup-20261120-010101"));
        assert_eq!(pick_backup(&list, "0.7.53").as_deref(), Some("backup-20261101-000000"));
        // None under that version: the current settings are kept.
        assert_eq!(pick_backup(&list, "0.7.40"), None);
        assert_eq!(pick_backup(&[], "0.7.54"), None);
    }

    #[test]
    fn restoring_puts_the_files_back_and_hands_over_the_page_settings() {
        let root = std::env::temp_dir().join(format!("wowc-restore-test-{}", std::process::id()));
        let live = root.join("live");
        fs::create_dir_all(&live).unwrap();
        fs::write(live.join("profiles.json"), "{\"now\":1}").unwrap();
        let backups = root.join("backups");
        make_backup(&backups, "0.7.54", "{\"opacity\":50}", &[("profiles.json", live.join("old-profiles.json"))], 1_000_000).unwrap();
        // The backup of 0.7.54 holds the old profiles and settings.
        let old = backups.join(backup_name(1_000_000));
        fs::write(old.join("profiles.json"), "{\"old\":1}").unwrap();
        make_backup(&backups, "0.7.55", "{\"opacity\":99}", &[], 2_000_000).unwrap();
        let dir = find_backup(&backups, "0.7.54").unwrap();
        assert_eq!(dir, old);
        let got = restore_files(&dir, &[("profiles.json", live.join("profiles.json")), ("providers.json", live.join("providers.json"))]).unwrap();
        assert_eq!(fs::read_to_string(live.join("profiles.json")).unwrap(), "{\"old\":1}");
        assert!(!live.join("providers.json").exists(), "a file the backup lacks is not invented");
        assert_eq!(got.files.iter().map(|f| f.0.clone()).collect::<Vec<_>>(), vec!["profiles.json".to_string()]);
        assert_eq!(got.settings_json.as_deref(), Some("{\"opacity\":50}"));
        // No backup under a version: nothing to restore.
        assert!(find_backup(&backups, "0.7.40").is_none());
        // A tray backup has no page settings, so none are restored over the page's.
        make_backup(&backups, "0.7.56", "", &[], 3_000_000).unwrap();
        assert!(restore_files(&find_backup(&backups, "0.7.56").unwrap(), &[]).unwrap().settings_json.is_none());
        let _ = fs::remove_dir_all(&root);
    }

    #[test]
    fn a_cached_installer_is_trusted_offline_only_while_it_matches_its_record() {
        let dir = std::env::temp_dir().join(format!("wowc-cache-test-{}", std::process::id()));
        fs::create_dir_all(&dir).unwrap();
        let p = dir.join(installer_name("0.7.54"));
        fs::write(&p, b"abc").unwrap();
        assert!(cached_verified(&dir, "0.7.54").is_none(), "no record, no trust");
        fs::write(hash_record(&p), "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad\n").unwrap();
        assert_eq!(cached_verified(&dir, "0.7.54"), Some(p.clone()));
        fs::write(&p, b"tampered").unwrap();
        assert!(cached_verified(&dir, "0.7.54").is_none());
        assert!(!p.exists(), "a mismatch deletes the installer");
        assert!(cached_verified(&dir, "../x").is_none());
        let _ = fs::remove_dir_all(&dir);
    }

    #[test]
    fn timestamps_and_release_answers_are_read() {
        assert_eq!(parse_utc("2026-11-25T15:30:05Z"), Some(1_795_620_605));
        assert_eq!(parse_utc("1970-01-01T00:00:00Z"), Some(0));
        assert_eq!(parse_utc("2026-13-25T15:30:05Z"), None);
        assert_eq!(parse_utc("yesterday"), None);
        assert_eq!(latest_release(r#"{"tag_name":"v0.7.55","published_at":"2026-11-25T15:30:05Z"}"#), Some(("0.7.55".to_string(), 1_795_620_605)));
        assert_eq!(latest_release(r#"{"tag_name":"nightly"}"#), None);
        assert_eq!(latest_release("<html>"), None);
        assert_eq!(pulled_versions(r#"{"pulled":[{"version":"v0.7.52"},{"version":"0.7.53"},{}]}"#), vec!["0.7.52".to_string(), "0.7.53".to_string()]);
        assert!(pulled_versions("garbage").is_empty());
    }

    #[test]
    fn the_pull_of_the_running_version_names_where_to_go_back() {
        let t = r#"{"pulled":[{"version":"v0.7.52","back_to":"0.7.51","reason":"crash"},{"version":"0.7.53","back_to":"v0.7.51","reason":" bad save "}]}"#;
        assert_eq!(running_pull(t, "0.7.53"), Some(GoBack { back_to: "0.7.51".into(), reason: "bad save".into() }));
        assert_eq!(running_pull(t, "0.7.52").map(|g| g.back_to).as_deref(), Some("0.7.51"));
        assert_eq!(running_pull(t, "0.7.54"), None);
        assert_eq!(running_pull("garbage", "0.7.53"), None);
    }

    #[test]
    fn the_shell_offers_only_newer_unpulled_settled_releases() {
        let now = 1_800_000_000u64;
        let old = now - 3 * 86400;
        assert_eq!(judge("0.7.55", old, "0.7.54", &[], now), Check::Available("0.7.55".into()));
        assert_eq!(judge("0.7.55", now - 3600, "0.7.54", &[], now), Check::Settling("0.7.55".into()));
        assert_eq!(judge("0.7.55", 0, "0.7.54", &[], now), Check::Available("0.7.55".into()), "an unknown time does not block");
        assert_eq!(judge("0.7.55", old, "0.7.54", &["0.7.55".to_string()], now), Check::UpToDate);
        assert_eq!(judge("0.7.54", old, "0.7.54", &[], now), Check::UpToDate);
        assert_eq!(judge("0.7.53", old, "0.7.54", &[], now), Check::UpToDate);
        assert_eq!(judge("0.7.55", old, "dev", &[], now), Check::UpToDate);
    }
}
