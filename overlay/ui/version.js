// The settings version line and the update check, with no DOM or Tauri globals of their own so a
// test can drive them under Node. app.js passes in the page's pieces.
(function (root) {
  const CHECK_EVERY = 4 * 60 * 60 * 1000; // matches updates.js
  const START_DELAY = 3000; // after the handshake; well inside the 10 s the check is allowed

  function versionText(appV, advisorV) {
    return 'WoW Companion ' + (appV || '…') + ' · advisor ' + (advisorV || '…');
  }

  // deps: el, getConn() -> handshake or null, enabled() -> bool, fetchUpdate() -> Promise<{available,latest}>,
  // openPage(), setTimeout, setInterval.
  function createVersionLine(deps) {
    let appVersion = '', latest = '', started = false;
    function render() {
      const c = deps.getConn();
      deps.el.textContent = versionText(appVersion, c && c.version);
      if (!latest) return;
      deps.el.append(' · ' + latest + ' ');
      const a = deps.el.ownerDocument.createElement('a');
      a.href = '#'; a.textContent = 'available';
      a.onclick = ev => { ev.preventDefault(); deps.openPage(); };
      deps.el.append(a);
    }
    async function check() {
      if (!deps.enabled() || !deps.getConn()) return;
      try {
        const n = await deps.fetchUpdate();
        latest = n && n.available ? n.latest : '';
        if (deps.checked) deps.checked(n);
      } catch { latest = ''; } // a failed lookup shows nothing
      render();
    }
    return {
      render, check,
      setAppVersion(v) { appVersion = v; render(); },
      clearNotice() { latest = ''; render(); },
      // Every handshake, the first and each restart's: show the advisor version; the first also
      // schedules the check on start.
      onConnect() {
        render();
        if (started) return;
        started = true;
        deps.setTimeout(check, START_DELAY);
      },
      onDisconnect() { render(); },
      startTimer() { deps.setInterval(check, CHECK_EVERY); },
    };
  }

  const api = { createVersionLine, versionText, CHECK_EVERY, START_DELAY };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcVersion = api;
})(typeof window !== 'undefined' ? window : globalThis);
