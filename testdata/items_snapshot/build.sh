#!/bin/sh
# Rebuilds testdata/items_snapshot from a built dataset directory: the rows the offline item-level, set-piece and
# sell-price benchmark (make offline-benchmark, internal/paths/offline_items_benchmark_test.go) compares that the other
# snapshots do not hold: the item_sets rows of a spread of sets (class-bound PvP sets whose pieces the items table holds,
# sets of no class, three raid sets whose pieces the items table lacks, and sets whose name two set ids share) with the
# items rows of the members the table holds and their names index rows, and the mail and plate leg and foot pieces the item-level cases compare.
# Whole rows of the dataset; read-only on it. Needs duckdb. Usage: testdata/items_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table want_sets as
  select distinct set_id from '$src/item_sets.parquet'
  where set_id in (141, 142, 221, 281, 301, 321, 341, 342, 343, 345, 347, 361, 381, 401, 183, 209, 206)
     or name in ('The Elements', 'Lieutenant Commander''s Pursuit', 'Deathbone Guardian');
create temp table want_items as
  select item_id as id from '$src/item_sets.parquet' where set_id in (select set_id from want_sets)
  union select id from '$src/items.parquet' where id in (848, 849, 286, 287, 7921, 7933, 7926, 7936, 3480, 1445, 4251);
copy (select * from '$src/item_sets.parquet' where set_id in (select set_id from want_sets)) to '$out/item_sets.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select id from want_items)) to '$out/items.parquet' (format parquet, compression zstd);
copy (select * from '$src/names.parquet' where (\"table\" = 'item_sets' and id in (select set_id from want_sets)) or (\"table\" = 'items' and id in (select id from want_items)) order by norm) to '$out/names.parquet' (format parquet, compression zstd);
"
