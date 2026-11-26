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
assert.deepStrictEqual(v.actions.map(a => a.label), ['Install', 'What’s new', 'Later']);
// One row: three short buttons fit the default 340 px window (about 7 px a letter at 11 px, 16 px padding, 6 px gaps).
const rowPx = v.actions.reduce((n, a) => n + a.label.length * 7 + 16, 0) + 6 * (v.actions.length - 1);
assert.ok(v.actions.length <= 3 && rowPx <= 340 - 16 - 18, 'the buttons fit one row at the default width: ' + rowPx);
assert.strictEqual(v.always, true, 'Always update automatically is a line under the buttons');
// With the setting already on, the button is gone.
assert.deepStrictEqual(U.bannerView(U.bannerState(offer, cfg({ autoUpdate: true }), NOW, '', '')).actions.map(a => a.id), ['install', 'whatsnew', 'later']);
assert.strictEqual(U.bannerView(U.bannerState(offer, cfg({ autoUpdate: true }), NOW, '', '')).always, false);
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
assert.deepStrictEqual(U.updatedNote('0.7.54', '0.7.55'), { kind: 'up', version: '0.7.55', from: '0.7.54', text: '✓ Updated: app 0.7.55', addon: '', whatsNew: true });
assert.deepStrictEqual(U.updatedNote('0.7.55', '0.7.54'), { kind: 'back', version: '0.7.54', text: '✓ Went back to 0.7.54', whatsNew: false });
assert.strictEqual(U.updatedNote('0.7.55', '0.7.54', 'it loses plans.').text, '✓ Went back to 0.7.54: it loses plans');
assert.strictEqual(U.updatedNote('0.7.54', '0.7.55', 'ignored').text, '✓ Updated: app 0.7.55');
assert.strictEqual(U.updatedNote('0.7.73', '0.7.74', '', '0.6.46').text, '✓ Updated: app 0.7.74 and addon 0.6.46');
assert.strictEqual(U.updatedNote('0.7.74', '0.7.74', '', '0.6.46').text, '✓ Updated: addon 0.6.46');
const RELOAD = 'A /reload in game is required to load the new addon. Type /reload now if WoW is open.';
const RESTART = "This addon update adds new files: fully exit and restart WoW to load it (/reload isn't enough).";
assert.strictEqual(U.updatedNote('0.7.74', '0.7.74', '', '0.6.46').reload, RELOAD);
assert.strictEqual(U.updatedNote('0.7.74', '0.7.74', '', '0.6.46', true).reload, RESTART);
assert.strictEqual(U.updatedNote('0.7.73', '0.7.74', '', '0.6.46').reload, RELOAD);
assert.strictEqual(U.updatedNote('0.7.73', '0.7.74', '', '0.6.46', true).reload, RESTART);
assert.strictEqual(U.updatedNote('0.7.73', '0.7.74', '', '', true).reload, undefined);
assert.strictEqual(U.updatedNote('0.7.73', '0.7.74').reload, undefined);
assert.strictEqual(U.updatedNote('0.7.55', '0.7.54').reload, undefined);
assert.strictEqual(U.addonDoneNote('0.6.54', 'x').text, '✓ Updated: addon 0.6.54');
assert.strictEqual(U.addonDoneNote('unknown', 'Addon installed').reload, RELOAD);
assert.strictEqual(U.addonDoneNote('0.6.54', '', true).reload, RESTART);
assert.strictEqual(U.reloadDone(U.addonDoneNote('0.6.54', '', true), { addon: '0.6.54', savedAt: 200 }, 100), true);
{ // the reload line clears once a save shows the new version
  const n = U.addonDoneNote('0.6.54', '');
  assert.strictEqual(U.reloadDone(n, { addon: '0.6.53', savedAt: 200 }, 100), false);
  assert.strictEqual(U.reloadDone(n, { addon: '0.6.54', savedAt: 200 }, 100), true);
  assert.strictEqual(U.reloadDone(n, { addon: '0.6.55', savedAt: 200 }, 100), true);
  // no version recorded: the next write after the update clears it
  assert.strictEqual(U.reloadDone(n, { addon: '', savedAt: 100 }, 100), false);
  assert.strictEqual(U.reloadDone(n, { addon: '', savedAt: 101 }, 100), true);
  assert.strictEqual(U.reloadDone(U.updatedNote('0.7.54', '0.7.55'), { addon: '', savedAt: 300 }, 100), false);
}
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
assert.strictEqual(U.CHANGELOG_URL, 'https://github.com/WoW-Companion/advisor/releases');

// The addon's part of the notice: alone, together with an app update, and the compatibility line.
{
  const ad = { installed: '1.0.0', available: true, latest: '1.1.0', notes: '## 1.1.0\n- Fixed a thing', auto: 'install' };
  let s = U.bannerState(null, cfg(), NOW, '', '', ad);
  assert.strictEqual(s.kind, 'addon');
  let v = U.bannerView(s);
  assert.strictEqual(v.text, 'Addon 1.1.0 is ready');
  assert.deepStrictEqual(v.actions.map(a => a.id), ['install', 'whatsnew', 'later']);
  s = U.bannerState(offer, cfg(), NOW, '', '', ad);
  assert.strictEqual(s.kind, 'offer', 'one notice covers both');
  assert.strictEqual(U.bannerView(s).text, 'Update 0.7.54 and addon 1.1.0 are ready');
  assert.deepStrictEqual(U.noteView(s.notes).groups.map(g => g.version), ['0.7.54', 'Addon 1.1.0']);
  assert.strictEqual(U.bannerState(null, cfg({ checkUpdates: false }), NOW, '', '', ad), null);
  const need = U.bannerView(U.bannerState(null, cfg(), NOW, '', '', { ...ad, installed: '0.1.0', needs_newer: true, min: '0.6.0' })).text;
  assert.ok(/needs a newer addon/.test(need) && need.includes('0.1.0') && need.includes('1.1.0'), need);
  const back = U.bannerView(U.bannerState(null, cfg(), NOW, '', '', { ...ad, reinstall: true, reason: 'crashes.' })).text;
  assert.ok(back.includes('known problem: crashes. Version 1.1.0'), back);
  // Later hides the addon too, and keeps both keys.
  const c = cfg();
  U.later(c, '0.7.54', NOW, ['addon:1.1.0']);
  assert.strictEqual(U.bannerState(offer, c, NOW, '', '', ad), null);
  assert.ok(U.bannerState(offer, c, NOW + U.LATER_MS + 1, '', '', ad));
}

// Settings: the installed version, the update and the versions kept.
{
  const w = U.addonView({ installed: '1.1.0', available: true, latest: '1.2.0', back: ['1.0.0'] });
  assert.strictEqual(w.text, 'Addon 1.1.0: 1.2.0 is ready');
  assert.strictEqual(w.button, 'Update addon');
  assert.strictEqual(w.back[0].button, 'Go back to 1.0.0');
  assert.strictEqual(U.addonView({ message: 'Choose the WoW folder.' }).text, 'Choose the WoW folder.');
}

// The automatic install follows the app's switch and waits for quiet; a pulled version goes back.
{
  const ad = { installed: '1.0.0', available: true, latest: '1.1.0', auto: 'install' };
  assert.strictEqual(U.addonAuto(ad, cfg(), NOW, true), '', 'off by default');
  assert.strictEqual(U.addonAuto(ad, cfg({ autoUpdate: true }), NOW, false), '', 'not while an answer is written');
  assert.strictEqual(U.addonAuto(ad, cfg({ autoUpdate: true }), NOW, true), 'install');
  assert.strictEqual(U.addonAuto({ ...ad, auto: 'goback', reinstall: true }, cfg({ autoUpdate: true, later: { 'addon:1.1.0': NOW + 5 } }), NOW, true), 'goback');
  assert.strictEqual(U.addonAuto(ad, cfg({ autoUpdate: true, later: { 'addon:1.1.0': NOW + 5 } }), NOW, true), '', 'Later holds back a plain update');
}

// Every update notice carries the reload or restart line and a What's new button: the auto and manual
// addon installs, a go-back, an app update with a bundled addon, and an addon-only start.
{
  const ONLY = x => x.reload === RELOAD || x.reload === RESTART;
  const auto = U.addonDoneNote('0.6.67', 'Addon 0.6.67 is installed.', false); // what the auto-install draws
  assert.strictEqual(auto.text, '✓ Updated: addon 0.6.67');
  assert.strictEqual(auto.reload, RELOAD);
  assert.strictEqual(U.addonDoneNote('0.6.67', 'x', true).reload, RESTART, 'restart wording when the update adds files');
  const back = U.addonDoneNote('', 'Went back to addon 0.6.50. Type /reload.', false);
  assert.ok(ONLY(back) && back.whatsNew && back.text.startsWith('✓ Went back to addon'), 'a go-back keeps the line');
  const both = U.updatedNote('0.7.73', '0.7.74', '', '0.6.46', true);
  assert.strictEqual(both.reload, RESTART);
  assert.strictEqual(U.updatedNote('0.7.74', '0.7.74', '', '0.6.46').reload, RELOAD);
  for (const n of [auto, back, both, U.updatedNote('0.7.74', '0.7.74', '', '0.6.46'), U.updatedNote('0.7.73', '0.7.74')]) assert.strictEqual(n.whatsNew, true);
  // The notice is redrawn in place: the later install is the whole text, nothing stacks.
  const later = U.addonDoneNote('0.6.68', '', false);
  assert.strictEqual(later.text, '✓ Updated: addon 0.6.68');
  // A restart notice is not cleared by a save that only proves a /reload.
  assert.strictEqual(U.reloadDone(U.addonDoneNote('0.6.67', '', true), { addon: '', savedAt: 500 }, 100), false);
  // A save time that could not be read at install time is not a base to beat.
  assert.strictEqual(U.reloadDone(U.addonDoneNote('0.6.67', '', false), { addon: '', savedAt: 500 }, null), false);
}

// Notes since last viewed.
{
  const c = cfg();
  // First run: seeded with what is installed, so nothing is listed.
  assert.strictEqual(U.seedNotesSeen(c, '0.7.94', '0.6.67'), true);
  assert.deepStrictEqual(c.notesSeen, { app: '0.7.94', addon: '0.6.67' });
  assert.strictEqual(U.seedNotesSeen(c, '0.7.99', '0.6.70'), false, 'a later start does not move it');
  assert.strictEqual(U.notesQuery(c, '0.7.94', '0.6.67'), '?app_from=0.7.94&app=0.7.94&addon_from=0.6.67&addon=0.6.67');
  assert.ok(U.notesView({ app: { entries: [] }, addon: { entries: [] } }).empty, 'first run shows nothing');
  // Two app and three addon versions later, the range runs from the last seen.
  assert.strictEqual(U.notesQuery(c, '0.7.96', '0.6.70'), '?app_from=0.7.94&app=0.7.96&addon_from=0.6.67&addon=0.6.70');
  const v = U.notesView({
    app: { entries: [{ version: '0.7.96', lines: ['Fixed a.'] }, { version: '0.7.95', lines: ['Added b.', 'Fixed c.'] }], more: false },
    addon: { entries: [{ version: '0.6.70', lines: ['x'] }, { version: '0.6.69', lines: ['y'] }, { version: '0.6.68', lines: ['z'] }], more: false },
  });
  assert.deepStrictEqual(v.sections.map(s => [s.component, s.entries.map(e => e.version)]), [['App', ['0.7.96', '0.7.95']], ['Addon', ['0.6.70', '0.6.69', '0.6.68']]]);
  assert.strictEqual(v.more, null);
  // Opening marks the installed versions seen; the next view is empty.
  assert.strictEqual(U.openNotes(c, '0.7.96', '0.6.70'), true);
  assert.deepStrictEqual(c.notesSeen, { app: '0.7.96', addon: '0.6.70' });
  assert.strictEqual(U.openNotes(c, '0.7.96', 'unknown'), false, 'an unknown addon version keeps its mark');
  // The cap: when the server left versions out, the last line points at the release page.
  const capped = U.notesView({ app: { entries: [{ version: '0.7.96', lines: ['a'] }], more: true }, addon: { entries: [] } });
  assert.deepStrictEqual(capped.more, { text: 'Older changes: see the release page', url: U.CHANGELOG_URL });
}
