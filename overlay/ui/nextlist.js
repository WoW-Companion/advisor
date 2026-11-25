// The Next list's entries, laid out from the digest line's structured fields like the in-game panel.
// No DOM or Tauri globals of its own so a test can drive it under Node. A line without structured
// fields falls back to its text. Nothing is truncated: the stylesheet wraps long lines.
(function (root) {
  // The quest title's colour class against the character's level, WoW's own scale: red 5+ above,
  // orange 3-4 above, yellow within 2 either side, green down to the green range, then grey.
  function greenRange(level) {
    return level < 8 ? 5 : level < 10 ? 6 : level < 12 ? 7 : level < 16 ? 8 : level < 20 ? 9 : 10;
  }
  function difficulty(questLevel, charLevel) {
    if (!questLevel || !charLevel) return 'diff-yellow';
    const d = questLevel - charLevel;
    if (d >= 5) return 'diff-red';
    if (d >= 3) return 'diff-orange';
    if (d >= -2) return 'diff-yellow';
    return -d <= greenRange(charLevel) ? 'diff-green' : 'diff-grey';
  }

  const plural = (n, one, many) => n + ' ' + (n === 1 ? one : many);

  // describe: one line to rows [{ cls, parts: [{ text, cls? }] }]. The first row is the head.
  function describe(line, charLevel) {
    if (typeof line === 'string') line = { text: line };
    const text = (line && line.text) || '';
    if (line.kind === 'talent' && line.points > 0 && !/^Spend /.test(text)) {
      return [{ cls: 'nx-head nx-talent', parts: [{ text: plural(line.points, 'unspent talent point', 'unspent talent points') }] }];
    }
    const structured = (line.kind === 'pickup' || line.kind === 'turnin' || line.kind === 'progress') && line.title;
    if (!structured) return [{ cls: 'nx-head', parts: [{ text }] }];
    const head = [{ text: (line.level ? '[' + line.level + '] ' : '') + line.title, cls: difficulty(line.level, charLevel) }];
    if (line.class) head.push({ text: ' (Class)', cls: 'nx-tag' });
    if (line.elite) head.push({ text: ' (Elite)', cls: 'nx-tag' });
    const rows = [{ cls: 'nx-head', parts: head }];
    for (const o of line.objectives || []) {
      const count = o.has_count ? o.have + '/' + o.need + ' ' : '';
      const done = o.has_count && o.have >= o.need;
      rows.push({ cls: 'nx-obj' + (done ? ' nx-done' : ''), parts: [{ text: count + o.text + (o.optional ? ' (optional)' : '') }] });
    }
    if (line.kind === 'turnin') {
      rows.push({ cls: 'nx-sub nx-ready', parts: [{ text: 'Ready to turn in' }] });
      if (line.npc) rows.push({ cls: 'nx-sub', parts: [{ text: 'Turn in to ' + line.npc }] });
      if (line.unlocks && line.unlocks.length) rows.push({ cls: 'nx-sub nx-unlocks', parts: [{ text: 'Unlocks: ' + line.unlocks.join('; ') }] });
    } else if (line.kind === 'pickup' && line.npc) {
      rows.push({ cls: 'nx-sub', parts: [{ text: 'Pick up from ' + line.npc }] });
    }
    return rows;
  }

  // render: the list item for one line, built from describe's rows.
  function render(doc, line, charLevel) {
    const li = doc.createElement('li');
    describe(line, charLevel).forEach((r, i) => {
      const row = doc.createElement('div');
      row.className = r.cls;
      for (const p of r.parts) {
        const s = doc.createElement('span');
        if (p.cls) s.className = p.cls;
        s.textContent = p.text;
        row.append(s);
      }
      li.append(row);
    });
    return li;
  }

  const api = { difficulty, describe, render };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcNext = api;
})(typeof window !== 'undefined' ? window : globalThis);
