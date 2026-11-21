// The overlay's page. It holds no game logic: it asks the Go advisor's companion API (the sidecar)
// and shows what comes back. The window itself is driven through the Rust shell's commands.
const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;
const win = window.__TAURI__.window.getCurrentWindow();

const $ = id => document.getElementById(id);
const log = $('log'), who = $('who'), badge = $('badge'), text = $('text'), send = $('send');
const DEFAULTS = { opacity: 92, solidHover: false, mode: 'overlay', keyVis: 'Ctrl+Shift+W', keyCt: 'Ctrl+Shift+T', nextOpen: true, who: '' };
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
//   ready*           -> stopped          the sidecar exited, or the first data call failed
//   stopped / ready* -> starting         Reconnect, Re-scan or a new AI client choice restarts it
//
// "Starting" ends with the handshake, not later. The handshake and the page's own start-up
// status query race, so only a state that is still `starting` may be set from a start-up answer
// (see the start block). The red banner is for starting (with Reconnect) and stopped only.
let phase = 'starting';
function setPhase(next, msg) {
  phase = next;
  const bar = next === 'starting' || next === 'stopped';
  $('banner').hidden = !bar;
  $('banner-text').textContent = bar ? msg || (next === 'starting' ? 'Starting the advisor…' : 'The advisor stopped.') : '';
  $('aibar').hidden = next !== 'ready-no-agent';
}
function notice(msg) {
  const n = $('notice');
  n.textContent = msg; n.hidden = false;
  clearTimeout(notice.t);
  notice.t = setTimeout(() => { n.hidden = true; }, 6000);
}

async function connect(c) {
  conn = c;
  if (events) events.close();
  setPhase(c.agent ? 'ready' : 'ready-no-agent');
  showAgent();
  try {
    await load();
  } catch (e) {
    if (conn !== c) return; // a newer run took over
    return setPhase('stopped', 'The advisor did not answer. ' + e.message);
  }
  events = sse('/api/events');
  events.addEventListener('game', ev => { state = JSON.parse(ev.data); setChars(); showBadge(true); });
  events.addEventListener('history', ev => render(JSON.parse(ev.data).filter(e => e.source === 'game')));
  events.onerror = () => {
    // The stream drops when the sidecar dies; the shell's "sidecar-down" explains why.
    events.close();
    api('/api/state').catch(() => setPhase('stopped', 'Lost the advisor. Reconnect to start it again.'));
  };
}

listen('sidecar-up', ev => connect(ev.payload));
listen('sidecar-down', ev => { conn = null; showAgent(); if (events) events.close(); setPhase('stopped', 'The advisor stopped.' + (ev.payload ? ' ' + ev.payload : '')); });
listen('notice', ev => notice(ev.payload));
listen('mode', ev => { cfg.mode = ev.payload; save(); showMode(); });
listen('clickthrough', ev => { $('ct').hidden = !ev.payload; showCt(); });
$('retry').onclick = () => { setPhase('starting'); invoke('restart_sidecar'); };

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
  setPhase('starting', 'Restarting the advisor…');
  try { await invoke('set_agent', { agent: value }); } catch (e) { setPhase('stopped', 'Could not save the AI client: ' + e); }
}
$('agent').onchange = ev => {
  if (ev.target.value === 'custom') { agentChoice = $('agent-cmd').value.trim() || 'custom'; showAgent(); $('agent-cmd').focus(); }
  else chooseAgent(ev.target.value);
};
// The sidecar looks for AI clients when it starts, so looking again is a restart.
$('agent-rescan').onclick = () => { setPhase('starting', 'Looking for AI clients…'); invoke('restart_sidecar'); };
$('agent-cmd').onchange = ev => { if (ev.target.value.trim()) chooseAgent(ev.target.value.trim()); };
for (const a of document.querySelectorAll('#aibar a')) {
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
$('settings-btn').onclick = () => { $('settings').hidden = !$('settings').hidden; };

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

function ago(t) {
  if (!t) return 'no capture yet';
  const s = Math.max(0, Date.now() / 1000 - t);
  const when = new Date(t * 1000).toLocaleTimeString();
  const age = s < 90 ? 'just now' : s < 5400 ? Math.round(s / 60) + ' min ago' : Math.round(s / 3600) + ' h ago';
  return when + ' (' + age + ')';
}
function showBadge(fresh) {
  if (!state) return;
  badge.textContent = 'data from game: ' + ago(state.captured_at) + (state.error ? ' — ' + state.error : '');
  badge.className = fresh ? 'fresh' : '';
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
  $('next-list').replaceChildren(...(items.length ? items : ['Nothing queued yet.']).map(t => {
    const li = document.createElement('li');
    li.textContent = t;
    if (!items.length) li.className = 'none';
    return li;
  }));
}
$('next').ontoggle = ev => { cfg.nextOpen = ev.target.open; save(); };

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
  log.scrollTop = log.scrollHeight;
  return el;
}
function render(history) {
  for (const e of history) {
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
  setChars();
  showBadge(false);
  render(state.history);
}

// Streaming text is painted at most once per frame, so a fast stream stays smooth.
function ask(question) {
  busy = true; send.disabled = true;
  put(null, 'q', question);
  const a = put(null, 'a live', '…');
  let got = '', queued = false;
  const paint = () => { queued = false; a.textContent = got; log.scrollTop = log.scrollHeight; };
  const stop = () => { a.classList.remove('live'); busy = false; send.disabled = false; text.focus(); };
  api('/api/ask', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ character: who.value, text: question }) })
    .then(r => r.ok ? r.json() : r.text().then(t => { throw new Error(t); }))
    .then(({ id }) => {
      const es = sse('/api/stream/' + id);
      es.addEventListener('chunk', ev => { got += JSON.parse(ev.data).text; if (!queued) { queued = true; requestAnimationFrame(paint); } });
      es.addEventListener('done', ev => {
        const e = JSON.parse(ev.data);
        es.close(); a.textContent = e.answer;
        if (e.status === 'error') a.classList.add('err');
        stop();
      });
      es.onerror = () => { es.close(); if (busy) { a.textContent = got || 'The connection dropped.'; stop(); } };
    })
    .catch(err => { a.textContent = err.message; a.classList.add('err'); stop(); });
}
$('form').onsubmit = ev => { ev.preventDefault(); const q = text.value.trim(); if (q && !busy && conn) { text.value = ''; ask(q); } };
text.onkeydown = ev => { if (ev.key === 'Enter' && !ev.shiftKey) { ev.preventDefault(); $('form').requestSubmit(); } };
setInterval(() => showBadge(false), 30000);

// --- start ---------------------------------------------------------------------------------

(async () => {
  setPhase('starting');
  $('next').open = cfg.nextOpen;
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
