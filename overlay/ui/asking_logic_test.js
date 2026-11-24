// Driven by asking_test.go under Node: the entry list, the ask box, the play-style card and the wording.
const assert = require('assert');
const A = require('./asking.js');

class El {
  constructor(tag) { this.tag = tag; this.children = []; this.className = ''; this._text = ''; this.style = {}; this.value = ''; this.scrollHeight = 0; }
  append(...c) { this.children.push(...c); }
  set textContent(t) { this._text = t; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
  find(cls) { return this.children.flatMap(c => [c, ...c.find(cls)]).filter(c => c.className.split(' ').includes(cls)); }
}
const doc = { createElement: t => new El(t) };
const all = e => [e.textContent];

// A 1.5 digest's entries: rows with chips; a chip sends the question with the entry's ref.
const entries = [
  { group: 'next', kind: 'turnin', title: 'Wolves at the Door', status: 'turn in', asks: ['What does turning in Wolves at the Door unlock?', 'What should I do after I turn it in?'], ref: 'quest:100' },
  { group: 'next', kind: 'talent', title: '2 unspent talent points', asks: ['Where should my talent points go?'], ref: 'talents' },
  { group: 'dungeons', kind: 'dungeon', title: 'Wolf Den', status: 'lvl 10-14', asks: ['Am I ready for Wolf Den?'], ref: 'dungeon:1' },
];
const sent = [];
const ul = A.entryList(doc, entries, (q, ref) => sent.push([q, ref]));
const rows = ul.find('entry-row');
assert.strictEqual(rows.length, 3);
assert.strictEqual(rows[0].textContent, '?Wolves at the Doorturn in');
assert.strictEqual(ul.find('chip-ask').length, 4);
rows[0].onclick();
assert(ul.children[0].className.includes(' open'));
rows[1].onclick();
assert(!ul.children[0].className.includes(' open') && ul.children[1].className.includes(' open'), 'one row open at a time');
ul.children[0].children[1].children[1].onclick({});
assert.deepStrictEqual(sent, [['What should I do after I turn it in?', 'quest:100']]);
ul.children[1].children[1].children[0].onclick();
assert.deepStrictEqual(sent[1], ['Where should my talent points go?', 'talents']);
assert(!all(ul).join().includes('quest:100') && !all(ul).join().includes('dungeon:1'), 'refs are never shown');

// The ask box grows with its content up to 6 lines, then scrolls, and shrinks back when emptied.
const box = new El('textarea');
const h = n => { box.scrollHeight = 18 * n + 10; return A.fit(box, 18, 6); };
assert.strictEqual(h(1), '28px');
assert.strictEqual(h(3), '64px');
assert.strictEqual(h(6), '118px'); assert.strictEqual(box.style.overflowY, 'hidden');
assert.strictEqual(h(9), '118px'); assert.strictEqual(box.style.overflowY, 'auto');
assert.strictEqual(h(0), '28px'); assert.strictEqual(box.style.overflowY, 'hidden');

// Enter sends; Shift+Enter is left to make a new line.
let submitted = 0, prevented = 0;
const key = (k, shift) => A.onAskKey({ key: k, shiftKey: shift, preventDefault() { prevented++; } }, () => submitted++);
key('Enter', false); assert(submitted === 1 && prevented === 1);
key('Enter', true); assert(submitted === 1 && prevented === 1);
// Any other key: no cancel, and nothing returned (a returned false from an on... handler cancels the key).
for (const k of ['a', 'Backspace', 'Delete', 'ArrowLeft', 'Home', 'End']) assert.strictEqual(key(k, false), undefined);
assert.strictEqual(key('Enter', true), undefined);
assert(submitted === 1 && prevented === 1);

// The play-style card: one click per row, Save after both, then the original question is re-asked by
// the advisor; the card follows a style saved elsewhere.
const needs = { roles: [{ value: 'damage', label: 'Damage' }, { value: 'tank', label: 'Tank' }], activities: [{ value: 'leveling', label: 'Leveling' }, { value: 'pvp', label: 'PvP' }], specs: ['Arms', 'Fury'] };
const saved = [];
const card = A.needsCard(doc, needs, st => saved.push(st));
const save = card.find('style-save')[0], opts = card.find('opt');
assert(save.disabled);
opts[0].onclick(); assert(save.disabled, 'both rows are needed');
opts[3].onclick(); assert(!save.disabled);
opts[1].onclick();
assert.strictEqual(card.find('on').length, 2);
card.find('style-spec')[0].value = 'Fury'; card.find('style-spec')[0].onchange();
save.onclick();
assert.deepStrictEqual(saved, [{ role: 'tank', focus: 'pvp', spec: 'Fury' }]);
card.setStyle({ role: 'damage', focus: 'leveling', spec: '' });
assert(opts[0].className.includes('on') && opts[2].className.includes('on') && !opts[1].className.includes('on'));
assert(A.NEEDS_LINE.startsWith('Tell me how you like to play'));

// Wording: a short question, no reload text, no preamble.
assert.strictEqual(A.shortQuestion("I'm a level 12 priest. Where should my talent point go?"), 'Where should my talent point go?');
assert.strictEqual(A.shortQuestion('Where should my talent point go?'), 'Where should my talent point go?');
const shown = [ul.textContent, card.textContent, A.NEEDS_LINE, A.shortQuestion("I'm a level 12 priest. Hi?")].join('\n');
for (const bad of ['/reload', 'Answers.lua', "I'm a level"]) assert(!shown.includes(bad), bad);
console.log('ok');

// Pasting: an empty web paste reads the clipboard natively and inserts once; a normal web paste is left alone.
(async () => {
  const mk = () => {
    const b = new El('textarea'); b.value = 'hello world'; b.selectionStart = 6; b.selectionEnd = 11; b.maxLength = 1000;
    b.h = {}; b.addEventListener = (n, f) => { b.h[n] = f; };
    return b;
  };
  const tick = () => new Promise(r => setImmediate(r));
  let reads = 0, fits = 0, frames = [];
  const wire = b => A.wirePaste(b, async () => { reads++; return 'a\r\nb'; }, () => fits++, f => frames.push(f));
  // empty web paste: native read, once
  let b = mk(); wire(b);
  let prevented = false;
  b.h.paste({ clipboardData: { getData: () => '' }, preventDefault: () => { prevented = true; } });
  await tick();
  assert(prevented); assert.strictEqual(reads, 1); assert.strictEqual(fits, 1);
  assert.strictEqual(b.value, 'hello a\nb', 'replaces the selection');
  // a normal web paste does not double, and Ctrl+V's frame check finds the paste event
  b = mk(); wire(b); reads = 0; frames = [];
  b.h.keydown({ key: 'v', ctrlKey: true });
  b.h.paste({ clipboardData: { getData: () => 'text' }, preventDefault: () => assert.fail('must not prevent') });
  frames.forEach(f => f()); await tick();
  assert.strictEqual(reads, 0);
  // Ctrl+V with no paste event and no change: read natively once
  b = mk(); wire(b); frames = [];
  b.h.keydown({ key: 'v', ctrlKey: true }); b.h.keydown({ key: 'Insert', shiftKey: true });
  frames.forEach(f => f()); await tick();
  assert.strictEqual(reads, 1, 'two frames, one insert');
  // Ctrl+V that changed the box: no native read
  b = mk(); wire(b); frames = []; reads = 0;
  b.h.keydown({ key: 'v', ctrlKey: true }); b.value = 'changed';
  frames.forEach(f => f()); await tick();
  assert.strictEqual(reads, 0);
  // the button inserts at the cursor
  b = mk(); b.selectionStart = b.selectionEnd = 5; fits = 0;
  await wire(b).paste();
  assert.strictEqual(b.value, 'helloa\nb world'); assert.strictEqual(fits, 1);
  console.log('paste ok');
})().catch(e => { console.error(e); process.exit(1); });

// Editing keys: the safety net applies a dropped key once, never twice, and leaves native edits alone.
{
  const st = (value, start, end, dir) => ({ value, start, end: end === undefined ? start : end, dir: dir || 'none' });
  const ap = (s, k, sh) => { const r = A.applyEditKey(s, k, sh); return [r.value, r.start, r.end]; };
  const eq = (got, want) => assert.deepStrictEqual(got, want);
  eq(ap(st('abc', 2), 'Backspace'), ['ac', 1, 1]);
  eq(ap(st('abc', 0), 'Backspace'), ['abc', 0, 0]);
  eq(ap(st('abc', 1), 'Delete'), ['ac', 1, 1]);
  eq(ap(st('abc', 3), 'Delete'), ['abc', 3, 3]);
  eq(ap(st('abcd', 1, 3), 'Backspace'), ['ad', 1, 1]);
  eq(ap(st('abc', 1), 'ArrowLeft'), ['abc', 0, 0]);
  eq(ap(st('abc', 1), 'ArrowRight'), ['abc', 2, 2]);
  eq(ap(st('abcd', 1, 3), 'ArrowLeft'), ['abcd', 1, 1]);
  eq(ap(st('abcd', 1, 3), 'ArrowRight'), ['abcd', 3, 3]);
  eq(ap(st('abc', 1), 'ArrowRight', true), ['abc', 1, 2]);
  eq(ap(st('abc', 1), 'ArrowLeft', true), ['abc', 0, 1]);
  eq(ap(st('abc', 1, 2, 'backward'), 'ArrowRight', true), ['abc', 2, 2]); // focus at 1 moves to 2, anchor 2
  eq(ap(st('ab\ncd', 4), 'Home'), ['ab\ncd', 3, 3]);
  eq(ap(st('ab\ncd', 3), 'End'), ['ab\ncd', 5, 5]);
  eq(ap(st('ab\ncd', 1), 'End', true), ['ab\ncd', 1, 2]);
  eq(ap(st('ab\ncd', 4), 'ArrowUp'), ['ab\ncd', 1, 1]);
  eq(ap(st('abcd\nx', 3), 'ArrowDown'), ['abcd\nx', 6, 6]);
  eq(ap(st('abcd\nxy', 2), 'ArrowDown'), ['abcd\nxy', 7, 7]);
  eq(ap(st('ab\ncd', 1), 'ArrowUp'), ['ab\ncd', 0, 0]);
  eq(ap(st('ab\ncd', 4), 'ArrowDown'), ['ab\ncd', 5, 5]);

  const mkBox = (value, s, e) => {
    const b = new El('textarea'); b.value = value; b.selectionStart = s; b.selectionEnd = e === undefined ? s : e; b.selectionDirection = 'none';
    b.h = {}; b.inputs = 0;
    b.addEventListener = (n, f) => { b.h[n] = f; };
    b.setSelectionRange = (x, y, d) => { b.selectionStart = x; b.selectionEnd = y; b.selectionDirection = d; };
    b.dispatchEvent = () => { b.inputs++; };
    return b;
  };
  const run = (b, ev, native) => {
    const frames = [];
    A.wireEditKeys(b, () => { b.inputs++; }, f => frames.push(f));
    b.h.keydown(Object.assign({ ctrlKey: false, altKey: false, metaKey: false }, ev));
    if (native) native(b);
    frames.forEach(f => f());
  };
  // dropped Backspace: applied once, with an input event
  let b = mkBox('abc', 2); run(b, { key: 'Backspace' });
  assert.strictEqual(b.value, 'ac'); assert.strictEqual(b.selectionStart, 1); assert.strictEqual(b.inputs, 1);
  // native Backspace (value changed, beforeinput seen): nothing more
  b = mkBox('abc', 2); run(b, { key: 'Backspace' }, x => { x.h.beforeinput({}); x.value = 'ac'; x.selectionStart = x.selectionEnd = 1; });
  assert.strictEqual(b.value, 'ac'); assert.strictEqual(b.inputs, 0);
  // native arrow (caret moved): nothing more
  b = mkBox('abc', 2); run(b, { key: 'ArrowLeft' }, x => { x.selectionStart = x.selectionEnd = 1; });
  assert.strictEqual(b.selectionStart, 1);
  // dropped Shift+Left selects; a Ctrl chord and other keys are ignored
  b = mkBox('abc', 2); run(b, { key: 'ArrowLeft', shiftKey: true });
  assert.deepStrictEqual([b.selectionStart, b.selectionEnd, b.selectionDirection], [1, 2, 'backward']); assert.strictEqual(b.inputs, 0);
  b = mkBox('abc', 2); run(b, { key: 'Backspace', ctrlKey: true }); assert.strictEqual(b.value, 'abc');
  b = mkBox('abc', 2); run(b, { key: 'x' }); assert.strictEqual(b.value, 'abc');
  console.log('edit keys ok');
}
