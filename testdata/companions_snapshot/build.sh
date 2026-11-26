#!/bin/sh
# Rebuilds testdata/companions_snapshot from a built dataset directory: the rows the offline companion-pet, fishing,
# cooking-recipe and rune benchmark (make offline-benchmark, internal/paths/offline_companions_benchmark_test.go)
# compares that the other snapshots do not hold: every companion pet item (items class 15, subclass 2) with the loot and
# vendor rows that name it, every Cooking and Fishing recipe with its recipe items, reagents and trainer rows, the
# fishing_catches rows, the Cooking and Fishing profession routes and tiers, the profession notes (ranks, rules, pairings), the class spells and talent trees of the Warrior, Paladin, Hunter and Mage (the rune and pet abilities), and the items and creatures those
# rows name. Whole rows of the dataset; read-only on it. Needs duckdb.
# Usage: testdata/companions_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table rec as select * from '$src/recipes.parquet' where skill_line_name in ('Cooking', 'Fishing');
create temp table ri as select * from '$src/recipe_items.parquet' where teaches_spell_id in (select spell_id from rec);
create temp table rr as select * from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from rec);
create temp table ts as select * from '$src/trainer_spells.parquet' where spell_id in (select spell_id from rec);
create temp table want as
  select id from '$src/items.parquet' where class_id = 15 and subclass_id = 2
  union select item_id from '$src/fishing_catches.parquet'
  union select creates_item_id from rec where creates_item_id is not null
  union select item_id from ri union select reagent_item_id from rr;
create temp table ls as select * from '$src/loot_sources.parquet' where item_id in (select id from want);
create temp table vs as select * from '$src/vendor_sources.parquet' where item_id in (select id from want);
create temp table wnpc as select source_id as id from ls where source_kind = 'creature' union select npc_id from vs union select npc_id from ts;
copy (select * from '$src/items.parquet' where id in (select id from want)) to '$out/items.parquet' (format parquet, compression zstd);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy vs to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select id from wnpc)) to '$out/npcs.parquet' (format parquet, compression zstd);
copy rec to '$out/recipes.parquet' (format parquet, compression zstd);
copy ri to '$out/recipe_items.parquet' (format parquet, compression zstd);
copy rr to '$out/recipe_reagents.parquet' (format parquet, compression zstd);
copy ts to '$out/trainer_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainers.parquet' where npc_id in (select npc_id from ts)) to '$out/trainers.parquet' (format parquet, compression zstd);
copy (select * from '$src/fishing_catches.parquet') to '$out/fishing_catches.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_routes.parquet' where profession in ('Cooking', 'Fishing')) to '$out/profession_routes.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_tiers.parquet' where profession in ('Cooking', 'Fishing')) to '$out/profession_tiers.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_notes.parquet') to '$out/profession_notes.parquet' (format parquet, compression zstd);
copy (select * from '$src/class_spells.parquet' where class_id in (1, 2, 3, 8)) to '$out/class_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_trees.parquet' where class_id in (1, 2, 3, 8)) to '$out/talent_trees.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_nodes.parquet' where tree_id in (select tree_id from '$src/talent_trees.parquet' where class_id in (1, 2, 3, 8))) to '$out/talent_nodes.parquet' (format parquet, compression zstd);
copy (select * from '$src/names.parquet' where (\"table\" = 'items' and id in (select id from want)) or (\"table\" = 'recipes' and id in (select spell_id from rec)) or (\"table\" = 'npcs' and id in (select id from wnpc)) order by norm) to '$out/names.parquet' (format parquet, compression zstd);
"
