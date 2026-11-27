#!/bin/sh
# Rebuilds testdata/drops_snapshot from a built dataset directory: the rows the offline rare-spawn, named-elite, world-drop and drop-source benchmark (make offline-benchmark,
# internal/paths/offline_rares_drops_benchmark_test.go) compares that the other snapshots do not hold: the loot_sources rows of a spread of items (cloth and a gem carried by many
# creatures, bind-on-pickup and pattern items that a few named creatures drop, items nobody reported a creature for), the npcs rows of the creatures that carry them (named
# creatures with no level, zone or classification, and critters that have a level) and the items rows. Whole rows of the dataset; read-only on it. Needs duckdb.
# Usage: testdata/drops_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table want_items as select id from '$src/items.parquet' where id in (4306, 2592, 1529, 3864, 5805, 3916, 1524, 4614, 4352, 7450, 4633, 276160, 1708, 5637);
create temp table ls as select * from '$src/loot_sources.parquet' where source_kind = 'creature' and item_id in (select id from want_items);
create temp table want_npcs as select source_id as id from ls
  union select id from '$src/npcs.parquet' where id in (2266, 7873, 4151, 4093, 7335, 4109, 1032, 7547, 7394, 14878, 272210);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select id from want_items)) to '$out/items.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select id from want_npcs)) to '$out/npcs.parquet' (format parquet, compression zstd);
copy (select * from '$src/names.parquet' where (\"table\" = 'npcs' and id in (select id from want_npcs)) or (\"table\" = 'items' and id in (select id from want_items)) order by norm) to '$out/names.parquet' (format parquet, compression zstd);
"
