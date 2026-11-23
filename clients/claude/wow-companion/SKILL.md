---
name: wow-companion
description: Advice about the player's own World of Warcraft: Forever character (level, gear, professions, quests, talents, dungeons, spec). Use whenever the player asks what to do, wear, learn, quest or run next for their character, or asks about their character at all.
---

<!--
Generated from clients/body.md; do not edit by hand (go test ./clients -update rewrites it).
Format source: Claude Code skills docs, https://code.claude.com/docs/en/skills
(SKILL.md with YAML frontmatter on the first line; claude.ai uploads allow only name,
description, license, compatibility, metadata, allowed-tools).
-->

# WoW Companion

WoW: Forever launched after your training data. Your memory of the game is likely wrong or
missing for it, so trust the `wowc-advisor` tools (the `wowc-advisor` MCP server) over anything
you remember.

## Tools

All character tools take an optional `character` name (omit it when only one was captured).

- `server_info`: the advisor version, dataset and game build, and the status of the character file.
  Call it first when anything looks wrong.
- `list_characters`: the captured characters with level, class, realm and capture time.
- `character_brief`: level, class, XP, money, spec, gear, professions, bank and bags, quests.
- `whats_next`: ranked next steps (quests to turn in, unspent talent points, open quests near the
  level, failed quests to abandon); `limit` defaults to 8.
- `quest_plan`: an efficient ordered quest route (pick-ups, objectives, turn-ins with map coordinates, then the next zone), from
  the player's own Questie; the order is computed, so relay it as is.
- `upgrade_finder`: gear upgrades per slot (optional `slot`, `limit`) with where each drops.
- `upgrade_paths`: how the character can dependably get an item (`item`) or the best gettable upgrades
  for a slot (`slot`): quest reward, craft with materials held and missing, Auction House, dungeon boss,
  named creature. Call it for every "how do I get ..." or "which upgrade first" question; never say a
  source is unknown without having called it. When it says an item is a random drop, say plainly it is
  not worth chasing and name the gettable item it gives for that slot.
- `profession_plan`: the next recipes that still give skill-ups per profession (optional
  `profession`), with reagents held and missing.
- `talent_plan`: where the next talent points go against the curated build (optional `spec`,
  `limit`).
- `get_character_profile` / `set_character_profile`: the character's saved play style (role, focus,
  spec, guide) and talent plan, kept on the player's PC. `talent_tree`: the exact captured talent
  node names a plan may use.
- `dungeon_plan`: which dungeons fit the character and which boss drops are upgrades (optional
  `limit`).
- `farm_spots`: where an item drops (item id or name, optional `limit`): creatures and objects by level fit
  and drop chance, with zones where known.
- `quest_chain`: where a quest sits in its quest line (quest id or name): the line, every step and the quest's position.
- `inventory_review`: what in the bags and bank to equip, keep for professions, hand in for quests,
  vendor or bank, with the reason for each.
- `update_digest`: writes the in-game digest into the WoWCompanion_Digest addon. Call it only when
  the player asks to refresh the in-game digest.

## Before advising about the player's character

1. Call `list_characters` to see which characters were captured. If there is one, or the
   player named one, call `character_brief` (pass `character` when there are several).
2. Pick the tool that matches the question: `whats_next` for "what should I do", `upgrade_paths`
   for any "how do I get ..." or "which upgrade first" question, `upgrade_finder` for gear, `profession_plan` for professions, `talent_plan` for talents, `dungeon_plan` for
   where to run, `inventory_review` for what to do with bags and bank. Each step carries its reason and source; relay them, and say "unknown" where a
   step says a level or zone is unknown.
3. Talents: call `get_character_profile` first. If `needs_profile` is true, ask the player its short
   multiple-choice question and stop; save the answer with `set_character_profile`. With a saved plan,
   relay its next picks as the tools give them and credit the guide with its link. Build or refresh the
   plan only when none is saved, the player asks to refresh it, or the spec changed: call `talent_tree`,
   read the player's chosen guide (default: the Icy Veins WoW: Forever guide, found from the class
   overview page; only that site, only if your client can fetch it narrowly, else show the link), then
   save a plan of node names copied exactly from `talent_tree` and target ranks. Never store, quote or
   copy guide text.
4. Base every statement about their character on what the tools returned.
5. Name your source in the answer: "ForeverScribe capture at <time>", as the brief states it.

## Missing or stale data

- If the brief says a field is "not captured", say so plainly. Do not guess it.
- If the brief carries a WARNING that the capture is old, say so and tell the player to
  `/reload` in game (or log out) so the addon writes a fresh capture, then ask again.
- If a tool returns an error (no file configured, file not found, character not found, no dataset
  configured), relay it in plain words and say what to fix. Do not answer from memory instead.

## Never invent

- Never invent items, quests, spells, talents, vendors, drop rates or numbers.
- If you are unsure how something works in WoW: Forever, say you are unsure.

## Answer shape

Keep answers short: the finding first, then the reason, then the source line.
