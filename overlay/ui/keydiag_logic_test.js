// Driven by keydiag_test.go under Node: the keyboard test records and caps its lines.
const assert = require('assert');
const K = require('./keydiag.js');

assert.strictEqual(K.describe({ type: 'keydown', key: 'ArrowLeft', code: 'ArrowLeft', defaultPrevented: false }), 'keydown key=ArrowLeft code=ArrowLeft');
assert.ok(K.describe({ type: 'keydown', key: 'Backspace', code: 'Backspace', defaultPrevented: true }).endsWith('PREVENTED'));
assert.strictEqual(K.describe({ type: 'beforeinput', inputType: 'deleteContentBackward' }), 'beforeinput deleteContentBackward');
assert.strictEqual(K.describe({ type: 'paste', clipboardData: { types: ['text/plain', 'text/html'] } }), 'paste types=text/plain,text/html');
assert.strictEqual(K.describe({ type: 'paste' }), 'paste types=none');
assert.strictEqual(K.describe({ type: 'click' }), '');

let seen = null;
const l = K.log(ls => { seen = ls; });
for (let i = 0; i < 12; i++) l.record({ type: 'keydown', key: 'k' + i, code: 'K' + i });
l.record({ type: 'mousemove' });
assert.strictEqual(l.lines.length, 8);
assert.ok(l.lines[0].includes('k4') && l.lines[7].includes('k11'));
assert.strictEqual(seen.length, 8);

const handlers = {};
K.attach({ addEventListener: (t, f, cap) => { assert.strictEqual(cap, true); handlers[t] = f; } }, l);
assert.deepStrictEqual(Object.keys(handlers).sort(), ['beforeinput', 'keydown', 'paste']);
handlers.keydown({ type: 'keydown', key: 'Delete', code: 'Delete' });
assert.ok(l.lines[7].includes('Delete'));
console.log('keydiag ok');
