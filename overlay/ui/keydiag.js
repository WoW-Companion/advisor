// The keyboard test under Settings → Updates: the last few key events the page receives, so one try
// shows where keys stop (never reaching the page, or reaching it and being cancelled).
(function (root) {
  const MAX = 8;

  // describe: one line for a keydown, beforeinput or paste event; '' for anything else.
  function describe(ev) {
    switch (ev.type) {
      case 'keydown':
        return `keydown key=${ev.key} code=${ev.code}${ev.defaultPrevented ? ' PREVENTED' : ''}`;
      case 'beforeinput':
        return `beforeinput ${ev.inputType}${ev.defaultPrevented ? ' PREVENTED' : ''}`;
      case 'paste': {
        const types = ev.clipboardData && ev.clipboardData.types ? Array.from(ev.clipboardData.types) : [];
        return `paste types=${types.length ? types.join(',') : 'none'}`;
      }
      default:
        return '';
    }
  }

  // log: keeps the last 8 lines and calls onChange(lines) when one is added.
  function log(onChange) {
    const lines = [];
    return {
      lines,
      add(line) {
        if (!line) return;
        lines.push(line);
        while (lines.length > MAX) lines.shift();
        if (onChange) onChange(lines.slice());
      },
      record(ev) { this.add(describe(ev)); },
    };
  }

  // attach: listens on the target (capture, so a handler that stops propagation is still seen).
  function attach(target, l) {
    for (const t of ['keydown', 'beforeinput', 'paste']) target.addEventListener(t, ev => l.record(ev), true);
  }

  const api = { MAX, describe, log, attach };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcKeyDiag = api;
})(typeof window !== 'undefined' ? window : globalThis);
