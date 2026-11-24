# Changelog

## 0.7.34

- Ask how to level any profession: a step-by-step path from your skill, with what to gather, fish or buy first. Each step says what to make and how many, which materials you already hold, where to get the rest, how to learn the recipe and which trainer to visit; asking "which professions should I pick?" suggests pairings for your class and play style.

## 0.7.33

- With Questie installed, dungeon advice knows each dungeon's entrance, what lives inside, what drops for you and which quests to bring. It reads your own Questie files on your PC and says so; without Questie nothing changes.

## 0.7.32

- Dungeon advice is clearer: when you're ready, where they are, which quests to grab first, and what drops for you. Too low for any dungeon? It names the first one, about which levels it opens at, and where its entrance is. In range, it lists up to three dungeons with their entrance zones, the quests to pick up first (and who gives them), and the boss drops that are upgrades for your class and spec. Dungeons from the other faction are left out.

## 0.7.31

- The quick buttons ask broader questions: Gear now reviews all the upgrades within reach (a short ranked list of at most five, each with where to get it), Talents asks which point to take next and why, and What next? asks what to do next and why. Specific questions stay on the cards.

## 0.7.30

- Follow-up questions stay cheap: the companion remembers what you're talking about (the quest, NPC or item, and your last question) instead of replaying whole earlier answers, and shows it as "Following up on: …" above the ask box. Click the × or Clear to start fresh; it also lapses after 30 minutes or when you switch character. Finished answers now scroll fully into view, including the usage line; if you had scrolled up to read, a "↓ New answer" button appears instead of moving you.

## 0.7.29

- Crafting advice now checks your profession skill and, when you're close, tells you what to craft to get there. It says whether you can make an item now, how many points short you are (with up to three recipes to level on, how many crafts each needs and the materials you're missing), or, if you lack the profession or are far short, points to the Auction House (when the item can be traded) or a crafter. It also says where the recipe comes from: a vendor and price, a drop, a quest or a trainer. Quest pick-ups in your plan now name the giver players have seen when the game data lacks one.

## 0.7.28

- Advice now names vendors who sell what you need, and knows more WoW Forever quest givers, from Everloot's player-reported data. Upgrade and crafting answers say "Buy from <vendor> in <zone> for <price>" (and "limited stock" when it is), and a cheap vendor offer is listed beside the Auction House ahead of farming. Quest answers fill in the level, giver, faction and category players have seen, marked "seen by players on Everloot (CC BY-SA)".

## 0.7.27

- Use your own AI key (Claude, OpenAI, OpenRouter) or a free model on your PC (Ollama) — Settings → AI. Choose how the companion answers: an AI app on this PC, an AI account with a key, or a model on this PC. Paste a key once (it is saved privately and never shown again), pick Fast and cheap, Balanced or Best, and press Test to see that it works. Ollama and LM Studio are found automatically. Under Advanced you can pick a backup to use if the first one fails.

## 0.7.26

- You can now ask through an AI provider with your own API key, not only through a command-line AI client. Anthropic (Claude), OpenAI and OpenRouter work with a key you save; Ollama and LM Studio run on your own PC and need no key, and any other OpenAI-compatible server works with its address. The AI uses the same advisor tools and the same answer settings as before, and the usage line shows tokens including cached ones. If a model can't use tools, the app says so instead of guessing.
- Keys are stored in Windows Credential Manager (a private file in your data folder on macOS and Linux), never in a settings file or a log.
- You can list several AI clients in order; if the first can't start, the next one answers. Setups that only use a command-line client work exactly as before.
- New page: [AI providers and API keys](docs/providers.md).

## 0.7.25

- The AI can now follow WoW Forever's own quest lines using your Questie database. Ask "What does The Skybreaker Bulwark lead to?" and it lists the quests that come before and after it, with who gives and takes in each one and what to do, instead of saying the quest has no data. When no follow-up is listed anywhere, it now says so plainly.

## 0.7.24

- Plan steps no longer double up verbs like "Collect Obtain". If the quest's own objective already starts with a verb (Obtain, Listen, Investigate and many more), the step uses it as written.

## 0.7.23

- You can type and edit in the ask box again: typing, Backspace, Delete, the arrow keys, Home, End, Ctrl+A and Ctrl+V all work. Enter sends and Shift+Enter adds a line.
- The Keyboard test now also tells you when a key was cancelled after it arrived ("cancelled later").

## 0.7.22

- Typing and editing keys work in the ask box again. The overlay no longer makes any native focus call when you click the box, so it takes keys exactly like the Keyboard test field.
- The Keyboard test now also shows the app's focus line after a key in the ask box, and logs `ask: focus` and `ask: blur`.

## 0.7.21

- Backspace, Delete, the arrow keys, Home and End now work in the overlay's ask box. The overlay no longer pulls keyboard focus away from the box when you click it, and if a key still gets lost, the box applies it itself.
- In Settings → Shortcuts, Backspace or Delete clears a shortcut box back to its default. Tab moves on, and Escape leaves the box unchanged.
- The Keyboard test now also watches the ask box: its lines start with `test:` or `ask:`, and say "no change" when an editing key did nothing.

## 0.7.20

- The overlay's ask box now takes the arrow keys, Backspace, Delete, Ctrl+V and the menu's Paste. The overlay now takes keyboard focus as soon as you click the box, turning click-through off puts the window back exactly as it was, and a shortcut you set for show/hide or click-through can no longer be an editing key or a bare Ctrl+V or Ctrl+C.
- Settings → Updates has a new Keyboard test field. Click it and press a key: it lists the last 8 key events the overlay gets and a line from the app on focus and window state, so if a key still does nothing you can see where it stops.

## 0.7.19

- A question copied in game now pastes into the overlay's ask box. The game puts plain text on the clipboard that Notepad understands but the overlay's own paste can miss, so the overlay now reads the clipboard directly whenever Ctrl+V or Shift+Insert pastes nothing. It pastes once, at the cursor, replacing any selected text.
- A small paste button sits inside the right end of the ask box (hover for "Paste from clipboard") and inserts the clipboard text at the cursor.

## 0.7.18

- The advisor now notices when the game data is republished for the same game build, for example when new talent builds are added. It checks when it starts and then once a day. If the data changed, it downloads the new copy, checks it and swaps it in. It leaves your current copy alone if you are offline or the download fails.
- When new data installs, the overlay shows a dim line, "Game data updated (talent builds and more)", and your in-game advice is rewritten from it. The "Game data" row in the setup checklist shows the date of the data you have.

## 0.7.17

- Talent advice now comes from the advisor's own curated builds, with no web lookup. Ask which talent to take next and the answer is "Take <talent> (<rank>): <why>", then at most two short bullets: the next picks in your level band, and why they fit your play style. It uses your chosen spec, or the spec that matches your role (damage, tank or healer) when none is set.
- The guide web address is gone from the play style questions, the settings and the help text, and the advisor never fetches a guide for talents. A guide address saved earlier is quietly ignored.
- For a class or spec with no curated build yet, the answer says so in one line and lists the talents you can take right now as options, not a recommendation.

## 0.7.16

- Each upgrade's first suggested question now follows the way we suggest getting it. A craft asks "How do I make Gemmed Copper Gauntlets?"; a quest asks how to get the item from that quest; an Auction House item or shopping list asks how to find it there; a dungeon drop asks how to get it from the boss; a creature drop asks where to farm it. The earlier follow-up questions come after it, three at most.
- The answer to that first question gives the steps rather than repeating the card: for a craft, where the trainer is if you don't know the recipe, the materials with what you have and still need and where to get the rest, and where to craft it. One answer line, then at most three steps.

## 0.7.15

- Suggested questions now follow what you clicked. Each one names the thing itself (the item, the quest, the NPC, the talent or the ability) and asks the natural next thing: whether a different suffix would do as well, whether a craft is worth making now or the materials should be saved, what a quest leads to, or why a talent suits your play style. They no longer ask what the card already shows, so "What should I search for on the Auction House?" is gone.
- A question pasted into the overlay on its own still makes sense, because it carries its subject in its own words.
- Answers to those questions skip what the card already said and cover only the new part.

## 0.7.14

- Cleaner upgrade cards. Each fact now shows once: the item's name, then its slot, when to wear it, how big the gain is, and the item it replaces (just its name). The same words no longer repeat in the title, the badges and the details, and a shopping entry no longer starts with "Cloak: Cloak:".
- Weapons and off-hands you can really use. You are only offered an off-hand weapon once you have learned Dual Wield (a level 10 warrior is not), a shield is only replaced by a better shield, and a two-hander is only suggested over a one-hand weapon and shield for a damage play style where it beats both together. The slot label always matches the item: One-hand, Main hand, Two-hand, Off hand, Shield or Held in off hand.
- Plainer crafting lines. "Learn it from a Blacksmithing trainer, then make it; have 2/4 Copper Bar", or "You can make this (Blacksmithing)" when you already know the recipe. The "recipe needs skill 1, you have 35" wording is gone and the line is no longer cut off.

## 0.7.13

- A suggestion now always gets a useful answer. Click a suggested question about a class quest and the companion starts from what your Next list already knows (who gives it, where, its level, whether it is a class quest and why it is listed) instead of searching by name and saying it found no match. When little is known it says so plainly and never makes up a reward.
- Spec is now a dropdown of your class's specs, such as Arms, Fury and Protection, plus "Not sure yet". A spec you typed before is kept when it matches one of them. Stat preference starts on Balanced.
- Updates now shows what's new: the last five versions in plain words, newest first, each one collapsible, with a link to the full list on GitHub. It works without a network connection.
- Questions are asked in the overlay only. The companion no longer watches for questions asked in game, the "You asked in game" card is gone, and Answers.lua is no longer written. In game, the question buttons copy the question so you can paste it into the overlay; anything already on disk is left alone.
- Tidier in-game advice. An Auction House upgrade now reads as one short line, such as "Cheap on the Auction House: search 'of Strength', level 7–10", with the full search steps one click away in its details. Nothing in the advice is cut off mid-sentence any more, and the old long-question field is gone.

## 0.7.12

- New stat check. Once you have chosen a play style, the companion reads your stats in plain words: what to raise and why it helps how you play ("Fury wants more Strength: more attack power, so every hit lands harder"), what is wasted ("2 items give Spirit, which a warrior barely uses: swap them first"), and what is already fine. At low levels it reminds you that quest gear matters more than fine-tuning. Ask about your stats, or "what should I look for?", and the new stat_check tool answers; with no play style it offers the choices first.
- Play style gains an optional Stat preference (Balanced, More damage, More survivability) in Settings.
- Upgrades and the Auction House list now follow the same stat priorities, so the advice agrees everywhere: a Fury warrior sees "of the Tiger" ahead of "of the Bear", a Protection warrior the reverse, and an upgrade can say "Adds the Strength you're short on". The in-game digest gains a short stats entry.
- The priorities are ballpark guidance, not a rule: players differ.

## 0.7.11

- Cheap world-drop gear is now an Auction House shopping list. When the best way to upgrade a slot is the Auction House, or no quest or craft helps, the upgrade reads like "Chest: any green mail chest, level 8-10, 'of the Bear' - cheap on the Auction House", with a few example base items and the stat suffixes that fit your class and play style ("of the Bear (Strength + Stamina)"). Ask "What should I search for on the Auction House?" and the companion gives the in-game search in plain steps: what to type, the level range, the armor type or slot filter.
- Any random world drop you can trade, and any Bind on Equip item, now counts as an Auction House way: "Common world drop. Usually cheap on the Auction House." It ranks after a quest in your log and a craft you can make now, and before quests you have not picked up, dungeons, crafts missing materials and named creatures.
- No Auction House prices are shown or used any more; the data behind them was too thin. Stat suffixes are listed only when the game data confirms them.

## 0.7.10

- Upgrades are now only gear you can actually get. Each one comes with its best way, in plain words: finish a quest and choose it, make it with your own profession (with the materials you have and still need), buy it on the Auction House, or kill a dungeon boss or a named creature that drops it often. Random world drops are left out.
- Each upgrade reads like a sentence: the slot ("Cloak", "Chest", "Wrists", "Two-hand weapon"), the item, "Wear now" or "From level 12", "Big upgrade", "Solid upgrade" or "Small upgrade", and what it replaces ("instead of your ..." or "fills an empty slot"). The "ilvl" and doubled slot text are gone, and the list's summary reads "N upgrades you can get".
- A quest that rewards an upgrade says so ("Rewards Traveler's Shawl, a big cloak upgrade"), and a material you need for a craftable upgrade shows "for <item>" in Keep materials.
- Ask "How do I get <item>?" or "Which upgrade first?" and the companion now looks the answer up with the new upgrade_paths tool instead of guessing; an item with no dependable way is called a random drop, not worth chasing, and the best gettable item for that slot is named instead.
- Auction House prices are shown only when the game data holds a real auction price, with the date it was seen; a vendor sell price is never shown as one.

## 0.7.9

- Settings is now a full-height page. Click the gear and every section shows at once with a single scroll bar for the whole page (and only when it is taller than the window), instead of a small box scrolling inside the window. Open a section such as Play style and the page grows; the version line sits at the bottom.
- The gear lights up in gold while settings are open, its tooltip reads "Close settings", and it shows a clear keyboard focus ring. Click the gear again, press Esc, or use the new Done button to close; your chat is exactly where you left it.

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
