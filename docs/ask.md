# Asking questions

Questions are asked in the overlay (or the browser companion, `wowc-advisor companion`): type one, or
click a suggestion or a quick button. The overlay passes it to your AI client, which answers from your
character's data. Nothing is written to the game folder.

In game, the right-click **Ask about this** and the panel's question chips **copy** the question. Paste it
into the overlay. The old in-game `/ask` queue, the "asked in game" cards and `Answers.lua` are gone;
`wowc-advisor ask` was removed with them. Leftover `WoWCompanion_Answers` folders and files are no
longer read and can be deleted.

## The agent

`--agent` takes a preset or a command; without it, `"agent"` is read from
`<user config dir>/wowc-advisor/ask.json` (`--config` to move it): `{"agent": "codex"}`.

| Preset | Runs |
| ------ | ---- |
| `claude` | `claude -p <prompt> --mcp-config <file> --strict-mcp-config --tools "" --setting-sources "" --disable-slash-commands --no-session-persistence --exclude-dynamic-system-prompt-sections --allowedTools mcp__wowc-advisor` |

| `codex` | `codex exec --skip-git-repo-check --ephemeral -s read-only -c 'mcp_servers.wowc-advisor={ command, args, env, default_tools_approval_mode = "approve" }' <prompt>` |
| `agy` | `agy -p <prompt>` (run in a per-run folder holding `.agents/mcp_config.json`; `--effort <level>` is added) |
| `gemini` (legacy, API key only) | `gemini --allowed-mcp-server-names wowc-advisor --policy <file> -p <prompt>` |

### What each preset allows

A headless run cannot ask permission, so a tool that is not pre-approved is refused. Each preset
pre-approves the advisor's tools and nothing else. They are read-only, except `update_digest`, which
writes the digest file.

| Preset | Advisor tools | Everything else | The player's other MCP servers |
| ------ | ------------- | --------------- | ------------------------------ |
| `claude` | allowed (`mcp__wowc-advisor`, the whole server) | no built-in tools (`--tools ""`); only the advisor server is callable | not loaded (`--strict-mcp-config`) |
| `codex` | server defined on the command line (nothing is written to `~/.codex`), its tools set to `approve` | read-only sandbox; no approval bypass | still loaded from the player's Codex config |
| `agy` | the one rule `mcp(wowc-advisor/*)` in `permissions.allow` of agy's settings, added by `wowc-advisor setup agy --write` | not allowed: shell, file writes and the web stay at agy's default (Ask), which a headless run refuses; no `--dangerously-skip-permissions` | the player's global MCP servers still load, but nothing allows their tools |
| `gemini` | `--policy` file (generated beside the MCP config) allows server `wowc-advisor` | the same file denies shell, file writes and web fetch/search; no `--yolo` | only `wowc-advisor` is enabled (`--allowed-mcp-server-names`) |

Antigravity CLI (`agy`) has no flag for an MCP config or a settings file ([headless docs](https://antigravity.google/docs/cli/headless/),
[CLI reference](https://antigravity.google/docs/cli/reference/)). The advisor server is therefore written for each
run to `<temp>/agy/.agents/mcp_config.json`, the workspace config agy reads ([MCP docs](https://antigravity.google/docs/mcp/)), and agy starts in
that folder; your `~/.gemini/config/mcp_config.json` is not touched. Pre-approving the tools is only possible in
agy's global settings (`~/.gemini/antigravity-cli/settings.json`, [permissions](https://antigravity.google/docs/permissions?tab=cli)), so run
`wowc-advisor setup agy --write` once; it adds `mcp(wowc-advisor/*)` and keeps your other settings. Answers are
read as text: agy's `stream-json` is not used yet, so agy answers appear whole and show no token line.
On Windows `agy.exe` is installed to `%LOCALAPPDATA%\agy\bin`.

Gemini CLI is legacy: since June 18, 2026 it serves only paid Gemini API and Enterprise keys, not Google AI
Pro/Ultra or free Code Assist accounts (Google's developer blog post "Transitioning Gemini CLI to Antigravity CLI"). Auto picks it only when no
other client is installed. It does not take an MCP server on the command line: the server must already be in
`~/.gemini/settings.json` (`wowc-advisor setup gemini`, see `docs/clients.md`), and the preset only
approves it. Codex's `exec` mode has had cases of cancelling MCP calls it should have approved; if
Codex answers that the tools were not available, update Codex or use `--agent claude`.

The prompt tells the agent to use the tools for anything about the character, never to tell the player
to change permissions or settings, and, if the tools are refused or missing, to answer "The advisor's
tools weren't available to the AI client (setup problem)."

### The config file

| System | Path |
| ------ | ---- |
| Windows | `%AppData%\wowc-advisor\ask.json` |
| macOS | `~/Library/Application Support/wowc-advisor/ask.json` |
| Linux | `$XDG_CONFIG_HOME/wowc-advisor/ask.json`, else `~/.config/wowc-advisor/ask.json` |

`wowc-advisor config path` prints it; `wowc-advisor config set agent <preset|command|auto>` writes the
`"agent"` field and keeps the file's other keys (an empty value removes it).

### Auto-detect (companion and the overlay)

`wowc-advisor companion` starts with no agent at all: the character list, game-data badge, NEXT strip and
digest work, and asking answers "No AI client set up yet…". When no `--agent` or config value is set (or the
value is `auto`) it looks for `claude`, `codex`, `agy` and `gemini` (legacy), in that order, and uses the first found. For
each name it checks every directory of `PATH` first, then these (`.exe`, `.cmd`, `.bat` and `.com` on Windows):

| System | Extra directories |
| ------ | ----------------- |
| Windows | `%USERPROFILE%\.local\bin`, `%USERPROFILE%\.claude\local`, `%APPDATA%\npm`, `%LOCALAPPDATA%\Programs\claude`, `%LOCALAPPDATA%\agy\bin`, `%USERPROFILE%\scoop\shims`, `%ProgramFiles%\nodejs` |
| macOS, Linux | `~/.local/bin`, `~/.claude/local`, `~/.npm-global/bin`, `/opt/homebrew/bin`, `/usr/local/bin` |

An explicit `--agent` or config value always wins. `companion --handshake` reports the result in its JSON
line: `"agent":"claude","agent_source":"auto"` (`agent_source` is `flag`, `config`, `auto` or `none`).

A custom command gets the prompt as its last argument, or wherever `{prompt}` / `{mcp_config}` appear.
The agent's environment also has `WOWC_QUESTION`, `WOWC_CHARACTER` and `WOWC_MCP_CONFIG`. The MCP
config starts `wowc-advisor serve --stdio` on your capture; claude, codex and agy are handed it, gemini needs the
same server in its own settings (see `docs/clients.md`). The capture file itself is never sent; the agent
gets the question and the character key, and reads facts through the tools.

## Asking naturally: refs, short answers, play style

Each digest entry offers 1 to 3 short questions (`asks`) and a subject `ref` (schema 1.5, see `digest.md`).
A question record in `WoWCompanionDB.asks`, and the overlay's `POST /api/ask` body, may carry that `ref`
(`{ "character": ..., "text": ..., "ref": "quest:123" }`). With a ref the advisor expands the subject itself
into the prompt: the entry's facts and its place in the plan, beside the character and play style. A typed
question that exactly matches (ignoring case and punctuation) one of the character's current `asks` gets that
entry's ref. Without a ref nothing changes.

Every answer, at every length, answers the question asked in its first sentence, adds at most 3 short bullets
about that question only, never restates the plan, and may end with one `Heads-up:` line. If the answer would
change what the plan says, it says so in one sentence. The length setting stays an upper bound.

**Play style as choices.** A talent question (or `ref` of `talents`) for a character with no saved play style is
not sent to the AI. The overlay API answers with an entry whose `status` is `"needs"` and
`needs = { needs: "play_style", character, question, ref, roles: [{value,label}], activities: [{value,label}], specs: [...] }`:
roles damage, tank, healer, balanced; activities leveling, dungeons, PvP, mixed; specs optional, the class's trees.
Saving the choice with `POST /api/profile` asks the original question once more automatically and sends a
`reask` event (`{ id, character, question }`) on `/api/events`; stream that `id` as usual.

## Know-how questions and Check Icy Veins

A question is **personal** ("what should I do", "my gear", "can I make X": answered strictly from the
player's data) or **game know-how** (strategy, tactics, boss mechanics, rotations, how a profession works,
lore, "thinking ahead"). A know-how word wins, then a personal phrase; when unsure it is know-how.

For know-how the model uses its own WoW knowledge, framed by a grounding pack: for a named dungeon, its
level range, entrance, bosses and creatures, quests with givers and the drops for the saved class and
spec (no level gate); for a class, its spells at the character's level; for a profession, the curated
notes. Tactics attach only to bosses and creatures in the pack, and where our facts differ from the
model's memory, ours win. The server, not the model, adds a footer line from what grounded the answer
("Tactics: general WoW knowledge · Levels, bosses, quests and drops: your game"); each part shows only
when present.

**Check Icy Veins** (overlay button, opt-in) asks the same question again and lets the player's own AI
client read the matching Icy Veins WoW Forever guide at runtime; nothing is stored or shipped. API
clients get a fetch tool limited to `https://www.icy-veins.com/wow-forever/` pages; Claude Code gets its
own WebFetch, pre-approved for that site; other CLI clients can't browse, so the button is hidden. The
footer then ends with "Icy Veins", linked to the page that was read.

## Limits

stdout is the answer, cut to 1,500 characters (at most 64 KB is read). Each run has a timeout
(`--timeout`, default 2m) and a pass answers at most `--max` questions (default 5); the rest wait for
the next pass. A failure or timeout becomes a short "Could not answer: ..." answer, and the file stays
valid. If the output names one of the advisor's tools, it is listed on the answer's sources line.
Answers already in the file are kept while their question is still queued. The file is checked against
the slot rule (one data-only assignment) before an atomic replace.

## What's new at my next level

"What's new at my next level?", "What do I get at level 12?" and "what will I learn in the next two levels" are answered from the dataset's `class_spells` for the saved character's class: the new spells and ranks in the window (spells the capture lists as known, ranks a higher rank in the window replaces, talents and abilities granted without a trainer are left out; a higher rank is a new spell, not "a new rank", when the character has no lower rank of it), the trainer cost from `trainer_spells` where a corroborated offer carries one (unknown costs are said to be unknown and never added to the total), the nearest class trainer for the character's faction from the player's QuestieDB ("Warrior Trainer" subnames), a money check against the character's gold with a pointer to the Bags review, and the class quests (Questie quests restricted to the class) available now or within five levels, with giver, zone and coordinates. The "what it does" lines follow the know-how rules.

The level can be named in everyday words: "at level 12", "at 12", "by 40", "when I hit 30"; a number followed by an amount ("at 5 gold", "by 10 minutes") is no level. "What do I learn next" and "when I ding" mean the next level. A money question about training ("do I have enough gold to train", "can I afford my next spells") gets the money check for the spells due now (the opened class trainer's own list when the player has opened it); one that names a level ("how much will training cost me at level 20") gets that level's list.

"What next" is any short question for the player's own plan: "what's next", "what now", "what do I do now", "what's my next step", "where should I go next", "what should I work on next", "next steps". These are the player's own questions (never answered from game knowledge) and may end with one `Heads-up:` line; a question that goes on ("what should I do with this sword") is not one.

A question what a character should have by a level ("what unlocks at level 40", "level 40 milestones"), how much gold to have or save ("how much gold should I have at 40", "what should I spend my gold on"), or whether they are geared for a level or a dungeon ("am I geared for level 40") is told that the game data holds no such list or target, and no figure is given from memory; a question about profession ranks with no profession named ("when can I train expert", "what skill cap do I have at level 20") gets the rank table (skill range and the character level each rank needs).

The NEXT bar shows "New spells to train: <first two>" with the trainer when the player opened their class trainer (without a trainer visit on record it says "Check the trainer's list" instead) and the capture lists known spells and some trainable spell at or below the character's level is still untrained. Class quests open now lead What next?, wherever their giver stands.

A reputation question ("reputation", "reputations", "rep", "standing", or a standing's name) is answered from the capture's own standings: the standing, the points in it and the points to the next. A faction is found by its full name, or by the short name players write (the first word of the name without a leading "The", or its last long word: "Timbermaw", "Gnomeregan", "Nozdormu", "Defilers"); a question that names none lists up to eight captured standings. The data holds no faction table, no standing a vendor or item asks for and no grind route, and the answer says so.

## Session plans and quest triage

"I've got 30 minutes", "quick session" and "an hour to play" build a plan from the quest route (`internal/session`). The time model is fixed and documented in the package: travel is the straight-line distance in map percent (1% is 12 yards), times 1.3 for paths, at 7 yards a second (14 from level 40); a pick-up or turn-in is 20 seconds; each creature still to kill is 25 seconds and each item or object 20, from the count in the objective ("Kill Wolf (2/8)" is six kills); a trainer visit is 60 seconds. Ready turn-ins go first, then quests by XP for the time they add, never one that needs a group; sessions of 45 minutes or less end at the nearest flight point or inn, and that walk is counted. The answer says "about N minutes" (rounded to five), the XP and its share of the level.

"Is <quest> worth it?", "What should I abandon?" and "My log is full" rate each quest in the log by XP a minute with the same model and flag it: grey (the character is six or more levels above it, or it gives under 1% of a level), far (no stop on the character's map, or the first is over 60% of a map away), group (an elite objective or a dungeon creature) and chain starter (another quest requires it). A quest that rewards a gear upgrade gets a bonus worth a tenth of the level (at least 300 XP) and is never offered to drop; neither is a ready turn-in or a chain starter. Quest XP comes from the player's quest XP table (`xpDB-classic.lua`) when it is found in the game folder, else it is estimated from the quest's level (40 a level less 50) and the answer says so; the game's cut for a quest below the character's level is applied either way.
