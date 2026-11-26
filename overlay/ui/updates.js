// The update banner, the version list and the check schedule, with no DOM or Tauri globals of their
// own so a test can drive them under Node. app.js passes in the page's pieces and does the install
// through the shell (the page never downloads or runs anything).
(function (root) {
  const CHECK_EVERY = 4 * 60 * 60 * 1000; // the regular check, and the longest a focused window waits
  const LATER_MS = 24 * 60 * 60 * 1000;   // "Later" hides the banner this long
  const WAKE_GAP = 3 * 60 * 1000;         // a one-minute tick that arrives this late means the PC slept
  const TICK = 60 * 1000;

  const bare = v => String(v || '').replace(/^v/, '');

  // The query for /api/update: the player's switches. fresh asks the sidecar to look again now.
  function updateQuery(cfg, fresh) {
    const q = [];
    if (cfg.updateNow) q.push('now=1');
    const skip = (cfg.skipped || []).map(bare).filter(Boolean);
    if (skip.length) q.push('skip=' + encodeURIComponent(skip.join(',')));
    if (fresh) q.push('fresh=1');
    return q.length ? '?' + q.join('&') : '';
  }

  // The in-game addon's offer, from /api/addon/update, or null when none is shown ("Later" hides it).
  function addonOffer(addon, cfg, now) {
    if (!addon || !addon.available || !addon.latest) return null;
    const v = bare(addon.latest);
    if (((cfg.later || {})['addon:' + v] || 0) > now) return null;
    return { version: v, installed: addon.installed || '', notes: addon.notes || '', reinstall: !!addon.reinstall,
      reason: addon.reason || '', needsNewer: !!addon.needs_newer, min: addon.min || '' };
  }

  // Addon notes under the app's, their version headings marked "Addon" so the two do not blur.
  function addonNotes(md) {
    return String(md || '').replace(/^##\s+(\S+)/gm, '## Addon $1');
  }

  // What the banner shows. status is /api/update's answer; busy is the install in progress ("" for
  // none); failure is the last install's plain error; addon is /api/addon/update's answer (optional).
  // A pulled running version always wins, and is not dismissible: only going back (or the version
  // being replaced) clears it. When the app and the addon both have an update, one notice covers both.
  function bannerState(status, cfg, now, busy, failure, addon) {
    if (!cfg.checkUpdates) return null;
    const a = addonOffer(addon, cfg, now);
    if (!status && !a) return null;
    status = status || {};
    const p = status.pulled;
    if (p) return { kind: 'pulled', version: p.version, reason: p.reason || 'it was pulled', backTo: p.back_to || '', busy, failure };
    const v = bare(status.latest);
    const hidden = !status.available || (cfg.skipped || []).map(bare).includes(v) || ((cfg.later || {})[v] || 0) > now;
    if (!hidden) {
      const both = a ? [status.notes || '', addonNotes(a.notes)].filter(Boolean).join('\n') : (status.notes || '');
      return { kind: 'offer', version: v, notes: both, auto: !!cfg.autoUpdate, busy, failure, addon: a };
    }
    if (a) return { kind: 'addon', version: a.version, notes: addonNotes(a.notes), auto: !!cfg.autoUpdate, busy, failure, addon: a };
    // An install the player started from the version list has its own progress and error line.
    if (busy || failure) return { kind: 'task', busy, failure };
    return null;
  }

  // The headline of an addon-only notice, in plain words.
  function addonText(a) {
    if (a.reinstall) return 'Addon ' + a.installed + ' has a known problem' + (a.reason ? ': ' + a.reason.replace(/[.\s]+$/, '') : '') + '. Version ' + a.version + ' is safe to go back to';
    if (a.needsNewer) return 'This app needs a newer addon than the one you have (' + a.installed + '). Addon ' + a.version + ' is ready';
    return 'Addon ' + a.version + ' is ready';
  }

  // The banner's words and buttons as data, so the page only has to draw them.
  function bannerView(s) {
    if (!s) return null;
    if (s.busy) return { cls: s.kind === 'pulled' ? 'bad' : 'ok', text: s.busy, actions: [] };
    const err = s.failure ? [{ id: 'retry', label: 'Try again' }] : [];
    if (s.kind === 'task') return { cls: 'bad', text: 'The update did not install: ' + s.failure, actions: [...err, { id: 'dismiss', label: 'Dismiss' }] };
    if (s.kind === 'pulled') {
      const back = s.backTo ? [{ id: 'back', label: 'Go back to ' + s.backTo }] : [];
      return {
        cls: 'bad',
        text: 'This version has a known problem: ' + s.reason.replace(/[.\s]+$/, '') + '.' + (s.failure ? ' ' + s.failure : ''),
        actions: [...back, { id: 'details', label: 'Details' }],
      };
    }
    const both = s.kind === 'offer' && s.addon;
    return {
      cls: 'ok',
      text: s.failure ? 'The update did not install: ' + s.failure
        : s.kind === 'addon' ? addonText(s.addon)
        : both ? 'Update ' + s.version + ' and addon ' + s.addon.version + ' are ready' : 'Update ' + s.version + ' is ready',
      actions: s.failure
        ? [...err, { id: 'later', label: 'Later' }]
        : [{ id: 'install', label: 'Install' }, { id: 'whatsnew', label: 'What’s new' }, { id: 'later', label: 'Later' }],
      // A small line under the buttons, not a button; gone once the setting is on.
      always: !s.failure && !s.auto,
    };
  }

  // "Always update automatically": the setting goes on; the caller then installs now.
  function turnOnAuto(cfg) { cfg.autoUpdate = true; return cfg; }
  // What the shell is told about the setting: it applies the banner's rules itself, without the page.
  function autoArgs(cfg) {
    return { enabled: !!cfg.autoUpdate && cfg.checkUpdates !== false, updateNow: !!cfg.updateNow, skipped: (cfg.skipped || []).map(bare).filter(Boolean) };
  }

  function skip(cfg, version) {
    const v = bare(version);
    cfg.skipped = [...new Set([...(cfg.skipped || []).map(bare), v])];
  }
  // keys are more things hidden by the same "Later" (the addon's "addon:<v>").
  function later(cfg, version, now, keys) {
    cfg.later = {}; // only the latest "Later" is kept
    for (const k of [bare(version), ...(keys || [])]) if (k && k !== 'addon:') cfg.later[k] = now + LATER_MS;
  }

  const NOTICE_BULLETS = 4; // the notice shows this many bullets, then a See all changes link

  // The page "See all changes" opens: the public release list (the one page the shell lets through).
  const CHANGELOG_URL = 'https://github.com/WoW-Companion/advisor/releases';
  // The note's request: everything after the version left, up to the new one.
  function whatsNewQuery(note) { return '?version=' + encodeURIComponent(note.version) + '&from=' + encodeURIComponent(note.from || ''); }

  // A release body as plain lines: no markdown marks, bullets as dots.
  function plainNotes(md) {
    const lines = String(md || '').replace(/\r/g, '').split('\n').map(l => l
      .replace(/^#{1,6}\s*/, '').replace(/^\s*[-*]\s+/, '• ').replace(/[`*_]{1,3}/g, '').replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').trimEnd());
    const text = lines.join('\n').replace(/\n{3,}/g, '\n\n').trim();
    return text || 'No notes were published for this version.';
  }

  // Notes as a list to draw: groups of bullets, one per version ("## 0.7.58" headings; a single
  // version has none, version ""). max caps the bullets in all (the notice shows 4); more says
  // something was left out, by the cap or by the server's "…and N more changes" line, so the
  // "See all changes" link shows. Marks and the leading dash are removed; no text is rewritten.
  function noteView(md, max) {
    const cap = max > 0 ? max : Infinity;
    const groups = [];
    let shown = 0, hidden = 0;
    for (const raw of String(md || '').replace(/\r/g, '').split('\n')) {
      const line = raw.trim();
      const h = /^##\s+(.+?)\s*$/.exec(line);
      if (h) { groups.push({ version: h[1], bullets: [] }); continue; }
      if (/^…\s*and \d+ more/.test(line)) { hidden += 1; continue; }
      const m = /^[-*•]\s+(.*)$/.exec(line);
      if (!m) continue;
      const t = m[1].replace(/[`*_]{1,3}/g, '').replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').trim();
      if (!t) continue;
      if (shown >= cap) { hidden += 1; continue; }
      if (!groups.length) groups.push({ version: '', bullets: [] });
      groups[groups.length - 1].bullets.push(t);
      shown++;
    }
    return { groups: groups.filter(g => g.bullets.length), more: hidden > 0 };
  }

  // Versions as numbers, for ordering: 0.7.58 > 0.7.9. Anything that is not x.y.z sorts last.
  function cmp(a, b) {
    const k = v => (/^(\d+)\.(\d+)\.(\d+)/.exec(bare(v)) || []).slice(1).map(Number);
    const x = k(a), y = k(b);
    if (x.length !== 3 || y.length !== 3) return x.length === y.length ? 0 : x.length ? -1 : 1;
    for (let i = 0; i < 3; i++) if (x[i] !== y[i]) return x[i] < y[i] ? 1 : -1;
    return 0; // 1 when a is older, -1 when newer: sorts newest first
  }

  // The Settings list, newest first with the running version in its place. A newer row offers
  // "Update to <v>" (primary); an older one "Go back to <v>" with a one-line confirm.
  function versionRows(status) {
    const list = ((status && status.versions) || []).slice().sort((a, b) => cmp(a.version, b.version));
    const cur = list.find(v => v.current);
    return list.map(v => {
      const r = { version: v.version, date: v.date || '', current: !!v.current, button: '', primary: false, confirm: '',
        label: v.version + (v.current ? ' (running)' : '') + (v.date ? ' · ' + v.date : '') };
      if (!v.current && cur) {
        if (cmp(v.version, cur.version) < 0) { r.button = 'Update to ' + v.version; r.primary = true; }
        else { r.button = 'Go back to ' + v.version; r.confirm = 'Go back to ' + v.version + '? Your settings from that version are restored.'; }
      } else if (!v.current) r.button = 'Update to ' + v.version;
      return r;
    });
  }

  // The line at the top of Updates: an offered update with its button, else when we last looked.
  function noticeView(status, checkedAt) {
    if (!status) return null;
    if (status.available && !status.pulled) return { cls: 'ok', text: 'Update ' + bare(status.latest) + ' is ready', button: 'Update now', version: bare(status.latest) };
    if (!checkedAt) return { cls: '', text: 'You’re up to date', button: '' };
    const d = new Date(checkedAt);
    const hm = String(d.getHours()).padStart(2, '0') + ':' + String(d.getMinutes()).padStart(2, '0');
    return { cls: '', text: 'You’re up to date (checked ' + hm + ')', button: '' };
  }

  // Settings > Updates, addon part: the installed version, its update and the kept versions to go
  // back to. checked is only for the up-to-date line.
  function addonView(addon) {
    if (!addon) return null;
    if (!addon.installed) return { text: addon.message || '', button: '', version: '', back: [] };
    const text = addon.installed === 'unknown' ? 'Addon installed (version unknown)'
      : 'Addon ' + addon.installed + (addon.available ? ': ' + addon.latest + ' is ready' : ' (up to date)');
    return {
      text, version: addon.available ? bare(addon.latest) : '',
      button: addon.available ? (addon.reinstall ? 'Go back to ' + bare(addon.latest) : 'Update addon') : '',
      back: (addon.back || []).map(v => ({ version: v, button: 'Go back to ' + v, confirm: 'Go back to addon ' + v + '? The addon files from that version are restored.' })),
    };
  }

  // Whether the page should install the addon by itself now: "Install updates automatically" is on,
  // nothing is being written or typed (quiet), and the server says so (auto is "install" or "goback").
  // A version pulled from the addon goes back whatever "Later" says; a plain update respects it.
  function addonAuto(addon, cfg, now, quiet) {
    if (!quiet || !cfg.autoUpdate || cfg.checkUpdates === false || !addon || !addon.available || !addon.auto) return '';
    if (addon.auto === 'install' && !addonOffer(addon, cfg, now)) return '';
    return addon.auto;
  }

  // The line under an addon update: a /reload is required, or a full restart when the update added
  // files (the game reads an addon's TOC and file list only at start).
  const RELOAD_TEXT = 'A /reload in game is required to load the new addon. Type /reload now if WoW is open.';
  const RESTART_TEXT = "This addon update adds new files: fully exit and restart WoW to load it (/reload isn't enough).";
  const reloadLine = restart => restart ? RESTART_TEXT : RELOAD_TEXT;

  // --- "What's new": the release history with a "New since you last looked" divider ------------
  // cfg.notesSeen = { app, addon } holds the newest version of each component the player has seen.
  // The view always lists the history; seen versions (<= notesSeen) sit under the divider.
  const BELOW_ALL = '0.0.0'; // seeded when the previous version is unknown: every capped note is new
  const known = v => !!bare(v) && bare(v) !== 'unknown';
  // Seeds an empty notesSeen[component]. prev = { app, addon: the version before the update ("" when
  // unknown), upgrade: whether this is an update rather than a fresh install }. An upgrade seeds the
  // previous version (or below everything when unknown); only a fresh install seeds the installed one.
  function seedNotesSeen(cfg, app, addon, prev) {
    const seen = cfg.notesSeen = Object.assign({ app: '', addon: '' }, cfg.notesSeen);
    const p = prev || {};
    let changed = false;
    for (const [k, v] of [['app', app], ['addon', addon]]) {
      if (seen[k] || !known(v)) continue;
      seen[k] = known(p[k]) ? bare(p[k]) : p.upgrade ? BELOW_ALL : bare(v);
      changed = true;
    }
    return changed;
  }
  // One-time fix: the first 0.7.95 run seeded notesSeen.app with the version it had just installed,
  // which hid that release. That value is treated as unseeded once (cfg.notesSeedFixed).
  function fixBuggySeed(cfg) {
    if (cfg.notesSeedFixed) return false;
    cfg.notesSeedFixed = true;
    const seen = cfg.notesSeen = Object.assign({ app: '', addon: '' }, cfg.notesSeen);
    if (bare(seen.app) === '0.7.95') seen.app = BELOW_ALL;
    return true;
  }
  // The query for /api/notes-since: the whole history within the server's cap, up to the installed one.
  function notesQuery(cfg, app, addon) {
    const a = bare(app), d = bare(addon);
    const q = { app_from: BELOW_ALL, app: a, addon_from: BELOW_ALL, addon: d };
    return '?' + Object.keys(q).map(k => k + '=' + encodeURIComponent(q[k])).join('&');
  }
  const OLDER_TEXT = 'Older changes: see the release page';
  const DIVIDER_TEXT = 'You’re caught up: everything below you’ve seen';
  const headingText = n => n + ' new since you last looked';
  // The view always opens at the top, the newest release.
  const notesScrollTop = () => 0;
  const UPTODATE_TEXT = 'You’re up to date';
  // The server's answer as a list to draw, top to bottom: App and Addon releases interleaved by date
  // (newest first), the unseen ones above the divider, the seen ones below. Items are
  // { type: 'heading', text } | { type: 'uptodate' } | { type: 'divider', text } | { type: 'entry', component, version, date,
  // bullets, unseen }. Unseen entries open under "N new since you last looked"; with nothing unseen the list
  // opens with "You're up to date" and has no divider; with everything unseen the divider is last. unseen counts the new entries; more is the older-releases link.
  function historyView(resp, cfg) {
    const seen = (cfg && cfg.notesSeen) || {};
    const entries = [];
    let more = false;
    for (const [key, title] of [['app', 'App'], ['addon', 'Addon']]) {
      const part = (resp && resp[key]) || {};
      if (part.more) more = true;
      let date = '9999-12-31'; // an undated release follows the one above it in its own component
      for (const e of part.entries || []) {
        if (!(e.lines || []).length) continue;
        if (e.date) date = e.date;
        entries.push({ type: 'entry', component: title, version: bare(e.version), date, bullets: e.lines.slice(), unseen: cmp(e.version, seen[key] || '') < 0, n: entries.length });
      }
    }
    entries.sort((x, y) => (x.date < y.date ? 1 : x.date > y.date ? -1 : x.n - y.n));
    entries.forEach(e => delete e.n);
    const fresh = entries.filter(e => e.unseen), old = entries.filter(e => !e.unseen);
    const items = fresh.length ? [{ type: 'heading', text: headingText(fresh.length) }, ...fresh, { type: 'divider', text: DIVIDER_TEXT }, ...old] : [{ type: 'uptodate', text: UPTODATE_TEXT }, ...old];
    return { items, unseen: fresh.length, more: more ? { text: OLDER_TEXT, url: CHANGELOG_URL } : null };
  }
  // Closing the view (or reading it for NOTES_READ_MS) marks the installed versions seen. A component
  // whose installed version is not known keeps its mark. True when it changed.
  const NOTES_READ_MS = 3000;
  function markSeen(cfg, app, addon) {
    const seen = cfg.notesSeen = Object.assign({ app: '', addon: '' }, cfg.notesSeen);
    const a = bare(app), d = bare(addon);
    const next = { app: a || seen.app, addon: d && d !== 'unknown' ? d : seen.addon };
    const changed = next.app !== seen.app || next.addon !== seen.addon;
    cfg.notesSeen = next;
    return changed;
  }
  // The notice's count: " · 3 new" when there are unseen entries, "" otherwise.
  const newCount = n => n > 0 ? ' · ' + n + ' new' : '';

  // The note at the top of the chat on the first start of a different version than the last one.
  // last is the version seen at the previous start ("" the first time ever: no note).
  // reason is why an automatic go-back happened ("" when the player went back themselves).
  // addon is the addon version installed with this update ("" when none was): the line then names
  // the app, the addon or both. restart is true when the addon update added files.
  function updatedNote(last, running, reason, addon, restart) {
    const a = bare(last), b = bare(running), d = bare(addon || '');
    if (!a || !b || a === b || cmp(a, b) === 0) return d ? { kind: 'up', version: '', addon: d, text: '✓ Updated: addon ' + d, reload: reloadLine(restart), restart: !!restart, whatsNew: true } : null;
    if (cmp(b, a) >= 0) return { kind: 'back', version: b, text: '✓ Went back to ' + b + (reason ? ': ' + String(reason).replace(/[.\s]+$/, '') : ''), whatsNew: false };
    return { kind: 'up', version: b, from: a, addon: d, text: '✓ Updated: app ' + b + (d ? ' and addon ' + d : ''), ...(d ? { reload: reloadLine(restart), restart: !!restart } : {}), whatsNew: true };
  }
  // The notice for an addon installed while the app keeps running; addon is "" or "unknown" when the
  // version is not known (the reload line then clears on the next save).
  function addonDoneNote(addon, message, restart) {
    const d = bare(addon || '');
    const known = d && d !== 'unknown';
    return { kind: 'up', version: '', addon: known ? d : '', text: '✓ ' + (known ? 'Updated: addon ' + d : message), reload: reloadLine(restart), restart: !!restart, whatsNew: true };
  }
  // Whether the game now runs the addon the notice named. game is the app's view of the last save:
  // { addon: the addonVersion it recorded ("" when none), savedAt: when the file was written }, and
  // base is savedAt at the time of the update. With a recorded version, the save must show the new
  // one; without, the next write after the update (a /reload writes it) is enough.
  function reloadDone(note, game, base) {
    if (!note || !note.reload || !game) return false;
    const v = bare(game.addon || '');
    if (v && v !== 'unknown' && note.addon) return cmp(v, note.addon) <= 0; // cmp is 1 when v is older
    if (note.restart) return false; // a save after a /reload does not show a restart happened
    return base != null && (game.savedAt || 0) > base;
  }
  // When to look again: on start, every CHECK_EVERY, after the PC slept (a late one-minute tick), and
  // when the window gains focus more than CHECK_EVERY after the last look. now() is the clock.
  function createSchedule(deps) {
    let last = deps.now(), tickAt = deps.now();
    return {
      checked() { last = deps.now(); },
      tick() { // call every TICK
        const t = deps.now(), gap = t - tickAt;
        tickAt = t;
        if (gap > WAKE_GAP) { deps.check(); return 'wake'; }
        return '';
      },
      focus() {
        if (deps.now() - last > CHECK_EVERY) { deps.check(); return true; }
        return false;
      },
    };
  }

  const api = { addonOffer, addonView, addonAuto, CHECK_EVERY, LATER_MS, WAKE_GAP, TICK, updateQuery, bannerState, bannerView, turnOnAuto, autoArgs, skip, later, plainNotes, noteView, NOTICE_BULLETS, versionRows, noticeView, updatedNote, addonDoneNote, reloadDone, whatsNewQuery, seedNotesSeen, fixBuggySeed, notesQuery, historyView, notesScrollTop, DIVIDER_TEXT, markSeen, newCount, NOTES_READ_MS, BELOW_ALL, OLDER_TEXT, CHANGELOG_URL, cmp, createSchedule };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcUpdates = api;
})(typeof window !== 'undefined' ? window : globalThis);
