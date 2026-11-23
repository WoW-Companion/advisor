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

  // onAskKey: Enter sends, Shift+Enter leaves the newline alone.
  function onAskKey(ev, submit) {
    if (ev.key === 'Enter' && !ev.shiftKey && !ev.isComposing) { ev.preventDefault(); submit(); return true; }
    return false;
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
  const api = { entryList, fit, onAskKey, needsCard, shortQuestion, NEEDS_LINE };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcAsking = api;
})(typeof window !== 'undefined' ? window : globalThis);
