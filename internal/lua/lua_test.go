package lua

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestParse(t *testing.T) {
	cases := []struct {
		name, src string
		want      map[string]any
	}{
		{"scalars", `A = 1 B = "x\n\"y\"" C = true D = nil E = -2.5e1 F = 0x1F G = [[long
text]]`, map[string]any{"A": int64(1), "B": "x\n\"y\"", "C": true, "E": -25.0, "F": int64(31), "G": "long\ntext"}},
		{"table keys", `T = { 10, 20; ["k"] = 1, [3] = "c", name = true, -- [1]
}`, map[string]any{"T": Table{int64(1): int64(10), int64(2): int64(20), "k": int64(1), int64(3): "c", "name": true}}},
		{"comments and bom", "\xef\xbb\xbf-- hi\n--[[ block\ncomment ]] A = 1 -- tail", map[string]any{"A": int64(1)}},
		{"decimal escape", `A = "\65\066"`, map[string]any{"A": "AB"}},
	}
	for _, c := range cases {
		got, err := Parse(c.src)
		if err != nil || !reflect.DeepEqual(got, c.want) {
			t.Errorf("%s: got %#v, %v; want %#v", c.name, got, err, c.want)
		}
	}
}

func TestParseErrorsNameTheLine(t *testing.T) {
	cases := []struct {
		src  string
		line int
		msg  string
	}{
		{"A = {", 1, "unterminated table"},
		{"A = 1\nB = os.exit()", 2, "only literals"},
		{"A = 1\n\nB = \"open", 3, "unterminated string"},
		{"A = {1 2}", 1, "expected ','"},
		{"A = {[nil] = 1}", 1, "table key"},
		{"A = \"\\q\"", 1, "invalid escape"},
		{"A = \"\\300\"", 1, "out of range"},
		{"= 1", 1, "variable name"},
		{"A = [[never closed", 1, "unterminated long string"},
		{"A = f(1)", 1, "only literals"},
	}
	for _, c := range cases {
		_, err := Parse(c.src)
		var pe *ParseError
		if !errors.As(err, &pe) || pe.Line != c.line || !strings.Contains(err.Error(), c.msg) {
			t.Errorf("%q: err = %v, want line %d containing %q", c.src, err, c.line, c.msg)
		}
	}
}

func TestList(t *testing.T) {
	got := List(Table{int64(1): "a", int64(2): "b", int64(4): "d", "x": 1})
	if !reflect.DeepEqual(got, []any{"a", "b"}) || List("no") != nil {
		t.Errorf("List = %v", got)
	}
}
