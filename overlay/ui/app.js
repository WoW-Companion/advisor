// The overlay's page. It holds no game logic: it asks the Go advisor's companion API (the sidecar)
// and shows what comes back. The window itself is driven through the Rust shell's commands.
const { invoke } = window.__TAURI__.core;
const { listen } = window.__TAURI__.event;
const win = window.__TAURI__.window.getCurrentWindow();

const $ = id => document.getElementById(id);
const log = $('log'), who = $('who'), badge = $('badge'), text = $('text'), send = $('send');
const DEFAULTS = { schema: 1, opacity: 92, solidHover: false, mode: 'overlay', keyVis: 'Ctrl+Shift+W', keyCt: 'Ctrl+Shift+T', nextOpen: null, settingsOpen: false, chatOpen: true, size: 'M', checkUpdates: true, updateNow: false, autoUpdate: false, skipped: [], later: {}, cleared: [], who: '', setupSeen: false, setupHidden: [], gameDir: '', length: 'standard', byClient: {}, usage: true, follow: true, pinned: '', groups: { ai: true }, quickBar: true, dismissedSugg: {}, notesSeen: { app: '', addon: '' }, notesSeedFixed: false };
// nextOpen null means automatic: open until the chat has a message, then collapsed. A click makes it a choice.
const cfg = { ...DEFAULTS, ...JSON.parse(localStorage.getItem('wowc') || '{}') };
cfg.settingsOpen = false; // the settings open from the gear for this session only; a launch starts calm
const save = () => { localStorage.setItem('wowc', JSON.stringify(cfg)); if (typeof syncAuto === 'function') syncAuto(); };
const chats = WowcLaunch.chatStore(localStorage); // each character's last 50 turns, on this PC only

let conn = null, state = null, busy = false, events = null;
const upd = { status: null, addon: null, retry: null, addonAutoFailed: '', busy: '', failure: '', last: '', fresh: false, ok: false, checkedAt: 0, confirm: '', notesFor: '' }; // the update banner's state

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
  conn = null; showAgent(); versionLine.onDisconnect();
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
  versionLine.onConnect();
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
  events.addEventListener('profile', ev => { state = JSON.parse(ev.data); setChars(); showStyle(); }); // a play style changed (the AI client saved it)
  events.addEventListener('reask', onReask);
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
listen('sidecar-down', ev => { conn = null; showAgent(); versionLine.onDisconnect(); if (events) events.close(); clearTimeout(restartTimer); setPhase('stopped', String(ev.payload || '').startsWith('The planner stopped') ? ev.payload : 'The advisor stopped.' + (ev.payload ? ' ' + ev.payload : '')); });
listen('notice', ev => notice(ev.payload));
listen('mode', ev => { cfg.mode = ev.payload; save(); showMode(); });
listen('clickthrough', ev => { $('ct').hidden = !ev.payload; showCt(); });
$('retry').onclick = () => restart('Reconnecting…');

// --- AI client -----------------------------------------------------------------------------
// The choice is kept by the shell and passed to the sidecar as --agent: "" (no choice), "auto",
// a preset name or a custom command. Changing it restarts the sidecar.

const PRESETS = ['claude', 'codex', 'agy', 'gemini'];
// The one place client ids become the names people know; anything else shows as given.
const CLIENT_NAMES = { claude: 'Claude Code', codex: 'Codex', agy: 'Antigravity', gemini: 'Gemini CLI' };
const clientName = id => CLIENT_NAMES[id] || id;
let agentChoice = '';

function showAgent() {
  const known = agentChoice === '' || agentChoice === 'auto' || PRESETS.includes(agentChoice);
  $('agent').value = known ? (agentChoice || 'auto') : 'custom';
  // The command box is for Custom only.
  $('agent-cmd-row').hidden = $('agent').value !== 'custom';
  if (!known) $('agent-cmd').value = agentChoice;
  const auto = $('agent').options[0], found = conn && conn.agent_source === 'auto' && conn.agent;
  auto.textContent = 'Auto' + (found ? ' (' + clientName(conn.agent) + ')' : conn && !conn.agent ? ' (none found)' : '');
  // The picker's tooltip names what is in use and where it was found.
  $('agent').title = !conn ? '' : !conn.agent ? 'No AI client found. Install one, then Re-scan.'
    : 'Using ' + clientName(conn.agent) + (conn.agent_path ? ' at ' + conn.agent_path : ' (' + conn.agent_source + ')');
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
// Settings -> AI: the question, the key, the model and Test live in aiset.js; this hands it the page's pieces.
const aiJson = async (path, body) => {
  try {
    const r = await api(path, body === undefined ? {} : { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
    return r.ok ? await r.json() : { ok: false, message: (await r.text()).trim() };
  } catch (e) { return { ok: false, message: 'connection: ' + e }; }
};
const aiEls = {};
for (const [k, id] of Object.entries({ mode: 'ai-mode', cliRows: 'ai-cli', apiRows: 'ai-api', provider: 'ai-provider', baseRow: 'ai-base-row', base: 'ai-base',
  localRow: 'ai-local', localStatus: 'ai-local-status', localLink: 'ai-local-link', rescan: 'ai-rescan', keyRow: 'ai-key-row', keyBox: 'ai-key-box', keyInput: 'ai-key',
  keySave: 'ai-key-save', keyCancel: 'ai-key-cancel', keySaved: 'ai-key-saved', keyStatus: 'ai-key-status', keyReplace: 'ai-key-replace', keyRemove: 'ai-key-remove',
  keyLink: 'ai-key-link', model: 'ai-model', modelName: 'ai-model-name', modelOtherRow: 'ai-model-other-row', modelOther: 'ai-model-other', note: 'ai-note',
  costRows: 'ai-cost', cost: 'ai-cost-line', subscription: 'ai-sub-line', test: 'ai-test', testOut: 'ai-test-out', advanced: 'ai-advanced', backup: 'ai-backup' })) aiEls[k] = $(id);
const aiSettings = WowcAiSettings.create(aiEls, {
  call: aiJson,
  openPage: url => invoke('open_install_page', { url }).catch(e => notice(String(e))),
  option: o => Object.assign(document.createElement('option'), o),
  agent: () => (conn && conn.agent) || '',
  onChange: () => {},
});
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
let chatScroll = 0; // the chat's scroll offset while the settings page covers it
function showPanes() {
  const wasOpen = !$('settings').hidden;
  if (cfg.settingsOpen && !wasOpen) chatScroll = log.scrollTop; // the chat keeps its place underneath
  $('settings').hidden = !cfg.settingsOpen;
  $('panel').classList.toggle('settings-open', cfg.settingsOpen);
  const gear = $('settings-btn');
  gear.setAttribute('aria-expanded', String(cfg.settingsOpen));
  gear.setAttribute('aria-pressed', String(cfg.settingsOpen));
  gear.title = cfg.settingsOpen ? 'Close settings' : 'Settings';
  gear.setAttribute('aria-label', gear.title);
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
  if (!cfg.settingsOpen && wasOpen) log.scrollTop = chatScroll;
  renderUpdates();
}
const nextOpen = () => cfg.nextOpen === null ? !log.children.length : cfg.nextOpen;
// Settings groups: the AI group starts open, the rest closed; each remembers what the user chose.
for (const d of document.querySelectorAll('details[data-group]')) {
  const g = d.dataset.group;
  d.open = g in cfg.groups ? !!cfg.groups[g] : g === 'ai';
  d.ontoggle = () => { cfg.groups[g] = d.open; save(); };
}
const setSettings = on => { cfg.settingsOpen = on; save(); showPanes(); if (on) $('settings').scrollTop = 0; };
$('settings-btn').onclick = () => setSettings(!cfg.settingsOpen);
$('settings-done').onclick = () => setSettings(false);
document.addEventListener('keydown', ev => { if (ev.key === 'Escape' && cfg.settingsOpen && !ev.target.classList.contains('key')) setSettings(false); });
$('next-head').onclick = () => { cfg.nextOpen = !nextOpen(); save(); showPanes(); };
$('chat-toggle').onclick = () => { cfg.chatOpen = !cfg.chatOpen; save(); showPanes(); if (cfg.chatOpen) scrollDown(true); };
$('size').onchange = ev => { cfg.size = ev.target.value; save(); showPanes(); };
$('clear').onclick = () => {
  cfg.cleared = [...new Set([...cfg.cleared, ...[...log.children].map(e => e.id).filter(Boolean)])].slice(-500);
  chats.clear(who.value);
  clearFollow(); newAnswer.hidden = true;
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
$('opacity').oninput = ev => { setOpacity(+ev.target.value); };

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
    if (ev.key === 'Tab') return; // Tab and Shift+Tab move focus as usual
    ev.preventDefault();
    if (ev.key === 'Escape') { el.blur(); return; } // leave the box, unchanged
    if ((ev.key === 'Backspace' || ev.key === 'Delete') && !(ev.ctrlKey || ev.altKey || ev.metaKey || ev.shiftKey)) {
      el.value = cfg[field] = DEFAULTS[field]; save();
      applyShortcuts();
      return;
    }
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
  $('follow').checked = cfg.follow !== false;
  $('auto-digest-note').textContent = d && d.status === 'addon not found' ? 'addon not found' : d && d.status === 'error' ? d.error : '';
}
// Answer length, model, reasoning level and the usage line are pushed to the advisor, which applies
// them to the next ask; no restart. The model and reasoning choice is kept for each AI client
// (cfg.byClient), so switching clients restores that client's last choice. The advisor says what each
// client offers (state.settings.choices); a client with no such option gets the dropdown disabled,
// and its tooltip says why. The defaults are the smallest model and the lowest reasoning level.
const clientKey = () => (state && state.settings && state.settings.client) || 'custom';
function clientChoice() {
  const ch = (state && state.settings && state.settings.choices) || { models: [], reasoning: [] };
  const c = { model: 'efficient', custom: '', reasoning: 'low', ...cfg.byClient[clientKey()] };
  if (!ch.models.some(m => m.tier === c.model)) c.model = 'efficient';
  if (!ch.reasoning.includes(c.reasoning)) c.reasoning = 'low';
  return { c, ch };
}
function fill(sel, items, value, note, none) {
  sel.replaceChildren(...items.map(([v, t]) => Object.assign(document.createElement('option'), { value: v, textContent: t })));
  if (!items.length) sel.append(Object.assign(document.createElement('option'), { textContent: none }));
  sel.value = value; sel.disabled = !items.length; sel.title = note;
}
function showAnswerSettings() {
  const { c, ch } = clientChoice();
  $('length').value = cfg.length; $('usage').checked = cfg.usage !== false;
  fill($('model'), ch.models.map(m => [m.tier, m.label]), c.model, ch.model_note || '', 'Own model');
  fill($('reasoning'), ch.reasoning.map(r => [r, r[0].toUpperCase() + r.slice(1)]), c.reasoning, ch.reasoning_note || '', 'Own default');
  $('model-custom').value = c.custom;
  $('model-custom-row').hidden = c.model !== 'custom';
}
function setClientChoice(patch) {
  cfg.byClient[clientKey()] = { ...clientChoice().c, ...patch }; save(); pushSettings();
}
async function pushSettings() {
  showAnswerSettings();
  const { c } = clientChoice();
  try { await api('/api/settings', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ length: cfg.length, model: c.model, custom_model: c.custom, reasoning: c.reasoning, usage: cfg.usage !== false }) }); } catch (e) { /* the next state load pushes again */ }
}
$('length').onchange = ev => { cfg.length = ev.target.value; save(); pushSettings(); };
$('model').onchange = ev => { setClientChoice({ model: ev.target.value }); if (ev.target.value === 'custom') $('model-custom').focus(); };
$('reasoning').onchange = ev => setClientChoice({ reasoning: ev.target.value });
$('model-custom').onchange = ev => setClientChoice({ custom: ev.target.value.trim() });
$('usage').onchange = ev => { cfg.usage = ev.target.checked; save(); pushSettings(); };
async function pushDigest() {
  try { await api('/api/auto-digest', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ on: cfg.autoDigest !== false }) }); } catch (e) { /* shown by the next state load */ }
}
function showBadge() {
  if (!state) return;
  $('data-note').textContent = state.data_notice || ''; $('data-note').hidden = !state.data_notice;
  showDigest();
  const f = WowcLaunch.freshness(state.captured_at, Date.now());
  badge.className = 'dot' + (f.cls ? ' ' + f.cls : '');
  badge.title = f.tip + (state.error ? '\n' + state.error : '') + digestNote();
}
function setChars() {
  who.replaceChildren(...state.characters.map(c => {
    const o = document.createElement('option');
    o.value = c.key; o.textContent = c.name + (c.level ? ' (' + c.level + ' ' + c.class + ')' : '');
    return o;
  }));
  const before = who.value || cfg.who;
  const pick = WowcLaunch.followChoice({ characters: state.characters, newest: state.default, current: before, follow: cfg.follow !== false, pinned: cfg.pinned });
  who.value = pick.key;
  cfg.who = pick.key;
  const note = $('follow-note');
  note.textContent = pick.line; note.hidden = !pick.line;
  showNext();
  if (pick.switched) { save(); if (!busy) restoreChat(); }
}
// Choosing a character by hand pins it; choosing the one being played, or turning the setting back
// on, lets the page follow again.
who.onchange = () => { cfg.who = who.value; cfg.pinned = who.value === (state && state.default) ? '' : who.value; $('follow-note').hidden = true; save(); showNext(); if (!busy) restoreChat(); };
$('follow').onchange = ev => { cfg.follow = ev.target.checked; if (cfg.follow) cfg.pinned = ''; save(); if (state) setChars(); };

// Play style: the chosen character's role, focus and spec, saved locally by the advisor. Both role
// and focus may each be "Not set" (both: the style is cleared); the spec is a dropdown of the class's specs.
function showStyle() {
  const c = state && state.characters.find(c => c.key === who.value), st = (c && c.style) || {};
  for (const card of document.querySelectorAll('.style-card')) if (card.dataset.character === who.value && card.setStyle) card.setStyle(st);
  $('style-role').value = st.role || ''; $('style-focus').value = st.focus || '';
  const sel = $('style-spec'), specs = (c && c.specs) || [];
  sel.replaceChildren(Object.assign(document.createElement('option'), { value: '', textContent: 'Not sure yet' }));
  for (const sp of specs) sel.append(Object.assign(document.createElement('option'), { value: sp, textContent: sp }));
  sel.value = specs.find(sp => sp.toLowerCase() === String(st.spec || '').toLowerCase()) || ''; // the sidecar already normalised an old typed spec
  $('style-pref').value = st.stat_pref || 'balanced';
  $('seasonal').checked = !!(c && c.seasonal);
}
// saveStyle stores a play style for one character; the settings section and the inline card both use it.
async function saveStyle(character, vals) {
  return (await styleFlow(character, vals)).ok;
}
async function styleFlow(character, vals) {
  return WowcLaunch.saveStyleFlow({
    post: async body => {
      const r = await api('/api/profile', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) });
      if (!r.ok) return { ok: false, text: await r.text() };
      state.characters = await r.json();
      return { ok: true };
    },
    // After every save the profile is read again from the advisor, so the dropdowns show what is saved.
    reread: async () => {
      try { const r = await api('/api/state'); if (r.ok) state = await r.json(); } catch (e) { /* keep what we have */ }
      showNext();
    },
  }, character, vals);
}
let savedTimer = null;
function showSaved(res) {
  const tick = $('style-saved'), err = $('style-error');
  clearTimeout(savedTimer);
  err.hidden = res.ok; err.textContent = res.ok ? '' : res.error;
  tick.hidden = !res.ok; tick.classList.remove('fade');
  if (res.ok) savedTimer = setTimeout(() => { tick.classList.add('fade'); savedTimer = setTimeout(() => { tick.hidden = true; }, 700); }, 1500);
}
async function pushStyle() {
  if (!who.value) return;
  showSaved(await styleFlow(who.value, { role: $('style-role').value, focus: $('style-focus').value, spec: $('style-spec').value, stat_pref: $('style-pref').value }));
}
// Include holiday event quests: opt-in per character; the advisor re-reads the next steps.
$('seasonal').onchange = async ev => {
  if (!who.value) return;
  const on = ev.target.checked;
  try {
    const r = await api('/api/seasonal', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ character: who.value, on }) });
    showSaved(r.ok ? { ok: true } : { ok: false, error: await r.text() });
    if (r.ok) { state.characters = await r.json(); showNext(); }
  } catch (e) { showSaved({ ok: false, error: String(e) }); }
};
for (const id of ['style-role', 'style-focus', 'style-spec', 'style-pref']) $(id).onchange = pushStyle;

function showNext() {
  showStyle();
  const c = state && state.characters.find(c => c.key === who.value);
  const items = (c && c.next) || [];
  // next_lines carries the structured fields; an older advisor sends only the text.
  const lines = (c && c.next_lines && c.next_lines.length) ? c.next_lines : items;
  $('next-line').textContent = items.length ? items[0] : 'Nothing queued yet.';
  const entries = (c && c.entries) || [];
  $('next-list').hidden = entries.length > 0;
  $('entries').replaceChildren(...(entries.length ? [WowcAsking.entryList(document, entries, askEntry)] : []));
  $('next-list').replaceChildren(...(lines.length ? lines : ['Nothing queued yet.']).map(t => {
    if (!lines.length) { const li = document.createElement('li'); li.textContent = t; li.className = 'none'; return li; }
    return WowcNext.render(document, t, c && c.level);
  }));
  // plan_steps carries the lock state; an older advisor sends only the text.
  const plan = (c && c.plan_steps && c.plan_steps.length) ? c.plan_steps : ((c && c.plan) || []);
  $('plan').hidden = !plan.length;
  $('plan-line').textContent = WowcLaunch.planLine(c && c.plan_line);
  $('plan-list').replaceChildren(...plan.map(t => {
    const li = document.createElement('li');
    li.textContent = typeof t === 'string' ? t : t.text;
    if (t && t.x) { // the place, shown dimly at the end
      const xy = document.createElement('span'); xy.className = 'plan-xy'; xy.textContent = ` · ${Math.round(t.x)}, ${Math.round(t.y)}`;
      li.appendChild(xy);
    }
    if (t && t.locked) li.className = 'locked';
    return li;
  }));
  showSuggestions(c);
  showQuick();
}

// --- suggestions row and quick bar ---------------------------------------------------------
// At most three chips on one line, hidden when empty. A chip's × hides it for the rest of today.
function currentChar() { return state && state.characters.find(c => c.key === who.value); }
function showSuggestions(c) {
  const day = WowcLaunch.dayKey(new Date());
  cfg.dismissedSugg = Object.fromEntries(Object.entries(cfg.dismissedSugg || {}).filter(([, d]) => d === day)); // yesterday's are forgotten
  const items = WowcLaunch.visibleSuggestions(c && c.suggestions, cfg.dismissedSugg, who.value, day);
  $('suggs').hidden = !items.length;
  $('suggs').replaceChildren(...(items.length ? [WowcLaunch.suggestionRow(document, items, runSuggestion, s => {
    cfg.dismissedSugg[who.value + '|' + s.id] = day; save(); showSuggestions(currentChar());
  })] : []));
}
function runSuggestion(s) {
  if (s.action === 'ask') askEntry(s.question, s.ref);
  else if (s.action === 'play_style') showStyleCard();
  else if (s.action === 'refresh_plan') refreshPlan();
  else if (s.action === 'open_entry') openEntry(s.ref);
}
// open_entry: scroll the Next list to the entry and open its question chips.
function openEntry(ref) {
  cfg.nextOpen = true; save(); showPanes();
  const c = currentChar(), i = ((c && c.entries) || []).findIndex(e => e.ref === ref);
  const row = i >= 0 && $('entries').querySelectorAll('.entry-row')[i];
  if (row && !row.parentNode.className.includes(' open')) row.click();
  if (row && row.scrollIntoView) row.scrollIntoView({ block: 'nearest' });
}
// The play-style card in the chat, saved with the same store as the settings section; no question waits on it.
function showStyleCard() {
  const c = currentChar();
  if (!c) return;
  const a = put(null, 'a', WowcAsking.NEEDS_LINE);
  const card = WowcAsking.needsCard(document, WowcLaunch.styleNeeds(), async style => {
    if (await saveStyle(c.key, style)) a.remove(); else a.append(Object.assign(document.createElement('small'), { textContent: 'Could not save that. Try again.' }));
  }, c.style);
  card.dataset.character = c.key;
  a.append(card);
  scrollDown(true);
}
// Refresh: the advisor rebuilds the plan from the newest save; no AI is asked. One sentence comes back.
async function refreshPlan() {
  if (!conn) return;
  try {
    const r = await api('/api/refresh-plan', { method: 'POST' });
    if (!r.ok) { toast((await r.text()).trim() || 'Could not update the plan.'); return; }
    toast((await r.json()).message || 'Plan updated.');
    const r2 = await api('/api/state');
    if (r2.ok) { state = await r2.json(); setChars(); showBadge(); }
  } catch (e) { toast('Could not reach the advisor.'); }
}
function quickBar() {
  const slot = $('quick-slot');
  slot.replaceChildren(WowcLaunch.quickBar(document, id => {
    const act = WowcLaunch.quickAction(id, currentChar());
    if (!act) return;
    if (act.act === 'ask') askEntry(act.question, act.ref);
    else if (act.act === 'play_style') showStyleCard();
    else if (act.act === 'refresh') refreshPlan();
  }));
}
function showQuick() { $('quick-slot').hidden = cfg.quickBar === false; }
quickBar();
$('quick-on').checked = cfg.quickBar !== false;
$('quick-on').onchange = ev => { cfg.quickBar = ev.target.checked; save(); showQuick(); };

// A chip: the player's short question goes out with the entry's ref; nothing else shows in the chat.
function askEntry(question, ref) { if (!busy && conn) ask(question, ref); }

// Follow the newest text unless the reader has scrolled up.
let stick = true;
const newAnswer = $('new-answer');
log.onscroll = () => { stick = WowcFollow.onScroll(log, newAnswer); };
newAnswer.onclick = () => { WowcFollow.pillClick(log, newAnswer); stick = true; };

// The follow-up chip: what the next question will be understood against. The server holds the card
// and drops it after 30 minutes; the chip is the player's view of it and its × clears it.
let followCtx = null;
const nowS = () => Math.floor(Date.now() / 1000);
function showFollow() { WowcFollow.show(document, $('follow'), followCtx, who.value, nowS(), clearFollow); }
function clearFollow() {
  followCtx = null; showFollow();
  if (conn) api('/api/context/clear', { method: 'POST' }).catch(() => { /* the card expires on its own */ });
}
setInterval(showFollow, 60000);
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
// A game entry's character may be a name or a key; the page keys everything by the key.
const keyOf = c => { const k = state && state.characters.find(x => x.key === c || x.name === c); return k ? k.key : c; };
// restoreChat: the character's saved chat, scrolled to the bottom, with a dim divider for the gap.
// It shows what was said; it never sends anything.
function restoreChat() {
  log.replaceChildren();
  const turns = chats.load(who.value);
  for (const t of turns) {
    put('q-' + t.id, 'q', WowcAsking.shortQuestion(t.q), null);
    put('a-' + t.id, 'a', t.a);
  }
  if (turns.length) log.appendChild(WowcLaunch.gapDivider(document, turns[turns.length - 1].at, Date.now()));
  if (state) render(state.history);
  followCtx = state && state.context || null; showFollow();
  showPanes(); scrollDown(true); newAnswer.hidden = true;
}
function render(history) {
  for (const e of history) {
    if (who.value && keyOf(e.character) !== who.value) continue; // each character has its own chat
    if (cfg.cleared.includes('q-' + e.id) || cfg.cleared.includes('a-' + e.id)) continue;
    put('q-' + e.id, 'q', WowcAsking.shortQuestion(e.question), null);
    const body = e.status === 'waiting' ? 'Waiting for an answer…' : (e.status === 'answering' && !e.answer ? 'Answering…' : e.answer);
    put('a-' + e.id, 'a' + (e.status === 'error' ? ' err' : e.status === 'answering' ? ' live' : ''), body, null);
  }
}
function toast(msg) {
  const el = document.createElement('div'); el.className = 'toast'; el.textContent = msg;
  document.body.appendChild(el);
  setTimeout(() => el.remove(), 4000);
}

async function load() {
  const r = await api('/api/state');
  if (!r.ok) throw new Error('HTTP ' + r.status);
  state = await r.json();
  if (state.digest && state.digest.enabled !== (cfg.autoDigest !== false)) pushDigest(); // the saved choice wins over the sidecar's default
  pushSettings(); // the saved choices win over the sidecar's defaults
  aiSettings.refresh();
  setChars();
  showBadge();
  showWhatsNew();
  restoreChat(); // the saved chat first, then the game's asks; nothing is sent
}

// Streaming text is painted at most once per frame, so a fast stream stays smooth.
function ask(question, ref, icy) {
  busy = true; send.disabled = true; reportActivity();
  scrollDown(true);
  put(null, 'q', question);
  const a = put(null, 'a live', WowcAsking.workingLine(question, ref));
  a.wowc = { q: question, char: who.value };
  const body = { character: who.value, text: question };
  if (ref) body.ref = ref;
  if (icy) body.icy = true;
  api('/api/ask', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) })
    .then(r => r.ok ? r.json() : r.text().then(t => { throw new Error(t); }))
    .then(({ id }) => follow(id, a))
    .catch(err => { a.textContent = err.message; a.classList.add('err'); settle(); });
}
function settle() { busy = false; send.disabled = false; text.focus(); reportActivity(); }
// follow streams one answer into bubble a. A "needs" answer turns the bubble into the play-style card.
function follow(id, a) {
  busy = true; send.disabled = true; reportActivity();
  let got = '', queued = false;
  const paint = () => { queued = false; a.textContent = got; scrollDown(); };
  const stop = () => { a.classList.remove('live'); settle(); };
  const es = sse('/api/stream/' + id);
  es.addEventListener('chunk', ev => { got += JSON.parse(ev.data).text; if (!queued) { queued = true; requestAnimationFrame(paint); } });
  es.addEventListener('done', ev => {
    const e = JSON.parse(ev.data);
    es.close();
    if (e.status === 'needs' && e.needs) { showNeeds(a, e.needs); stop(); return; }
    a.textContent = e.answer;
    if (e.status === 'done' && a.wowc && e.answer) chats.add(a.wowc.char, { id: 't' + Date.now().toString(36) + Math.random().toString(36).slice(2, 6), q: a.wowc.q, a: e.answer, at: Math.floor(Date.now() / 1000) });
    if (e.status === 'error') a.classList.add('err');
    for (const t of [e.usage, e.notice]) if (t) { const s = document.createElement('small'); s.textContent = t; if (t === e.usage && e.usage_tip) s.title = e.usage_tip; a.appendChild(s); }
    for (const el of WowcAsking.sourceLine(document, e, () => { if (!busy && conn && a.wowc) ask(a.wowc.q, '', true); })) a.appendChild(el);
    if (e.status === 'done' && a.wowc) { const tc = WowcAsking.timeChips(document, a.wowc.q, q => { if (!busy && conn) ask(q, ''); }); if (tc) a.appendChild(tc); }
    if (e.status === 'done') { followCtx = e.context || null; showFollow(); }
    // Last, once the text, usage and notice lines and the chip are all in: the whole answer comes into view.
    WowcFollow.answerDone(log, stick, requestAnimationFrame, newAnswer);
    stop();
  });
  es.onerror = () => { es.close(); if (busy) { a.textContent = got || 'The connection dropped.'; stop(); } };
}
// The play-style card: saving stores the style (the settings section's store) and the server asks the
// original question again, which arrives as a "reask" event into a new bubble.
function showNeeds(a, needs) {
  const c = state && state.characters.find(c => c.key === needs.character);
  const spec = needs.needs === 'spec';
  a.textContent = spec ? 'Which spec are you playing?' : WowcAsking.NEEDS_LINE;
  a.classList.remove('live');
  const card = (spec ? WowcAsking.specCard : WowcAsking.needsCard)(document, needs, async style => {
    reaskBubble = a;
    if (!(await saveStyle(needs.character, style))) { reaskBubble = null; a.append(Object.assign(document.createElement('small'), { textContent: 'Could not save that. Try again.' })); }
  }, c && c.style);
  card.dataset.character = needs.character;
  a.append(card);
  scrollDown();
}
let reaskBubble = null;
function onReask(ev) {
  const { id } = JSON.parse(ev.data);
  const a = put(null, 'a live', WowcAsking.workingLine(reaskBubble && reaskBubble.wowc && reaskBubble.wowc.q, ''));
  if (reaskBubble) a.wowc = reaskBubble.wowc;
  if (reaskBubble) { reaskBubble.querySelector('.style-card')?.remove(); reaskBubble = null; }
  follow(id, a);
}
const fitBox = () => WowcAsking.fit(text, 18, 6);
$('form').onsubmit = ev => { ev.preventDefault(); const q = text.value.trim(); if (q && !busy && conn) { text.value = ''; fitBox(); ask(q); } };
text.addEventListener('keydown', ev => { WowcAsking.onAskKey(ev, () => $('form').requestSubmit()); });
text.oninput = fitBox;
const pasting = WowcAsking.wirePaste(text, () => invoke('read_clipboard'), fitBox);
$('paste').onclick = () => pasting.paste();
fitBox();
// No native focus call on click: the web view gives the ask box focus itself.
// Safety net: an editing key the web view drops is applied by the page itself (see asking.js).
WowcAsking.wireEditKeys(text, fitBox);
setInterval(() => showBadge(), 30000);

// --- version line, update banner and version list --------------------------------------------
// "WoW Companion 0.2.0 · advisor 0.2.0", plus "· 0.7.54 available" when the sidecar's release lookup
// (on start, every 4 hours, after the PC wakes, and on "Check now"; only when "Check for updates" is on)
// finds a version to offer: newest, not pulled, not skipped, and a day old unless "Get updates right
// away" is on. The same answer drives the chat banner and the Settings version list. The install itself
// is the Rust shell's: it downloads, checks the SHA-256, backs up the settings, runs the installer
// and exits. A failed lookup shows nothing.
const RELEASES_URL = 'https://github.com/WoW-Companion/advisor/releases';
const versionLine = WowcVersion.createVersionLine({
  el: $('version'),
  getConn: () => conn,
  enabled: () => cfg.checkUpdates,
  fetchUpdate: async () => {
    const r = await api('/api/update' + WowcUpdates.updateQuery(cfg, upd.fresh));
    const n = r.ok ? await r.json() : {};
    upd.status = n; upd.ok = true; upd.checkedAt = Date.now(); renderUpdates(); renderVersionList();
    await refreshAddon();
    return n;
  },
  checked: () => schedule.checked(),
  openPage: () => { cfg.later = {}; save(); renderUpdates(); },
  setTimeout: (f, ms) => setTimeout(f, ms),
  setInterval: (f, ms) => setInterval(f, ms),
});
const schedule = WowcUpdates.createSchedule({ now: () => Date.now(), check: () => versionLine.check() });
setInterval(() => schedule.tick(), WowcUpdates.TICK);
window.addEventListener('focus', () => schedule.focus());

// The in-game addon rides the same schedule, settle delay and switches: /api/addon/update says what
// is installed, what to offer now and which older versions are kept. The sidecar downloads, checks
// the SHA-256 and replaces the addon files; nothing is typed or sent to the game.
async function refreshAddon() {
  try {
    const q = [];
    if (cfg.gameDir) q.push('game=' + encodeURIComponent(cfg.gameDir));
    if (cfg.updateNow) q.push('now=1');
    if (upd.fresh) q.push('fresh=1');
    const r = await api('/api/addon/update' + (q.length ? '?' + q.join('&') : ''));
    upd.addon = r.ok ? await r.json() : null;
  } catch { upd.addon = null; }
  upd.addonAutoFailed = '';
  if (upd.addon && upd.addon.installed) {
    const prev = { addon: localStorage.getItem('wowc-addonlast') || '', upgrade: hadLastVersion };
    if (WowcUpdates.seedNotesSeen(cfg, '', upd.addon.installed, prev)) save();
    localStorage.setItem('wowc-addonlast', upd.addon.installed);
  }
  renderUpdates(); renderVersionList(); autoAddon();
}
async function installAddon(goback) {
  upd.failure = ''; upd.retry = () => installAddon(goback);
  upd.busy = goback ? 'Going back to addon ' + goback + '\u2026' : 'Installing the addon\u2026';
  renderUpdates(); renderVersionList();
  let ok = false;
  try {
    const r = await api('/api/addon/install', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ game: cfg.gameDir, goback: goback || '' }) });
    const j = await r.json();
    if (j.status) upd.addon = j.status;
    if (j.ok) { ok = true; showAddonDone(j.message, j.restart); } else upd.failure = j.error || 'The addon did not install.';
  } catch (e) { upd.failure = String(e); }
  upd.busy = '';
  renderUpdates(); renderVersionList();
  return ok;
}
// "Install" on the notice: the addon first, then the app when it has an update too (the app restarts).
async function installBoth(s) {
  upd.retry = () => installBoth(s);
  if (!(await installAddon(''))) { upd.retry = () => installBoth(s); return; }
  if (s.kind === 'offer') {
    // The app restarts: keep the addon version so the note after the restart names both.
    if (upd.addon && upd.addon.installed) localStorage.setItem('wowc-addonver', upd.addon.installed);
    localStorage.setItem('wowc-addonrestart', upd.lastRestart ? '1' : '');
    // (showAddonDone has kept the save time seen at the install, for the reload line after the restart.)
    installVersion(s.version);
  }
}
function doInstall(s) { return s.kind === 'addon' ? installAddon('') : s.addon ? installBoth(s) : installVersion(s.version); }
async function showAddonDone(message, restart) {
  upd.lastRestart = !!restart;
  const g = await gameSnap();
  const base = g ? g.savedAt : null; // null: not known yet, the first save time read later is the base
  localStorage.setItem('wowc-addonsaved', String(base == null ? 0 : base));
  drawUpdated(WowcUpdates.addonDoneNote(upd.addon && upd.addon.installed, message, restart), base);
}
// What the last game save recorded: the addon version it runs and when the file was written.
async function gameSnap() {
  try { const r = await api('/api/state'); if (!r.ok) return null; const j = await r.json(); return { addon: j.game_addon || '', savedAt: j.saved_at || 0 }; } catch { return null; }
}
// The update notice, with its /reload line when the addon was updated. The line stays (the notice can
// still be dismissed) until a newer save shows the game running the new addon, then goes by itself.
let reloadTimer = null;
// The version of the app that is running, for the notes view.
let appVer = '';
// Whether an earlier start left its version behind: an update, not a fresh install (read before this start writes it).
const hadLastVersion = !!localStorage.getItem('wowc-lastver');
// The "What's new" button: opens the release history (app and addon) into box, opened at the top (newest first), with a line
// under the unseen entries saying the rest was seen. The versions are marked seen when the view is closed, or after it has been
// open for NOTES_READ_MS, never on open, so the divider stays put while the player reads.
function whatsNewButton(box) {
  const b = Object.assign(document.createElement('button'), { type: 'button', textContent: 'What’s new' });
  let timer = 0;
  const seen = () => { clearTimeout(timer); timer = 0; if (WowcUpdates.markSeen(cfg, appVer, (upd.addon && upd.addon.installed) || '')) save(); };
  b.onclick = async () => {
    if (box.hidden || !box.hasChildNodes()) {
      await openNotesView(box); box.hidden = false; box.scrollTop = WowcUpdates.notesScrollTop();
      clearTimeout(timer); timer = setTimeout(seen, WowcUpdates.NOTES_READ_MS);
    } else { box.hidden = true; seen(); }
  };
  return b;
}
async function loadNotes() {
  const app = appVer, addon = (upd.addon && upd.addon.installed) || '';
  let resp = null;
  try {
    const r = await api('/api/notes-since' + WowcUpdates.notesQuery(cfg, app, addon));
    if (r.ok) resp = await r.json();
  } catch { /* offline and nothing kept: only the up-to-date line */ }
  return WowcUpdates.historyView(resp, cfg);
}
async function openNotesView(box) {
  const view = await loadNotes();
  box.replaceChildren();
  box.classList.add('notes-history');
  for (const it of view.items) {
    if (it.type === 'entry') {
      const head = Object.assign(document.createElement('b'), { textContent: it.component + ' ' + it.version + (it.date ? ' · ' + it.date : '') });
      const ul = document.createElement('ul');
      for (const t of it.bullets) ul.append(Object.assign(document.createElement('li'), { textContent: t }));
      box.append(head, ul);
    } else {
      box.append(Object.assign(document.createElement('div'), { className: it.type === 'divider' ? 'notes-divider' : it.type === 'heading' ? 'notes-heading' : 'notes-uptodate', textContent: it.text }));
    }
  }
  if (view.more) {
    const a = Object.assign(document.createElement('a'), { href: '#', textContent: view.more.text });
    a.onclick = ev => { ev.preventDefault(); invoke('open_install_page', { url: view.more.url }).catch(e => notice(String(e))); };
    box.append(a);
  }
}
function drawUpdated(n, base) {
  const bar = $('updated-note');
  clearInterval(reloadTimer);
  bar.hidden = false;
  const text = Object.assign(document.createElement('span'), { className: 'upd-text', textContent: n.text });
  const line = n.reload ? Object.assign(document.createElement('span'), { className: 'upd-text upd-reload', textContent: n.reload }) : null;
  bar.replaceChildren(text);
  if (line) { text.append(document.createElement('br'), line); }
  const body = Object.assign(document.createElement('div'), { hidden: true });
  if (n.whatsNew) {
    bar.append(whatsNewButton(body));
    loadNotes().then(v => { if (v.unseen) text.firstChild.textContent += WowcUpdates.newCount(v.unseen); });
  }
  const x = Object.assign(document.createElement('button'), { type: 'button', textContent: 'Dismiss' });
  x.onclick = () => { clearInterval(reloadTimer); bar.hidden = true; };
  bar.append(x, body);
  if (line) {
    const check = async () => {
      const g = await gameSnap();
      if (base == null) { if (g) base = g.savedAt; return; }
      if (WowcUpdates.reloadDone(n, g, base)) { clearInterval(reloadTimer); line.previousSibling.remove(); line.remove(); }
    };
    reloadTimer = setInterval(check, 15000);
    check();
  }
}
// "Install updates automatically" covers the addon: it installs when nothing is being written or
// typed, and a pulled addon goes back by itself. One failed try is not repeated until the next check.
function autoAddon() {
  const el = document.activeElement, tag = el && el.tagName;
  const quiet = !busy && !upd.busy && !(document.hasFocus() && (tag === 'TEXTAREA' || tag === 'INPUT'));
  const act = WowcUpdates.addonAuto(upd.addon, cfg, Date.now(), quiet);
  if (!act || upd.addonAutoFailed === upd.addon.latest) return;
  const v = upd.addon.latest;
  installAddon('').then(ok => { if (!ok) upd.addonAutoFailed = v; });
}
setInterval(autoAddon, 60000);

async function installVersion(version) {
  upd.failure = ''; upd.last = version; upd.retry = () => installVersion(version); upd.busy = 'Getting ' + version + '\u2026';
  renderUpdates(); renderVersionList();
  try {
    await invoke('install_update', { version, settingsJson: JSON.stringify(cfg) });
  } catch (e) {
    upd.busy = ''; upd.failure = String(e);
    renderUpdates(); renderVersionList();
  }
}
// The shell installs by itself when "Install updates automatically" is on, so it is told the setting
// and whether an answer is being written or an input has focus (it waits for five quiet minutes).
var autoSent = '';
function syncAuto() {
  const settingsJson = JSON.stringify(cfg), args = WowcUpdates.autoArgs(cfg);
  const key = JSON.stringify(args) + settingsJson;
  if (key === autoSent) return;
  autoSent = key;
  invoke('auto_update_config', { ...args, settingsJson }).catch(() => { autoSent = ''; });
}
function reportActivity() {
  const el = document.activeElement, tag = el && el.tagName;
  const focused = document.hasFocus() && (tag === 'TEXTAREA' || tag === 'INPUT');
  invoke('report_activity', { asking: !!busy, focused }).catch(() => {});
}
document.addEventListener('focusin', reportActivity);
document.addEventListener('focusout', () => setTimeout(reportActivity, 0));
window.addEventListener('blur', () => setTimeout(reportActivity, 0));
setInterval(reportActivity, 30000);
// "Updating to <v>…" for a few seconds before the shell starts (empty text clears it).
listen('auto-update-note', ev => {
  const t = String(ev.payload || '');
  if (t) { upd.busy = t; upd.autoNote = true; } else if (upd.autoNote) { upd.busy = ''; upd.autoNote = false; }
  renderUpdates();
});
listen('update-progress', ev => { if (upd.busy) { upd.busy = String(ev.payload); renderUpdates(); } });

// Notes as a bulleted list, grouped by version when the text has headings. link adds the
// "See all changes" link (opens the full list) when more than is shown exists. null for no notes.
function drawNotes(view, link) {
  if (!view.groups.length) return null;
  const box = Object.assign(document.createElement('div'), { className: 'upd-notes' });
  for (const g of view.groups) {
    if (g.version) box.append(Object.assign(document.createElement('div'), { className: 'upd-ver', textContent: g.version }));
    const ul = document.createElement('ul');
    for (const b of g.bullets) ul.append(Object.assign(document.createElement('li'), { textContent: b }));
    box.append(ul);
  }
  if (link && view.more) {
    const a = Object.assign(document.createElement('a'), { textContent: 'See all changes', href: '#', role: 'button' });
    a.onclick = ev => { ev.preventDefault(); invoke('open_install_page', { url: WowcUpdates.CHANGELOG_URL }).catch(e => notice(String(e))); };
    box.append(a);
  }
  return box;
}

function renderUpdates() {
  const bar = $('update-bar');
  const s = WowcUpdates.bannerState(upd.status, cfg, Date.now(), upd.busy, upd.failure, upd.addon);
  const view = WowcUpdates.bannerView(s);
  bar.hidden = !view || cfg.settingsOpen || !cfg.chatOpen;
  if (!view) return;
  bar.className = 'update-bar ' + view.cls;
  bar.replaceChildren();
  const t = document.createElement('span');
  t.className = 'upd-text'; t.textContent = view.text;
  bar.append(t);
  const act = {
    install: () => doInstall(s),
    always: () => { WowcUpdates.turnOnAuto(cfg); $('auto-update').checked = true; save(); syncAuto(); doInstall(s); },
    back: () => installVersion(s.backTo),
    retry: () => (upd.retry ? upd.retry() : installVersion(upd.last)),
    later: () => { WowcUpdates.later(cfg, s.kind === 'addon' ? '' : (s.version || upd.last), Date.now(), s.addon ? ['addon:' + s.addon.version] : []); upd.failure = ''; save(); renderUpdates(); },
    whatsnew: () => { upd.notesFor = upd.notesFor === s.version ? '' : s.version; renderUpdates(); },
    details: () => invoke('open_install_page', { url: RELEASES_URL }).catch(e => notice(String(e))),
    dismiss: () => { upd.failure = ''; renderUpdates(); },
  };
  const row = document.createElement('div');
  row.className = 'upd-actions';
  for (const a of view.actions) {
    const b = document.createElement('button');
    b.type = 'button'; b.textContent = a.label; b.onclick = act[a.id];
    row.append(b);
  }
  if (view.actions.length) bar.append(row);
  if (view.always) {
    const l = Object.assign(document.createElement('label'), { className: 'upd-always' });
    const c = Object.assign(document.createElement('input'), { type: 'checkbox' });
    c.onchange = act.always;
    l.append(c, ' Always update automatically');
    bar.append(l);
  }
  // What's new opens the notes, with See all changes and Skip this version beneath them.
  if (s.kind === 'offer' && !s.busy && !s.failure && upd.notesFor === s.version) {
    const n = drawNotes(WowcUpdates.noteView(s.notes, WowcUpdates.NOTICE_BULLETS), true)
      || Object.assign(document.createElement('div'), { className: 'upd-notes', textContent: 'No notes were published for this version.' });
    const a = Object.assign(document.createElement('a'), { textContent: 'Skip this version', href: '#', role: 'button' });
    a.onclick = ev => { ev.preventDefault(); WowcUpdates.skip(cfg, s.version); save(); renderUpdates(); };
    n.append(a);
    bar.append(n);
  }
}

function renderAddonList() {
  const box = $('addon-list');
  box.replaceChildren();
  const v = cfg.checkUpdates ? WowcUpdates.addonView(upd.addon) : null;
  box.hidden = !v;
  if (!v) return;
  const line = (text, buttons) => {
    const row = Object.assign(document.createElement('div'), { className: 'ver-row' });
    row.append(Object.assign(document.createElement('span'), { textContent: text }));
    for (const b of buttons) row.append(Object.assign(document.createElement('button'), { type: 'button', className: 'mini ' + (b.primary ? 'primary' : ''), textContent: b.text, disabled: !!upd.busy, onclick: b.fn }));
    box.append(row);
  };
  line(v.text, v.button ? [{ text: v.button, primary: true, fn: () => installAddon('') }] : []);
  for (const b of v.back) {
    if (upd.confirm === 'addon:' + b.version) line(b.confirm, [{ text: 'Go back', fn: () => { upd.confirm = ''; installAddon(b.version); } }, { text: 'Cancel', fn: () => { upd.confirm = ''; renderVersionList(); } }]);
    else line('Addon ' + b.version + ' (kept)', [{ text: b.button, fn: () => { upd.confirm = 'addon:' + b.version; renderVersionList(); } }]);
  }
}

function renderVersionList() {
  renderAddonList();
  const box = $('version-list');
  box.replaceChildren();
  const nv = WowcUpdates.noticeView(upd.status, upd.checkedAt);
  const nb = $('update-notice');
  nb.hidden = !nv || !cfg.checkUpdates;
  nb.replaceChildren();
  if (nv) {
    nb.className = 'update-notice ' + nv.cls;
    nb.append(Object.assign(document.createElement('span'), { textContent: nv.text }));
    if (nv.button) {
      const b = Object.assign(document.createElement('button'), { type: 'button', textContent: nv.button, className: 'primary', disabled: !!upd.busy });
      b.onclick = () => installVersion(nv.version);
      nb.append(b);
    }
  }
  const rows = WowcUpdates.versionRows(upd.status);
  if (!rows.length) { box.textContent = cfg.checkUpdates ? 'Press Check now to see the versions you can install.' : ''; return; }
  for (const r of rows) {
    const row = document.createElement('div');
    row.className = 'ver-row' + (r.current ? ' running' : '');
    const confirming = upd.confirm === r.version;
    const l = document.createElement('span');
    l.textContent = confirming ? r.confirm : r.label;
    row.append(l);
    const mk = (text, cls, fn) => {
      const b = Object.assign(document.createElement('button'), { type: 'button', className: 'mini ' + cls, textContent: text, disabled: !!upd.busy });
      b.onclick = fn; row.append(b);
    };
    if (confirming) {
      mk('Go back', '', () => { upd.confirm = ''; installVersion(r.version); });
      mk('Cancel', '', () => { upd.confirm = ''; renderVersionList(); });
    } else if (r.button) {
      mk(r.button, r.primary ? 'primary' : '', r.confirm ? () => { upd.confirm = r.version; renderVersionList(); } : () => installVersion(r.version));
    }
    box.append(row);
  }
}

// The note at the top of the chat after a start on a different version. The version is remembered
// at once, so the note is gone on the next launch (kept outside the settings, which a go-back restores).
async function showUpdatedNote(running) {
  const last = localStorage.getItem('wowc-lastver') || '';
  localStorage.setItem('wowc-lastver', running);
  const addonVer = localStorage.getItem('wowc-addonver') || '';
  localStorage.removeItem('wowc-addonver');
  const addonRestart = !!localStorage.getItem('wowc-addonrestart');
  localStorage.removeItem('wowc-addonrestart');
  const first = WowcUpdates.updatedNote(last, running, '', addonVer, addonRestart);
  // An automatic go-back keeps its reason in the shell: "✓ Went back to <v>: <reason>".
  const reason = first && first.kind === 'back' ? await invoke('take_went_back', { version: running }).catch(() => '') : '';
  const n = WowcUpdates.updatedNote(last, running, reason, addonVer, addonRestart);
  if (!n) return;
  drawUpdated(n, Number(localStorage.getItem('wowc-addonsaved')) || 0);
}

// --- why the app closed ---------------------------------------------------------------------
// The page tells the shell it is alive (a silent page is reloaded), shows "closed unexpectedly" once
// after a start that follows an unexpected end, and offers the logs and the diagnostics text.
setInterval(() => invoke('page_alive').catch(() => {}), 20000);
invoke('page_alive').catch(() => {});
$('open-logs').onclick = () => invoke('open_logs').catch(e => notice(String(e)));
// "Copy diagnostics": the shell reads its log tail and the launch record, the advisor strips names,
// paths and keys and builds the text, and the page puts it on the clipboard. Nothing is saved or sent.
async function copyText(text) {
  try { await navigator.clipboard.writeText(text); return; } catch (e) { /* fall back to the selection copy */ }
  const ta = Object.assign(document.createElement('textarea'), { value: text });
  ta.style.cssText = 'position:fixed;left:-9999px;top:0';
  document.body.appendChild(ta);
  ta.select();
  const ok = document.execCommand('copy');
  ta.remove();
  if (!ok) throw new Error('the clipboard refused it');
}
let diagTimer = 0;
$('copy-diag').onclick = async () => {
  const out = $('diag-out');
  clearTimeout(diagTimer);
  out.textContent = 'Copying\u2026';
  try {
    const files = await invoke('diagnostics_files').catch(() => ({ shell_log: '', guard: '' }));
    const r = await api('/api/diagnostics', { method: 'POST', headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify({ shell_log: files.shell_log, guard: files.guard, os: navigator.userAgent.replace(/^Mozilla\/5\.0 /, '').replace(/\).*$/, ')') }) });
    if (!r.ok) throw new Error('the app could not build it');
    await copyText((await r.json()).text);
    out.textContent = 'Copied';
  } catch (e) { out.textContent = 'Could not copy: ' + e.message; }
  diagTimer = setTimeout(() => { out.textContent = ''; }, 4000);
};
(async () => {
  const text = await invoke('take_unexpected').catch(() => null);
  if (!text) return;
  const bar = $('crash-note');
  bar.hidden = false;
  const logs = Object.assign(document.createElement('button'), { type: 'button', textContent: 'Open logs' });
  logs.onclick = () => invoke('open_logs').catch(e => notice(String(e)));
  const x = Object.assign(document.createElement('button'), { type: 'button', textContent: 'Dismiss' });
  x.onclick = () => { bar.hidden = true; };
  bar.replaceChildren(Object.assign(document.createElement('span'), { className: 'upd-text', textContent: text }), logs, x);
})();

$('check-updates').checked = cfg.checkUpdates;
$('check-updates').onchange = ev => { cfg.checkUpdates = ev.target.checked; save(); $('check-now').disabled = !cfg.checkUpdates; if (!cfg.checkUpdates) { versionLine.clearNotice(); renderUpdates(); } else versionLine.check(); };
$('update-now').checked = cfg.updateNow;
$('auto-update').checked = cfg.autoUpdate;
$('auto-update').onchange = ev => { cfg.autoUpdate = ev.target.checked; save(); renderUpdates(); };
$('update-now').onchange = ev => { cfg.updateNow = ev.target.checked; save(); versionLine.check(); };
$('check-now').disabled = !cfg.checkUpdates;
syncAuto();
$('check-now').onclick = async () => {
  const out = $('check-now-out');
  out.textContent = 'Checking\u2026';
  upd.fresh = true; upd.ok = false;
  await versionLine.check();
  upd.fresh = false;
  out.textContent = !upd.ok ? 'Could not check right now.' : upd.status.available ? 'Update ' + upd.status.latest + ' is ready.' : 'You are up to date.';
};
versionLine.startTimer();
versionLine.render();
renderVersionList();
window.__TAURI__.app.getVersion().then(v => { appVer = v; WowcUpdates.fixBuggySeed(cfg); WowcUpdates.seedNotesSeen(cfg, v, '', { app: localStorage.getItem('wowc-lastver') || '', upgrade: hadLastVersion }); save(); versionLine.setAppVersion(v); showUpdatedNote(v); });

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
    } else if (it.action === 'write-data') {
      const r = await api('/api/setup/write-data', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ game: cfg.gameDir }) });
      const j = await r.json();
      if (j.items) setupItems = j.items;
      setupMsg(j.error || j.message);
    } else if (it.action === 'download-dataset') {
      setupMsg('Downloading the game data (under 1 MB)…');
      const r = await api('/api/setup/download-dataset', { method: 'POST' });
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

// --- What's new (Updates): the last few versions' changelog, bundled with the advisor ----------
const RELEASES_PAGE = 'https://github.com/WoW-Companion/advisor/releases';
async function showWhatsNew() {
  const box = $('whats-new');
  try {
    const r = await api('/api/whats-new');
    if (!r.ok) return;
    const list = await r.json();
    box.replaceChildren(Object.assign(document.createElement('b'), { textContent: 'What’s new' }));
    list.forEach((rel, i) => {
      const d = document.createElement('details');
      d.open = i === 0;
      d.append(Object.assign(document.createElement('summary'), { textContent: rel.version }));
      const ul = document.createElement('ul');
      for (const t of rel.lines) ul.append(Object.assign(document.createElement('li'), { textContent: t }));
      d.append(ul);
      box.append(d);
    });
    const a = Object.assign(document.createElement('a'), { href: '#', textContent: 'Full changelog on GitHub' });
    a.onclick = ev => { ev.preventDefault(); invoke('open_install_page', { url: RELEASES_PAGE }).catch(e => notice(String(e))); };
    box.append(a);
    // The same view as the update notice's button: everything since the notes were last viewed.
    const since = Object.assign(document.createElement('div'), { className: 'notes-since', hidden: true });
    const row = document.createElement('div');
    row.append(whatsNewButton(since));
    box.append(row, since);
  } catch (e) { /* the list is a nicety; the page works without it */ }
}
