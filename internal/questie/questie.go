// Package questie reads the player's own installed QuestieDB, if there is one, to find quests the
// character could pick up and where quest givers and objectives are. It is an optional, read-only
// integration: nothing here ships Questie data or code, and the index built from the player's
// install is cached only in their local cache folder. See docs/design/questie-interop.md.
package questie

import (
	"bufio"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// SupportedContract is the QuestieDB baked-storage contract this reader understands
// (the TOC's X-Contract-Version).
const SupportedContract = 4

// Status is what is installed. Readable is true only when the database is in a format this reader
// understands; Reason says why not otherwise.
type Status struct {
	Questie  string // Questie's version, "" when not installed
	DB       string // QuestieDB's version, "" when not installed
	Flavor   string
	TOC      string // path of the QuestieDB TOC that holds the data
	Readable bool
	Reason   string
}

var tocField = regexp.MustCompile(`(?m)^##\s*([A-Za-z-]+):\s*(\S.*?)\s*$`)

// readHeader returns the "## Key: value" lines at the top of a TOC (up to the first file line).
func readHeader(path string) (map[string]string, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	out := map[string]string{}
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		l := sc.Text()
		if strings.HasPrefix(l, "## X-Quest-") || strings.HasPrefix(l, "## X-Npc-") {
			break // entity rows follow the header
		}
		if m := tocField.FindStringSubmatch(l); m != nil {
			out[m[1]] = m[2]
		}
	}
	return out, sc.Err()
}

func addons(game string) string { return filepath.Join(game, "Interface", "AddOns") }

// Probe looks for <game>/Interface/AddOns/Questie and QuestieDB and reads only their TOC headers.
// It never fails: an unreadable database is reported in the Status.
func Probe(game string) Status {
	var s Status
	if h, err := readHeader(filepath.Join(addons(game), "Questie", "Questie.toc")); err == nil {
		s.Questie = h["Version"]
		if s.Questie == "" {
			s.Questie = "unknown"
		}
	}
	dir := filepath.Join(addons(game), "QuestieDB")
	if st, err := os.Stat(dir); err != nil || !st.IsDir() {
		return s
	}
	s.DB = "unknown"
	tocs, _ := filepath.Glob(filepath.Join(dir, "QuestieDB*.toc"))
	sort.Strings(tocs)
	// The client prefers a flavour-suffixed TOC; the Forever build is the one this advisor reads.
	for _, t := range tocs {
		if strings.EqualFold(filepath.Base(t), "QuestieDB_Forever.toc") {
			tocs = []string{t}
			break
		}
	}
	if len(tocs) == 0 {
		s.Reason = "no QuestieDB TOC file"
		return s
	}
	s.TOC = tocs[0]
	h, err := readHeader(s.TOC)
	if err != nil {
		s.Reason = "cannot read its TOC"
		return s
	}
	if v := h["Version"]; v != "" {
		s.DB = v
	}
	contract, _ := strconv.Atoi(h["X-Contract-Version"])
	switch {
	case h["X-Mode"] != "baked":
		s.Reason = "not a generated (baked) build"
	case contract != SupportedContract:
		s.Reason = fmt.Sprintf("storage contract %q, expected %d", h["X-Contract-Version"], SupportedContract)
	default:
		s.Readable = true
	}
	return s
}

// Describe is the one-line setup text for the status.
func (s Status) Describe() (ok bool, text string) {
	switch {
	case s.Questie == "" && s.DB == "":
		return false, "Not installed; quest suggestions limited to ForeverScribe's saved data."
	case s.DB == "":
		return false, "Questie " + vers(s.Questie) + " found, but not its QuestieDB addon; quest suggestions limited."
	case !s.Readable:
		return false, "QuestieDB unreadable (" + vers(s.DB) + "): " + s.Reason + "; quest suggestions limited."
	case s.Questie == "":
		return true, "QuestieDB " + vers(s.DB) + " found (Questie itself not found)."
	}
	return true, "Found Questie " + vers(s.Questie) + " with QuestieDB " + vers(s.DB) + "."
}

func vers(v string) string {
	if v == "unknown" {
		return v
	}
	return "v" + strings.TrimPrefix(v, "v")
}

// parseLine splits "## X-<Type>-<id>-<field>[-<chunk>]: <value>". chunk is 0 for a whole value and
// the 1-based part number for one piece of a long value, which a "~N~" head line announces.
func parseLine(l string) (typ string, id int, field string, chunk int, val string, ok bool) {
	colon := strings.Index(l, ": ")
	if !strings.HasPrefix(l, "## X-") || colon < 0 {
		return
	}
	parts := strings.Split(l[5:colon], "-")
	if len(parts) != 3 && len(parts) != 4 {
		return
	}
	id, err := strconv.Atoi(parts[1])
	if err != nil {
		return
	}
	if len(parts) == 4 {
		if chunk, err = strconv.Atoi(parts[3]); err != nil {
			return
		}
	}
	return parts[0], id, parts[2], chunk, l[colon+2:], true
}

func decodeField(val string) (any, error) {
	raw, err := base64.StdEncoding.DecodeString(val)
	if err != nil {
		return nil, err
	}
	return decodeCBOR(raw)
}

type rawRow map[string]any

// wanted are the fields the reader keeps; every other line is skipped without decoding.
var wanted = map[string]map[string]bool{
	"Quest":  {"S": true, "2": true, "3": true, "10": true, "12": true, "13": true, "16": true},
	"Npc":    {"S": true, "7": true},
	"Object": {"S": true, "4": true},
}

// ErrUnreadable means the database is not in a format this reader understands.
var ErrUnreadable = errors.New("QuestieDB format not recognised")

func (r rawRow) scalars() map[any]any {
	m, _ := r["S"].(map[any]any)
	return m
}
