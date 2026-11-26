#!/bin/sh
# Rebuilds testdata/places_snapshot from a built dataset directory: the rows the offline places benchmark
# (make offline-benchmark, internal/professions/offline_places_benchmark_test.go) compares: dungeons with their level
# ranges and entrance zones, dungeon bosses and quests, the quests table,
# the quest lines and the quests players observed, the named creatures, who sells what, the profession
# trainers, the weapon masters and the profession rank trainers. Whole tables; the rest of the data the
# benchmark reads (items, loot) comes from testdata/advice_snapshot. Read-only on the dataset. Needs duckdb.
# Usage: testdata/places_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
for t in instances instance_bosses instance_quests quests quest_lines quests_observed npcs vendor_sources trainers weapon_masters weapon_skills profession_tiers; do
  duckdb -no-agent -c "copy (select * from '$src/$t.parquet') to '$out/$t.parquet' (format parquet, compression zstd);"
done
