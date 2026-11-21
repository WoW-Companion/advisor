// Package lua reads the Lua table-literal subset that SavedVariables files use.
//
// The file is a sequence of `Name = <value>` assignments. Values are tables, strings, numbers,
// booleans and nil. Nothing is executed: the text is tokenised and parsed by hand. Every Lua table
// becomes a Table; positional entries get the integer keys 1..n, as Lua numbers them.
package lua

import (
	"fmt"
	"math"
	"strconv"
	"strings"
)

// Table is a parsed Lua table. Keys are string or int64; values are Table, string, int64,
// float64 or bool. A nil value is simply absent.
type Table map[any]any

// ParseError is malformed SavedVariables text. Its message names the 1-based line.
type ParseError struct {
	Line int
	Msg  string
}

func (e *ParseError) Error() string { return fmt.Sprintf("line %d: %s", e.Line, e.Msg) }

type parser struct {
	text string
	pos  int
}

// Parse reads a SavedVariables file into {variable name: value}. A variable set to nil is absent.
func Parse(text string) (map[string]any, error) {
	p := &parser{text: strings.TrimPrefix(text, "\xef\xbb\xbf")}
	out := map[string]any{}
	for {
		if err := p.skip(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.text) {
			return out, nil
		}
		name := p.name()
		if name == "" {
			return nil, p.errf(p.pos, "expected a variable name, found %s", p.found())
		}
		if err := p.expect('='); err != nil {
			return nil, err
		}
		v, err := p.value()
		if err != nil {
			return nil, err
		}
		if v != nil {
			out[name] = v
		}
	}
}

func (p *parser) line(pos int) int { return strings.Count(p.text[:pos], "\n") + 1 }

func (p *parser) errf(pos int, format string, args ...any) error {
	return &ParseError{Line: p.line(pos), Msg: fmt.Sprintf(format, args...)}
}

func (p *parser) found() string {
	if p.pos >= len(p.text) {
		return "end of file"
	}
	return strconv.QuoteRune(rune(p.text[p.pos]))
}

func isSpace(c byte) bool {
	return c == ' ' || c == '\t' || c == '\n' || c == '\r' || c == '\v' || c == '\f'
}
func isDigit(c byte) bool     { return c >= '0' && c <= '9' }
func isNameStart(c byte) bool { return c == '_' || c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' }

// longOpen reports the level of a long bracket opening at pos (`[[`, `[=[`), or -1.
func (p *parser) longOpen(pos int) (level, end int) {
	if pos >= len(p.text) || p.text[pos] != '[' {
		return -1, 0
	}
	i := pos + 1
	for i < len(p.text) && p.text[i] == '=' {
		i++
	}
	if i < len(p.text) && p.text[i] == '[' {
		return i - pos - 1, i + 1
	}
	return -1, 0
}

func (p *parser) longString(level, bodyStart int) (string, error) {
	closer := "]" + strings.Repeat("=", level) + "]"
	n := strings.Index(p.text[bodyStart:], closer)
	if n < 0 {
		return "", p.errf(bodyStart, "unterminated long string")
	}
	body := p.text[bodyStart : bodyStart+n]
	p.pos = bodyStart + n + len(closer)
	body = strings.TrimPrefix(body, "\r")
	return strings.TrimPrefix(body, "\n"), nil
}

// skip passes whitespace and comments.
func (p *parser) skip() error {
	for p.pos < len(p.text) {
		c := p.text[p.pos]
		switch {
		case isSpace(c):
			p.pos++
		case strings.HasPrefix(p.text[p.pos:], "--"):
			p.pos += 2
			if level, end := p.longOpen(p.pos); level >= 0 {
				if _, err := p.longString(level, end); err != nil {
					return err
				}
			} else if n := strings.IndexByte(p.text[p.pos:], '\n'); n < 0 {
				p.pos = len(p.text)
			} else {
				p.pos += n
			}
		default:
			return nil
		}
	}
	return nil
}

func (p *parser) expect(ch byte) error {
	if err := p.skip(); err != nil {
		return err
	}
	if p.pos >= len(p.text) || p.text[p.pos] != ch {
		return p.errf(p.pos, "expected %s, found %s", strconv.QuoteRune(rune(ch)), p.found())
	}
	p.pos++
	return nil
}

// name consumes an identifier at pos, or returns "".
func (p *parser) name() string {
	i := p.pos
	if i >= len(p.text) || !isNameStart(p.text[i]) {
		return ""
	}
	for i < len(p.text) && (isNameStart(p.text[i]) || isDigit(p.text[i])) {
		i++
	}
	s := p.text[p.pos:i]
	p.pos = i
	return s
}

var simpleEscapes = map[byte]byte{
	'a': '\a', 'b': '\b', 'f': '\f', 'n': '\n', 'r': '\r', 't': '\t', 'v': '\v',
	'\\': '\\', '"': '"', '\'': '\'', '\n': '\n',
}

func (p *parser) str() (string, error) {
	start := p.pos
	quote := p.text[p.pos]
	p.pos++
	var out strings.Builder
	for {
		if p.pos >= len(p.text) || p.text[p.pos] == '\n' {
			return "", p.errf(start, "unterminated string")
		}
		c := p.text[p.pos]
		p.pos++
		if c == quote {
			return out.String(), nil
		}
		if c != '\\' {
			out.WriteByte(c)
			continue
		}
		if p.pos >= len(p.text) {
			return "", p.errf(start, "unterminated string")
		}
		esc := p.text[p.pos]
		if r, ok := simpleEscapes[esc]; ok {
			out.WriteByte(r)
			p.pos++
		} else if isDigit(esc) {
			n := 0
			for n < 3 && p.pos+n < len(p.text) && isDigit(p.text[p.pos+n]) {
				n++
			}
			v, _ := strconv.Atoi(p.text[p.pos : p.pos+n])
			if v > 255 {
				return "", p.errf(p.pos, "decimal escape \\%s out of range", p.text[p.pos:p.pos+n])
			}
			out.WriteByte(byte(v))
			p.pos += n
		} else {
			return "", p.errf(p.pos, "invalid escape sequence \\%c", esc)
		}
	}
}

func (p *parser) number() (any, bool, error) {
	i := p.pos
	if i < len(p.text) && p.text[i] == '-' {
		i++
	}
	start := i
	if strings.HasPrefix(p.text[i:], "0x") || strings.HasPrefix(p.text[i:], "0X") {
		i += 2
		for i < len(p.text) && strings.IndexByte("0123456789abcdefABCDEF", p.text[i]) >= 0 {
			i++
		}
		v, err := strconv.ParseInt(p.text[start+2:i], 16, 64)
		if err != nil {
			return nil, false, p.errf(p.pos, "invalid hexadecimal number")
		}
		if p.text[p.pos] == '-' {
			v = -v
		}
		p.pos = i
		return v, true, nil
	}
	digits := 0
	for i < len(p.text) && isDigit(p.text[i]) {
		i++
		digits++
	}
	float := false
	if i < len(p.text) && p.text[i] == '.' {
		float = true
		i++
		for i < len(p.text) && isDigit(p.text[i]) {
			i++
			digits++
		}
	}
	if digits == 0 {
		return nil, false, nil
	}
	if i < len(p.text) && (p.text[i] == 'e' || p.text[i] == 'E') {
		j := i + 1
		if j < len(p.text) && (p.text[j] == '+' || p.text[j] == '-') {
			j++
		}
		if j < len(p.text) && isDigit(p.text[j]) {
			for j < len(p.text) && isDigit(p.text[j]) {
				j++
			}
			float = true
			i = j
		}
	}
	raw := p.text[p.pos:i]
	if !float {
		if v, err := strconv.ParseInt(raw, 10, 64); err == nil {
			p.pos = i
			return v, true, nil
		}
	}
	f, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		return nil, false, p.errf(p.pos, "invalid number %q", raw)
	}
	p.pos = i
	return f, true, nil
}

func (p *parser) value() (any, error) {
	if err := p.skip(); err != nil {
		return nil, err
	}
	if p.pos >= len(p.text) {
		return nil, p.errf(p.pos, "expected a value, found end of file")
	}
	c := p.text[p.pos]
	switch {
	case c == '{':
		return p.table()
	case c == '"' || c == '\'':
		return p.str()
	case c == '[':
		if level, end := p.longOpen(p.pos); level >= 0 {
			return p.longString(level, end)
		}
	}
	if v, ok, err := p.number(); err != nil || ok {
		return v, err
	}
	at := p.pos
	if word := p.name(); word != "" {
		switch word {
		case "true":
			return true, nil
		case "false":
			return false, nil
		case "nil":
			return nil, nil
		}
		return nil, p.errf(at, "unexpected identifier %q (only literals are allowed)", word)
	}
	return nil, p.errf(p.pos, "unexpected character %s", p.found())
}

func normalizeKey(k any) any {
	if f, ok := k.(float64); ok && f == math.Trunc(f) && math.Abs(f) < 1<<53 {
		return int64(f)
	}
	return k
}

func (p *parser) table() (Table, error) {
	if err := p.expect('{'); err != nil {
		return nil, err
	}
	result := Table{}
	index := int64(1)
	for {
		if err := p.skip(); err != nil {
			return nil, err
		}
		if p.pos >= len(p.text) {
			return nil, p.errf(p.pos, "unterminated table: expected '}', found end of file")
		}
		c := p.text[p.pos]
		if c == '}' {
			p.pos++
			return result, nil
		}
		if level, _ := p.longOpen(p.pos); c == '[' && level < 0 {
			p.pos++
			key, err := p.value()
			if err != nil {
				return nil, err
			}
			switch key.(type) {
			case nil, Table:
				return nil, p.errf(p.pos, "a table key must be a string, number or boolean")
			}
			if err := p.expect(']'); err != nil {
				return nil, err
			}
			if err := p.expect('='); err != nil {
				return nil, err
			}
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			if v != nil {
				result[normalizeKey(key)] = v
			}
		} else if name, ok := p.namedField(); ok {
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			if v != nil {
				result[name] = v
			}
		} else {
			v, err := p.value()
			if err != nil {
				return nil, err
			}
			if v != nil {
				result[index] = v
			}
			index++
		}
		if err := p.skip(); err != nil {
			return nil, err
		}
		if p.pos < len(p.text) && (p.text[p.pos] == ',' || p.text[p.pos] == ';') {
			p.pos++
		} else if p.pos >= len(p.text) || p.text[p.pos] != '}' {
			return nil, p.errf(p.pos, "expected ',' or '}' in table, found %s", p.found())
		}
	}
}

// namedField consumes `name =` (not `==`) and returns the name.
func (p *parser) namedField() (string, bool) {
	save := p.pos
	name := p.name()
	if name == "" {
		return "", false
	}
	rest := strings.TrimLeft(p.text[p.pos:], " \t\r\n\v\f")
	if strings.HasPrefix(rest, "=") && !strings.HasPrefix(rest, "==") {
		p.pos = len(p.text) - len(rest) + 1
		return name, true
	}
	p.pos = save
	return "", false
}

// List returns the positional entries (keys 1..n) of a table, in order.
func List(v any) []any {
	t, ok := v.(Table)
	if !ok {
		return nil
	}
	var out []any
	for i := int64(1); ; i++ {
		e, ok := t[i]
		if !ok {
			return out
		}
		out = append(out, e)
	}
}
