// Driven by updates_test.go under Node: the banner states, the offer's switches as the page sends
// them, the notes text, the version list and the check schedule, with no DOM.
const assert = require('assert');
const U = require('./updates.js');

const NOW = 1_000_000_000_000;
const cfg = (o = {}) => ({ checkUpdates: true, updateNow: false, skipped: [], later: {}, ...o });
const offer = { available: true, latest: '0.7.54', notes: '## 0.7.54\n- **Fixed** a thing' };

// The query carries only the player's switches.
assert.strictEqual(U.updateQuery(cfg(), false), '');
assert.strictEqual(U.updateQuery(cfg({ updateNow: true, skipped: ['v0.7.52', '0.7.53'] }), true), '?now=1&skip=0.7.52%2C0.7.53&fresh=1');

// Banner states.
let s = U.bannerState(offer, cfg(), NOW, '', '');
assert.deepStrictEqual([s.kind, s.version], ['offer', '0.7.54']);
let v = U.bannerView(s);
assert.strictEqual(v.text, 'Update 0.7.54 is ready');
assert.deepStrictEqual(v.actions.map(a => a.label), ['Install', 'Always update automatically', 'Later', 'Skip this version']);
assert.strictEqual(v.actions.find(a => a.label === 'Always update automatically').id, 'always');
// With the setting already on, the button is gone.
assert.deepStrictEqual(U.bannerView(U.bannerState(offer, cfg({ autoUpdate: true }), NOW, '', '')).actions.map(a => a.id), ['install', 'later', 'skip']);
// The button turns the setting on, and the shell is told with the banner's own switches.
const on = cfg({ updateNow: true, skipped: ['v0.7.52'] });
assert.strictEqual(U.turnOnAuto(on).autoUpdate, true);
assert.deepStrictEqual(U.autoArgs(on), { enabled: true, updateNow: true, skipped: ['0.7.52'] });
assert.strictEqual(U.autoArgs(cfg()).enabled, false, 'off by default');
assert.strictEqual(U.autoArgs(cfg({ autoUpdate: true, checkUpdates: false })).enabled, false, 'no checks, no installs');
assert.strictEqual(v.cls, 'ok');
assert.strictEqual(U.bannerState(offer, cfg({ checkUpdates: false }), NOW, '', ''), null, 'the master switch hides it');
assert.strictEqual(U.bannerState(null, cfg(), NOW, '', ''), null);
assert.strictEqual(U.bannerState({ available: false }, cfg(), NOW, '', ''), null);
assert.strictEqual(U.bannerState(offer, cfg({ skipped: ['0.7.54'] }), NOW, '', ''), null, 'skipped');
assert.notStrictEqual(U.bannerState(offer, cfg({ skipped: ['0.7.53'] }), NOW, '', ''), null, 'skipping another version does not hide it');

// Later hides for 24 hours for that version, then it returns; a newer version shows at once.
const c = cfg();
U.later(c, 'v0.7.54', NOW);
assert.strictEqual(c.later['0.7.54'], NOW + 24 * 3600 * 1000);
assert.strictEqual(U.bannerState(offer, c, NOW + 3600 * 1000, '', ''), null);
assert.notStrictEqual(U.bannerState(offer, c, NOW + 24 * 3600 * 1000 + 1, '', ''), null);
assert.notStrictEqual(U.bannerState({ ...offer, latest: '0.7.55' }, c, NOW + 1000, '', ''), null);
U.skip(c, '0.7.54'); U.skip(c, 'v0.7.54');
assert.deepStrictEqual(c.skipped, ['0.7.54']);

// Installing and failing.
s = U.bannerState(offer, cfg(), NOW, 'Downloading 0.7.54…', '');
assert.deepStrictEqual([U.bannerView(s).text, U.bannerView(s).actions.length], ['Downloading 0.7.54…', 0]);
s = U.bannerState(offer, cfg(), NOW, '', 'The download did not match its checksum.');
v = U.bannerView(s);
assert.ok(v.text.includes('did not install') && v.actions[0].id === 'retry');
s = U.bannerState({ available: false }, cfg(), NOW, '', 'no network');
assert.strictEqual(s.kind, 'task');
assert.deepStrictEqual(U.bannerView(s).actions.map(a => a.id), ['retry', 'dismiss']);

// A pulled running version: red, with the reason and the way back; it ignores Later and Skip.
const pulled = { available: true, latest: '0.7.54', pulled: { version: '0.7.52', reason: 'it crashes on login.', back_to: '0.7.51' } };
s = U.bannerState(pulled, cfg({ skipped: ['0.7.54'], later: { '0.7.54': NOW + 1e9 } }), NOW, '', '');
v = U.bannerView(s);
assert.strictEqual(v.cls, 'bad');
assert.strictEqual(v.text, 'This version has a known problem: it crashes on login.');
assert.deepStrictEqual(v.actions.map(a => a.label), ['Go back to 0.7.51', 'Details']);
v = U.bannerView(U.bannerState({ pulled: { version: '0.7.52', reason: 'bad', back_to: '' } }, cfg(), NOW, '', ''));
assert.deepStrictEqual(v.actions.map(a => a.id), ['details'], 'no earlier version to go back to');

// Release notes as plain text.
assert.strictEqual(U.plainNotes(offer.notes), '0.7.54\n• Fixed a thing');
assert.strictEqual(U.plainNotes(''), 'No notes were published for this version.');
assert.strictEqual(U.plainNotes('See [the page](http://x.y/z)\r\n\r\n\r\n\r\n- `a`'), 'See the page\n\n• a');

// The version list: newest first, running in place, Update to / Go back to, pulled never listed.
const rows = U.versionRows({ versions: [{ version: '0.7.53', date: '2026-11-24', current: true }, { version: '0.7.9' }, { version: '0.7.55', date: '2026-11-25' }, { version: '0.7.52', date: '2026-11-20' }] });
assert.deepStrictEqual(rows.map(r => r.version), ['0.7.55', '0.7.53', '0.7.52', '0.7.9']);
assert.deepStrictEqual(rows.map(r => r.button), ['Update to 0.7.55', '', 'Go back to 0.7.52', 'Go back to 0.7.9']);
assert.deepStrictEqual(rows.map(r => r.primary), [true, false, false, false]);
assert.strictEqual(rows[1].label, '0.7.53 (running) · 2026-11-24');
assert.ok(rows[1].current);
assert.strictEqual(rows[2].confirm, 'Go back to 0.7.52? Your settings from that version are restored.');
assert.strictEqual(rows[0].confirm, '');
assert.deepStrictEqual(U.versionRows(null), []);

// The notice at the top of Updates.
const at = new Date(2026, 10, 25, 9, 5).getTime();
assert.deepStrictEqual(U.noticeView({ available: true, latest: 'v0.7.55' }, at), { cls: 'ok', text: 'Update 0.7.55 is ready', button: 'Update now', version: '0.7.55' });
assert.strictEqual(U.noticeView({ available: false }, at).text, 'You’re up to date (checked 09:05)');
assert.strictEqual(U.noticeView({ available: false }, at).button, '');
assert.strictEqual(U.noticeView(null, at), null);

// The note after a relaunch: upgrade, go-back, first ever start and no change.
assert.deepStrictEqual(U.updatedNote('0.7.54', '0.7.55'), { kind: 'up', version: '0.7.55', from: '0.7.54', text: '✓ Updated to 0.7.55', whatsNew: true });
assert.deepStrictEqual(U.updatedNote('0.7.55', '0.7.54'), { kind: 'back', version: '0.7.54', text: '✓ Went back to 0.7.54', whatsNew: false });
assert.strictEqual(U.updatedNote('0.7.55', '0.7.54', 'it loses plans.').text, '✓ Went back to 0.7.54: it loses plans');
assert.strictEqual(U.updatedNote('0.7.54', '0.7.55', 'ignored').text, '✓ Updated to 0.7.55');
assert.strictEqual(U.updatedNote('', '0.7.55'), null);
assert.strictEqual(U.updatedNote('0.7.55', '0.7.55'), null);
assert.strictEqual(U.updatedNote('0.7.9', '0.7.10').kind, 'up');

// The schedule: a late tick is a wake-up; focus checks only after 4 hours.
let t = NOW, checks = 0;
const sch = U.createSchedule({ now: () => t, check: () => checks++ });
t += U.TICK; assert.strictEqual(sch.tick(), ''); assert.strictEqual(checks, 0);
t += 2 * 3600 * 1000; assert.strictEqual(sch.tick(), 'wake'); assert.strictEqual(checks, 1);
assert.strictEqual(sch.focus(), false);
t += U.CHECK_EVERY + 1; assert.strictEqual(sch.focus(), true); assert.strictEqual(checks, 2);
sch.checked(); assert.strictEqual(sch.focus(), false);
assert.strictEqual(U.CHECK_EVERY, 4 * 3600 * 1000);
console.log('ok');

// The note after an update knows the version it left, "See all changes" is the notice's link,
// and a go-back shows no what's new.
assert.strictEqual(U.updatedNote('0.7.55', '0.7.58').from, '0.7.55');
assert.strictEqual(U.whatsNewQuery(U.updatedNote('0.7.55', '0.7.58')), '?version=0.7.58&from=0.7.55');
assert.strictEqual(U.updatedNote('0.7.58', '0.7.55').whatsNew, false);
assert.strictEqual(U.updatedNote('0.7.58', '0.7.55').text, '✓ Went back to 0.7.55');
assert.deepStrictEqual(U.bannerView(U.bannerState(offer, cfg(), NOW, '', '')).actions.map(a => a.id).slice(0, 1), ['install']);

// Notes as bullets: the notice shows at most 4 and says there is more; nothing is rewritten.
const six = '- a one\n- b **two**\n- c\n- d\n- e\n- f';
let nv = U.noteView(six, U.NOTICE_BULLETS);
assert.strictEqual(U.NOTICE_BULLETS, 4);
assert.deepStrictEqual(nv, { groups: [{ version: '', bullets: ['a one', 'b two', 'c', 'd'] }], more: true });
nv = U.noteView('- a\n- b\n- c\n- d', 4);
assert.strictEqual(nv.more, false);
assert.strictEqual(nv.groups[0].bullets.length, 4);
// What's new keeps its versions and the server's tail; the cap only applies when given.
nv = U.noteView('## 0.7.62\n- a\n- b\n## 0.7.61\n- c\n…and 3 more changes');
assert.deepStrictEqual(nv, { groups: [{ version: '0.7.62', bullets: ['a', 'b'] }, { version: '0.7.61', bullets: ['c'] }], more: true });
assert.deepStrictEqual(U.noteView(''), { groups: [], more: false });
assert.ok(/^https:\/\/github\.com\/WoW-Companion\/advisor\/.*CHANGELOG\.md$/.test(U.CHANGELOG_URL));
