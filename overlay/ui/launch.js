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
  // The four buttons. quickAction says what a button does for a character as data, so the page runs
  // it and a test can read it: { act: 'ask', question, ref } sends a question, { act: 'play_style' }
  // shows the play-style card, { act: 'refresh' } calls the refresh endpoint and asks nothing.
  const QUICK = [
    { id: 'next', icon: '➤', label: 'What next?' },
    { id: 'talents', icon: '✦', label: 'Talents' },
    { id: 'gear', icon: '↑', label: 'Gear' },
    { id: 'refresh', icon: '⟳', label: 'Refresh' },
  ];
  function quickAction(id, c) {
    const entries = (c && c.entries) || [];
    const first = (pred, fallbackQ, fallbackRef) => {
      const e = entries.find(pred);
      return e && e.asks && e.asks.length ? { act: 'ask', question: e.asks[0], ref: e.ref || '' } : { act: 'ask', question: fallbackQ, ref: fallbackRef };
    };
    switch (id) {
      case 'next': { // the plan's top entry, so the answer is about what is in front of the player
        const top = entries.find(e => e.group === 'next' && e.ref);
        return { act: 'ask', question: 'What should I do next?', ref: top ? top.ref : '' };
      }
      case 'talents':
        if (!(c && c.style && c.style.role && c.style.focus)) return { act: 'play_style' };
        return first(e => e.group === 'talents' || e.kind === 'talent', 'Where should my talent points go?', 'talents');
      case 'gear':
        return first(e => e.group === 'upgrades', 'What gear should I upgrade next?', '');
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

  const api = { chatStore, gapLabel, gapDivider, visibleSuggestions, suggestionRow, QUICK, quickAction, quickBar, styleNeeds, freshness, planLine, dayKey, MAX_TURNS, MAX_SUGGESTIONS };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcLaunch = api;
})(typeof window !== 'undefined' ? window : globalThis);
