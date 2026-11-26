package wording

import "testing"

func TestA(t *testing.T) {
	for in, want := range map[string]string{
		"Enchanting": "an Enchanting", "Alchemy": "an Alchemy", "Engineer": "an Engineer", "Tailoring": "a Tailoring",
		"Cook": "a Cook", "inscription": "an inscription", "": "a ",
	} {
		if got := A(in); got != want {
			t.Errorf("A(%q) = %q, want %q", in, got, want)
		}
	}
}
