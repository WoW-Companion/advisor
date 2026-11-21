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
