#!/bin/sh
# Rebuilds testdata/advice_snapshot from a built dataset directory: the rows the offline advice benchmark
# (make offline-benchmark) plans and compares. Read-only on the dataset. Needs duckdb. The snapshot is the
# recipes of eight professions up to skill 75 with their reagents, trainers, vendors and patterns, the items the
# stat cases use (a sample of armor with its stats and armor value), every profession route and rank trainer,
# and the talent builds, nodes and priorities. Usage: scripts/advice-snapshot.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: advice-snapshot.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")/../testdata/advice_snapshot}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table r as select * from '$src/recipes.parquet' where skill_line_name in
  ('First Aid','Cooking','Alchemy','Tailoring','Leatherworking','Blacksmithing','Enchanting','Engineering') and learn_skill <= 75;
create temp table ri as select * from '$src/recipe_items.parquet' where teaches_spell_id in (select spell_id from r);
create temp table armor_pick as select id from '$src/items.parquet'
  where class_id = 4 and armor is not null and allowable_class_mask = -1 and required_level between 10 and 45 and quality in (1, 2)
    and ((slot in (5, 20) and subclass_id between 1 and 4) or (slot = 14 and subclass_id = 6))
  qualify row_number() over (partition by subclass_id, slot, stats_status, quality order by id) % 12 = 1;
create temp table want as
  select reagent_item_id as id from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from r)
  union select creates_item_id from r where creates_item_id is not null
  union select item_id from ri
  union select m.item_id from (select unnest(materials) as m from '$src/profession_routes.parquet')
  union select id from armor_pick
  union select unnest([7334,4324,5770,7511,7512,9515,9516,7065,277514,7051,3750,250487,250533,7914,7374,7375,2570,9761,810]);
copy r to '$out/recipes.parquet' (format parquet, compression zstd);
copy (select * from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from r)) to '$out/recipe_reagents.parquet' (format parquet, compression zstd);
copy ri to '$out/recipe_items.parquet' (format parquet, compression zstd);
copy (select * from '$src/items.parquet' where id in (select id from want)) to '$out/items.parquet' (format parquet, compression zstd);
copy (select * from '$src/vendor_sources.parquet' where item_id in (select id from want)) to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/loot_sources.parquet' where item_id in (select id from want)) to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainer_spells.parquet' where spell_id in (select spell_id from r)) to '$out/trainer_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainers.parquet' where npc_id in (select npc_id from '$src/trainer_spells.parquet' where spell_id in (select spell_id from r))) to '$out/trainers.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select npc_id from '$src/trainer_spells.parquet' where spell_id in (select spell_id from r))) to '$out/npcs.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_notes.parquet') to '$out/profession_notes.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_routes.parquet') to '$out/profession_routes.parquet' (format parquet, compression zstd);
copy (select * from '$src/profession_tiers.parquet') to '$out/profession_tiers.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_builds.parquet') to '$out/talent_builds.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_nodes.parquet') to '$out/talent_nodes.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_priorities.parquet') to '$out/talent_priorities.parquet' (format parquet, compression zstd);
copy (select * from '$src/disenchant_yields.parquet') to '$out/disenchant_yields.parquet' (format parquet, compression zstd);
"
