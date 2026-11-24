// Driven by followup_test.go under Node: the follow-up chip and the end-of-answer scroll.
const assert = require('assert');
const F = require('./followup.js');

class El {
  constructor(tag) { this.tag = tag; this.children = []; this.className = ''; this.hidden = false; this.title = ''; this._text = ''; this.attrs = {}; }
  append(...c) { this.children.push(...c); }
  replaceChildren(...c) { this.children = c; }
  setAttribute(k, v) { this.attrs[k] = v; }
  set textContent(t) { this._text = t; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
}
const doc = { createElement: t => new El(t) };
const ctx = { character: 'c', names: ['The Skybreaker Bulwark', 'Seena Skybreaker', 'Zephras Isle'], at: 1000 };

// The chip shows the newest name, lists the rest in the tooltip, and its × clears.
const box = new El('div');
let cleared = 0;
assert.strictEqual(F.show(doc, box, ctx, 'c', 1100, () => cleared++), true);
assert.strictEqual(box.hidden, false);
assert.strictEqual(box.textContent, 'Following up on: The Skybreaker Bulwark×');
const chip = box.children[0];
assert(chip.className.split(' ').includes('chip'));
assert.strictEqual(chip.title, 'Talking about: The Skybreaker Bulwark; Seena Skybreaker; Zephras Isle');
chip.children[1].onclick();
assert.strictEqual(cleared, 1);

// Gone: no card, another character, or older than 30 minutes (the Clear button passes null).
assert.strictEqual(F.show(doc, box, ctx, 'd', 1100, () => {}), false);
assert(box.hidden && box.children.length === 0);
assert.strictEqual(F.show(doc, box, ctx, 'c', 1000 + 1800, () => {}), true);
assert.strictEqual(F.show(doc, box, ctx, 'c', 1000 + 1801, () => {}), false);
assert.strictEqual(F.show(doc, box, null, 'c', 1100, () => {}), false);
assert(box.hidden);

// A finished answer: stuck means scroll after layout and no pill; not stuck means no move and the pill.
const log = { scrollTop: 10, scrollHeight: 500, clientHeight: 100 };
const pill = { hidden: true };
let frame = null;
F.answerDone(log, true, f => { frame = f; }, pill);
assert.strictEqual(log.scrollTop, 10, 'scrolls on the next frame, not now');
frame();
assert.strictEqual(log.scrollTop, 500);
assert.strictEqual(pill.hidden, true);
log.scrollTop = 100; frame = null;
F.answerDone(log, false, f => { frame = f; }, pill);
assert.strictEqual(frame, null, 'a reader who scrolled up is never moved');
assert.strictEqual(log.scrollTop, 100);
assert.strictEqual(pill.hidden, false);

// The pill hides when the reader reaches the bottom, and a click scrolls down.
assert.strictEqual(F.onScroll(log, pill), false);
assert.strictEqual(pill.hidden, false);
log.scrollTop = 390;
assert.strictEqual(F.onScroll(log, pill), true);
assert.strictEqual(pill.hidden, true);
pill.hidden = false; log.scrollTop = 0;
F.pillClick(log, pill);
assert.strictEqual(log.scrollTop, 500);
assert.strictEqual(pill.hidden, true);
console.log('ok');
