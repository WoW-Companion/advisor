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
assert(key('Enter', false) && submitted === 1 && prevented === 1);
assert(!key('Enter', true) && submitted === 1 && prevented === 1);
assert(!key('a', false));

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

// Wording: a short question, the game mark, no reload text, no preamble.
assert.strictEqual(A.shortQuestion("I'm a level 12 priest. Where should my talent point go?"), 'Where should my talent point go?');
assert.strictEqual(A.shortQuestion('Where should my talent point go?'), 'Where should my talent point go?');
assert.strictEqual(A.gameMark, 'from game');
assert.strictEqual(A.savedNote({ source: 'game', status: 'done' }, false), '');
assert.strictEqual(A.savedNote({ source: 'web', status: 'done' }, true), '');
assert.strictEqual(A.savedNote({ source: 'game', status: 'done' }, true), 'Also in game after your next login');
const shown = [ul.textContent, card.textContent, A.NEEDS_LINE, A.gameMark, A.savedNote({ source: 'game', status: 'done' }, true), A.shortQuestion("I'm a level 12 priest. Hi?")].join('\n');
for (const bad of ['/reload', 'Answers.lua', "I'm a level"]) assert(!shown.includes(bad), bad);
console.log('ok');
