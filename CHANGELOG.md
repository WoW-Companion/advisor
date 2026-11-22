# Changelog

## 0.7.2

- The in-game digest lines now carry structured, untruncated fields (kind, full quest title, level, class and elite flags, NPC, zone, every objective with its count, unspent talent points, and an upgrade's slot, item level and level needed), so the addon can lay Next out like the quest tracker. The existing `text` is unchanged for older addons. Digest schema 1.3.

## 0.7.1

- Fixed a false "Advice may be out of date" warning in the addon after a /reload: `Digest.lua` and `Plan.lua` are now rewritten with a fresh time whenever ForeverScribe has saved a newer capture, even when the advice is unchanged.

## 0.7.0

- Class quests and unspent talent points now lead the advice: "Next", the digest and answers put class quests to pick up (read from your own Questie) and "You have N unspent talent points" (including profession talent points) ahead of routine quests. The route plan always includes class quests.
- New talent advisor: ask "What talents should I get?" and it asks about your play style, then builds a talent plan for your spec. Talent plans read the guide you choose (Icy Veins by default) at the time you ask, and nothing from it is stored in WoW Companion.
- The advisor remembers each character's talent choices and play style.
- The in-game `Plan.lua` and `Digest.lua` are now written as soon as the overlay starts, and missing data files are stubbed, so the addon no longer shows an "error loading Plan.lua" warning at login.
- Fixed quest route plans: a quest is never routed to its turn-in before its objectives are done, and item and kill-credit objectives are now resolved so they appear in the route.

## 0.6.0

- New quest route plan: pick-ups, objectives and turn-ins in an efficient order with map coordinates, computed by a distance optimiser from your own Questie. It shows as a numbered Plan under Next in the overlay, is available to the AI as `quest_plan`, and is written to `Plan.lua` beside the in-game digest. The map pins for the plan need WoW Companion addon 0.3.0; update the addon from the setup checklist.

## 0.5.1

- Fixed the upgrade check failing with a tool error in a question's answer.
- Quest progress now prints once per objective, and a Next line shows one objective plus "+N more".
- The setup checklist now reads Questie's version from its per-game TOC file ("Found Questie v12.11.1 (QuestieDB v1.1.3)").
- Quests not in your log are marked "Pick up: ..." in answers and the Next list; a failed tool is now named in the answer.

## 0.5.0

- Answers now use your saved quest progress, such as "3/8" on an objective, so the advisor knows what you still have left to do.
- The advisor can point you to nearby quest givers you have already met, with their positions.
- Optional: if you have Questie and QuestieDB installed, the advisor reads them on your own PC to suggest quests you have not picked up (with the giver's position) and to say where a logged quest's objective or turn-in is. Nothing from Questie is copied or shared. The setup checklist has a Questie row.

## 0.4.0

- Antigravity (`agy`) is now supported as an AI client. Gemini CLI still works but is now legacy.
- The Getting Started guide now has setup steps for each AI client.
- The setup checklist has a "Game data" row, and clearer messages appear when there is no game data yet.
- Settings are easier to use: links are readable, checkboxes line up, AI clients show friendly names, related settings are grouped, and the settings page scrolls.
- Each question now sends far fewer tokens: Claude runs with only the advisor's tools (no built-in tools, user settings or skills), and the character's facts are put in the prompt so most questions need no tool call.
- The usage line shows fresh input, output and cached tokens separately, with a tooltip on cached tokens.
- The advisor now suggests quests you have not picked up yet.

## 0.3.0

- The overlay now writes the in-game digest for you, so there is no separate step to run.
- Overlay-first answering: questions are answered in the overlay first.
- A setup checklist shows what is left to connect, and a Getting Started guide walks you through it.
- Fixed the launch of Codex as your AI client.
- The overlay shows its version, checks for updates on start, and upgrades in place.
  This one install may ask you to uninstall 0.2.0 first; later upgrades will not.
