package benchsummary

import (
	"strings"
	"testing"
)

func TestCount(t *testing.T) {
	same, differ := Count([]string{"match", "differ", "differ: 2 bands", "match", "skipped"})
	if same != 2 || differ != 2 {
		t.Fatalf("same %d differ %d, want 2 and 2", same, differ)
	}
}

func TestAreaLineAndTotal(t *testing.T) {
	out := strings.Join([]string{
		"=== RUN   TestProfessionAdviceBenchmark",
		AreaLine("professions", []string{"match", "match", "differ: a"}),
		"--- PASS",
		AreaLine("stats", []string{"match", "match"}),
	}, "\n")
	got, ok := Total(strings.NewReader(out))
	want := "advice-benchmark: 5 checks, 4 same, 1 differ (professions 2/1, stats 2/0)"
	if !ok || got != want {
		t.Fatalf("got %q (%v), want %q", got, ok, want)
	}
	if _, ok := Total(strings.NewReader("ok  pkg 0.1s\n")); ok {
		t.Fatal("output with no count line gave a summary")
	}
}
