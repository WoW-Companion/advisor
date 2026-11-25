package gamesetup

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"strings"
	"time"

	"github.com/WoW-Companion/advisor/internal/release"
)

// AddonLatestURL is the addon repo's latest-release endpoint.
const AddonLatestURL = "https://api.github.com/repos/WoW-Companion/addon/releases/latest"

// Download limits.
const (
	maxZip  = 16 << 20
	maxSums = 64 << 10
	maxFile = 8 << 20 // one extracted file
)

// The folders the release zip may hold. Anything else, WoWCompanion_Answers included, is refused.
var allowedTop = map[string]bool{"WoWCompanion": true, "WoWCompanion_Digest": true}

// AddonRelease is the newest addon release.
type AddonRelease struct {
	Version  string // without a leading v
	ZipURL   string
	SumsURL  string
	ZipName  string
	Checksum bool // a SHA256SUMS asset exists
}

// httpsGitHub accepts only https URLs on github.com; the CDN a redirect lands on is checked by
// downloadClient.
func httpsGitHub(raw string) error {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || (u.Host != "github.com" && u.Host != "api.github.com") {
		return fmt.Errorf("refusing %q: only https github.com URLs are fetched", raw)
	}
	return nil
}

// strictClient follows redirects only over https (GitHub serves assets from its own CDN).
func strictClient(c *http.Client) *http.Client {
	if c == nil {
		c = http.DefaultClient
	}
	cc := *c
	cc.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if req.URL.Scheme != "https" {
			return errors.New("refusing a redirect off https")
		}
		if len(via) > 5 {
			return errors.New("too many redirects")
		}
		return nil
	}
	return &cc
}

func get(ctx context.Context, c *http.Client, raw string, limit int64, accept string) ([]byte, error) {
	if err := httpsGitHub(raw); err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", "wowc-advisor")
	if accept != "" {
		req.Header.Set("Accept", accept)
	}
	resp, err := strictClient(c).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("download: %s", resp.Status)
	}
	b, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(b)) > limit {
		return nil, errors.New("download is larger than allowed")
	}
	return b, nil
}

// LatestAddon looks up the newest addon release. It downloads nothing but the release's JSON.
func LatestAddon(ctx context.Context, c *http.Client, apiURL string) (*AddonRelease, error) {
	b, err := get(ctx, c, apiURL, 1<<20, "application/vnd.github+json")
	if err != nil {
		return nil, err
	}
	var body struct {
		Tag    string `json:"tag_name"`
		Assets []struct {
			Name string `json:"name"`
			URL  string `json:"browser_download_url"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(b, &body); err != nil {
		return nil, fmt.Errorf("release lookup: %w", err)
	}
	r := &AddonRelease{Version: strings.TrimPrefix(body.Tag, "v")}
	if r.Version == "" {
		return nil, errors.New("release lookup: no tag in the answer")
	}
	want := "WoWCompanion-" + r.Version + ".zip"
	for _, a := range body.Assets {
		switch a.Name {
		case want:
			r.ZipName, r.ZipURL = a.Name, a.URL
		case "SHA256SUMS":
			r.SumsURL, r.Checksum = a.URL, true
		}
	}
	if r.ZipURL == "" {
		return nil, fmt.Errorf("release %s has no %s", body.Tag, want)
	}
	return r, nil
}

// InstallAddon downloads the release zip and its SHA256SUMS, verifies the hash and extracts into
// <game>/Interface/AddOns. Nothing is written when the checksum is missing or does not match.
func InstallAddon(ctx context.Context, c *http.Client, game string, r *AddonRelease) error {
	_, err := InstallAddonNote(ctx, c, game, r)
	return err
}

// VerifySum checks data's SHA-256 against the line for name in a sha256sum-format file.
func VerifySum(data []byte, name string, sums []byte) error {
	got := sha256.Sum256(data)
	for _, line := range strings.Split(string(sums), "\n") {
		f := strings.Fields(line)
		if len(f) == 2 && strings.TrimPrefix(f[1], "*") == name {
			if strings.EqualFold(f[0], hex.EncodeToString(got[:])) {
				return nil
			}
			return fmt.Errorf("checksum mismatch for %s: refusing to install", name)
		}
	}
	return fmt.Errorf("SHA256SUMS has no line for %s: refusing to install", name)
}

// safeName is the clean slash path of a zip entry, or an error for one that is absolute, climbs
// out with "..", names a drive, or reaches outside the two addon folders or into protected data.
func safeName(raw string) (string, error) {
	if raw == "" || strings.ContainsAny(raw, "\\\x00") || strings.HasPrefix(raw, "/") || (len(raw) > 1 && raw[1] == ':') {
		return "", fmt.Errorf("refusing zip entry %q: absolute or unsafe path", raw)
	}
	for _, seg := range strings.Split(raw, "/") {
		if seg == ".." {
			return "", fmt.Errorf("refusing zip entry %q: contains ..", raw)
		}
	}
	p := path.Clean(raw)
	top, rest, _ := strings.Cut(p, "/")
	if !allowedTop[top] {
		return "", fmt.Errorf("refusing zip entry %q: outside the WoW Companion addon folders", raw)
	}
	if strings.EqualFold(p, "WoWCompanion_Digest/Digest.lua") || strings.EqualFold(rest, "..") {
		return "", fmt.Errorf("refusing zip entry %q: it would replace your data file", raw)
	}
	return p, nil
}

// ExtractAddon validates every entry first, then writes. The zip must not hold Digest.lua or
// anything outside WoWCompanion/ and WoWCompanion_Digest/. addons is the Interface/AddOns folder.
func ExtractAddon(zipb []byte, addons string) error {
	zr, err := zip.NewReader(bytes.NewReader(zipb), int64(len(zipb)))
	if err != nil {
		return fmt.Errorf("not a valid zip: %w", err)
	}
	type job struct {
		f    *zip.File
		dest string
	}
	var jobs []job
	for _, f := range zr.File {
		name, err := safeName(f.Name)
		if err != nil {
			return err
		}
		if f.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("refusing zip entry %q: a link", f.Name)
		}
		if f.FileInfo().IsDir() {
			continue
		}
		if f.UncompressedSize64 > maxFile {
			return fmt.Errorf("refusing zip entry %q: too large", f.Name)
		}
		jobs = append(jobs, job{f, filepath.Join(addons, filepath.FromSlash(name))})
	}
	if len(jobs) == 0 {
		return errors.New("the zip holds no addon files")
	}
	for _, j := range jobs {
		if err := writeEntry(j.f, j.dest); err != nil {
			return err
		}
	}
	return nil
}

func writeEntry(f *zip.File, dest string) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	rc, err := f.Open()
	if err != nil {
		return err
	}
	defer rc.Close()
	tmp, err := os.CreateTemp(filepath.Dir(dest), ".install-*.tmp")
	if err != nil {
		return err
	}
	_, cerr := io.Copy(tmp, io.LimitReader(rc, maxFile+1))
	if e := tmp.Close(); cerr == nil {
		cerr = e
	}
	if cerr == nil {
		cerr = os.Chmod(tmp.Name(), 0o644)
	}
	if cerr == nil {
		cerr = os.Rename(tmp.Name(), dest)
	}
	if cerr != nil {
		os.Remove(tmp.Name())
	}
	return cerr
}

// AddonOffer is what the addon check may put in front of the player: nothing, an install or update
// to Version, or (Reinstall) going back from an installed version that has been pulled.
type AddonOffer struct {
	Version   string // "" when nothing is offered
	Reinstall bool   // the installed version is pulled and Version is the one to go back to
	Reason    string // why the installed version was pulled (Reinstall, or no safe version to offer)
	Pulled    bool   // the installed version is pulled (whether or not a safe one exists to go back to)
}

func pullFor(pulls []release.Pull, version string) (release.Pull, bool) {
	v := strings.TrimPrefix(strings.TrimSpace(version), "v")
	for _, p := range pulls {
		if strings.TrimPrefix(strings.TrimSpace(p.Version), "v") == v {
			return p, true
		}
	}
	return release.Pull{}, false
}

// PlanAddon decides the addon offer. A pulled version is never offered: when the newest release is
// pulled its back_to stands in (and so on, a few steps), and when the installed version is pulled
// the offer is to reinstall the version it goes back to. have is the installed version, "" when the
// addon is not installed.
func PlanAddon(have, latest string, pulls []release.Pull) AddonOffer {
	safe := latest
	for i := 0; i < 4 && safe != ""; i++ {
		p, bad := pullFor(pulls, safe)
		if !bad {
			break
		}
		safe = strings.TrimPrefix(p.BackTo, "v")
	}
	if _, bad := pullFor(pulls, safe); bad {
		safe = ""
	}
	if have == "" {
		return AddonOffer{Version: safe}
	}
	if p, bad := pullFor(pulls, have); bad {
		target := strings.TrimPrefix(strings.TrimSpace(p.BackTo), "v")
		if _, tb := pullFor(pulls, target); tb || target == "" {
			target = ""
			if safe != "" && safe != have {
				target = safe // the newest release that is not pulled
			}
		}
		return AddonOffer{Version: target, Reinstall: target != "", Reason: p.Reason, Pulled: true}
	}
	if safe != "" && Newer(safe, have) {
		return AddonOffer{Version: safe}
	}
	return AddonOffer{}
}

// NewFiles lists the addon files in the zip that are not in the addons folder yet. The game reads an
// addon's file list only when it starts, so a release that adds files needs a full restart, and one
// that only changes files needs only /reload.
func NewFiles(zipb []byte, addons string) ([]string, error) {
	zr, err := zip.NewReader(bytes.NewReader(zipb), int64(len(zipb)))
	if err != nil {
		return nil, fmt.Errorf("not a valid zip: %w", err)
	}
	var out []string
	for _, f := range zr.File {
		if f.FileInfo().IsDir() {
			continue
		}
		name, err := safeName(f.Name)
		if err != nil {
			return nil, err
		}
		if _, err := os.Stat(filepath.Join(addons, filepath.FromSlash(name))); err != nil {
			out = append(out, name)
		}
	}
	return out, nil
}

// ReloadNote is the sentence an addon update notice ends with.
func ReloadNote(newFiles []string) string {
	if len(newFiles) == 0 {
		return "Type /reload in game to use it."
	}
	return "This version adds new files, so quit the game fully and start it again (a /reload is not enough)."
}

// InstallAddonNote is InstallAddon that also says whether the game needs /reload or a full restart.
func InstallAddonNote(ctx context.Context, c *http.Client, game string, r *AddonRelease) (string, error) {
	if r == nil || r.ZipURL == "" {
		return "", errors.New("no release to install")
	}
	if r.SumsURL == "" {
		return "", errors.New("the release has no SHA256SUMS; refusing an unverified install")
	}
	zipb, err := get(ctx, c, r.ZipURL, maxZip, "")
	if err != nil {
		return "", err
	}
	sums, err := get(ctx, c, r.SumsURL, maxSums, "")
	if err != nil {
		return "", err
	}
	if err := VerifySum(zipb, r.ZipName, sums); err != nil {
		return "", err
	}
	addons := filepath.Join(game, "Interface", "AddOns")
	fresh, err := NewFiles(zipb, addons)
	if err != nil {
		return "", err
	}
	if err := ExtractAddon(zipb, addons); err != nil {
		return "", err
	}
	return ReloadNote(fresh), nil
}

// AddonTagURL is the release endpoint for one addon version.
func AddonTagURL(version string) string {
	return "https://api.github.com/repos/WoW-Companion/addon/releases/tags/v" + strings.TrimPrefix(version, "v")
}
