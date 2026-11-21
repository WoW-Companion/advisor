# Changelog

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
