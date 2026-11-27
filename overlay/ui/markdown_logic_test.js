// Driven by markdown_test.go under Node: the safe Markdown renderer.
const assert = require('assert');
const M = require('./markdown.js');

class El {
  constructor(tag) { this.tag = tag; this.children = []; this.className = ''; this._text = ''; }
  append(...c) { this.children.push(...c); }
  set textContent(t) { this._text = t; }
  get textContent() { return this._text + this.children.map(c => c.textContent).join(''); }
  find(tag) { return [].concat(this.tag === tag ? [this] : [], ...this.children.map(c => c.find(tag))); }
}
const doc = { createElement: t => new El(t) };
const r = (s, o) => M.render(doc, s, o);

// Bold, a heading and a list become the expected elements.
let n = r('## Sell\n**Zehen: sell** the bars\n- one\n- two\n  - nested\n1. first');
assert.strictEqual(n.children[0].className, 'md-h');
assert.strictEqual(n.children[0].textContent, 'Sell');
assert.strictEqual(n.find('strong')[0].textContent, 'Zehen: sell');
assert(!n.textContent.includes('**'));
const ul = n.find('ul')[0];
assert.strictEqual(ul.children.length, 2);
assert.strictEqual(ul.children[1].find('ul')[0].children[0].textContent, 'nested');
assert.strictEqual(n.find('ol')[0].children[0].textContent, 'first');

// Raw HTML stays text: no script element exists, and the characters survive as text.
n = r('<script>alert(1)</script> and <b>x</b>');
assert.strictEqual(n.find('script').length, 0);
assert.strictEqual(n.find('b').length, 0);
assert.strictEqual(n.textContent, '<script>alert(1)</script> and <b>x</b>');

// Density is a class.
assert.strictEqual(r('a').className, 'md md-compact');
assert.strictEqual(r('a', { density: 'comfortable' }).className, 'md md-comfortable');
assert.strictEqual(r('a', { density: 'weird' }).className, 'md md-compact');

assert.strictEqual(r('- **a').find('li')[0].textContent, '**a');
// A streaming partial marker stays literal and does not throw.
for (const s of ['**bol', 'x **bol', '*it', '`co', '[l](http://a', '| a |', '#']) {
  assert.strictEqual(r(s).textContent.replace(/\s/g, ''), s.replace(/\s/g, ''), s);
}

// Links: text plus an open button; only http(s); javascript: is plain text.
let opened = '';
n = r('[wiki](https://example.com/x) [bad](javascript:alert(1))', { onOpen: u => { opened = u; } });
const btn = n.find('button');
assert.strictEqual(btn.length, 1);
btn[0].onclick();
assert.strictEqual(opened, 'https://example.com/x');

// Tables and rules.
n = r('| a | b |\n|---|---|\n| 1 | **2** |\n\n---');
assert.strictEqual(n.find('th').length, 2);
assert.strictEqual(n.find('td')[1].find('strong')[0].textContent, '2');
assert.strictEqual(n.find('hr').length, 1);
console.log('ok');
