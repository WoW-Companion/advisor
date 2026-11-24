// Asking without pasting: the plan's entries with one-click questions, the growing ask box, the
// play-style card and the player-facing wording of the chat. No Tauri globals of its own, so a test
// can drive it under Node with a stand-in document.
(function (root) {
  const GLYPH = { pickup: '!', giver: '!', turnin: '?', progress: '›', talent: '✦', training: '✎', upgrade: '↑', dungeon: '⚔', note: 'i' };
  const el = (doc, tag, cls, text) => {
    const e = doc.createElement(tag);
    if (cls) e.className = cls;
    if (text !== undefined) e.textContent = text;
    return e;
  };

  // entryList: one row per entry (icon, title, short status); a click on a row shows its questions as
  // chips beneath it; a click on a chip calls onAsk(question, ref). Only one row is open at a time.
  function entryList(doc, entries, onAsk) {
    const ul = el(doc, 'ul', 'entries');
    let open = null;
    for (const e of entries || []) {
      const li = el(doc, 'li', 'entry');
      const row = el(doc, 'button', 'entry-row');
      row.type = 'button';
      row.append(el(doc, 'span', 'ic ic-' + (e.kind || e.group || 'note'), GLYPH[e.kind] || '•'), el(doc, 'span', 'entry-title', e.title));
      if (e.status) row.append(el(doc, 'span', 'entry-status', e.status));
      const chips = el(doc, 'div', 'chips');
      for (const q of e.asks || []) {
        const c = el(doc, 'button', 'chip-ask', q);
        c.type = 'button';
        c.onclick = ev => { if (ev && ev.stopPropagation) ev.stopPropagation(); onAsk(q, e.ref || ''); };
        chips.append(c);
      }
      row.onclick = () => {
        const was = li.className.includes(' open');
        if (open) open.className = open.className.replace(' open', '');
        open = null;
        if (!was && chips.children.length) { li.className += ' open'; open = li; }
      };
      li.append(row);
      if (e.why) li.append(el(doc, 'div', 'entry-why', e.why)); // one short reason, dim, under the title
      li.append(chips);
      ul.append(li);
    }
    return ul;
  }

  // fit: the ask box grows with its content up to maxLines, then scrolls; an empty box shrinks back.
  function fit(box, lineHeight, maxLines) {
    box.style.height = 'auto';
    const max = lineHeight * maxLines + 10; // the box's own padding
    const want = Math.max(box.scrollHeight, lineHeight + 10);
    box.style.height = Math.min(want, max) + 'px';
    box.style.overflowY = want > max ? 'auto' : 'hidden';
    return box.style.height;
  }

  // insertText: puts text at the cursor, replacing any selection, and leaves the cursor after it.
  function insertText(box, str) {
    if (!str) return false;
    const v = String(box.value || '');
    const a = box.selectionStart == null ? v.length : box.selectionStart;
    const b = box.selectionEnd == null ? a : box.selectionEnd;
    const room = box.maxLength > 0 ? box.maxLength : Infinity;
    const add = String(str).replace(/\r\n?/g, '\n').slice(0, Math.max(0, room - (v.length - (b - a))));
    box.value = v.slice(0, a) + add + v.slice(b);
    box.selectionStart = box.selectionEnd = a + add.length;
    return true;
  }

  // wirePaste: a paste into the ask box that the web view leaves empty (the game can put only
  // ANSI text on the clipboard) is read natively instead. readNative() resolves to the clipboard's
  // text; after is called once text went in (the box refits). Returns { paste } for the paste button.
  function wirePaste(box, readNative, after, raf) {
    raf = raf || (fn => requestAnimationFrame(fn));
    let reading = false, sawPaste = false;
    const fromNative = async () => {
      if (reading) return; // never paste twice
      reading = true;
      try { if (insertText(box, await readNative())) after(); } catch (e) { /* nothing to paste */ } finally { reading = false; }
    };
    box.addEventListener('paste', ev => {
      sawPaste = true;
      const t = ev.clipboardData && ev.clipboardData.getData ? ev.clipboardData.getData('text') : '';
      if (t) return; // the web paste has it
      ev.preventDefault();
      fromNative();
    });
    box.addEventListener('keydown', ev => {
      const key = String(ev.key || '').toLowerCase();
      if (!((ev.ctrlKey || ev.metaKey) && key === 'v') && !(ev.shiftKey && key === 'insert')) return;
      const before = box.value;
      sawPaste = false;
      raf(() => { if (!sawPaste && box.value === before) fromNative(); });
    });
    return { paste: () => { box.focus && box.focus(); return fromNative(); } };
  }

  const EDIT_KEYS = ['Backspace', 'Delete', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'];

  // applyEditKey: what an editing or caret key does to { value, start, end, dir } (dir 'backward' when
  // the moving end of the selection is its start). Pure; returns the new { value, start, end, dir }.
  function applyEditKey(st, key, shift) {
    const v = String(st.value || '');
    const s = Math.min(st.start, st.end), e = Math.max(st.start, st.end);
    const back = st.dir === 'backward';
    const anchor = back ? e : s, focus = back ? s : e;
    const same = { value: v, start: s, end: e, dir: st.dir || 'none' };
    if (key === 'Backspace' || key === 'Delete') {
      let a = s, b = e;
      if (s === e) {
        if (key === 'Backspace') a = Math.max(0, s - 1); else b = Math.min(v.length, e + 1);
      }
      if (a === b) return same;
      return { value: v.slice(0, a) + v.slice(b), start: a, end: a, dir: 'none' };
    }
    const lineStart = i => v.lastIndexOf('\n', i - 1) + 1;
    const lineEnd = i => { const n = v.indexOf('\n', i); return n < 0 ? v.length : n; };
    let to;
    if (!shift && s !== e && (key === 'ArrowLeft' || key === 'ArrowRight')) {
      to = key === 'ArrowLeft' ? s : e; // a plain arrow collapses the selection to its edge
      return { value: v, start: to, end: to, dir: 'none' };
    }
    switch (key) {
      case 'ArrowLeft': to = Math.max(0, focus - 1); break;
      case 'ArrowRight': to = Math.min(v.length, focus + 1); break;
      case 'Home': to = lineStart(focus); break;
      case 'End': to = lineEnd(focus); break;
      case 'ArrowUp': {
        const ls = lineStart(focus), col = focus - ls;
        if (ls === 0) { to = 0; break; }
        const ps = lineStart(ls - 1);
        to = Math.min(ps + col, ls - 1);
        break;
      }
      case 'ArrowDown': {
        const le = lineEnd(focus), col = focus - lineStart(focus);
        if (le >= v.length) { to = v.length; break; }
        const ns = le + 1;
        to = Math.min(ns + col, lineEnd(ns));
        break;
      }
      default: return same;
    }
    if (!shift) return { value: v, start: to, end: to, dir: 'none' };
    const a = Math.min(anchor, to), b = Math.max(anchor, to);
    return { value: v, start: a, end: b, dir: to < anchor ? 'backward' : 'forward' };
  }

  // wireEditKeys: the ask box's safety net. If the web view drops an editing key (Backspace, Delete,
  // the arrows, Home, End; no Ctrl/Alt/Meta), the key is applied here one frame later, but only when
  // the box did not change and no beforeinput came for it, so a native edit is never applied twice.
  // after() is the box's input handling (it refits); onApplied(key) is told when a key was applied here.
  function wireEditKeys(box, after, raf, onApplied) {
    raf = raf || (fn => requestAnimationFrame(fn));
    let snap = null;
    const read = () => ({ value: String(box.value || ''), start: box.selectionStart == null ? 0 : box.selectionStart, end: box.selectionEnd == null ? 0 : box.selectionEnd, dir: box.selectionDirection || 'none' });
    box.addEventListener('beforeinput', () => { if (snap) snap.native = true; }, true);
    box.addEventListener('keydown', ev => {
      if (ev.ctrlKey || ev.altKey || ev.metaKey || ev.isComposing || ev.defaultPrevented || EDIT_KEYS.indexOf(ev.key) < 0) return;
      const mine = snap = { ...read(), native: false, key: ev.key, shift: !!ev.shiftKey };
      raf(() => {
        if (snap !== mine) return; // a newer key took over
        snap = null;
        const now = read();
        if (mine.native || now.value !== mine.value || now.start !== mine.start || now.end !== mine.end) return; // the web view did it
        const r = applyEditKey(mine, mine.key, mine.shift);
        if (r.value === mine.value && r.start === mine.start && r.end === mine.end) return; // nothing to do at an edge
        const changed = r.value !== mine.value;
        if (changed) box.value = r.value;
        if (box.setSelectionRange) box.setSelectionRange(r.start, r.end, r.dir); else { box.selectionStart = r.start; box.selectionEnd = r.end; }
        if (changed) {
          if (typeof Event === 'function' && box.dispatchEvent) box.dispatchEvent(new Event('input', { bubbles: true })); else after();
        }
        if (onApplied) onApplied(mine.key);
      });
    });
  }

  // onAskKey: Enter sends, Shift+Enter leaves the newline alone. Returns nothing: an on... property handler
  // that returns false cancels the key, so attach this with addEventListener and never return its result.
  function onAskKey(ev, submit) {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); submit(); }
  }

  // needsCard: the inline play-style card. One click per row, an optional spec, then Save, which calls
  // onSave({ role, focus, spec }). set(style) syncs the card with a saved play style.
  function needsCard(doc, needs, onSave, style) {
    const card = el(doc, 'div', 'style-card');
    const pick = { role: (style && style.role) || '', focus: (style && style.focus) || '', spec: (style && style.spec) || '' };
    const save = el(doc, 'button', 'style-save', 'Save');
    save.type = 'button';
    const rows = {};
    const sync = () => {
      for (const k of ['role', 'focus']) for (const b of rows[k]) b.className = 'opt' + (b.value === pick[k] ? ' on' : '');
      save.disabled = !(pick.role && pick.focus);
    };
    const group = (key, label, opts) => {
      const r = el(doc, 'div', 'style-row');
      r.append(el(doc, 'span', 'style-lbl', label));
      rows[key] = (opts || []).map(o => {
        const b = el(doc, 'button', 'opt', o.label);
        b.type = 'button'; b.value = o.value;
        b.onclick = () => { pick[key] = o.value; sync(); };
        r.append(b);
        return b;
      });
      card.append(r);
    };
    group('role', 'Role', needs.roles);
    group('focus', 'Activity', needs.activities);
    let sel = null;
    if (needs.specs && needs.specs.length) {
      const r = el(doc, 'div', 'style-row');
      sel = el(doc, 'select', 'style-spec');
      sel.append(Object.assign(el(doc, 'option', '', 'Not sure yet'), { value: '' }));
      for (const s of needs.specs) sel.append(Object.assign(el(doc, 'option', '', s), { value: s }));
      sel.value = pick.spec;
      sel.onchange = () => { pick.spec = sel.value; };
      r.append(el(doc, 'span', 'style-lbl', 'Spec'), sel);
      card.append(r);
    }
    save.onclick = () => { if (pick.role && pick.focus) { save.disabled = true; onSave({ role: pick.role, focus: pick.focus, spec: pick.spec }); } };
    card.append(save);
    // setStyle: keep the card in step with the play style saved elsewhere (the settings section).
    card.setStyle = st => {
      pick.role = (st && st.role) || ''; pick.focus = (st && st.focus) || ''; pick.spec = (st && st.spec) || '';
      if (sel) sel.value = pick.spec;
      save.disabled = false; sync();
    };
    sync();
    return card;
  }

  const NEEDS_LINE = 'Tell me how you like to play and I’ll answer:';

  // shortQuestion: what the chat shows for a question. A who-am-I preamble from an older addon never shows.
  function shortQuestion(text) {
    const t = String(text || '').trim();
    const rest = t.replace(/^I(?:'|’)?m an? (?:level \d+ )?[^.?!]*\.\s+/i, '');
    return rest || t;
  }
  const api = { entryList, fit, insertText, wirePaste, applyEditKey, wireEditKeys, EDIT_KEYS, onAskKey, needsCard, shortQuestion, NEEDS_LINE };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcAsking = api;
})(typeof window !== 'undefined' ? window : globalThis);
