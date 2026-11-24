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
  // name prefixes each line ('test:' or 'ask:'), so the two fields can be told apart. After an editing
  // key, watchChange logs "no change" when the value and caret did not move by the next frame.
  const EDITING = ['Backspace', 'Delete', 'ArrowLeft', 'ArrowRight', 'ArrowUp', 'ArrowDown', 'Home', 'End'];
  function attach(target, l, name, raf, defer) {
    const tag = name ? name + ': ' : '';
    raf = raf || (fn => (typeof requestAnimationFrame === 'function' ? requestAnimationFrame(fn) : setTimeout(fn, 16)));
    defer = defer || (fn => setTimeout(fn, 0));
    // A cancel after the capture listener ran (an on... handler returning false, say) is only seen later:
    // check each keydown once its dispatch is over and log "cancelled later" when it was not PREVENTED before.
    const rec = ev => {
      const d = describe(ev);
      if (d) l.add(tag + d);
      if (ev.type === 'keydown' && !ev.defaultPrevented) defer(() => { if (ev.defaultPrevented) l.add(tag + 'cancelled later key=' + ev.key); });
    };
    for (const t of ['keydown', 'beforeinput', 'paste']) target.addEventListener(t, rec, true);
    target.addEventListener('keydown', ev => {
      if (EDITING.indexOf(ev.key) < 0 || ev.ctrlKey || ev.altKey || ev.metaKey) return;
      const was = [target.value, target.selectionStart, target.selectionEnd].join('|');
      raf(() => { if ([target.value, target.selectionStart, target.selectionEnd].join('|') === was) l.add(tag + 'no change after ' + ev.key); });
    }, true);
  }

  const api = { MAX, describe, log, attach };
  if (typeof module !== 'undefined' && module.exports) module.exports = api;
  else root.WowcKeyDiag = api;
})(typeof window !== 'undefined' ? window : globalThis);
