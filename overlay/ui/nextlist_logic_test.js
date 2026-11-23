// Driven by nextlist_test.go under Node: the Next list's entries from structured fields.
const assert = require('assert');
const { difficulty, describe, render } = require('./nextlist.js');

const flat = rows => rows.map(r => r.parts.map(p => p.text).join(''));

// Pick-up of a class quest: title with level, tag, and the giver, in full.
const long = 'The Skybreaker Bulwark of the Eternal Vigil and Other Very Long Names';
let rows = describe({ text: 'Class quest: pick up The Skybreaker Bulwark from Seena Sk', kind: 'pickup', title: 'The Skybreaker Bulwark', level: 10, class: true, npc: 'Seena Skybreaker' }, 10);
assert.deepStrictEqual(flat(rows), ['[10] The Skybreaker Bulwark (Class)', 'Pick up from Seena Skybreaker']);
assert.strictEqual(rows[0].parts[0].cls, 'diff-yellow');

// No line is clipped, however long.
rows = describe({ text: 'x', kind: 'pickup', title: long, level: 12, elite: true, npc: 'N'.repeat(80) }, 12);
for (const t of flat(rows)) assert(!t.includes('...') && !t.includes('…'), t);
assert(flat(rows)[0].endsWith(long + ' (Elite)'));

// Progress: objective rows with counts, optional marked, done dimmed.
rows = describe({ text: 't', kind: 'progress', title: 'Crawler Trouble', level: 8, objectives: [
  { text: 'Windsong Crawler Meat', have: 2, need: 6, has_count: true },
  { text: 'Scout the ridge', have: 1, need: 1, has_count: true },
  { text: 'Find the lost charm', optional: true },
] }, 8);
assert.deepStrictEqual(flat(rows), ['[8] Crawler Trouble', '2/6 Windsong Crawler Meat', '1/1 Scout the ridge', 'Find the lost charm (optional)']);
assert(!rows[1].cls.includes('nx-done') && rows[2].cls.includes('nx-done'));

// Turn-in.
rows = describe({ text: 't', kind: 'turnin', title: 'Done Deal', level: 9, npc: 'Brann' }, 9);
assert.deepStrictEqual(flat(rows), ['[9] Done Deal', 'Ready to turn in', 'Turn in to Brann']);

// A turn-in that unlocks a follow-up says so in one quiet row.
rows = describe({ text: 't', kind: 'turnin', title: 'Done Deal', level: 9, npc: 'Brann', unlocks: ['Next Deal (from Brann, here)'] }, 9);
assert.deepStrictEqual(flat(rows).slice(3), ['Unlocks: Next Deal (from Brann, here)']);

// Talent entry.
assert.deepStrictEqual(flat(describe({ text: 'x', kind: 'talent', points: 3 }, 10)), ['3 unspent talent points']);
assert.deepStrictEqual(flat(describe({ text: 'x', kind: 'talent', points: 1 }, 10)), ['1 unspent talent point']);

// Difficulty against the character's level.
assert.deepStrictEqual([15, 13, 12, 10, 8, 3, 1].map(l => difficulty(l, 10)), ['diff-red', 'diff-orange', 'diff-yellow', 'diff-yellow', 'diff-yellow', 'diff-green', 'diff-grey']);

// Fallback: text only, no structure, or a plain string.
assert.deepStrictEqual(flat(describe({ text: 'Full text line' }, 10)), ['Full text line']);
assert.deepStrictEqual(flat(describe('Old advisor line', 10)), ['Old advisor line']);
assert.deepStrictEqual(flat(describe({ text: 'Fallback text', kind: 'pickup' }, 10)), ['Fallback text']);

// render builds one item with a row per described row.
const mk = () => ({ children: [], className: '', textContent: '', append(c) { this.children.push(c); } });
const li = render({ createElement: mk }, { text: 't', kind: 'turnin', title: 'A', level: 5, npc: 'B' }, 5);
assert.strictEqual(li.children.length, 3);
assert.strictEqual(li.children[2].children[0].textContent, 'Turn in to B');
console.log('ok');
