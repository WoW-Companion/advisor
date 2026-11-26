#!/bin/sh
# Rebuilds testdata/consumables_snapshot from a built dataset directory: the rows the offline enchant, gem,
# consumable and mount benchmark (make offline-benchmark, internal/professions/offline_consumables_benchmark_test.go)
# compares: every item of the consumable class and every mount item, the Enchanting, Alchemy and Cooking recipes with
# their reagents, recipe items and trainers, the items those rows name, and who sells or drops them. Whole rows
# of the dataset; read-only on it. Needs duckdb. Usage: testdata/consumables_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table r as select * from '$src/recipes.parquet' where skill_line_name in ('Enchanting','Alchemy','Cooking');
create temp table ri as select * from '$src/recipe_items.parquet' where teaches_spell_id in (select spell_id from r);
create temp table want as
  select id from '$src/items.parquet' where class_id in (0, 3) or (class_id = 15 and subclass_id = 5)
  union select reagent_item_id from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from r)
  union select creates_item_id from r where creates_item_id is not null
  union select item_id from ri;
create temp table ts as select * from '$src/trainer_spells.parquet' where spell_id in (select spell_id from r);
create temp table vs as select * from '$src/vendor_sources.parquet' where item_id in (select id from want);
create temp table ls as select * from '$src/loot_sources.parquet' where item_id in (select id from want);
copy r to '$out/recipes.parquet' (format parquet, compression zstd);
copy ri to '$out/recipe_items.parquet' (format parquet, compression zstd);
copy (select * from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from r)) to '$out/recipe_reagents.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select id from want)) to '$out/items.parquet' (format parquet, compression zstd);
copy ts to '$out/trainer_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainers.parquet' where npc_id in (select npc_id from ts)) to '$out/trainers.parquet' (format parquet, compression zstd);
copy vs to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select npc_id from ts) or id in (select npc_id from vs)
  or id in (select source_id from ls where source_kind = 'creature')) to '$out/npcs.parquet' (format parquet, compression zstd);
"
