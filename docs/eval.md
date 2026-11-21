# Evaluation question set

`eval/` is a fixed set of advice questions with expected properties. It pins what good advice looks
like, so a change to the ranking, the gear rules or the digest that makes the advice worse fails
`make verify` (`go test ./eval/...`) and names the question that broke.

**What it holds.** Six synthetic characters, a Priest and a Rogue at levels 12, 24 and 40
(ForeverScribe format), and a small synthetic Parquet dataset that covers them (`quests`, `items`,
`loot_sources`, `npcs`). Both are built in code from the specs in `eval/gen_test.go` when the tests
start, in a temporary directory; nothing generated is committed. Names, ids and items are
invented; nothing is taken from a real capture or the real dataset. `eval/questions.json` holds 183 questions in all: 12 per character for the first four tools, and the rest for the newer
tools (below).

**What a question is.** A question maps to one tool and one property:

```json
{"ask": "Which quests are worth doing now? Is any of them grey?", "tool": "whats_next", "check": "no_grey_quest"}
```

`tool` is `whats_next`, `character_brief`, `upgrade_finder`, `update_digest`, `profession_plan`,
`talent_plan`, `dungeon_plan`, `inventory_review` or `doctor`. The test runs the function that tool
calls (`advice.WhatsNext`, `brief.Character`, `gear.Find`, `digest.Build`, `professions.Plan`,
`talents.Plan`, `dungeons.Plan`, `inventory.Review`, `doctor.Run`; no mocks) on the character and the dataset, then runs the property named by `check`
(`eval/checks_test.go`). A property compares the answer with what the capture and the dataset say,
worked out independently of the tool's own output. It checks properties, never exact text. A failure
reads:

```
rogue24 question 7 "Which armor can I upgrade? Is it all my armor type?" [upgrade_finder] broke
property "no_wrong_armor": head: "Eval Cloth Hood 24": cloth armor offered to a ROGUE for head
```

## Questions per tool

| Tool | Questions | On a Rogue |
| ---- | --------- | ---------- |
| `whats_next` | 32 | 15 |
| `character_brief` | 9 | 4 |
| `upgrade_finder` | 20 | 12 |
| `update_digest` | 11 | 5 |
| `profession_plan` | 19 | 11 |
| `talent_plan` | 26 | 16 |
| `dungeon_plan` | 24 | 15 |
| `inventory_review` | 25 | 14 |
| `doctor` | 17 | 10 |

Every tool has at least 4 questions, and at least half of the newer tools' questions are on the Rogues.
The newer properties are in `eval/checks_tools_test.go`; their fixtures (recipes, talent builds,
dungeons, vendor prices, the grey and spare-armor items each character carries in its bags, and the
doctor's temporary game folder and dataset cache) are in `eval/gen_tools_test.go`. The doctor
questions build a healthy setup in a temporary directory, break one thing and run `doctor.Run`.

## Add a question

1. **Use an existing property** on another character: add a line to that character's list in
   `eval/questions.json` with the `tool` and `check` of the property. A property appears once per
   character, and a character holds 8 to 40 questions.
2. **Or write a property.** Add a function `func(e *env) error` in `eval/checks_test.go` that asks
   the tool function for its answer and returns an error saying what is wrong (name the thing and
   the values), then register it in `checks` with its tool. A property no question uses fails
   `TestSetShape`.
3. **Make the fixture able to fail it.** A property that passes on every character proves
   nothing. If no character has the situation (a failed quest, a class-locked item, an unknown
   quest level), add it to the spec in `eval/gen_test.go` (`charSpecs`, `bandItems`, the quest
   mix). The fixtures are rebuilt on every run. Keep them synthetic: no real names, no real capture.
4. **Try it break.** Change the tool to give the wrong answer (for example `advice.MaxBelow = 50`)
   and see the question fail by name; then revert.

`go test ./eval/...` needs no network and runs in about a second.

## Real-data integration run

`make integration` is the other half of the evaluation: it runs every tool against a **real** built
dataset instead of the synthetic one, to catch what a fixture cannot, such as a wrong column or
source-kind name (which gives silent nulls, not an error) or a slow path at real size. It needs the
network and is **not** part of `make verify`.

```
make integration                        # latest dataset release, else build from ../data
make integration DATASET=/path/to/dir   # a dataset directory you already built
make integration DATA_REPO=~/src/wowc/data BUILD=1.60.1.71037
```

The dataset comes from `wowc-advisor dataset update` into a temporary cache; when the data repo has
no release yet, the target runs the data repo's `fetch` and `build` into a temporary directory
(nothing is written to the data repo) and checks the manifest. Then `integration/` builds six
synthetic captures (a Priest and a Rogue at 12, 24 and 40; the level 24 Priest also carries a
tree with talent node ids) whose items, quests, recipes and talent nodes are real ids read from the
dataset, with invented names, and calls each tool over an in-memory MCP session. The knowledge-only
lookups run against the dataset served over HTTP. Nothing real is committed or kept.

It prints one row per tool and character (non-empty, empty or ERROR, the number of results and the
latency) and, per tool, every field that was null, zero or empty in every answer. Read the second
list: a field that should have a value and never does is a mismatch, in the advisor or in the data.
The run fails on a tool error or an answer slower than 1 s. `WOWC_INTEGRATION_TEXT=Priestmid` (a
comma-separated list of character names, or `knowledge-only`) also prints those answers.
