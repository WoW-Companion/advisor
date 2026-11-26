#!/bin/sh
# Rebuilds testdata/levelling_snapshot from a built dataset directory: the rows the offline class-ability, talent-point,
# level-pacing and XP benchmark (make offline-benchmark, internal/paths/offline_levelling_benchmark_test.go) compares: every
# class_spells row of the nine classes, every talent tree, node, build and priority row, every trainer_spells row (the
# training costs), the instances (their level ranges), and empty items and instance_bosses tables (the dungeon plan reads them). Whole rows of
# the dataset; read-only on it. Needs duckdb.
# Usage: testdata/levelling_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
copy (select * from '$src/class_spells.parquet') to '$out/class_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_trees.parquet') to '$out/talent_trees.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_nodes.parquet') to '$out/talent_nodes.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_builds.parquet') to '$out/talent_builds.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_priorities.parquet') to '$out/talent_priorities.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainer_spells.parquet') to '$out/trainer_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' limit 0) to '$out/items.parquet' (format parquet, compression zstd);
copy (select * from '$src/instance_bosses.parquet' limit 0) to '$out/instance_bosses.parquet' (format parquet, compression zstd);
copy (select * from '$src/instances.parquet') to '$out/instances.parquet' (format parquet, compression zstd);
"
