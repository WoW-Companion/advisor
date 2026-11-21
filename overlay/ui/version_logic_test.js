// Driven by version_test.go under Node: the version line and the update check, with fakes.
const assert = require('assert');
const { createVersionLine, START_DELAY, DAY_HALF } = require('./version.js');

function rig({ fetchUpdate, enabled = true } = {}) {
  const timers = [], intervals = [], calls = { fetch: 0, open: 0 };
  const el = { textContent: '', parts: [], ownerDocument: { createElement: () => ({}) }, append(x) { this.parts.push(x); } };
  const st = { conn: null };
  const v = createVersionLine({
    el, getConn: () => st.conn, enabled: () => enabled,
    fetchUpdate: () => { calls.fetch++; return fetchUpdate(); }, openPage: () => calls.open++,
    setTimeout: (f, ms) => timers.push({ f, ms }), setInterval: (f, ms) => intervals.push({ f, ms }),
  });
  return { v, el, st, timers, intervals, calls };
}

(async () => {
  // Before the handshake the advisor part is a placeholder; the handshake fills it in.
  let r = rig({ fetchUpdate: async () => ({}) });
  r.v.setAppVersion('0.2.0');
  assert.strictEqual(r.el.textContent, 'WoW Companion 0.2.0 · advisor …');
  r.st.conn = { version: '0.2.0' }; r.v.onConnect();
  assert.strictEqual(r.el.textContent, 'WoW Companion 0.2.0 · advisor 0.2.0');
  // The handshake can land before the app version too.
  r = rig({ fetchUpdate: async () => ({}) });
  r.st.conn = { version: '0.2.0' }; r.v.onConnect(); r.v.setAppVersion('0.2.0');
  assert.strictEqual(r.el.textContent, 'WoW Companion 0.2.0 · advisor 0.2.0');
  // A restart blanks the advisor part, and its new handshake re-renders it.
  r.st.conn = null; r.v.onDisconnect();
  assert.strictEqual(r.el.textContent, 'WoW Companion 0.2.0 · advisor …');
  r.st.conn = { version: '0.3.0' }; r.v.onConnect();
  assert.strictEqual(r.el.textContent, 'WoW Companion 0.2.0 · advisor 0.3.0');

  // The check is scheduled once after the first handshake, inside 10 s, then on a 12-hour timer.
  r = rig({ fetchUpdate: async () => ({ available: true, latest: 'v0.3.0' }) });
  r.v.setAppVersion('0.2.0'); r.v.startTimer();
  assert.strictEqual(r.timers.length, 0);
  r.st.conn = { version: '0.2.0' }; r.v.onConnect(); r.v.onConnect();
  assert.strictEqual(r.timers.length, 1);
  assert.ok(r.timers[0].ms <= 10000 && r.timers[0].ms === START_DELAY);
  await r.timers[0].f();
  assert.strictEqual(r.calls.fetch, 1);
  assert.ok(r.el.textContent.endsWith('· v0.3.0 ') || r.el.parts.join('').includes('v0.3.0'));
  assert.strictEqual(r.intervals.length, 1);
  assert.strictEqual(r.intervals[0].ms, DAY_HALF);
  await r.intervals[0].f();
  assert.strictEqual(r.calls.fetch, 2);
  // The timer with no sidecar does nothing.
  r.st.conn = null; await r.intervals[0].f();
  assert.strictEqual(r.calls.fetch, 2);

  // A failed lookup is silent: no notice, no throw. A turned-off check never asks.
  r = rig({ fetchUpdate: async () => { throw new Error('offline'); } });
  r.st.conn = { version: '0.2.0' }; r.v.setAppVersion('0.2.0'); r.v.onConnect();
  await r.timers[0].f();
  assert.strictEqual(r.el.textContent, 'WoW Companion 0.2.0 · advisor 0.2.0');
  assert.strictEqual(r.el.parts.length, 0);
  r = rig({ fetchUpdate: async () => ({}), enabled: false });
  r.st.conn = { version: '0.2.0' }; r.v.onConnect(); await r.timers[0].f();
  assert.strictEqual(r.calls.fetch, 0);
  console.log('ok');
})().catch(e => { console.error(e); process.exit(1); });
