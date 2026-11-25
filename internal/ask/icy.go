package ask

import (
	"context"
	"encoding/json"
	"errors"
	"html"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/WoW-Companion/advisor/internal/provider"
)

// Icy Veins is read only at runtime, by the player's own AI client, when the player presses Check Icy
// Veins. Nothing is stored or shipped, and no text of it is copied into this repository.

// IcyTool is the name of the fetch tool the API agent loop gets for that button.
const IcyTool = "icy_veins_fetch"

// icyPrefix is the only part of Icy Veins the fetch tool may read.
const icyPrefix = "/wow-forever/"

// ErrIcyURL is why a fetch was refused.
var ErrIcyURL = errors.New("only https://www.icy-veins.com/wow-forever/ pages can be fetched")

// AllowedIcyURL parses raw and returns it when it is an https page under icy-veins.com/wow-forever/.
func AllowedIcyURL(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil || u.Scheme != "https" || u.User != nil || u.Port() != "" {
		return nil, ErrIcyURL
	}
	if h := strings.ToLower(u.Hostname()); h != "www.icy-veins.com" && h != "icy-veins.com" {
		return nil, ErrIcyURL
	}
	if strings.Contains(u.Path, "..") || !strings.HasPrefix(u.Path, icyPrefix) || len(u.Path) == len(icyPrefix) {
		return nil, ErrIcyURL
	}
	return u, nil
}

// CanBrowse reports whether the client that will answer can read a web page: an API client (the
// advisor gives it the fetch tool) or Claude Code (its own WebFetch). Other CLI clients can't, so the
// Check Icy Veins button is hidden for them.
func CanBrowse(cfg Config) bool {
	return cfg.API != nil || ClientName(cfg.Command) == "claude"
}

// WithBrowse lets Claude Code's own WebFetch read Icy Veins and nothing else: the built-in tool list
// is no longer empty and the one tool is pre-approved for the site. Other argv is returned unchanged.
func WithBrowse(argv, command []string) []string {
	if ClientName(command) != "claude" {
		return argv
	}
	out := append([]string{}, argv...)
	for i := 0; i+1 < len(out); i++ {
		switch out[i] {
		case "--tools":
			if out[i+1] == "" {
				out[i+1] = "WebFetch"
			}
		case "--allowedTools":
			out[i+1] += ",WebFetch(domain:www.icy-veins.com),WebFetch(domain:icy-veins.com)"
		}
	}
	return out
}

// icyTools adds the fetch tool to an agent loop's tools and remembers the pages it read.
type icyTools struct {
	provider.Tools
	hc    *http.Client
	mu    sync.Mutex
	pages []string
}

func newIcyTools(inner provider.Tools, hc *http.Client) *icyTools {
	if hc == nil {
		hc = &http.Client{Timeout: 20 * time.Second}
	}
	c := *hc
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) > 3 {
			return errors.New("too many redirects")
		}
		_, err := AllowedIcyURL(req.URL.String())
		return err
	}
	return &icyTools{Tools: inner, hc: &c}
}

func (t *icyTools) List(ctx context.Context) ([]provider.ToolDef, error) {
	defs, err := t.Tools.List(ctx)
	if err != nil {
		return nil, err
	}
	return append(defs, provider.ToolDef{Name: IcyTool,
		Description: "Read one WoW Forever guide page of Icy Veins (an https://www.icy-veins.com/wow-forever/ URL) as plain text, to refine tactics. Any other address is refused.",
		Schema:      json.RawMessage(`{"type":"object","properties":{"url":{"type":"string","description":"the guide page, https://www.icy-veins.com/wow-forever/..."}},"required":["url"]}`)}), nil
}

func (t *icyTools) Call(ctx context.Context, name string, args json.RawMessage) (string, bool, error) {
	if name != IcyTool {
		return t.Tools.Call(ctx, name, args)
	}
	var in struct {
		URL string `json:"url"`
	}
	if json.Unmarshal(args, &in) != nil {
		return "the arguments weren't valid JSON", true, nil
	}
	u, err := AllowedIcyURL(in.URL)
	if err != nil {
		return err.Error(), true, nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return err.Error(), true, nil
	}
	req.Header.Set("User-Agent", "wowc-advisor")
	resp, err := t.hc.Do(req)
	if err != nil {
		return "couldn't read the page", true, nil
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "the page answered " + resp.Status, true, nil
	}
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 2<<20))
	t.mu.Lock()
	t.pages = append(t.pages, u.String())
	t.mu.Unlock()
	return pageText(string(body), 20000), false, nil
}

// Pages are the guide pages read, in order.
func (t *icyTools) Pages() []string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return append([]string(nil), t.pages...)
}

var (
	dropBlocks = regexp.MustCompile(`(?is)<(script|style|noscript|svg|nav|footer|header)\b.*?</(script|style|noscript|svg|nav|footer|header)>`)
	tagRE      = regexp.MustCompile(`(?s)<[^>]*>`)
	spaceRE    = regexp.MustCompile(`[ \t\r\f\v]+`)
	lineRE     = regexp.MustCompile(`\n\s*\n+`)
)

// pageText is html reduced to its text, cut at max runes.
func pageText(h string, max int) string {
	h = dropBlocks.ReplaceAllString(h, " ")
	h = regexp.MustCompile(`(?i)</?(p|div|li|h[1-6]|br|tr|ul|ol|table)\b[^>]*>`).ReplaceAllString(h, "\n")
	h = html.UnescapeString(tagRE.ReplaceAllString(h, " "))
	h = lineRE.ReplaceAllString(spaceRE.ReplaceAllString(h, " "), "\n")
	h = strings.TrimSpace(h)
	if r := []rune(h); len(r) > max {
		h = string(r[:max])
	}
	return h
}

// IcyPage is the first Icy Veins guide page among urls, "" when none (a URL a client's own tool read is
// checked the same way the fetch tool checks it).
func IcyPage(urls []string) string {
	for _, u := range urls {
		if p, err := AllowedIcyURL(u); err == nil {
			return p.String()
		}
	}
	return ""
}
