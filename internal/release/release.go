// Package release looks up the latest published advisor release. It only reads: it never
// downloads or replaces the binary.
package release

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LatestURL is the unauthenticated GitHub endpoint for the newest release.
const LatestURL = "https://api.github.com/repos/WoW-Companion/advisor/releases/latest"

// Result is what a lookup found.
type Result struct {
	Latest string // the release tag, for example "v0.1.0"
	Newer  bool   // Latest is newer than the running version
	Known  bool   // the running version is a release number that can be compared
}

// Check asks url for the latest release and compares it with current.
func Check(ctx context.Context, client *http.Client, url, current string) (Result, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Result{}, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "wowc-advisor")
	resp, err := client.Do(req)
	if err != nil {
		return Result{}, err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusNotFound {
		return Result{}, errors.New("no release has been published yet")
	}
	if resp.StatusCode != http.StatusOK {
		return Result{}, fmt.Errorf("release lookup: %s", resp.Status)
	}
	var body struct {
		Tag string `json:"tag_name"`
	}
	if err := json.NewDecoder(io.LimitReader(resp.Body, 1<<20)).Decode(&body); err != nil {
		return Result{}, fmt.Errorf("release lookup: %w", err)
	}
	if body.Tag == "" {
		return Result{}, errors.New("release lookup: no tag_name in the answer")
	}
	cmp, ok := Compare(body.Tag, current)
	return Result{Latest: body.Tag, Newer: ok && cmp > 0, Known: ok}, nil
}

// Compare orders two dotted release numbers, ignoring a leading "v" and any "-suffix" or
// "+build" part. It returns ok=false when either is not made of numbers (a "dev" build).
func Compare(a, b string) (cmp int, ok bool) {
	pa, oka := parse(a)
	pb, okb := parse(b)
	if !oka || !okb {
		return 0, false
	}
	for i := 0; i < max(len(pa), len(pb)); i++ {
		var x, y int
		if i < len(pa) {
			x = pa[i]
		}
		if i < len(pb) {
			y = pb[i]
		}
		if x != y {
			if x > y {
				return 1, true
			}
			return -1, true
		}
	}
	return 0, true
}

func parse(v string) ([]int, bool) {
	v = strings.TrimPrefix(strings.TrimSpace(v), "v")
	if i := strings.IndexAny(v, "-+"); i >= 0 {
		v = v[:i]
	}
	var out []int
	for _, p := range strings.Split(v, ".") {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil, false
		}
		out = append(out, n)
	}
	return out, true
}
