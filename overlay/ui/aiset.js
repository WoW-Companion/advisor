// Settings -> AI: "How should the companion answer?" Pick an AI app, an AI account with a key, or a model
// on this PC. No DOM or Tauri globals of its own, so a test can drive it under Node: app.js passes in the
// page's elements and two small functions (call the advisor, open a page). The key is write-only: it goes
// to the advisor once, the field is cleared at once, and nothing here ever reads it back.
(function (root) {
  const MODES = { cli: 'An AI app on this PC', key: 'An AI account with a key', local: 'A model on this PC (free)' };
  const KEY_PROVIDERS = ['anthropic', 'openai', 'openrouter', 'custom'];
  const LOCAL_PROVIDERS = ['ollama', 'lmstudio'];
  const TIERS = [['efficient', 'Fast and cheap'], ['balanced', 'Balanced'], ['best', 'Best']];
  const SHORT_NAMES = { anthropic: 'Anthropic (Claude)', openai: 'OpenAI', openrouter: 'OpenRouter', custom: 'Other (base URL)', ollama: 'Ollama', lmstudio: 'LM Studio' };
  // Pages the shell may open (its fixed list in main.rs).
  const KEY_PAGES = { anthropic: 'https://platform.claude.com/settings/keys', openai: 'https://platform.openai.com/api-keys', openrouter: 'https://openrouter.ai/keys' };
  const INSTALL_PAGES = { ollama: 'https://ollama.com/download', lmstudio: 'https://lmstudio.ai/download' };
  const COST_NOTE = 'Uses your own account; the provider bills you. Questions are small: the usage line under each answer shows what each one cost.';
  const SUBSCRIPTION_NOTE = "Have Claude Pro or ChatGPT Plus? Use the AI app option; subscriptions don't work with keys.";

  // Which of the three choices the saved list means: the first client decides; none is the AI-app setup.
  function modeOf(clients, presets) {
    const first = (clients || [])[0];
    if (!first || first.kind !== 'api') return 'cli';
    const p = (presets || []).find(x => x.id === first.provider);
    return p && p.local ? 'local' : 'key';
  }

  // The dropdown: tier presets (with the real model name beside them) or, for a local server, what is installed.
  function modelOptions(preset, installed) {
    const out = [];
    if (preset && preset.local) {
      for (const id of installed || []) out.push({ value: 'id:' + id, label: id });
    } else {
      for (const [tier, label] of TIERS) {
        const name = preset && preset.models && preset.models[tier];
        out.push({ value: 'tier:' + tier, label, name: name || '' });
      }
    }
    out.push({ value: 'other', label: 'Other model…' });
    return out;
  }
  // The choice as the advisor wants it: {model, tier}. A typed id wins; a tier leaves the model empty.
  function choiceOf(value, typed) {
    if (value === 'other') return { model: (typed || '').trim(), tier: '' };
    if (value.startsWith('id:')) return { model: value.slice(3), tier: '' };
    return { model: '', tier: value.replace(/^tier:/, '') };
  }
  // The dropdown value for a saved client.
  function valueOf(client, options) {
    if (client && client.model) {
      const v = 'id:' + client.model;
      return options.some(o => o.value === v) ? v : 'other';
    }
    return 'tier:' + ((client && client.tier) || 'efficient');
  }
  function modelName(preset, choice) {
    return choice.model || (preset && preset.models && preset.models[choice.tier]) || '';
  }

  // Test results in plain words, from what the advisor returns ({ok, message}).
  function testMessage(res, model) {
    if (res && res.ok) return { ok: true, text: '✓ Connected — answers will use ' + (model || 'the chosen model') };
    const m = String((res && res.message) || '');
    const rules = [
      [/can't use tools|no tools/i, "This model can't use tools. Pick another model."],
      [/choose a model/i, 'Choose a model first.'],
      [/no api key|refused the key|\b(401|403)\b|invalid.*key|incorrect.*key/i, "That key wasn't accepted. Check that you pasted all of it, then save it again."],
      [/\b402\b|credit|billing|quota|insufficient|payment|balance/i, 'The account has no credit left. Add some on the provider’s site, then test again.'],
      [/\b429\b|rate limit|too many/i, 'The provider says to slow down. Wait a moment and test again.'],
      [/\b404\b|not found|does not exist|no such model|unknown model/i, "That model wasn't found. Pick another one."],
      [/connect|refused|unreachable|no such host|dial |timed out|timeout|deadline|reach/i, "Couldn't reach it. Check that it is running and the address is right."],
    ];
    for (const [re, text] of rules) if (re.test(m)) return { ok: false, text };
    return { ok: false, text: m ? "That didn't work: " + (m.length > 160 ? m.slice(0, 160) + '…' : m) : "That didn't work." };
  }

  // The local line: "Found Ollama with 3 models", or where to install it.
  function localStatus(name, res) {
    if (res && res.ok) {
      const n = (res.models || []).length;
      return { found: true, text: 'Found ' + name + ' with ' + n + ' model' + (n === 1 ? '' : 's') };
    }
    return { found: false, text: name + " wasn't found on this PC." };
  }

  // The key area: no key (a box), saved (a check, Replace and Remove), or replacing (a box again).
  function keyView(state) {
    return {
      box: state !== 'saved',
      saved: state === 'saved',
      status: state === 'saved' ? 'Key saved ✓' : '',
    };
  }

  // The ordered list to save: the active client, then the optional backup.
  function clientList(active, backup) {
    return backup ? [active, backup] : [active];
  }

  // els: the page's elements by short name. deps: call(path, body?) -> parsed JSON, openPage(url), onChange().
  function create(els, deps) {
    let presets = [], clients = [], mode = 'cli', keyState = 'none', installed = [], opts = [];
    const preset = () => presets.find(p => p.id === els.provider.value);
    const active = () => clients.find(c => c.kind === 'api');
    const mk = deps.option || (o => o);
    const setHidden = (e, h) => { e.hidden = !!h; };

    function fillProviders() {
      const ids = mode === 'local' ? LOCAL_PROVIDERS : KEY_PROVIDERS;
      const keep = ids.includes(els.provider.value) ? els.provider.value : '';
      els.provider.replaceChildren(...ids.filter(id => presets.some(p => p.id === id)).map(id => mk({ value: id, textContent: SHORT_NAMES[id] })));
      els.provider.value = keep || (els.provider.options[0] || {}).value;
    }
    function showKey() {
      const p = preset(), v = keyView(keyState);
      const needs = mode === 'key' && p && p.needs_key;
      setHidden(els.keyRow, !needs);
      setHidden(els.keyBox, !v.box);
      setHidden(els.keySaved, !v.saved);
      setHidden(els.keyCancel, keyState !== 'replacing');
      els.keyStatus.textContent = v.status;
      els.keyInput.value = ''; // the box never holds a key between actions
      setHidden(els.keyLink, !(p && KEY_PAGES[p.id]));
    }
    function showModels() {
      const p = preset();
      opts = modelOptions(p, installed);
      const cur = active() && active().provider === (p && p.id) ? active() : null;
      els.model.replaceChildren(...opts.map(o => mk({ value: o.value, textContent: o.label + (o.name ? ' — ' + o.name : '') })));
      els.model.value = valueOf(cur, opts);
      showModelName();
    }
    function choice() { return choiceOf(els.model.value, els.modelOther.value); }
    function showModelName() {
      const p = preset(), c = choice();
      setHidden(els.modelOtherRow, els.model.value !== 'other');
      els.modelName.textContent = els.model.value === 'other' ? '' : modelName(p, c);
    }
    function showMode() {
      els.mode.value = mode;
      setHidden(els.cliRows, mode !== 'cli');
      setHidden(els.apiRows, mode === 'cli');
      setHidden(els.baseRow, !(mode === 'key' && els.provider.value === 'custom'));
      setHidden(els.localRow, mode !== 'local');
      setHidden(els.costRows, mode !== 'key');
      setHidden(els.advanced, mode === 'cli');
      els.cost.textContent = COST_NOTE;
      els.subscription.textContent = SUBSCRIPTION_NOTE;
      const p = preset();
      els.note.textContent = p && p.id === 'openrouter' ? 'OpenRouter has free models: pick “Other model…” and type one that ends in :free.' : '';
      showKey();
      showModels();
    }
    function ref() {
      const c = choice();
      return { provider: els.provider.value, base_url: els.provider.value === 'custom' ? els.base.value.trim() : '', model: c.model, tier: c.tier };
    }
    // Ready to answer: has a key where one is needed, and a model if the server has no default.
    function ready() {
      const p = preset();
      if (!p) return false;
      if (p.needs_key && keyState !== 'saved') return false;
      const c = choice();
      return !!(c.model || p.models && p.models[c.tier]);
    }
    function backupRef() {
      const v = els.backup.value;
      if (!v) return null;
      if (v === 'cli') return { kind: 'cli', agent: deps.agent() || 'auto' };
      return { kind: 'api', provider: v };
    }
    async function saveActive() {
      if (mode === 'cli' || !ready()) return;
      const r = ref();
      const out = await deps.call('/api/providers/clients', { clients: clientList({ kind: 'api', ...r }, backupRef()) });
      if (out && out.clients) clients = out.clients;
      deps.onChange();
    }
    async function refresh() {
      const out = await deps.call('/api/providers');
      if (!out || !out.presets) return;
      presets = out.presets; clients = out.clients || [];
      mode = modeOf(clients, presets);
      const a = active();
      fillProviders();
      if (a && els.provider.options.some(o => o.value === a.provider)) {
        els.provider.value = a.provider;
        els.base.value = a.base_url || '';
      }
      keyState = preset() && preset().has_key ? 'saved' : 'none';
      fillBackup();
      showMode();
      if (mode === 'local') await scanLocal();
    }
    function fillBackup() {
      const items = [{ value: '', textContent: 'Nothing' }];
      for (const p of presets) {
        if (p.id !== els.provider.value && (p.has_key || p.local) && p.id !== 'custom') items.push({ value: p.id, textContent: SHORT_NAMES[p.id] || p.name });
      }
      if (deps.agent()) items.push({ value: 'cli', textContent: 'An AI app on this PC' });
      els.backup.replaceChildren(...items.map(mk));
      const second = clients[1];
      els.backup.value = !second ? '' : second.kind === 'cli' ? 'cli' : second.provider;
    }
    async function scanLocal() {
      const p = preset();
      if (!p || !p.local) return;
      const out = await deps.call('/api/providers/models?provider=' + p.id);
      installed = out && out.ok ? out.models || [] : [];
      const st = localStatus(SHORT_NAMES[p.id] || p.name, out);
      els.localStatus.textContent = st.text;
      setHidden(els.localLink, st.found);
      els.localLink.textContent = 'Get ' + (SHORT_NAMES[p.id] || p.name);
      els.localLink.title = INSTALL_PAGES[p.id] || '';
      showModels();
      if (st.found) await saveActive();
    }

    els.mode.replaceChildren(...Object.entries(MODES).map(([value, textContent]) => mk({ value, textContent })));
    els.mode.onchange = async () => {
      mode = els.mode.value;
      if (mode === 'cli') {
        // Back to the AI app: the saved list is emptied, which is exactly how the app always worked.
        const out = await deps.call('/api/providers/clients', { clients: [] });
        clients = (out && out.clients) || [];
        showMode();
        return;
      }
      fillProviders(); keyState = preset() && preset().has_key ? 'saved' : 'none';
      fillBackup(); showMode();
      if (mode === 'local') await scanLocal(); else await saveActive();
    };
    els.provider.onchange = async () => {
      keyState = preset() && preset().has_key ? 'saved' : 'none';
      els.testOut.textContent = '';
      fillBackup(); showMode();
      if (mode === 'local') await scanLocal(); else await saveActive();
    };
    els.base.onchange = () => saveActive();
    els.model.onchange = () => { showModelName(); if (els.model.value !== 'other') saveActive(); };
    els.modelOther.onchange = () => { showModelName(); saveActive(); };
    els.backup.onchange = () => saveActive();
    els.keySave.onclick = async () => {
      const key = els.keyInput.value;
      els.keyInput.value = ''; // gone from the page before the request leaves
      if (!key.trim()) { els.keyStatus.textContent = 'Paste your key first.'; return; }
      const out = await deps.call('/api/providers/key', { provider: els.provider.value, key });
      const p = preset();
      if (out && out.has_key) { if (p) p.has_key = true; keyState = 'saved'; showKey(); await saveActive(); }
      else els.keyStatus.textContent = "That key couldn't be saved. Check that you pasted all of it.";
    };
    els.keyReplace.onclick = () => { keyState = 'replacing'; showKey(); };
    els.keyCancel.onclick = () => { keyState = 'saved'; showKey(); };
    els.keyRemove.onclick = async () => {
      const out = await deps.call('/api/providers/key/delete', { provider: els.provider.value });
      if (out && out.has_key === false) { const p = preset(); if (p) p.has_key = false; keyState = 'none'; showKey(); }
    };
    els.keyLink.onclick = ev => { if (ev && ev.preventDefault) ev.preventDefault(); deps.openPage(KEY_PAGES[els.provider.value]); };
    els.localLink.onclick = ev => { if (ev && ev.preventDefault) ev.preventDefault(); deps.openPage(INSTALL_PAGES[els.provider.value]); };
    els.rescan.onclick = () => scanLocal();
    els.test.onclick = async () => {
      els.testOut.textContent = 'Testing…';
      els.test.disabled = true;
      try {
        await saveActive();
        const r = ref();
        const res = await deps.call('/api/providers/test', r);
        const m = testMessage(res, modelName(preset(), choice()));
        els.testOut.textContent = m.text;
        els.testOut.className = m.ok ? 'ai-ok' : 'ai-bad';
      } finally { els.test.disabled = false; }
    };

    return { refresh, showMode, state: () => ({ mode, keyState }) };
  }

  const api = { MODES, KEY_PAGES, INSTALL_PAGES, COST_NOTE, SUBSCRIPTION_NOTE, modeOf, modelOptions, choiceOf, valueOf, modelName, testMessage, localStatus, keyView, clientList, create };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcAiSettings = api;
})(typeof window !== 'undefined' ? window : globalThis);
