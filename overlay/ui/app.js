// The overlay's page. It holds no game logic: it asks the Go advisor's companion API (the sidecar)
// and shows what comes back. The window itself is driven through the Rust shell's commands.
const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;
const win = window.__TAURI__.window.getCurrentWindow();

const $ = id => document.getElementById(id);
const log = $('log'), who = $('who'), badge = $('badge'), text = $('text'), send = $('send');
const DEFAULTS = { opacity: 92, solidHover: false, mode: 'overlay', keyVis: 'Ctrl+Shift+W', keyCt: 'Ctrl+Shift+T', nextOpen: null, settingsOpen: false, chatOpen: true, size: 'M', checkUpdates: true, cleared: [], who: '', setupSeen: false, setupHidden: [], gameDir: '', length: 'standard', model: 'efficient', customModel: '', usage: true };
// nextOpen null means automatic: open until the chat has a message, then collapsed. A click makes it a choice.
const cfg = { ...DEFAULTS, ...JSON.parse(localStorage.getItem('wowc') || '{}') };
const save = () => localStorage.setItem('wowc', JSON.stringify(cfg));

let conn = null, state = null, busy = false, events = null;

// --- connection to the sidecar -------------------------------------------------------------

function api(path, opts = {}) {
  return fetch(conn.url + path, { ...opts, headers: { ...opts.headers, 'X-Wowc-Token': conn.token } });
}
function sse(path) { return new EventSource(conn.url + path + (path.includes('?') ? '&' : '?') + 'token=' + encodeURIComponent(conn.token)); }

// What the page says about the advisor (the sidecar), one state at a time:
//
//   starting         -> ready            the handshake arrived and the sidecar has an AI client
//   starting         -> ready-no-agent   the handshake arrived and it has none: only the calm
//                                        "Answers need an AI client" notice shows
//   ready*           -> stopped          the sidecar exited, or its data stayed unreachable
//   stopped / ready* -> reconnecting     Reconnect, Re-scan or a new AI client choice restarts it
//   reconnecting     -> ready*|stopped   the new handshake arrives, or ~10 s pass without one
//
// "Starting" ends with the handshake, not later. The handshake and the page's own start-up
// status query race, so only a state that is still `starting` may be set from a start-up answer
// (see the start block). The red banner (with Reconnect) is for `stopped` only and always carries
// a message; starting and reconnecting show a calm status line instead.
let phase = 'starting';
function setPhase(next, msg) {
  phase = next;
  const waiting = next === 'starting' || next === 'reconnecting';
  $('banner').hidden = next !== 'stopped';
  $('banner-text').textContent = next === 'stopped' ? msg || 'The advisor stopped.' : '';
  $('status').hidden = !waiting;
  $('status').textContent = waiting ? msg || (next === 'starting' ? 'Starting the advisor…' : 'Reconnecting…') : '';
  $('aibar').hidden = next !== 'ready-no-agent';
}
function notice(msg) {
  const n = $('notice');
  n.textContent = msg; n.hidden = false;
  clearTimeout(notice.t);
  notice.t = setTimeout(() => { n.hidden = true; }, 6000);
}

const RECONNECT_MS = 10000;
const sleep = ms => new Promise(r => setTimeout(r, ms));
// The sidecar's last stderr line, when the shell has one.
async function lastError() {
  try { return (await invoke('get_status')).error || ''; } catch { return ''; }
}
// load() with a short backoff: right after a (re)start the data endpoint may not answer yet.
async function loadSoon(c) {
  const until = Date.now() + RECONNECT_MS;
  for (let wait = 150; ; wait = Math.min(wait * 2, 1000)) {
    try { return await load(); } catch (e) {
      if (conn !== c) throw e;
      if (Date.now() + wait > until) throw e;
      await sleep(wait);
    }
  }
}
// Restarts the sidecar and waits for its handshake; if none comes in time, say why once.
let restartTimer = 0;
async function restart(msg, go = () => invoke('restart_sidecar')) {
  conn = null; showAgent();
  if (events) events.close();
  setPhase('reconnecting', msg);
  clearTimeout(restartTimer);
  const mine = restartTimer = setTimeout(async () => {
    if (conn || phase !== 'reconnecting') return;
    const err = await lastError();
    if (restartTimer === mine && !conn && phase === 'reconnecting') setPhase('stopped', 'The advisor did not start.' + (err ? ' ' + err : ''));
  }, RECONNECT_MS);
  try { await go(); } catch (e) { clearTimeout(restartTimer); setPhase('stopped', 'Could not restart the advisor: ' + e); }
}

async function connect(c) {
  conn = c;
  clearTimeout(restartTimer);
  if (events) events.close();
  setPhase(c.agent ? 'ready' : 'ready-no-agent');
  showAgent();
  try {
    await loadSoon(c);
  } catch (e) {
    if (conn !== c) return; // a newer run took over
    const err = await lastError();
    return setPhase('stopped', 'The advisor did not answer. ' + (err || e.message));
  }
  if (conn !== c) return;
  loadSetup(true);
  events = sse('/api/events');
  const es = events;
  events.addEventListener('game', ev => { state = JSON.parse(ev.data); setChars(); showBadge(); });
  events.addEventListener('history', ev => render(JSON.parse(ev.data).filter(e => e.source === 'game')));
  events.onerror = () => {
    // The stream drops when the sidecar dies or is restarted; only a drop of the current stream
    // while nothing is restarting is news. The shell's "sidecar-down" explains why.
    es.close();
    if (conn !== c || events !== es) return;
    api('/api/state').catch(async () => {
      if (conn !== c || events !== es) return;
      const err = await lastError();
      setPhase('stopped', 'Lost the advisor.' + (err ? ' ' + err : ' Reconnect to start it again.'));
    });
  };
}

listen('sidecar-up', ev => connect(ev.payload));
listen('sidecar-down', ev => { conn = null; showAgent(); if (events) events.close(); clearTimeout(restartTimer); setPhase('stopped', 'The advisor stopped.' + (ev.payload ? ' ' + ev.payload : '')); });
listen('notice', ev => notice(ev.payload));
listen('mode', ev => { cfg.mode = ev.payload; save(); showMode(); });
listen('clickthrough', ev => { $('ct').hidden = !ev.payload; showCt(); });
$('retry').onclick = () => restart('Reconnecting…');

// --- AI client -----------------------------------------------------------------------------
// The choice is kept by the shell and passed to the sidecar as --agent: "" (no choice), "auto",
// a preset name or a custom command. Changing it restarts the sidecar.

const PRESETS = ['claude', 'codex', 'gemini'];
let agentChoice = '';

function showAgent() {
  const known = agentChoice === '' || agentChoice === 'auto' || PRESETS.includes(agentChoice);
  $('agent').value = known ? (agentChoice || 'auto') : 'custom';
  // The command box is for Custom only.
  $('agent-cmd-row').hidden = $('agent').value !== 'custom';
  if (!known) $('agent-cmd').value = agentChoice;
  const auto = $('agent').options[0], found = conn && conn.agent_source === 'auto' && conn.agent;
  auto.textContent = 'Auto' + (found ? ' (' + conn.agent[0].toUpperCase() + conn.agent.slice(1) + ')' : conn && !conn.agent ? ' (none found)' : '');
  // The picker's tooltip names what is in use and where it was found.
  $('agent').title = !conn ? '' : !conn.agent ? 'No AI client found. Install one, then Re-scan.'
    : 'Using ' + conn.agent + (conn.agent_path ? ' at ' + conn.agent_path : ' (' + conn.agent_source + ')');
}
async function chooseAgent(value) {
  agentChoice = value;
  await restart('Restarting the advisor…', () => invoke('set_agent', { agent: value }));
}
$('agent').onchange = ev => {
  if (ev.target.value === 'custom') { agentChoice = $('agent-cmd').value.trim() || 'custom'; showAgent(); $('agent-cmd').focus(); }
  else chooseAgent(ev.target.value);
};
// The sidecar looks for AI clients when it starts, so looking again is a restart.
$('agent-rescan').onclick = () => restart('Looking for AI clients…');
$('agent-cmd').onchange = ev => { if (ev.target.value.trim()) chooseAgent(ev.target.value.trim()); };
for (const a of document.querySelectorAll('a[data-page]')) {
  a.onclick = ev => { ev.preventDefault(); invoke('open_install_page', { url: a.dataset.page }).catch(e => notice(String(e))); };
}

// --- window controls -----------------------------------------------------------------------

function showMode() {
  const overlay = cfg.mode === 'overlay', b = $('mode');
  b.dataset.mode = overlay ? 'overlay' : 'window';
  b.setAttribute('aria-label', overlay ? 'Overlay mode' : 'Window mode');
  b.title = overlay ? 'Overlay mode: always on top (click for Window)' : 'Window mode (click for Overlay)';
}
function showCt() { $('ct').title = 'Click-through on (' + cfg.keyCt + ')'; }
$('mode').onclick = () => invoke('set_mode', { overlay: cfg.mode !== 'overlay' });
$('tray').onclick = () => invoke('hide_window');
$('close').onclick = () => invoke('quit');
$('grip').onmousedown = ev => { ev.preventDefault(); win.startResizeDragging('SouthEast'); };
function showPanes() {
  $('settings').hidden = !cfg.settingsOpen;
  $('settings-btn').setAttribute('aria-expanded', String(cfg.settingsOpen));
  const next = nextOpen();
  $('next').classList.toggle('open', next);
  $('next-head').setAttribute('aria-expanded', String(next));
  log.hidden = !cfg.chatOpen;
  const t = $('chat-toggle');
  t.setAttribute('aria-expanded', String(cfg.chatOpen));
  t.title = cfg.chatOpen ? 'Collapse the chat' : 'Show the chat';
  t.textContent = cfg.chatOpen ? '\u25BE' : '\u25B8';
  document.body.dataset.size = cfg.size;
  $('size').value = cfg.size;
}
const nextOpen = () => cfg.nextOpen === null ? !log.children.length : cfg.nextOpen;
$('settings-btn').onclick = () => { cfg.settingsOpen = !cfg.settingsOpen; save(); showPanes(); };
$('next-head').onclick = () => { cfg.nextOpen = !nextOpen(); save(); showPanes(); };
$('chat-toggle').onclick = () => { cfg.chatOpen = !cfg.chatOpen; save(); showPanes(); if (cfg.chatOpen) scrollDown(true); };
$('size').onchange = ev => { cfg.size = ev.target.value; save(); showPanes(); };
$('clear').onclick = () => {
  cfg.cleared = [...new Set([...cfg.cleared, ...[...log.children].map(e => e.id).filter(Boolean)])].slice(-500);
  save(); log.replaceChildren(); showPanes();
};

// The slider drives the panel background's alpha (--alpha) and the text outline (--outline), never
// the window or element opacity. The outline fades in as the background drops below ~55%.
const HOVER_MIN = 85, HOVER_LEAVE_MS = 1000;
let hovering = false, leaveTimer = 0;
const outlineFor = pct => Math.min(1, Math.max(0, (55 - pct) / 35));
function paint() {
  const pct = hovering && cfg.solidHover ? Math.max(cfg.opacity, HOVER_MIN) : cfg.opacity;
  const root = document.documentElement.style;
  root.setProperty('--alpha', pct / 100);
  root.setProperty('--outline', outlineFor(pct));
}
function setOpacity(v) {
  cfg.opacity = v; save(); paint();
  $('opacity').value = v; $('opacity-out').textContent = v + '%';
}
$('panel').onmouseenter = () => { clearTimeout(leaveTimer); hovering = true; paint(); };
$('panel').onmouseleave = () => { clearTimeout(leaveTimer); leaveTimer = setTimeout(() => { hovering = false; paint(); }, HOVER_LEAVE_MS); };
$('auto-digest').onchange = ev => { cfg.autoDigest = ev.target.checked; save(); pushDigest(); };
$('solid-hover').onchange = ev => { cfg.solidHover = ev.target.checked; save(); paint(); };
$('opacity').oninput = ev => setOpacity(+ev.target.value);

// A shortcut box turns a key press into the plugin's "Ctrl+Shift+W" form.
function combo(ev) {
  if (!(ev.ctrlKey || ev.altKey || ev.metaKey)) return null;
  const m = /^(?:Key([A-Z])|Digit(\d)|(F\d{1,2}))$/.exec(ev.code);
  if (!m) return null;
  return [ev.ctrlKey && 'Ctrl', ev.altKey && 'Alt', ev.shiftKey && 'Shift', ev.metaKey && 'Super', m[1] || m[2] || m[3]].filter(Boolean).join('+');
}
async function applyShortcuts() {
  try {
    await invoke('set_shortcuts', { visibility: cfg.keyVis, clickThrough: cfg.keyCt });
  } catch (e) {
    notice('Shortcut not available (another program may use it): ' + e);
  }
}
for (const [id, field] of [['key-vis', 'keyVis'], ['key-ct', 'keyCt']]) {
  const el = $(id);
  el.value = cfg[field];
  el.onkeydown = ev => {
    ev.preventDefault();
    const c = combo(ev);
    if (!c) return;
    el.value = cfg[field] = c; save();
    applyShortcuts();
  };
}

// --- content -------------------------------------------------------------------------------

// The chip is the data's age ("now", "5m", "2h"); the full capture time is in its tooltip.
function ago(t) {
  const s = Math.max(0, Date.now() / 1000 - t);
  return s < 60 ? 'now' : s < 3600 ? Math.floor(s / 60) + 'm' : s < 86400 ? Math.floor(s / 3600) + 'h' : Math.floor(s / 86400) + 'd';
}
// The tooltip's note on the in-game digest the advisor keeps up to date.
function digestNote() {
  const d = state && state.digest;
  if (!d || !d.enabled || d.status === 'addon not found') return '';
  return d.updated_at ? '\nin-game advice updated ' + new Date(d.updated_at * 1000).toLocaleTimeString() : '\nin-game advice: waiting for the next save';
}
// The setting under "Keep in-game advice up to date": its note says why it does nothing, if it does not.
function showDigest() {
  const d = state && state.digest;
  $('auto-digest').checked = cfg.autoDigest !== false;
  $('auto-digest-note').textContent = d && d.status === 'addon not found' ? 'addon not found' : d && d.status === 'error' ? d.error : '';
}
// Answer length, model tier and the usage line are kept here and pushed to the advisor, which
// applies them to the next ask; no restart. The tooltip says what the tiers mean for this client.
function showAnswerSettings() {
  $('length').value = cfg.length; $('model').value = cfg.model; $('usage').checked = cfg.usage !== false;
  $('model-custom').value = cfg.customModel;
  $('model-custom-row').hidden = cfg.model !== 'custom';
  const note = state && state.settings && state.settings.model_note;
  $('model').title = note || 'Efficient is the AI client\'s small, fast tier. For a custom command the tiers do nothing; use Custom with a model name.';
}
async function pushSettings() {
  showAnswerSettings();
  try { await api('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ length: cfg.length, model: cfg.model, custom_model: cfg.customModel, usage: cfg.usage !== false }) }); } catch (e) { /* the next state load pushes again */ }
}
$('length').onchange = ev => { cfg.length = ev.target.value; save(); pushSettings(); };
$('model').onchange = ev => { cfg.model = ev.target.value; save(); pushSettings(); if (cfg.model === 'custom') $('model-custom').focus(); };
$('model-custom').onchange = ev => { cfg.customModel = ev.target.value.trim(); save(); pushSettings(); };
$('usage').onchange = ev => { cfg.usage = ev.target.checked; save(); pushSettings(); };
async function pushDigest() {
  try { await api('/api/auto-digest', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ on: cfg.autoDigest !== false }) }); } catch (e) { /* shown by the next state load */ }
}
function showBadge() {
  if (!state) return;
  showDigest();
  const t = state.captured_at, s = t ? Date.now() / 1000 - t : 0;
  badge.textContent = t ? ago(t) : '—';
  badge.className = 'chip' + (!t || s > 7200 ? ' red' : s > 1800 ? ' amber' : '');
  badge.title = 'data from game: ' + (t ? new Date(t * 1000).toLocaleTimeString() : 'no capture yet') + (state.error ? ' — ' + state.error : '') + digestNote();
}
function setChars() {
  const cur = who.value || cfg.who;
  who.replaceChildren(...state.characters.map(c => {
    const o = document.createElement('option');
    o.value = c.key; o.textContent = c.name + (c.level ? ' (' + c.level + ' ' + c.class + ')' : '');
    return o;
  }));
  who.value = state.characters.some(c => c.key === cur) ? cur : state.default;
  showNext();
}
who.onchange = () => { cfg.who = who.value; save(); showNext(); };

function showNext() {
  const c = state && state.characters.find(c => c.key === who.value);
  const items = (c && c.next) || [];
  $('next-line').textContent = items.length ? items[0] : 'Nothing queued yet.';
  $('next-list').replaceChildren(...(items.length ? items : ['Nothing queued yet.']).map(t => {
    const li = document.createElement('li');
    li.textContent = t;
    if (!items.length) li.className = 'none';
    return li;
  }));
}

// Follow the newest text unless the reader has scrolled up.
let stick = true;
log.onscroll = () => { stick = log.scrollHeight - log.scrollTop - log.clientHeight < 24; };
function scrollDown(force) { if (force) stick = true; if (stick) log.scrollTop = log.scrollHeight; }

function bubble(id, cls) {
  let el = id && document.getElementById(id);
  if (!el) {
    el = document.createElement('div'); if (id) el.id = id;
    log.appendChild(el);
  }
  el.className = 'msg ' + cls;
  return el;
}
function put(id, cls, body, note) {
  const el = bubble(id, cls);
  el.textContent = body;
  if (note) { const s = document.createElement('small'); s.textContent = note; el.appendChild(s); }
  scrollDown();
  showPanes();
  return el;
}
function render(history) {
  for (const e of history) {
    if (cfg.cleared.includes('q-' + e.id) || cfg.cleared.includes('a-' + e.id)) continue;
    const where = e.character ? ' · ' + e.character : '';
    put('q-' + e.id, 'q', e.question, e.source === 'game' ? 'asked in game' + where : null);
    const body = e.status === 'waiting' ? 'Waiting for an answer…' : e.answer;
    const note = e.source === 'game' ? 'answered into Answers.lua too — /reload in game' : '';
    put('a-' + e.id, 'a' + (e.status === 'error' ? ' err' : ''), body, note);
  }
}

async function load() {
  const r = await api('/api/state');
  if (!r.ok) throw new Error('HTTP ' + r.status);
  state = await r.json();
  if (state.digest && state.digest.enabled !== (cfg.autoDigest !== false)) pushDigest(); // the saved choice wins over the sidecar's default
  pushSettings(); // the saved choices win over the sidecar's defaults
  setChars();
  showBadge();
  render(state.history);
}

// Streaming text is painted at most once per frame, so a fast stream stays smooth.
function ask(question) {
  busy = true; send.disabled = true;
  scrollDown(true);
  put(null, 'q', question);
  const a = put(null, 'a live', '…');
  let got = '', queued = false;
  const paint = () => { queued = false; a.textContent = got; scrollDown(); };
  const stop = () => { a.classList.remove('live'); busy = false; send.disabled = false; text.focus(); };
  api('/api/ask', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ character: who.value, text: question }) })
    .then(r => r.ok ? r.json() : r.text().then(t => { throw new Error(t); }))
    .then(({ id }) => {
      const es = sse('/api/stream/' + id);
      es.addEventListener('chunk', ev => { got += JSON.parse(ev.data).text; if (!queued) { queued = true; requestAnimationFrame(paint); } });
      es.addEventListener('done', ev => {
        const e = JSON.parse(ev.data);
        es.close(); a.textContent = e.answer; scrollDown();
        if (e.status === 'error') a.classList.add('err');
        if (e.usage) { const s = document.createElement('small'); s.textContent = e.usage; a.appendChild(s); }
        stop();
      });
      es.onerror = () => { es.close(); if (busy) { a.textContent = got || 'The connection dropped.'; stop(); } };
    })
    .catch(err => { a.textContent = err.message; a.classList.add('err'); stop(); });
}
$('form').onsubmit = ev => { ev.preventDefault(); const q = text.value.trim(); if (q && !busy && conn) { text.value = ''; ask(q); } };
text.onkeydown = ev => { if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); $('form').requestSubmit(); } };
setInterval(() => showBadge(), 30000);

// --- version line and update notice ---------------------------------------------------------
// "WoW Companion 0.2.0 · advisor 0.2.0", plus "· v0.3.0 available" when the sidecar's one
// unauthenticated release lookup (asked on start, then every 12 hours, only when "Check for
// updates" is on) finds a newer release. "available" opens the release page on click; nothing is
// downloaded or installed. A failed lookup shows nothing.
const RELEASE_PAGE = 'https://github.com/WoW-Companion/advisor/releases/latest';
let appVersion = '', latest = '';
function versionText(appV, advisorV) {
  return 'WoW Companion ' + appV + (advisorV ? ' \u00b7 advisor ' + advisorV : '');
}
function showVersion() {
  const el = $('version');
  el.textContent = versionText(appVersion, conn && conn.version);
  if (!latest) return;
  el.append(' \u00b7 ' + latest + ' ');
  const a = document.createElement('a');
  a.href = '#'; a.textContent = 'available';
  a.onclick = ev => { ev.preventDefault(); invoke('open_install_page', { url: RELEASE_PAGE }).catch(e => notice(String(e))); };
  el.append(a);
}
async function checkUpdates() {
  if (!cfg.checkUpdates || !conn) return;
  try {
    const r = await api('/api/update');
    const n = r.ok ? await r.json() : {};
    latest = n.available ? n.latest : '';
  } catch { latest = ''; }
  showVersion();
}
$('check-updates').checked = cfg.checkUpdates;
$('check-updates').onchange = ev => { cfg.checkUpdates = ev.target.checked; save(); if (!cfg.checkUpdates) { latest = ''; showVersion(); } else checkUpdates(); };
setInterval(checkUpdates, 12 * 60 * 60 * 1000);
window.__TAURI__.app.getVersion().then(v => { appVersion = v; showVersion(); });

// --- start ---------------------------------------------------------------------------------

(async () => {
  setPhase('starting');
  showPanes();
  $('solid-hover').checked = cfg.solidHover;
  setOpacity(cfg.opacity);
  showMode(); showCt();
  await invoke('set_mode', { overlay: cfg.mode === 'overlay' });
  await applyShortcuts();
  agentChoice = await invoke('get_agent');
  showAgent();
  // The sidecar may have said hello before this page loaded.
  // The sidecar's "sidecar-up" may have landed while this query was in flight; then the phase is
  // no longer `starting` and this older answer must not touch it.
  const st = await invoke('get_status');
  if (phase !== 'starting') return;
  if (st.conn) connect(st.conn);
  else if (st.error) setPhase('stopped', 'The advisor stopped. ' + st.error);
})();

// --- Setup checklist -----------------------------------------------------------------------
// The sidecar does every check and the addon install (Go); this only shows the rows. It opens on
// first run and whenever a row is red. The one write is the addon install, on the player's click.

let setupItems = [];
const gameQ = () => cfg.gameDir ? '?game=' + encodeURIComponent(cfg.gameDir) : '';
const MARK = { ok: '\u2713', warn: '\u26A0', bad: '\u2717' };

function showSetup() {
  const list = $('setup-list');
  list.replaceChildren();
  for (const it of setupItems) {
    if (it.optional && cfg.setupHidden.includes(it.id)) continue;
    const li = document.createElement('li');
    const mark = document.createElement('span');
    mark.className = 'mark ' + it.status; mark.textContent = MARK[it.status] || '?';
    mark.setAttribute('role', 'img'); mark.setAttribute('aria-label', it.status);
    const title = document.createElement('b'); title.textContent = it.title;
    const acts = document.createElement('span'); acts.className = 'acts';
    if (it.action) {
      const b = document.createElement('button'); b.type = 'button'; b.textContent = it.label || it.action;
      b.onclick = () => setupAct(it, b);
      acts.append(b);
    }
    if (it.optional && it.status !== 'ok') {
      const h = document.createElement('button'); h.type = 'button'; h.textContent = "Don't show again";
      h.onclick = () => { cfg.setupHidden = [...cfg.setupHidden, it.id]; save(); showSetup(); };
      acts.append(h);
    }
    const detail = document.createElement('span'); detail.className = 'detail'; detail.textContent = it.detail;
    li.append(mark, title, acts, detail);
    list.append(li);
  }
}

async function loadSetup(auto) {
  if (!conn) return;
  const c = conn;
  try {
    const r = await api('/api/setup' + gameQ());
    if (!r.ok || conn !== c) return;
    setupItems = (await r.json()).items || [];
  } catch (e) { return; }
  showSetup();
  const red = setupItems.some(it => it.status === 'bad' && !(it.optional && cfg.setupHidden.includes(it.id)));
  if (auto && (red || !cfg.setupSeen)) { $('setup').hidden = false; }
  if (auto) { cfg.setupSeen = true; save(); }
}

function setupMsg(t) { $('setup-msg').textContent = t || ''; }

async function setupAct(it, btn) {
  setupMsg('');
  if (it.action === 'choose-folder') { $('setup-folder').hidden = false; $('setup-folder-path').focus(); return; }
  if (it.action === 'open-url') { invoke('open_install_page', { url: it.url }).catch(e => setupMsg(String(e))); return; }
  btn.disabled = true;
  try {
    if (it.action === 'install-addon' || it.action === 'update-addon') {
      setupMsg('Downloading and verifying the addon…');
      const r = await api('/api/setup/install-addon', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ game: cfg.gameDir }) });
      const j = await r.json();
      if (j.items) setupItems = j.items;
      setupMsg(j.error || j.message);
    } else if (it.action === 'test-ai') {
      setupMsg('Asking your AI client…');
      const r = await api('/api/setup/test-ai', { method: 'POST' });
      const j = await r.json();
      setupMsg((j.ok ? '\u2713 ' : '\u2717 ') + j.message);
    }
  } catch (e) { setupMsg('Could not reach the advisor: ' + e.message); }
  btn.disabled = false;
  showSetup();
}

$('setup-folder').onsubmit = ev => {
  ev.preventDefault();
  cfg.gameDir = $('setup-folder-path').value.trim(); save();
  $('setup-folder').hidden = true;
  loadSetup(false);
};
$('setup-open').onclick = () => { $('setup').hidden = false; loadSetup(false); };
$('setup-close').onclick = () => { $('setup').hidden = true; };
