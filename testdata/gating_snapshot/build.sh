#!/bin/sh
# Rebuilds testdata/gating_snapshot from a built dataset directory: the rows the offline attunement, dungeon-key,
# reputation-gated gear and level-requirement benchmark (make offline-benchmark,
# internal/paths/offline_gating_benchmark_test.go) compares: the dungeon key and attunement items, the Thorium
# Brotherhood Blacksmithing, Leatherworking and Tailoring recipes (their plans, patterns, reagents and the gear they make),
# a handful of worn items with required levels, and who sells or drops them. Whole rows of the dataset; read-only on
# it. Needs duckdb. Usage: testdata/gating_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table r as select * from '$src/recipes.parquet' where skill_line_name in ('Blacksmithing','Leatherworking','Tailoring')
  and name in ('Dark Iron Boots','Chromatic Gauntlets','Dark Iron Destroyer','Fiery Chain Girdle','Dark Iron Helm','Dark Iron Gauntlets','Black Dragonscale Boots','Blackfury','Nightfall','Lava Belt','Corehound Boots','Molten Helm','Flarecore Gloves','Ebon Hand','Blackguard','Black Amnesty','Fiery Plate Gauntlets');
create temp table ri as select * from '$src/recipe_items.parquet' where teaches_spell_id in (select spell_id from r);
create temp table want as
  select id from '$src/items.parquet' where id in (11000,18249,13704,12382,13873,18266,11140,12323,16309,18987,22057,13140,9240,12784,17193,15071,19083,16397,19538,12631)
  union select reagent_item_id from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from r)
  union select creates_item_id from r where creates_item_id is not null
  union select item_id from ri;
create temp table vs as select * from '$src/vendor_sources.parquet' where item_id in (select id from want);
create temp table ls as select * from '$src/loot_sources.parquet' where item_id in (select id from want);
copy r to '$out/recipes.parquet' (format parquet, compression zstd);
copy ri to '$out/recipe_items.parquet' (format parquet, compression zstd);
copy (select * from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from r)) to '$out/recipe_reagents.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select id from want)) to '$out/items.parquet' (format parquet, compression zstd);
copy vs to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select npc_id from vs) or id in (select source_id from ls where source_kind = 'creature')) to '$out/npcs.parquet' (format parquet, compression zstd);
"
