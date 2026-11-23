# Changelog

## 0.7.8

- The overlay picks up where you left off. It opens on your last chat for the character (the last 50 turns, kept on your PC only), with a dim line marking the gap, such as "Yesterday, 11:58 PM", and sends nothing by itself. Clear empties that character's chat.
- An in-game question that is more than 12 hours old is no longer answered on launch. It shows as one small card, "You asked in game last night: ...", with "Answer now" and "Dismiss". Newer in-game questions are answered as before.
- Calmer by default: the settings stay behind the gear button (and the version line lives there), the Next list shows one entry until opened, and at most one row of up to three suggestions appears when something is worth doing, such as choosing a play style, turning in a ready quest or updating an old plan. Hover a suggestion to see why; its x hides it for today.
- A slim quick bar above the ask box: What next?, Talents, Gear and Refresh. Refresh rebuilds your plan from your latest save, asks no AI, and answers in one sentence. Settings, Shortcuts can hide the bar.
- Each Next entry shows a short reason under its title. The plan list reads as plain steps with the coordinates dim at the end, and says "23 stops". The red "17h" pill is now a small dot that turns amber after 12 hours and red after 3 days; hover it for the last save time.
- Behind this: every entry carries a short "why", plan steps are plain sentences, the advisor ranks at most three suggested actions, a plan refresh endpoint rebuilds the plan from the newest save, and a ledger remembers answered in-game questions so none is answered twice.

## 0.7.7

- Ask without pasting. The overlay's Next section now lists your character's current entries (quests, talents, training, upgrades, notes, dungeons) one line each, in the same order as the in-game panel. Click an entry to see its questions as buttons, such as "Where should my talent point go?"; click one and just that question appears in the chat with a short answer. Nothing needs copying from the game.
- The ask box grows as you type or paste, up to six lines, then scrolls, and shrinks back after you send. Enter sends; Shift+Enter starts a new line.
- A talent question now asks how you like to play first: role and activity buttons (and an optional spec) appear right in the chat. Save stores them, the same as the Play style settings, and your question is answered automatically.
- Questions asked in the game show as your short question with a small "from game" mark. The overlay no longer tells you to `/reload`.
- Behind this: every in-game entry carries 1 to 3 natural questions and a hidden reference (digest schema 1.5), so a typed question that matches one is answered with that entry's facts; answers are shorter, and the advisor says honestly when a quest giver cannot be found.

## 0.7.6

- The prompts and notes on the in-game cards now read as plain sentences: correct plurals ("1 unspent talent point"), your saved play style and talent plan named in the question, a separate question for picking up, working on and turning in a quest, upgrade sources in words (a drop from a creature, a quest reward, crafted by a profession) and never an id. Card notes are short player-facing lines (who and where, what is left, what it unlocks, how the level fits) with no internal labels or cut-off text.

## 0.7.5

- The route plan now knows what turning a quest in unlocks. Follow-up quests (read from your own Questie, the dataset's quest lines, and unlocks you have seen in game) are routed after the turn-in that makes them available, marked locked, at most two links ahead and within a 25-step total. A turn-in's entry says "Unlocks: <quest> (from <NPC>, here)"; the overlay's Plan list shows locked steps dimmed with "after step N". `Plan.lua` schema 1.1 adds `locked`, `unlockedBy`, `depth`, `chain` and `unlocks`; older addons ignore them.
- The advisor reads the addon's bounded log of quests accepted, turned in and abandoned (`WoWCompanionDB.observed`), so a quest you pick up outside the plan is routed on the next refresh without a `/reload`. Unlocks it sees are kept in a small file on your PC only, and never sent anywhere.

## 0.7.4

- The in-game digest gains class-training entries: when your level unlocks trainer-taught abilities or ranks, `next_quests` gets a `training` entry listing them (new, or an upgrade of a rank), with the nearest class trainer only when your capture names one. Every next-quest, upgrade, dungeon, talent and training entry now carries 1 to 4 `details` lines and a ready, context-rich `prompt` (at most 500 characters) built from your character, play style and talent plan. Digest schema 1.4.
- A question you queue in game now appears in the overlay's Ask feed as soon as it is saved, marked as from the game, and its answer streams in there. An optional "Question from game" toast is off by default.

## 0.7.3

- The overlay's Next list now lays out each quest from the digest's structured fields, like the in-game panel: `[level] Title` coloured by difficulty with (Class) and (Elite) tags, objective rows with counts, "Ready to turn in" / "Turn in to NPC" or "Pick up from NPC", and "N unspent talent points". Nothing is cut off with "..."; long lines wrap. An older advisor's plain text still shows.

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
