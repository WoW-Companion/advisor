// Driven by aiset_test.go under Node: the AI settings' key field, states, model lists and test messages.
const assert = require('assert');
const S = require('./aiset.js');

// Stand-in page pieces.
class El {
  constructor() { this.value = ''; this.hidden = false; this.textContent = ''; this.className = ''; this.disabled = false; this.options = []; }
  replaceChildren(...o) { this.options = o; }
}
const names = ['mode', 'cliRows', 'apiRows', 'provider', 'baseRow', 'base', 'localRow', 'localStatus', 'localLink', 'rescan', 'keyRow', 'keyBox', 'keyInput', 'keySave', 'keyCancel',
  'keySaved', 'keyStatus', 'keyReplace', 'keyRemove', 'keyLink', 'model', 'modelName', 'modelOtherRow', 'modelOther', 'note', 'costRows', 'cost', 'subscription', 'test', 'testOut', 'advanced', 'backup'];
const PRESETS = [
  { id: 'anthropic', name: 'Anthropic (Claude)', needs_key: true, local: false, has_key: false, models: { efficient: 'claude-haiku-5-5', balanced: 'claude-sonnet-5-5', best: 'claude-opus-5-5' } },
  { id: 'openrouter', name: 'OpenRouter', needs_key: true, local: false, has_key: false, models: { efficient: 'a/b-haiku', balanced: 'a/b-sonnet', best: 'a/b-opus' } },
  { id: 'custom', name: 'Other', needs_key: false, local: false, has_key: false, models: {} },
  { id: 'ollama', name: 'Ollama', needs_key: false, local: true, has_key: false, models: {} },
];
function rig(over = {}) {
  const els = {}; for (const n of names) els[n] = new El();
  const calls = [], opened = [];
  const server = { presets: JSON.parse(JSON.stringify(PRESETS)), clients: [], ollama: { ok: true, models: ['llama3', 'qwen3'] }, test: { ok: true }, ...over };
  const call = async (path, body) => {
    calls.push([path, body && JSON.parse(JSON.stringify(body))]);
    if (path === '/api/providers') return { presets: server.presets, clients: server.clients };
    if (path.startsWith('/api/providers/models')) return server.ollama;
    if (path === '/api/providers/key') { server.presets.find(p => p.id === body.provider).has_key = true; return { has_key: true }; }
    if (path === '/api/providers/key/delete') { server.presets.find(p => p.id === body.provider).has_key = false; return { has_key: false }; }
    if (path === '/api/providers/clients') { server.clients = body.clients; return { clients: body.clients }; }
    if (path === '/api/providers/test') return server.test;
    return {};
  };
  const ai = S.create(els, { call, openPage: u => opened.push(u), agent: () => 'claude', onChange() {} });
  return { els, calls, opened, server, ai };
}

(async () => {
  // Modes come from the saved list: nothing or a CLI is the AI app; an API provider is a key; a local one is free.
  assert.strictEqual(S.modeOf([], PRESETS), 'cli');
  assert.strictEqual(S.modeOf([{ kind: 'cli', agent: 'claude' }], PRESETS), 'cli');
  assert.strictEqual(S.modeOf([{ kind: 'api', provider: 'anthropic' }], PRESETS), 'key');
  assert.strictEqual(S.modeOf([{ kind: 'api', provider: 'ollama' }], PRESETS), 'local');

  // Model lists: tier presets with the real name beside them; the installed models for a local server; both end in Other.
  const tiers = S.modelOptions(PRESETS[0], []);
  assert.deepStrictEqual(tiers.map(o => o.label), ['Fast and cheap', 'Balanced', 'Best', 'Other model…']);
  assert.strictEqual(tiers[0].name, 'claude-haiku-5-5');
  const local = S.modelOptions(PRESETS[3], ['llama3', 'qwen3']);
  assert.deepStrictEqual(local.map(o => o.value), ['id:llama3', 'id:qwen3', 'other']);
  assert.deepStrictEqual(S.modelOptions(PRESETS[3], []).map(o => o.value), ['other'], 'no models installed: only Other');
  assert.deepStrictEqual(S.choiceOf('tier:best', ''), { model: '', tier: 'best' });
  assert.deepStrictEqual(S.choiceOf('id:llama3', ''), { model: 'llama3', tier: '' });
  assert.deepStrictEqual(S.choiceOf('other', ' my/model '), { model: 'my/model', tier: '' });
  assert.strictEqual(S.valueOf({ model: 'zzz' }, tiers), 'other');
  assert.strictEqual(S.valueOf({ model: 'llama3' }, local), 'id:llama3');
  assert.strictEqual(S.valueOf({ tier: 'balanced' }, tiers), 'tier:balanced');

  // Test messages: the provider's error kinds in plain words.
  assert.strictEqual(S.testMessage({ ok: true }, 'claude-haiku-5-5').text, '✓ Connected — answers will use claude-haiku-5-5');
  const bad = m => S.testMessage({ ok: false, message: m }, 'x');
  assert.match(bad('The provider refused the key (401): invalid x-api-key').text, /key wasn't accepted/);
  assert.match(bad('No API key is saved for OpenAI.').text, /key wasn't accepted/);
  assert.match(bad('the provider answered 402: Insufficient credits').text, /no credit/);
  assert.match(bad('the provider answered 404: model not found').text, /model wasn't found/);
  assert.match(bad("This model can't use tools; pick another.").text, /can't use tools/);
  assert.match(bad('Get http://localhost:11434: dial tcp: connection refused').text, /reach/);
  assert.match(bad('the provider answered 429: slow').text, /slow down/);
  assert.match(bad('weird').text, /^That didn't work: weird/);
  assert.strictEqual(bad('x').ok, false);
  assert.strictEqual(S.localStatus('Ollama', { ok: true, models: ['a', 'b', 'c'] }).text, 'Found Ollama with 3 models');
  assert.strictEqual(S.localStatus('Ollama', { ok: true, models: ['a'] }).text, 'Found Ollama with 1 model');
  assert.strictEqual(S.localStatus('Ollama', { ok: false }).found, false);

  // Key states: no key shows a box; saved shows "Key saved ✓" with Replace and Remove.
  assert.deepStrictEqual(S.keyView('none'), { box: true, saved: false, status: '' });
  assert.deepStrictEqual(S.keyView('saved'), { box: false, saved: true, status: 'Key saved ✓' });
  assert.deepStrictEqual(S.keyView('replacing'), { box: true, saved: false, status: '' });

  // The key field: pasted, saved, then gone from the field and never read back or sent twice.
  {
    const r = rig({ clients: [{ kind: 'api', provider: 'anthropic' }] });
    await r.ai.refresh();
    assert.strictEqual(r.ai.state().mode, 'key');
    assert.strictEqual(r.ai.state().keyState, 'none');
    assert.strictEqual(r.els.keyBox.hidden, false); assert.strictEqual(r.els.keySaved.hidden, true);
    r.els.keyInput.value = 'sk-secret-123';
    await r.els.keySave.onclick();
    assert.strictEqual(r.els.keyInput.value, '', 'the field is cleared after save');
    assert.strictEqual(r.els.keyStatus.textContent, 'Key saved ✓');
    assert.strictEqual(r.els.keyBox.hidden, true); assert.strictEqual(r.els.keySaved.hidden, false);
    const sent = r.calls.filter(c => JSON.stringify(c).includes('sk-secret-123'));
    assert.strictEqual(sent.length, 1, 'the key goes out once, to the key endpoint');
    assert.strictEqual(sent[0][0], '/api/providers/key');
    for (const n of names) for (const f of ['value', 'textContent']) assert.ok(!String(r.els[n][f]).includes('sk-secret-123'), n + '.' + f + ' echoes the key');
    // Replace shows an empty box again; Remove returns to no key.
    r.els.keyReplace.onclick();
    assert.strictEqual(r.els.keyBox.hidden, false); assert.strictEqual(r.els.keyInput.value, '');
    r.els.keyCancel.onclick();
    assert.strictEqual(r.els.keySaved.hidden, false);
    await r.els.keyRemove.onclick();
    assert.strictEqual(r.ai.state().keyState, 'none'); assert.strictEqual(r.els.keyStatus.textContent, '');
    // An empty paste sends nothing.
    const before = r.calls.length;
    r.els.keyInput.value = '   '; await r.els.keySave.onclick();
    assert.strictEqual(r.calls.length, before); assert.strictEqual(r.els.keyInput.value, '');
    // The key page opens through the shell's list.
    r.els.keyLink.onclick({ preventDefault() {} });
    assert.deepStrictEqual(r.opened, [S.KEY_PAGES.anthropic]);
  }

  // A key saved and a model chosen make that provider the answering one; Test shows the result in words.
  {
    const r = rig({ clients: [{ kind: 'api', provider: 'anthropic' }] });
    await r.ai.refresh();
    r.els.keyInput.value = 'k'; await r.els.keySave.onclick();
    const last = r.calls.filter(c => c[0] === '/api/providers/clients').pop();
    assert.deepStrictEqual(last[1].clients, [{ kind: 'api', provider: 'anthropic', base_url: '', model: '', tier: 'efficient' }]);
    assert.strictEqual(r.els.modelName.textContent, 'claude-haiku-5-5');
    await r.els.test.onclick();
    assert.strictEqual(r.els.testOut.textContent, '✓ Connected — answers will use claude-haiku-5-5');
    assert.strictEqual(r.els.testOut.className, 'ai-ok');
    r.server.test = { ok: false, message: 'the provider answered 402: no credit' };
    await r.els.test.onclick();
    assert.match(r.els.testOut.textContent, /no credit/); assert.strictEqual(r.els.test.disabled, false);
    // Other model: a typed id is what is saved.
    r.els.model.value = 'other'; r.els.model.onchange();
    assert.strictEqual(r.els.modelOtherRow.hidden, false);
    r.els.modelOther.value = 'claude-x'; await r.els.modelOther.onchange();
    assert.strictEqual(r.server.clients[0].model, 'claude-x');
  }

  // No key yet: nothing is saved as the active client, so asks keep working as they did.
  {
    const r = rig({ clients: [] });
    await r.ai.refresh();
    r.els.mode.value = 'key'; await r.els.mode.onchange();
    assert.strictEqual(r.calls.filter(c => c[0] === '/api/providers/clients').length, 0);
    assert.strictEqual(r.els.costRows.hidden, false);
    assert.strictEqual(r.els.cost.textContent, S.COST_NOTE);
  }

  // A local server: found with its models listed, or a link to install it; the free choice needs no key.
  {
    const r = rig({ clients: [{ kind: 'api', provider: 'ollama', model: 'qwen3' }] });
    await r.ai.refresh();
    assert.strictEqual(r.ai.state().mode, 'local');
    assert.strictEqual(r.els.localStatus.textContent, 'Found Ollama with 2 models');
    assert.deepStrictEqual(r.els.model.options.map(o => o.value), ['id:llama3', 'id:qwen3', 'other']);
    assert.strictEqual(r.els.model.value, 'id:qwen3');
    assert.strictEqual(r.els.keyRow.hidden, true);
    assert.strictEqual(r.els.costRows.hidden, true);
    const r2 = rig({ clients: [{ kind: 'api', provider: 'ollama' }], ollama: { ok: false, message: 'refused', models: [] } });
    await r2.ai.refresh();
    assert.strictEqual(r2.els.localStatus.textContent, "Ollama wasn't found on this PC.");
    assert.strictEqual(r2.els.localLink.hidden, false);
    r2.els.localLink.onclick({ preventDefault() {} });
    assert.deepStrictEqual(r2.opened, ['https://ollama.com/download']);
  }

  // Back to an AI app: the saved list is emptied, the CLI rows return, and the backup list offers the other accounts.
  {
    const r = rig({ clients: [{ kind: 'api', provider: 'anthropic' }] });
    r.server.presets[1].has_key = true;
    await r.ai.refresh();
    assert.ok(r.els.backup.options.some(o => o.value === 'openrouter'));
    assert.ok(!r.els.backup.options.some(o => o.value === 'anthropic'), 'the active one is not its own backup');
    r.els.mode.value = 'cli'; await r.els.mode.onchange();
    assert.deepStrictEqual(r.server.clients, []);
    assert.strictEqual(r.els.cliRows.hidden, false); assert.strictEqual(r.els.apiRows.hidden, true);
  }
  console.log('ok');
})().catch(e => { console.error(e); process.exit(1); });
