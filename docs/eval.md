# Evaluation question set

`eval/` is a fixed set of advice questions with expected properties. It pins what good advice looks
like, so a change to the ranking, the gear rules or the digest that makes the advice worse fails
`make verify` (`go test ./eval/...`) and names the question that broke.

**What it holds.** Six synthetic characters, a Priest and a Rogue at levels 12, 24 and 40
(ForeverScribe format), and a small synthetic Parquet dataset that covers them (`quests`, `items`,
`loot_sources`, `npcs`). Both are built in code from the specs in `eval/gen_test.go` when the tests
start, in a temporary directory; nothing generated is committed. Names, ids and items are
invented; nothing is taken from a real capture or the real dataset. Each character has 12 questions
in `eval/questions.json`, 72 in all.

**What a question is.** A question maps to one tool and one property:

```json
{"ask": "Which quests are worth doing now? Is any of them grey?", "tool": "whats_next", "check": "no_grey_quest"}
```

`tool` is `whats_next`, `character_brief`, `upgrade_finder` or `update_digest`. The test runs the
function that tool calls (`advice.WhatsNext`, `brief.Character`, `gear.Find`, `digest.Build`; no
mocks) on the character and the dataset, then runs the property named by `check`
(`eval/checks_test.go`). A property compares the answer with what the capture and the dataset say,
worked out independently of the tool's own output. It checks properties, never exact text. A failure
reads:

```
rogue24 question 7 "Which armor can I upgrade? Is it all my armor type?" [upgrade_finder] broke
property "no_wrong_armor": head: "Eval Cloth Hood 24": cloth armor offered to a ROGUE for head
```

## Add a question

1. **Use an existing property** on another character: add a line to that character's list in
   `eval/questions.json` with the `tool` and `check` of the property. A property appears once per
   character, and a character holds 8 to 12 questions.
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

`go test ./eval/...` needs no network and runs in well under a second.
