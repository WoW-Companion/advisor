package companion

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
)

// StaleAsk is how old an unanswered in-game question must be before it is no longer answered by
// itself: it is offered to the player instead ("pending"), so a launch never spends an AI call on
// last night's question.
const StaleAsk = 12 * 60 * 60 // seconds

// ledgerCap is the most answered asks kept; the oldest go first.
const ledgerCap = 300

// answerCap is the longest answer kept in the ledger, in bytes, so a launch can show the exchange again.
const answerCap = 4000

// LedgerPath is the answered-asks file: <user config dir>/wowc-advisor/answered-asks.json; empty
// when the system has no config directory.
func LedgerPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		return ""
	}
	return filepath.Join(dir, "wowc-advisor", "answered-asks.json")
}

// ledgerRow is one answered ask.
type ledgerRow struct {
	Answer     string `json:"answer,omitempty"`
	AnsweredAt int64  `json:"answered_at"`
}

// ledger remembers which in-game asks were answered, keyed by character, the ask's time and a hash
// of its text, so the same ask is never answered twice, even when the game's SavedVariables still
// hold it. With an empty path it lives in memory only.
type ledger struct {
	mu   sync.Mutex
	path string
	rows map[string]ledgerRow
}

func askKey(character string, askedAt int64, text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("%s|%d|%s", character, askedAt, hex.EncodeToString(h[:8]))
}

func openLedger(path string) *ledger {
	l := &ledger{path: path, rows: map[string]ledgerRow{}}
	if path == "" {
		return l
	}
	if b, err := os.ReadFile(path); err == nil {
		var f struct {
			Answered map[string]ledgerRow `json:"answered"`
		}
		if json.Unmarshal(b, &f) == nil && f.Answered != nil {
			l.rows = f.Answered
		}
	}
	return l
}

func (l *ledger) get(key string) (ledgerRow, bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	r, ok := l.rows[key]
	return r, ok
}

// add records an answered ask and saves the file (best effort: a failed save only means a repeat
// answer after a restart).
func (l *ledger) add(key, answer string, at int64) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if len(answer) > answerCap {
		answer = answer[:answerCap]
	}
	l.rows[key] = ledgerRow{Answer: answer, AnsweredAt: at}
	if len(l.rows) > ledgerCap {
		keys := make([]string, 0, len(l.rows))
		for k := range l.rows {
			keys = append(keys, k)
		}
		sort.Slice(keys, func(i, j int) bool { return l.rows[keys[i]].AnsweredAt < l.rows[keys[j]].AnsweredAt })
		for _, k := range keys[:len(keys)-ledgerCap] {
			delete(l.rows, k)
		}
	}
	if l.path == "" {
		return
	}
	b, err := json.MarshalIndent(map[string]any{"answered": l.rows}, "", " ")
	if err != nil || os.MkdirAll(filepath.Dir(l.path), 0o755) != nil {
		return
	}
	tmp := l.path + ".tmp"
	if os.WriteFile(tmp, b, 0o600) == nil {
		os.Rename(tmp, l.path)
	}
}
