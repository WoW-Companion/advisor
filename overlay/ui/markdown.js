// A small, safe Markdown renderer for chat answers. It builds DOM nodes with createElement and text
// nodes only (never innerHTML), so raw HTML in an answer is shown as text. Supported: bold, italic,
// inline code, links (text plus an "open" button), # to ### headings (small bold lines), bullet and
// numbered lists nested one level, simple tables and horizontal rules. An unfinished marker, as in a
// streaming "**bol", is left as plain text.
(function (root) {
  const DENSITIES = ['compact', 'comfortable'];
  const norm = d => (DENSITIES.includes(d) ? d : 'compact');

  const INLINE = /`([^`\n]+)`|\[([^\]\n]+)\]\((https?:\/\/[^\s)]+)\)|\*\*(?=\S)(.+?)(?<=\S)\*\*|\*(?=[^\s*])(.+?)(?<=[^\s*])\*/;

  function text(doc, s) { const t = doc.createElement('span'); t.textContent = s; return t; }

  // inline fills parent with the formatted runs of s.
  function inline(doc, parent, s, opts) {
    let rest = s;
    while (rest) {
      const m = INLINE.exec(rest);
      if (!m) { parent.append(text(doc, rest)); return; }
      if (m.index > 0) parent.append(text(doc, rest.slice(0, m.index)));
      let el;
      if (m[1] !== undefined) { el = doc.createElement('code'); el.textContent = m[1]; }
      else if (m[3] !== undefined) {
        el = doc.createElement('span'); el.className = 'md-link';
        inline(doc, el, m[2], opts);
        const b = doc.createElement('button');
        b.type = 'button'; b.className = 'md-open'; b.textContent = 'open'; b.title = m[3];
        b.onclick = () => { if (opts.onOpen) opts.onOpen(m[3]); };
        el.append(b);
      } else if (m[4] !== undefined) { el = doc.createElement('strong'); inline(doc, el, m[4], opts); }
      else { el = doc.createElement('em'); inline(doc, el, m[5], opts); }
      parent.append(el);
      rest = rest.slice(m.index + m[0].length);
    }
  }

  const HR = /^\s*([-*_])(\s*\1){2,}\s*$/;
  const HEAD = /^\s{0,3}#{1,3}\s+(.*?)\s*#*\s*$/;
  const ITEM = /^(\s*)([-*+]|\d+[.)])\s+(.*)$/;
  const SEP = /^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$/;
  const cells = l => l.trim().replace(/^\|/, '').replace(/\|$/, '').split('|').map(c => c.trim());

  function table(doc, rows, opts) {
    const t = doc.createElement('table'); t.className = 'md-table';
    const head = doc.createElement('thead'), body = doc.createElement('tbody');
    rows.forEach((r, i) => {
      const tr = doc.createElement('tr');
      for (const c of r) { const td = doc.createElement(i === 0 ? 'th' : 'td'); inline(doc, td, c, opts); tr.append(td); }
      (i === 0 ? head : body).append(tr);
    });
    t.append(head, body);
    return t;
  }

  // render turns Markdown source into a div.md with the density class.
  function render(doc, src, opts) {
    opts = opts || {};
    const box = doc.createElement('div');
    box.className = 'md md-' + norm(opts.density);
    const lines = String(src == null ? '' : src).replace(/\r\n?/g, '\n').split('\n');
    let para = null, list = null, listTag = '', sub = null, subTag = '';
    const endList = () => { list = sub = null; };
    const endPara = () => { para = null; };
    for (let i = 0; i < lines.length; i++) {
      const line = lines[i];
      if (!line.trim()) { endPara(); endList(); continue; }
      let m;
      if (HR.test(line)) { endPara(); endList(); const h = doc.createElement('hr'); h.className = 'md-hr'; box.append(h); continue; }
      if ((m = HEAD.exec(line))) {
        endPara(); endList();
        const h = doc.createElement('div'); h.className = 'md-h'; inline(doc, h, m[1], opts); box.append(h); continue;
      }
      if (line.includes('|') && i + 1 < lines.length && lines[i + 1].includes('-') && SEP.test(lines[i + 1])) {
        endPara(); endList();
        const rows = [cells(line)];
        i += 2;
        while (i < lines.length && lines[i].trim() && lines[i].includes('|')) rows.push(cells(lines[i++]));
        i--;
        box.append(table(doc, rows, opts)); continue;
      }
      if ((m = ITEM.exec(line))) {
        endPara();
        const tag = /\d/.test(m[2]) ? 'ol' : 'ul';
        const li = doc.createElement('li'); inline(doc, li, m[3], opts);
        if (m[1].length >= 2 && list && list.lastLi) {
          if (!sub || subTag !== tag) { sub = doc.createElement(tag); subTag = tag; list.lastLi.append(sub); }
          sub.append(li);
        } else {
          if (!list || listTag !== tag) { list = doc.createElement(tag); listTag = tag; list.className = 'md-list'; box.append(list); }
          sub = null; list.lastLi = li; list.append(li);
        }
        continue;
      }
      endList();
      if (!para) { para = doc.createElement('div'); para.className = 'md-p'; box.append(para); }
      else para.append(doc.createElement('br'));
      inline(doc, para, line.trim(), opts);
    }
    return box;
  }

  // applyDensity switches every rendered answer under rootEl to the density, live.
  function applyDensity(rootEl, density) {
    const d = norm(density);
    for (const el of rootEl.querySelectorAll('.md')) el.className = 'md md-' + d;
  }

  const api = { render, applyDensity, DENSITIES, norm };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcMd = api;
})(typeof window !== 'undefined' ? window : globalThis);
