# Profession plans

`profession_plan(character, profession?)` plans leveling for each of the character's professions (or
the one named) from the dataset's `recipes` and `recipe_reagents` tables, the captured skill level,
bags and bank.

**Which recipes.** A recipe is a candidate when its learn skill is at most the character's skill (a
recipe with no learn skill is kept and noted). With the dataset's thresholds, a recipe is **orange**
below `yellow_skill` and **yellow** from there up to `grey_skill`; at or above grey it is dropped.
The client stores no green threshold, so none is shown. Orange come first, then yellow; within a
difficulty the highest learn skill first, then the lowest spell id. Recipes with no thresholds
follow, ordered by learn skill, marked `unknown`: whether they give skill-ups is not known. At most
5 are listed.

**Materials.** Each reagent shows the count needed, the count in bags and bank, and what is missing.
A recipe with no reagent rows says "reagents unknown". With no bank capture, counts are bags only and
the plan says so.

**Keep list.** `professions.KeepList` returns the bag and bank items that are reagents of recipes
learnable within 50 skill points of the character's skill (learn skill at most skill + 50) and not
yet grey. It feeds the digest's `keep_materials`; with no recipes in the dataset for the character's
professions, the digest falls back to its name guess.

## Skill-up paths

`profession_plan` also computes a path (`professions.ComputePath`) from the character's skill to
`target_skill`, by default the highest tier cap the character's level allows (tiers and level gates come
from the curated `profession_notes`). It leads with the next step, then lists consecutive bands: per
band the cheapest recipe that is orange or yellow there, with an alternative. At most 8 bands show
per part; `part` asks for the next ones.

**Cost.** Materials held in bags or bank are free (and used up as the path goes). A missing material costs
its vendor price, or a little effort when gathered with a gathering profession the character has, fished
with Fishing, or disenchanted from the character's own gear; otherwise the Auction House is assumed cheap.
A recipe taught by a recipe item adds the cost of the item (vendor price, a drop, a quest); a trainer
recipe adds the trainer's cost.

**Prep.** Feeders and special cases from the notes, "fish in <zone> for <item>" from `fishing_catches`,
gathering nodes at a level-appropriate skill, which of the character's items disenchant into a missing
material, and for Enchanting the rod to make first. The trainer is the one in the character's zone, else
the one teaching the most recipes.

**Pairings.** `pairings=true` ranks the curated pairings by what the character already has, their class and
play style. Any pair is fine, and the answer says so.

`professions.NextSkillUp` gives the one-line "next skill-up: make N × X" for the digest's profession entries.

## Benchmark command

`wowc-advisor profession-plan --profession <name> [--from N] [--to N] [--level L] [--faction horde|alliance]
[--gather <profession>] [--known <spell ids>] [--dataset <dir or URL>] [--json]` runs the same planner for a synthetic fresh
character (level 60 by default, skill `--from` 1, empty bags and bank, no gathering profession unless
`--gather` is given). It needs no game install and no capture; `--dataset` is a directory or the base URL of a
published dataset (default `$WOWC_DATASET`).

The output lists every band (skill range, recipe, crafts, orange and yellow crafts, expected skill-ups, the
materials), then totals: each material with its count and where it comes from (a vendor with the copper cost,
gathering, fishing, disenchanting with the item class, quality and level range, a drop of 5% or more, or
"rare"), recipe items to buy or find, the tier gates with the trainer's cost for the recipes the path learns,
and an efficiency score. Model: an orange craft is 1 skill point, a yellow one 0.5; ties between recipes go
to fewer crafts, then the lower spell id, so the same dataset always gives the same output. Fishing has no
recipes: its bands are catches (one per skill point) with the catch list by zone.

Plans follow the dataset's `learn_skill` and `learn_skill_source` (trainer, recipe_item, client or estimate): no
recipe is planned before its learn skill, and an estimate is shown as "around skill N". A band never crosses a
rank cap (75, 150, 225): it stops there and the next band names the rank and the character level it needs.
By default only trainer and vendor recipes are planned; a recipe from a drop, quest or reputation is used only
when the character knows it (`--known` takes spell ids), and up to two such recipes are listed per band as "if
you have the pattern". A material made by a recipe the character can use (smelting for bars, the profession's
own grinding stones, bolts and cured leather) is expanded to its own reagents, with the skill-ups the crafts
give; ore, herbs, leather and cloth say how to gather them or that the Auction House sells them, and only a
material with no source at all is "rare". The "Check" section lists each band's recipe with its learn skill
and where it comes from.

## Where to learn a recipe

`where_to_learn` answers "who trains that?". For a trainer recipe it lists up to three trainers of the profession (those that teach the recipe when the dataset says so), for the character's faction or neutral, nearest first: the character's zone, then their continent, then the rest. Coordinates come from the dataset's trainers row, else from the player's QuestieDB spawns (marked "from your Questie database"). A recipe taught by an item lists the vendor, drop or quest that gives the item.

The skill gate is the trainer's required skill when the dataset has it. Where the only value is the client minimum of 1 for a trainer recipe, no number is stated: the trainer shows whether you can learn it yet. Called with no recipe, the tool counts those recipes per profession.

## Where to gather and fish

`where_to_gather` answers "where can I mine copper?" (an `item`, optionally with `kind`: mine, herb or fish) and "where should I fish at my level?" (a `kind` alone). It reads the herb nodes, ore veins and fishing schools from the player's own QuestieDB, so it needs Questie installed; the dataset's gathering tables are empty.

- **Item answer.** Up to three zones with the most nodes or schools that yield the item (ids from either source agree, and one item may come from several object ids), the character's own zone first, with one or two sample coordinates, the nearest to the character first within their own zone.
- **Level answer.** The character's skill comes from their profession. A node suits them when it needs no more than that skill; the best are those within 50 points under it, else the highest they can take. The skill a node needs is the curated `profession_notes` figure for its name (sections `nodes` or `gathering`, `skill_min`), else the dataset's `skill_required`, else the lowest required level of what it yields.
- **Planner.** `profession_plan` uses the same zones in the "Mine/Pick/Fish X: Zone: Node (x, y)" lines for the materials a path still needs.
- Every place read from Questie is marked "from your Questie database". Without Questie, or when its object rows are missing or unreadable, nothing is guessed: the answer says no place is known and the planner keeps its dataset lines.

## How a skill-up plan chooses (cost model)

- **Material cost.** A vendor-sold material costs its price; a gathered one 20 plus twice its node's skill; dust, essence and shard from disenchanting 100 ("disenchant level-N greens or buy"); anything else is weighted by tier (copper < tin < iron < mithril < thorium; linen < wool < silk < mageweave < runecloth; light < medium < heavy < thick < rugged leather). Gems and elemental, rare-drop or cooldown materials weigh 3000. A crafted material costs what its reagents cost; the expansion never follows a recipe inside itself.
- **Choice.** Each band takes the recipe with the lowest cost per expected skill point, counting the skill-ups the intermediate crafts give. A band over 25 raw materials per skill point is rejected when another recipe fits.
- **Recipe sources, in order:** trainer; vendor-sold recipe items; quest-reward recipes whose quest level and faction fit the character, named with the quest; drops and other patterns only as "if you have the pattern".
- **Left out by default:** specialisation recipes (Dragonscale, Elemental and Tribal Leatherworking, Armorsmith, Weaponsmith, Goblin and Gnomish Engineering) unless the character knows them, and recipes that make or use a cooldown material (Mooncloth, Salt Shaker, Arcanite, transmutes).
- **Gaps.** When no recipe covers a stretch, the band says "No trainer or vendor recipe covers 285–300", names the patterns or quests that would, and the plan goes on to the target.
- **Guide check.** The benchmark ends its band list with one "Guide check:" line: each band's recipe and how it is learnt (trainer, vendor pattern, quest recipe, known, or GAP).
