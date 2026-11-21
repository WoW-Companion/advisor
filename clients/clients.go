// Package clients holds the instruction files the advisor ships for each agent client. They are
// all generated from body.md; a test keeps the committed copies in sync.
package clients

import (
	_ "embed"
	"strings"
)

//go:embed body.md
var body string

// File is one generated instruction file, by path relative to the clients directory.
type File struct {
	Path string
	Text string
}

const skillHead = `---
name: wow-companion
description: Advice about the player's own World of Warcraft: Forever character (level, gear, professions, quests, talents, dungeons, spec). Use whenever the player asks what to do, wear, learn, quest or run next for their character, or asks about their character at all.
---

<!--
Generated from clients/body.md; do not edit by hand (go test ./clients -update rewrites it).
Format source: Claude Code skills docs, https://code.claude.com/docs/en/skills
(SKILL.md with YAML frontmatter on the first line; claude.ai uploads allow only name,
description, license, compatibility, metadata, allowed-tools).
-->

`

const agentsHead = `<!-- Generated from clients/body.md; do not edit by hand. Codex reads AGENTS.md:
https://learn.chatgpt.com/docs/agent-configuration/agents-md -->

`

const geminiHead = `<!-- Generated from clients/body.md; do not edit by hand. Gemini CLI reads GEMINI.md:
https://geminicli.com/docs/cli/gemini-md/ -->

`

// Files returns every generated instruction file.
func Files() []File {
	b := strings.TrimLeft(body, "\n")
	return []File{
		{"claude/wow-companion/SKILL.md", skillHead + b},
		{"codex/AGENTS.md", agentsHead + b},
		{"gemini/GEMINI.md", geminiHead + b},
	}
}
