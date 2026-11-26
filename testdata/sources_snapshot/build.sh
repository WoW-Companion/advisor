#!/bin/sh
# Rebuilds testdata/sources_snapshot from a built dataset directory: the rows the offline recipe-source, rare-spawn and
# elite-location benchmark (make offline-benchmark, internal/paths/offline_sources_benchmark_test.go) compares that the
# other snapshots do not hold: every recipe whose recipe item drops from a creature or object (the recipe, its item and
# the loot rows) and the creatures that drop them. Whole rows of the dataset; read-only on it. Needs duckdb.
# Usage: testdata/sources_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table ls as select l.* from '$src/loot_sources.parquet' l
  where l.source_kind in ('creature','object') and l.item_id in (select item_id from '$src/recipe_items.parquet');
create temp table ri as select * from '$src/recipe_items.parquet' where item_id in (select item_id from ls);
create temp table r as select * from '$src/recipes.parquet' where spell_id in (select teaches_spell_id from ri);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy ri to '$out/recipe_items.parquet' (format parquet, compression zstd);
copy r to '$out/recipes.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select item_id from ri)) to '$out/items.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select source_id from ls where source_kind = 'creature')) to '$out/npcs.parquet' (format parquet, compression zstd);
"
