// The "following up on" chip and the end-of-answer scroll, with no DOM globals of their own so a test
// can drive them under Node with a stand-in document.
(function (root) {
  const CARD_TTL_S = 30 * 60; // the server's CardTTL: the chat's gap divider

  // active: the card while it still applies to this character and is under 30 minutes old.
  function active(ctx, char, nowS) {
    return ctx && ctx.names && ctx.names.length && ctx.character === char && nowS - ctx.at <= CARD_TTL_S ? ctx : null;
  }

  // chip: "Following up on: <newest name> ×", on one line; the tooltip lists the other names.
  function chip(doc, ctx, onClear) {
    const c = doc.createElement('span');
    c.className = 'chip follow';
    const t = doc.createElement('span');
    t.className = 'follow-text';
    t.textContent = 'Following up on: ' + ctx.names[0];
    const x = doc.createElement('button');
    x.type = 'button';
    x.className = 'follow-x';
    x.textContent = '×';
    x.title = 'Start fresh: forget what we were talking about';
    x.setAttribute('aria-label', 'Stop following up on ' + ctx.names[0]);
    x.onclick = onClear;
    c.title = 'Talking about: ' + ctx.names.join('; ');
    c.append(t, x);
    return c;
  }

  // show fills box with the chip for ctx, or hides it when there is none for this character.
  function show(doc, box, ctx, char, nowS, onClear) {
    const a = active(ctx, char, nowS);
    box.replaceChildren();
    box.hidden = !a;
    if (a) box.append(chip(doc, a, onClear));
    return !!a;
  }

  // answerDone: once an answer is fully appended, follow the bottom after layout (next frame), but only
  // when the reader was already there; otherwise leave them and offer the "New answer" pill.
  function answerDone(log, stuck, raf, pill) {
    if (stuck) {
      raf(() => { log.scrollTop = log.scrollHeight; });
      pill.hidden = true;
    } else {
      pill.hidden = false;
    }
  }

  // atBottom: whether a scroll position counts as the bottom; the pill goes away there.
  function atBottom(log) { return log.scrollHeight - log.scrollTop - log.clientHeight < 24; }
  function onScroll(log, pill) { const s = atBottom(log); if (s) pill.hidden = true; return s; }
  function pillClick(log, pill) { log.scrollTop = log.scrollHeight; pill.hidden = true; }

  const api = { CARD_TTL_S, active, chip, show, answerDone, atBottom, onScroll, pillClick };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcFollow = api;
})(typeof window !== 'undefined' ? window : globalThis);
