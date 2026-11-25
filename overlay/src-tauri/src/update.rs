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
        let _ = fs::remove_file(dir.join(n));
    }
}

/// Copies the overlay's settings (sent by the page) and the profile files into a new dated folder
/// under `root`, then trims the older folders to the last `KEEP_BACKUPS`.
pub fn make_backup(root: &Path, settings_json: &str, files: &[(&str, PathBuf)], secs: u64) -> Result<PathBuf, String> {
    let dir = root.join(backup_name(secs));
    fs::create_dir_all(&dir).map_err(|e| format!("could not make the backup folder: {e}"))?;
    fs::write(dir.join("overlay-settings.json"), settings_json).map_err(|e| format!("could not save the settings backup: {e}"))?;
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

fn say(app: &AppHandle, text: &str) {
    let _ = app.emit("update-progress", text);
}

/// Makes sure the verified installer of `version` is in `cache`, downloading and checking it when it
/// is not. `sums` is that release's SHA256SUMS text.
fn ensure_installer(cache: &Path, version: &str, sums: &str) -> Result<PathBuf, String> {
    let name = installer_name(version);
    let want = sums_entry(sums, &name).ok_or_else(|| format!("Version {version} has no checksum to verify, so it was not installed."))?;
    let path = cache.join(&name);
    if path.is_file() && verify_or_delete(&path, &want).is_ok() {
        return Ok(path);
    }
    let part = cache.join(format!("{name}.part"));
    curl(&format!("{DOWNLOAD_BASE}/v{version}/{name}"), &part)?;
    verify_or_delete(&part, &want)?;
    fs::rename(&part, &path).map_err(|e| format!("could not keep the installer: {e}"))?;
    Ok(path)
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
    let cache = app.path().app_cache_dir().map_err(|e| e.to_string())?.join("updates");
    fs::create_dir_all(&cache).map_err(|e| format!("could not make the update folder: {e}"))?;
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
    let mut files: Vec<(&str, PathBuf)> = Vec::new();
    if let Ok(d) = app.path().app_config_dir() {
        files.push(("agent.txt", d.join("agent.txt")));
    }
    if let Ok(d) = app.path().config_dir() {
        files.push(("profiles.json", d.join("wowc-advisor").join("profiles.json")));
        files.push(("providers.json", d.join("wowc-advisor").join("providers.json")));
    }
    let root = app.path().app_data_dir().map_err(|e| e.to_string())?.join("backups");
    make_backup(&root, settings_json, &files, now_secs())?;
    Ok(installer)
}

/// Runs the installer in passive mode (progress bar only, relaunching the app when done) and exits
/// this app so the installer can replace it.
#[cfg(windows)]
pub fn run_and_exit(app: &AppHandle, installer: &Path) -> Result<(), String> {
    say(app, "Installing…");
    std::process::Command::new(installer).args(["/P", "/R"]).spawn().map_err(|e| format!("could not start the installer: {e}"))?;
    std::thread::sleep(std::time::Duration::from_millis(800));
    app.exit(0);
    Ok(())
}

#[cfg(not(windows))]
pub fn run_and_exit(_: &AppHandle, _: &Path) -> Result<(), String> {
    Err("Updating in the app works on Windows only.".into())
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
            make_backup(&backups, "{\"opacity\":50}", &[("profiles.json", src.join("profiles.json")), ("missing.json", src.join("missing.json"))], secs).unwrap();
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
}
