#!/bin/sh
# Rebuilds testdata/tradeskills_snapshot from a built dataset directory: the rows the offline trainer-location,
# profession-tool, reagent-source and skill-up-route benchmark (make offline-benchmark,
# internal/paths/offline_tradeskills_benchmark_test.go) compares. Whole tables: trainers, trainer_spells,
# profession_tiers, profession_routes, profession_notes, recipes, recipe_items, recipe_reagents, gathering_nodes and
# gathering_yields. Filtered: the items every recipe reagent, route material, gathering yield and profession tool
# names (with their loot and vendor rows), the items recipes create, and the creatures those rows name. Whole rows of
# the dataset; read-only on it. Needs duckdb.
# Usage: testdata/tradeskills_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table want as
  select reagent_item_id as id from '$src/recipe_reagents.parquet'
  union select item_id from '$src/gathering_yields.parquet'
  union select unnest(materials).item_id from '$src/profession_routes.parquet'
  union select id from '$src/items.parquet' where class_id = 2 and subclass_id = 14
  union select id from '$src/items.parquet' where name like 'Runed % Rod' or name = 'Fishing Pole' or name like '% Fishing Pole';
create temp table wantall as select id from want union select creates_item_id from '$src/recipes.parquet' where creates_item_id is not null;
create temp table ls as select * from '$src/loot_sources.parquet' where item_id in (select id from want);
create temp table vs as select * from '$src/vendor_sources.parquet' where item_id in (select id from want);
create temp table wnpc as select source_id as id from ls where source_kind = 'creature' union select npc_id from vs union select npc_id from '$src/trainers.parquet';
copy (select * from '$src/items.parquet' where id in (select id from wantall)) to '$out/items.parquet' (format parquet, compression zstd);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy vs to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select id from wnpc)) to '$out/npcs.parquet' (format parquet, compression zstd);
copy (select * from '$src/recipes.parquet') to '$out/recipes.parquet' (format parquet, compression zstd);
copy (select * from '$src/recipe_items.parquet') to '$out/recipe_items.parquet' (format parquet, compression zstd);
copy (select * from '$src/recipe_reagents.parquet') to '$out/recipe_reagents.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainers.parquet') to '$out/trainers.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainer_spells.parquet') to '$out/trainer_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_tiers.parquet') to '$out/profession_tiers.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_routes.parquet') to '$out/profession_routes.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_notes.parquet') to '$out/profession_notes.parquet' (format parquet, compression zstd);
copy (select * from '$src/gathering_nodes.parquet') to '$out/gathering_nodes.parquet' (format parquet, compression zstd);
copy (select * from '$src/gathering_yields.parquet') to '$out/gathering_yields.parquet' (format parquet, compression zstd);
copy (select * from '$src/names.parquet' where (\"table\" = 'items' and id in (select id from wantall)) or (\"table\" = 'recipes') or (\"table\" = 'npcs' and id in (select id from wnpc)) order by norm) to '$out/names.parquet' (format parquet, compression zstd);
"
