// The recovery window: the shell's own answer when the update check, going back or the safe screen
// needs showing. It only asks the Rust shell for the current view and sends back a button's id.
(function () {
  var invoke = window.__TAURI__ && window.__TAURI__.core && window.__TAURI__.core.invoke;
  var title = document.getElementById('title');
  var text = document.getElementById('text');
  var buttons = document.getElementById('buttons');
  var shown = '';

  function render(v) {
    var key = JSON.stringify([v.title, v.buttons, v.busy]);
    title.textContent = v.title || 'WoW Companion';
    text.textContent = v.text || '';
    if (key === shown) return;
    shown = key;
    buttons.replaceChildren();
    (v.buttons || []).forEach(function (b, i) {
      var el = document.createElement('button');
      el.textContent = b.label;
      if (i === 0) el.className = 'first';
      el.onclick = function () {
        invoke('recovery_do', { action: b.id }).catch(function (e) { text.textContent = String(e); });
      };
      buttons.append(el);
    });
  }

  function poll() {
    invoke('recovery_state').then(render).catch(function (e) { text.textContent = String(e); });
  }

  if (!invoke) {
    text.textContent = 'The app could not start this window.';
    return;
  }
  poll();
  setInterval(poll, 700);
})();
