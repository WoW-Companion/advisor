---
name: wow-companion
description: Advice about the player's own World of Warcraft: Forever character (level, gear, professions, quests, spec). Use whenever the player asks what to do, wear, learn or quest next for their character, or asks about their character at all.
---

<!--
Format source: Claude Code skills docs, https://code.claude.com/docs/en/skills
(SKILL.md with YAML frontmatter on the first line; claude.ai uploads allow only name,
description, license, compatibility, metadata, allowed-tools). Checked 2026-11-20.
-->

# WoW Companion

WoW: Forever launched after your training data. Your memory of the game is likely wrong or
missing for it, so trust the `wowc-advisor` tools over anything you remember.

## Before advising about the player's character

1. Call `list_characters` to see which characters were captured. If there is one, or the
   player named one, call `character_brief` (pass `character` when there are several).
2. When the player asks what to do next, call `whats_next` (same `character` argument; `limit`
   defaults to 8). Each step carries its reason and source; relay them, and say "unknown" where
   a step says a level or zone is unknown.
3. Base every statement about their character on what the tools returned.
4. Name your source in the answer: "ForeverScribe capture at <time>", as the brief states it.

## Missing or stale data

- If the brief says a field is "not captured", say so plainly. Do not guess it.
- If the brief carries a WARNING that the capture is old, say so and tell the player to
  `/reload` in game (or log out) so the addon writes a fresh capture, then ask again.
- If a tool returns an error (no file configured, file not found, character not found), relay
  it in plain words and say what to fix. Do not answer from memory instead.

## Never invent

- Never invent items, quests, spells, talents, vendors, drop rates or numbers.
- If you are unsure how something works in WoW: Forever, say you are unsure.
- Upgrade lists and profession training steps are not available yet; say so rather than guess.

## Answer shape

Keep answers short: the finding first, then the reason, then the source line.
