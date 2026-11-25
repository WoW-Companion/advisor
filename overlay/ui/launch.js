// Picking up where you left off: the saved chat, the old-ask card, the suggestions row, the quick bar
// and the freshness dot. No Tauri globals and no network of its own, so a test can drive it under Node
// with a stand-in document and storage. Nothing here sends a question; the page decides that.
(function (root) {
  const el = (doc, tag, cls, text) => {
    const e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  };

  const asking = () => root.WowcAsking || require('./asking.js');

  // --- saved chat ----------------------------------------------------------------------------
  // The last MAX_TURNS exchanges of each character, kept in the page's own local storage (this PC only).
  const MAX_TURNS = 50, ANSWER_CAP = 4000;
  function chatStore(storage, key = 'wowc-chat') {
    const read = () => { try { return JSON.parse(storage.getItem(key) || '{}') || {}; } catch { return {}; } };
    const write = all => { try { storage.setItem(key, JSON.stringify(all)); } catch { /* a full store only means a shorter memory */ } };
    return {
      load: char => (read()[char] || []).slice(-MAX_TURNS),
      // add keeps one row per id, newest last.
      add(char, turn) {
        const all = read();
        const rows = (all[char] || []).filter(t => t.id !== turn.id);
        rows.push({ id: turn.id, q: String(turn.q || ''), a: String(turn.a || '').slice(0, ANSWER_CAP), at: turn.at, src: turn.src || 'web' });
        all[char] = rows.slice(-MAX_TURNS);
        write(all);
      },
      clear(char) { const all = read(); delete all[char]; write(all); },
    };
  }

  const clock = d => { const h = d.getHours(), m = d.getMinutes(); return ((h + 11) % 12 + 1) + ':' + String(m).padStart(2, '0') + ' ' + (h < 12 ? 'AM' : 'PM'); };
  const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
  const dayStart = d => new Date(d.getFullYear(), d.getMonth(), d.getDate()).getTime();
  // dayKey: the local date as "2026-11-23", for the once-a-day dismissals.
  const dayKey = d => d.getFullYear() + '-' + String(d.getMonth() + 1).padStart(2, '0') + '-' + String(d.getDate()).padStart(2, '0');

  // gapLabel: the dim divider between restored chat and now: "Yesterday, 11:58 PM".
  function gapLabel(atSec, nowMs) {
    const at = new Date(atSec * 1000), days = Math.round((dayStart(new Date(nowMs)) - dayStart(at)) / 86400000);
    const when = days <= 0 ? 'Today' : days === 1 ? 'Yesterday' : MONTHS[at.getMonth()] + ' ' + at.getDate();
    return when + ', ' + clock(at);
  }
  function gapDivider(doc, atSec, nowMs) { return el(doc, 'div', 'gap', gapLabel(atSec, nowMs)); }

  // --- suggestions ---------------------------------------------------------------------------
  const MAX_SUGGESTIONS = 3;
  // visibleSuggestions: the advisor's list minus what was dismissed today, never more than three.
  // dismissed maps "<character>|<id>" to the day it was dismissed.
  function visibleSuggestions(list, dismissed, char, day) {
    return (list || []).filter(s => dismissed[char + '|' + s.id] !== day).slice(0, MAX_SUGGESTIONS);
  }
  // suggestionRow: one line of compact chips, label with its why on hover, and a small × per chip.
  function suggestionRow(doc, items, onPick, onDismiss) {
    const row = el(doc, 'div', 'sugg-row');
    for (const s of items) {
      const chip = el(doc, 'span', 'sugg');
      const go = el(doc, 'button', 'sugg-go', s.label);
      go.type = 'button';
      if (s.why) go.title = s.why;
      go.onclick = () => onPick(s);
      const x = el(doc, 'button', 'sugg-x', '×');
      x.type = 'button'; x.title = 'Hide for today'; x.setAttribute && x.setAttribute('aria-label', 'Hide ' + s.label + ' for today');
      x.onclick = () => onDismiss(s);
      chip.append(go, x);
      row.append(chip);
    }
    return row;
  }

  // --- quick bar -----------------------------------------------------------------------------
  // The five buttons. quickAction says what a button does for a character as data, so the page runs
  // it and a test can read it: { act: 'ask', question, ref } sends a question, { act: 'play_style' }
  // shows the play-style card, { act: 'refresh' } calls the refresh endpoint and asks nothing.
  const QUICK = [
    { id: 'next', icon: '➤', label: 'What next?' },
    { id: 'talents', icon: '✦', label: 'Talents' },
    { id: 'gear', icon: '↑', label: 'Gear' },
    { id: 'bags', icon: '▣', label: 'Bags' },
    { id: 'refresh', icon: '⟳', label: 'Refresh' },
  ];
  // Fixed, broad questions in plain player words: a drill-down belongs on a card, never on a button.
  // A ref only steers the tools; it never narrows the question.
  const QUICK_ASK = {
    next: 'What should I do next, and why?',
    talents: 'Which talent should I take next for my play style, and why?',
    gear: 'Look at my gear and suggest the best upgrades within reach right now: quest rewards, things I can make, vendor buys and cheap Auction House finds. Start with the biggest gain.',
  bags: 'Look at my bags and bank: what can I sell, what should I keep, and what should go to my other characters?',
  };
  function quickAction(id, c) {
    const entries = (c && c.entries) || [];
    switch (id) {
      case 'next': { // the plan's top entry stays as the ref, so tools look at what is in front of the player
        const top = entries.find(e => e.group === 'next' && e.ref);
        return { act: 'ask', question: QUICK_ASK.next, ref: top ? top.ref : '' };
      }
      case 'talents':
        if (!(c && c.style && c.style.role && c.style.focus)) return { act: 'play_style' };
        return { act: 'ask', question: QUICK_ASK.talents, ref: 'talents' };
      case 'gear':
        return { act: 'ask', question: QUICK_ASK.gear, ref: 'upgrades' };
      case 'bags':
        return { act: 'ask', question: QUICK_ASK.bags, ref: 'bags' };
      case 'refresh':
        return { act: 'refresh' };
    }
    return null;
  }
  function quickBar(doc, onPick) {
    const bar = el(doc, 'div', 'quick');
    for (const q of QUICK) {
      const b = el(doc, 'button', 'quick-btn quick-' + q.id);
      b.type = 'button';
      b.append(el(doc, 'span', 'quick-ic', q.icon), el(doc, 'span', 'quick-lbl', q.label));
      b.onclick = () => onPick(q.id);
      bar.append(b);
    }
    return bar;
  }

  // The play-style choices the card offers when no style is saved (the same values the advisor accepts).
  const styleNeeds = () => ({
    roles: [{ value: 'damage', label: 'Damage' }, { value: 'tank', label: 'Tank' }, { value: 'healer', label: 'Healer' }, { value: 'balanced', label: 'Balanced' }],
    activities: [{ value: 'leveling', label: 'Leveling' }, { value: 'dungeons', label: 'Dungeons' }, { value: 'pvp', label: 'PvP' }, { value: 'mixed', label: 'Mixed' }],
  });

  // saveStyleFlow saves one play-style change. Empty role or focus ("Not set") is a real value and is
  // sent, never dropped. deps.post(body) resolves { ok, text }; deps.reread() brings back what the advisor
  // really has saved. Resolves { ok: true } or { ok: false, error } with the reason in plain words.
  async function saveStyleFlow(deps, character, v) {
    const body = { character, role: v.role || '', focus: v.focus || '', spec: (v.spec || '').trim(), stat_pref: v.stat_pref || '' };
    let res;
    try { res = await deps.post(body); } catch (e) { res = { ok: false, text: '' }; }
    if (res.ok) { await deps.reread(res); return { ok: true }; }
    await deps.reread(res);
    return { ok: false, error: 'Not saved: ' + (String(res.text || '').trim() || 'the advisor did not answer. Your last saved choice is shown.') };
  }

  // --- freshness dot -------------------------------------------------------------------------
  const plural = (n, w) => n + ' ' + w + (n === 1 ? '' : 's');
  function agoWords(s) {
    return s < 60 ? 'just now' : s < 3600 ? plural(Math.floor(s / 60), 'minute') + ' ago' : s < 86400 ? plural(Math.floor(s / 3600), 'hour') + ' ago' : plural(Math.floor(s / 86400), 'day') + ' ago';
  }
  // freshness: the dot's colour (gold, amber after 12 hours, red after 3 days) and its tooltip.
  function freshness(capturedAt, nowMs) {
    if (!capturedAt) return { cls: 'red', tip: 'No game save found yet. Your plan updates by itself whenever the game saves.' };
    const s = Math.max(0, nowMs / 1000 - capturedAt);
    return {
      cls: s > 3 * 86400 ? 'red' : s > 12 * 3600 ? 'amber' : '',
      tip: 'Last game save ' + (s < 60 ? 'just now' : agoWords(s)) + '. Your plan updates by itself whenever the game saves.',
    };
  }

  // planLine: "23 stops", never a distance in the game's own units.
  function planLine(line) {
    const s = String(line || '');
    return /map units/.test(s) ? s.replace(/,?\s*(?:about\s+)?[\d.,]+\s*map units/i, '') : s;
  }

  // followChoice: which character the page shows. The newest capture is the one being played; with
  // "follow" on, and no character pinned, the page moves to it and says so in one line.
  function followChoice({ characters, newest, current, follow, pinned }) {
    const has = k => !!k && (characters || []).some(c => c.key === k);
    if (!has(newest) && !has(current)) return { key: (characters && characters[0] && characters[0].key) || '', switched: false, line: '' };
    if (!has(current)) return { key: newest, switched: false, line: '' };
    if (follow === false || has(pinned) || !has(newest) || newest === current) return { key: current, switched: false, line: '' };
    const c = characters.find(c => c.key === newest);
    return { key: newest, switched: true, line: 'Now showing ' + c.name + ': the newest game save is theirs.' };
  }

  const api = { followChoice, chatStore, gapLabel, gapDivider, visibleSuggestions, suggestionRow, QUICK, quickAction, quickBar, styleNeeds, saveStyleFlow, freshness, planLine, dayKey, MAX_TURNS, MAX_SUGGESTIONS };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcLaunch = api;
})(typeof window !== 'undefined' ? window : globalThis);
