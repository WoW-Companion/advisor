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

## Where to learn a recipe

`where_to_learn` answers "who trains that?". For a trainer recipe it lists up to three trainers of the profession (those that teach the recipe when the dataset says so), for the character's faction or neutral, nearest first: the character's zone, then their continent, then the rest. Coordinates come from the dataset's trainers row, else from the player's QuestieDB spawns (marked "from your Questie database"). A recipe taught by an item lists the vendor, drop or quest that gives the item.

The skill gate is the trainer's required skill when the dataset has it. Where the only value is the client minimum of 1 for a trainer recipe, no number is stated: the trainer shows whether you can learn it yet. Called with no recipe, the tool counts those recipes per profession.
