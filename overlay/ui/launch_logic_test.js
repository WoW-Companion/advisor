// Driven by launch_test.go under Node: the saved chat, the old-ask card, suggestions, quick bar and dot.
const assert = require('assert');
const A = require('./asking.js');
const L = require('./launch.js');

class El {
  constructor(tag) { this.tag = tag; this.children = []; this.className = ''; this._text = ''; this.title = ''; this.disabled = false; }
  append(...c) { this.children.push(...c); }
  set textContent(t) { this._text = t; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
  setAttribute() {}
  find(cls) { return this.children.flatMap(c => [c, ...c.find(cls)]).filter(c => c.className.split(' ').includes(cls)); }
}
const doc = { createElement: t => new El(t) };
const mem = () => { const m = {}; return { getItem: k => (k in m ? m[k] : null), setItem: (k, v) => { m[k] = String(v); } }; };

// Saved chat: the last 50 turns per character, restored in order; Clear empties one character.
const store = L.chatStore(mem());
for (let i = 0; i < 60; i++) store.add('Aria', { id: 't' + i, q: 'q' + i, a: 'a' + i, at: 1000 + i });
store.add('Bram', { id: 'b', q: 'hi', a: 'yo', at: 5 });
store.add('Bram', { id: 'b', q: 'hi', a: 'yo again', at: 6 }); // the same id replaces, never doubles
const aria = store.load('Aria');
assert.strictEqual(aria.length, 50);
assert.strictEqual(aria[0].id, 't10'); assert.strictEqual(aria[49].id, 't59');
assert.deepStrictEqual(store.load('Bram').map(t => t.a), ['yo again']);
store.clear('Aria');
assert.strictEqual(store.load('Aria').length, 0); assert.strictEqual(store.load('Bram').length, 1);
assert.deepStrictEqual(L.chatStore({ getItem: () => '{broken', setItem() {} }).load('x'), [], 'a damaged store is an empty chat');

// The gap divider reads like a person would say it.
const now = new Date(2026, 10, 23, 9, 0).getTime();
const at = (d, h, m) => new Date(2026, 10, d, h, m).getTime() / 1000;
assert.strictEqual(L.gapLabel(at(22, 23, 58), now), 'Yesterday, 11:58 PM');
assert.strictEqual(L.gapLabel(at(23, 0, 5), now), 'Today, 12:05 AM');
assert.strictEqual(L.gapLabel(at(20, 15, 30), now), 'Nov 20, 3:30 PM');

// Suggestions: at most three, a dismissal hides one for that day only, the hover text is its why.
const list = ['a', 'b', 'c', 'd', 'e'].map(id => ({ id, label: 'L' + id, why: 'because ' + id, action: 'ask', question: 'q' + id }));
assert.strictEqual(L.visibleSuggestions(list, {}, 'Aria', '2026-11-23').length, 3);
const gone = { 'Aria|a': '2026-11-23' };
assert.deepStrictEqual(L.visibleSuggestions(list, gone, 'Aria', '2026-11-23').map(s => s.id), ['b', 'c', 'd']);
assert.strictEqual(L.visibleSuggestions(list, gone, 'Aria', '2026-11-24')[0].id, 'a', 'back the next day');
assert.strictEqual(L.visibleSuggestions(list, gone, 'Bram', '2026-11-23')[0].id, 'a', 'per character');
assert.deepStrictEqual(L.visibleSuggestions(undefined, {}, 'Aria', 'x'), []);
const picked = [], hid = [];
const row = L.suggestionRow(doc, list.slice(0, 3), s => picked.push(s.id), s => hid.push(s.id));
assert.strictEqual(row.find('sugg').length, 3);
assert.strictEqual(row.find('sugg-go')[1].title, 'because b');
row.find('sugg-go')[1].onclick(); row.find('sugg-x')[2].onclick();
assert.deepStrictEqual([picked, hid], [['b'], ['c']]);
assert.strictEqual(L.dayKey(new Date(2026, 10, 3)), '2026-11-03');

// Quick bar: five buttons, each one the right action; Refresh asks nothing.
const ch = { style: { role: 'damage', focus: 'leveling' }, entries: [
  { group: 'next', kind: 'turnin', title: 'T', ref: 'quest:5', asks: ['Where do I turn in T?'] },
  { group: 'talents', kind: 'talent', title: '2 points', ref: 'talents', asks: ['Where should my talent points go?'] },
  { group: 'upgrades', kind: 'upgrade', title: 'Axe', ref: 'item:9', asks: ['Is the Axe an upgrade?'] }] };
const ran = [];
const bar = L.quickBar(doc, id => ran.push(id));
assert.deepStrictEqual(bar.find('quick-lbl').map(e => e.textContent), ['What next?', 'Talents', 'Gear', 'Bags', 'Refresh']);
bar.children.forEach(b => b.onclick());
assert.deepStrictEqual(ran, ['next', 'talents', 'gear', 'bags', 'refresh']);
assert.deepStrictEqual(L.quickAction('next', ch), { act: 'ask', question: 'What should I do next, and why?', ref: 'quest:5' });
assert.deepStrictEqual(L.quickAction('talents', ch), { act: 'ask', question: 'Which talent should I take next for my play style, and why?', ref: 'talents' });
assert.deepStrictEqual(L.quickAction('talents', { entries: [] }), { act: 'play_style' }, 'no stored style: the card');
assert.deepStrictEqual(L.quickAction('gear', ch), { act: 'ask', question: 'Look at my gear and suggest the best upgrades within reach right now: quest rewards, things I can make, vendor buys and cheap Auction House finds. Start with the biggest gain.', ref: 'upgrades' });
assert.deepStrictEqual(L.quickAction('gear', { entries: [] }), L.quickAction('gear', ch), 'Gear is the same whatever the entries');
for (const id of ['next', 'talents', 'gear', 'bags']) {
  const a = L.quickAction(id, ch);
  if (a.act === 'ask') for (const e of ch.entries) for (const k of e.asks || []) assert.notStrictEqual(a.question, k, 'a quick button never sends a card ask');
}
assert.deepStrictEqual(L.quickAction('bags', ch), { act: 'ask', question: 'Look at my bags and bank: what can I sell, what should I keep, and what should go to my other characters?', ref: 'bags' });
assert.deepStrictEqual(L.quickAction('bags', { entries: [] }), L.quickAction('bags', ch), 'Bags is the same whatever the entries');
assert.deepStrictEqual(L.quickAction('refresh', ch), { act: 'refresh' });
assert.strictEqual(L.quickAction('refresh', ch).question, undefined, 'Refresh makes no AI call');

// Why lines render dim under their entry.
const ul = A.entryList(doc, [{ kind: 'turnin', title: 'Wolves', why: 'Quick XP and a new quest', asks: ['q'] }, { kind: 'note', title: 'No why' }], () => {});
assert.deepStrictEqual(ul.find('entry-why').map(e => e.textContent), ['Quick XP and a new quest']);

// The freshness dot: gold, amber past 12 hours, red past 3 days.
const nowS = now / 1000;
assert.strictEqual(L.freshness(nowS - 3600, now).cls, '');
assert.strictEqual(L.freshness(nowS - 13 * 3600, now).cls, 'amber');
assert.strictEqual(L.freshness(nowS - 4 * 86400, now).cls, 'red');
assert.strictEqual(L.freshness(nowS - 17 * 3600, now).tip, 'Last game save 17 hours ago. Your plan updates by itself whenever the game saves.');

// The plan summary never speaks in map units.
assert.strictEqual(L.planLine('23 stops, about 4120 map units'), '23 stops');
assert.strictEqual(L.planLine('23 stops'), '23 stops');
assert.strictEqual(L.planLine(undefined), '');
console.log('ok');

// Play Style saves: "Not set" is sent as empty, never dropped; a refusal gives its reason and the
// advisor is re-read either way.
(async () => {
  const sent = [], reads = [];
  const deps = res => ({ post: async b => { sent.push(b); return res; }, reread: async () => { reads.push(1); } });
  let r = await L.saveStyleFlow(deps({ ok: true }), 'Aria', { role: '', focus: '', spec: ' ', stat_pref: 'balanced' });
  assert.deepStrictEqual(r, { ok: true });
  assert.deepStrictEqual(sent[0], { character: 'Aria', role: '', focus: '', spec: '', stat_pref: 'balanced' });
  r = await L.saveStyleFlow(deps({ ok: true }), 'Aria', { role: 'tank', focus: '' });
  assert.strictEqual(sent[1].role, 'tank'); assert.strictEqual(sent[1].focus, '');
  r = await L.saveStyleFlow(deps({ ok: false, text: 'role must be one of damage\n' }), 'Aria', { role: 'x', focus: '' });
  assert.deepStrictEqual(r, { ok: false, error: 'Not saved: role must be one of damage' });
  r = await L.saveStyleFlow({ post: async () => { throw new Error('down'); }, reread: async () => { reads.push(1); } }, 'Aria', {});
  assert.strictEqual(r.ok, false); assert.ok(/^Not saved: /.test(r.error));
  assert.strictEqual(reads.length, 4);
  console.log('ok styles');
})().catch(e => { console.error(e); process.exit(1); });

// Following the played character: the newest capture wins unless a character is pinned or the setting is off.
{
  const chars = [{ key: 'Aria', name: 'Aria' }, { key: 'Bram', name: 'Bram' }];
  const f = o => L.followChoice({ characters: chars, newest: 'Bram', current: 'Aria', follow: true, pinned: '', ...o });
  const sw = f({});
  assert.strictEqual(sw.key, 'Bram'); assert.strictEqual(sw.switched, true);
  assert.ok(/Bram/.test(sw.line) && !/\n/.test(sw.line), 'one line, naming the character');
  assert.strictEqual(f({ pinned: 'Aria' }).key, 'Aria', 'a pinned character stops following');
  assert.strictEqual(f({ pinned: 'Aria' }).switched, false);
  assert.strictEqual(f({ follow: false }).key, 'Aria', 'the setting off stops following');
  assert.strictEqual(f({ current: 'Bram' }).switched, false, 'already on the newest: no switch, no line');
  assert.strictEqual(f({ current: '' }).key, 'Bram', 'nothing chosen yet: the newest, with no line');
  assert.strictEqual(f({ current: '' }).line, '');
  assert.strictEqual(f({ pinned: 'Gone' }).key, 'Bram', 'a pin on a character no longer captured is ignored');
}
