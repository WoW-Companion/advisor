// Package ask answers the questions the addon queues with /ask (schema 1, see docs/ask.md). It
// reads WoWCompanionDB.asks from the addon's SavedVariables file, runs a headless agent CLI once
// per unanswered question, and writes the WoWCompanion_Answers slot addon's Answers.lua.
//
// The agent gets the question and the character key, never the capture: it reads facts through
// the advisor's MCP tools.
package ask

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/WoW-Companion/advisor/internal/lua"
)

// Schema is the Answers.lua format version this package writes.
const Schema = 1

// SlotAddon is the data-only addon the answers are written into.
const SlotAddon = "WoWCompanion_Answers"

// Limits. MaxAnswer is in characters. MaxOutput bounds what is read from the agent at all.
const (
	MaxAnswer        = 1500
	MaxOutput        = 64 << 10
	DefaultTimeout   = 2 * time.Minute
	DefaultMaxPerRun = 5
)

// ServerName is the advisor's name in the generated MCP config and in each CLI's own settings.
const ServerName = "wowc-advisor"

// Presets are the documented agent commands (docs/ask.md lists what each passes). {prompt} is the
// question text, {mcp_config} the path of a generated MCP config file, {mcp_codex} the advisor server
// as a Codex inline config table, {mcp_policy} the path of a generated Gemini policy file; a command
// without {prompt} gets the prompt as its last argument.
//
// Each preset pre-allows the advisor's tools and nothing else, because a headless run refuses what
// it cannot ask about.
var Presets = map[string][]string{
	// --allowedTools names the whole server; every other tool is denied by name as well, and
	// --strict-mcp-config keeps the player's own MCP servers out.
	"claude": {"claude", "-p", "{prompt}", "--mcp-config", "{mcp_config}", "--strict-mcp-config",
		"--allowedTools", "mcp__" + ServerName,
		"--disallowedTools", "Bash", "Edit", "Write", "NotebookEdit", "WebFetch", "WebSearch", "Read", "Glob", "Grep"},
	// The server is defined and its tools approved on the command line (nothing written to the
	// player's config); the sandbox is read-only.
	"codex": {"codex", "exec", "--skip-git-repo-check", "-s", "read-only", "-c", "mcp_servers." + ServerName + "={mcp_codex}", "{prompt}"},
	// The server comes from the player's gemini settings (setup gemini); the policy allows its
	// tools and denies shell, file writes and the web.
	"gemini": {"gemini", "--allowed-mcp-server-names", ServerName, "--policy", "{mcp_policy}", "-p", "{prompt}"},
}

// Tools are the advisor tool names; an answer that names one gets it in its sources line.
var Tools = []string{"character_brief", "list_characters", "server_info", "update_digest", "upgrade_finder", "whats_next"}

// Question is one queued question.
type Question struct {
	ID        string
	Character string
	Text      string
	AskedAt   int64
	Answered  bool
}

// Answer is one entry of Answers.lua.
type Answer struct {
	AnsweredAt int64
	Text       string
	Sources    []string
}

// Questions reads WoWCompanionDB.asks out of a SavedVariables file's text. Entries without an id or
// text are skipped.
func Questions(text string) ([]Question, error) {
	vars, err := lua.Parse(text)
	if err != nil {
		return nil, err
	}
	db, _ := vars["WoWCompanionDB"].(lua.Table)
	var qs []Question
	for _, e := range lua.List(db["asks"]) {
		t, ok := e.(lua.Table)
		if !ok {
			continue
		}
		q := Question{ID: str(t["id"]), Character: str(t["character"]), Text: str(t["text"])}
		q.AskedAt, _ = t["asked_at"].(int64)
		q.Answered, _ = t["answered"].(bool)
		if q.ID != "" && strings.TrimSpace(q.Text) != "" {
			qs = append(qs, q)
		}
	}
	return qs, nil
}

func str(v any) string { s, _ := v.(string); return s }

// Config is one pass's settings.
type Config struct {
	SavedVariables string   // the addon's WoWCompanion.lua
	AnswersPath    string   // Answers.lua to write
	Command        []string // the agent argv, see Presets; empty when no agent is set up (the companion only)
	Name, Source   string   // the agent's short name and where it came from (see Agent); may be empty
	MCPConfig      string   // path of the MCP config file handed to the agent; may be empty
	Options        Options  // answer length and model tier, see Options
	Timeout        time.Duration
	MaxPerRun      int
	Now            func() time.Time
}

func (c Config) now() time.Time {
	if c.Now != nil {
		return c.Now()
	}
	return time.Now()
}

// Result says what a pass did.
type Result struct {
	Answered int // new answers written, failures included
	Failed   int // of those, how many are error answers
	Pending  int // unanswered questions left over the per-run limit
}

// Run is one pass: answer each unanswered question (up to MaxPerRun), then rewrite Answers.lua. The
// file keeps the answers it already holds for questions still in the queue.
func Run(ctx context.Context, cfg Config) (Result, error) {
	var res Result
	raw, err := os.ReadFile(cfg.SavedVariables)
	if err != nil {
		return res, err
	}
	qs, err := Questions(string(raw))
	if err != nil {
		return res, fmt.Errorf("%s: %w", cfg.SavedVariables, err)
	}
	have := ReadAnswers(cfg.AnswersPath)
	answers := map[string]Answer{}
	var todo []Question
	for _, q := range qs {
		if a, ok := have[q.ID]; ok {
			answers[q.ID] = a
		} else if !q.Answered {
			todo = append(todo, q)
		}
	}
	limit := cfg.MaxPerRun
	if limit <= 0 {
		limit = DefaultMaxPerRun
	}
	if len(todo) > limit {
		res.Pending = len(todo) - limit
		todo = todo[:limit]
	}
	for _, q := range todo {
		if ctx.Err() != nil {
			return res, ctx.Err()
		}
		a := Answer{AnsweredAt: cfg.now().Unix()}
		out, err := runAgent(ctx, cfg, q)
		if err != nil {
			res.Failed++
			a.Text = "Could not answer: " + err.Error()
		} else {
			a.Text, a.Sources = Clean(out), Sources(out)
		}
		answers[q.ID] = a
		res.Answered++
	}
	if res.Answered == 0 && fileExists(cfg.AnswersPath) {
		return res, nil
	}
	return res, WriteAnswers(cfg.AnswersPath, answers)
}

func fileExists(p string) bool { _, err := os.Stat(p); return err == nil }

// runAgent runs the agent for q. The error is short and fit to show the player.
func runAgent(ctx context.Context, cfg Config, q Question) (string, error) {
	var out bytes.Buffer
	if err := execAgent(ctx, cfg, q, PromptFor(q, cfg.Options.Length), nil, &capped{w: &out, left: MaxOutput}); err != nil {
		return "", err
	}
	if strings.TrimSpace(out.String()) == "" {
		return "", errors.New("the agent gave no answer")
	}
	return out.String(), nil
}

// execAgent runs the agent once with prompt, extra argv appended, and its stdout sent to stdout.
func execAgent(ctx context.Context, cfg Config, q Question, prompt string, extra []string, stdout io.Writer) error {
	if len(cfg.Command) == 0 {
		return errors.New("no agent command is configured")
	}
	timeout := cfg.Timeout
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	argv := append(WithModel(Argv(cfg.Command, prompt, cfg.MCPConfig), cfg.Command, cfg.Options), extra...)
	cmd, err := Launch(ctx, argv, "WOWC_QUESTION="+q.Text, "WOWC_CHARACTER="+q.Character, "WOWC_MCP_CONFIG="+cfg.MCPConfig)
	if err != nil {
		return err
	}
	cmd.WaitDelay = time.Second
	var errOut bytes.Buffer
	out := &counter{w: stdout}
	cmd.Stdout = out
	cmd.Stderr = &capped{w: &errOut, left: 2048}
	err = cmd.Run()
	switch {
	case ctx.Err() == context.DeadlineExceeded:
		return fmt.Errorf("the agent took longer than %s", timeout)
	case ctx.Err() != nil:
		return ctx.Err()
	case err != nil:
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return exitError(ee, out.n > 0, errOut.String())
		}
		return startError(cmd.Path, err)
	}
	return nil
}

// capped keeps the first left bytes and discards the rest, so a chatty agent cannot fill memory.
type capped struct {
	w    io.Writer
	left int
}

func (c *capped) Write(p []byte) (int, error) {
	n := len(p)
	if len(p) > c.left {
		p = p[:c.left]
	}
	c.left -= len(p)
	_, err := c.w.Write(p)
	return n, err
}

// Prompt is PromptFor at the default (Standard) length.
func Prompt(q Question) string { return PromptFor(q, LengthStandard) }

// PromptFor is what the agent is asked: the question and the character key, where the facts are, what
// to say if the tools are missing, and how long the answer should be. It is kept short on purpose,
// as it is sent with every ask.
func PromptFor(q Question, length string) string {
	r := lengthRule[Options{Length: length}.Normalize().Length]
	return fmt.Sprintf("You are the WoW Companion advisor. Character key: %s. Use the %s MCP tools for facts about this character or the game (brief=true on upgrade_finder and dungeon_plan); do not guess. "+
		"Never tell the player to change permissions or settings. If the tools are refused or missing, answer exactly: %q "+
		"%s Plain text, under %d characters, no markdown.\n\nQuestion: %s", q.Character, ServerName, ToolsUnavailable, r.text, r.limit, q.Text)
}

// ToolsUnavailable is the answer when the AI client would not run the advisor's tools.
const ToolsUnavailable = "The advisor's tools weren't available to the AI client (setup problem)."

// Argv fills the placeholders in the command template; with no {prompt}, the prompt is appended.
// An argument that needs the MCP config is dropped, with the flag before it, when there is none.
func Argv(tmpl []string, prompt, mcpConfig string) []string {
	var out []string
	hasPrompt := false
	codex := codexServer(mcpConfig)
	for _, a := range tmpl {
		if mcpConfig == "" && (strings.Contains(a, "{mcp_config}") || strings.Contains(a, "{mcp_codex}") || strings.Contains(a, "{mcp_policy}")) {
			if len(out) > 0 && strings.HasPrefix(out[len(out)-1], "-") {
				out = out[:len(out)-1]
			}
			continue
		}
		hasPrompt = hasPrompt || strings.Contains(a, "{prompt}")
		out = append(out, strings.NewReplacer("{prompt}", prompt, "{mcp_config}", mcpConfig,
			"{mcp_codex}", codex, "{mcp_policy}", PolicyPath(mcpConfig)).Replace(a))
	}
	if !hasPrompt {
		out = append(out, prompt)
	}
	return out
}

// Clean trims the agent's output and cuts it to MaxAnswer characters, at a rune boundary.
func Clean(s string) string { return CleanTo(s, MaxAnswer) }

// CleanTo is Clean with a limit of max characters.
func CleanTo(s string, max int) string {
	s = strings.ToValidUTF8(strings.TrimSpace(s), "")
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	return strings.TrimRight(string([]rune(s)[:max-3]), " \n") + "..."
}

// Sources lists the advisor tools the output names, in Tools order.
func Sources(s string) []string {
	var out []string
	for _, t := range Tools {
		if strings.Contains(s, t) {
			out = append(out, t)
		}
	}
	return out
}
