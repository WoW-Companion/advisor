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

  // What the banner shows. status is /api/update's answer; busy is the install in progress ("" for
  // none); failure is the last install's plain error. A pulled running version always wins, and is
  // not dismissible: only going back (or the version being replaced) clears it.
  function bannerState(status, cfg, now, busy, failure) {
    if (!cfg.checkUpdates || !status) return null;
    const p = status.pulled;
    if (p) return { kind: 'pulled', version: p.version, reason: p.reason || 'it was pulled', backTo: p.back_to || '', busy, failure };
    const v = bare(status.latest);
    const hidden = !status.available || (cfg.skipped || []).map(bare).includes(v) || ((cfg.later || {})[v] || 0) > now;
    if (!hidden) return { kind: 'offer', version: v, notes: status.notes || '', busy, failure };
    // An install the player started from the version list has its own progress and error line.
    if (busy || failure) return { kind: 'task', busy, failure };
    return null;
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
    return {
      cls: 'ok',
      text: s.failure ? 'The update did not install: ' + s.failure : 'Update ' + s.version + ' is ready',
      actions: s.failure
        ? [...err, { id: 'later', label: 'Later' }]
        : [{ id: 'notes', label: 'What’s new' }, { id: 'install', label: 'Install' }, { id: 'later', label: 'Later' }, { id: 'skip', label: 'Skip this version' }],
    };
  }

  function skip(cfg, version) {
    const v = bare(version);
    cfg.skipped = [...new Set([...(cfg.skipped || []).map(bare), v])];
  }
  function later(cfg, version, now) {
    cfg.later = { [bare(version)]: now + LATER_MS }; // only the latest "Later" is kept
  }

  // A release body as plain lines: no markdown marks, bullets as dots.
  function plainNotes(md) {
    const lines = String(md || '').replace(/\r/g, '').split('\n').map(l => l
      .replace(/^#{1,6}\s*/, '').replace(/^\s*[-*]\s+/, '• ').replace(/[`*_]{1,3}/g, '').replace(/\[([^\]]+)\]\([^)]*\)/g, '$1').trimEnd());
    const text = lines.join('\n').replace(/\n{3,}/g, '\n\n').trim();
    return text || 'No notes were published for this version.';
  }

  // The Settings list: the running version (marked) then the others, each with an install button.
  function versionRows(status) {
    return ((status && status.versions) || []).map(v => ({
      version: v.version, date: v.date || '', current: !!v.current,
      label: v.version + (v.current ? ' (running)' : '') + (v.date ? ' · ' + v.date : ''),
    }));
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

  const api = { CHECK_EVERY, LATER_MS, WAKE_GAP, TICK, updateQuery, bannerState, bannerView, skip, later, plainNotes, versionRows, createSchedule };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcUpdates = api;
})(typeof window !== 'undefined' ? window : globalThis);
