#!/bin/sh
# Rebuilds testdata/world_snapshot from a built dataset directory: the rows the offline zone-connection and vendor-price
# benchmark (make offline-benchmark, internal/paths/offline_world_benchmark_test.go) compares that the other snapshots do
# not hold: every ui_maps row (a zone's parent continent), and the vendor offers of the items the benchmark prices (offers
# at the buy price, at a reputation discount, above the buy price, and with no price) with the items and the creatures that
# sell them. Whole rows of the dataset; read-only on it. Needs duckdb.
# Usage: testdata/world_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table want as select unnest([6256,2604,2324,2321,2692,2325,2678,2605,2880,2901,159,1205,1708,2320,1645,276200,276201,249479,252027,285291]) as item_id;
copy (select * from '$src/ui_maps.parquet') to '$out/ui_maps.parquet' (format parquet, compression zstd);
copy (select * from '$src/vendor_sources.parquet' where item_id in (select item_id from want)) to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select item_id from want)) to '$out/items.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select npc_id from '$src/vendor_sources.parquet' where item_id in (select item_id from want))) to '$out/npcs.parquet' (format parquet, compression zstd);
"
