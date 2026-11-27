#!/bin/sh
# Rebuilds testdata/usability_snapshot from a built dataset directory: the rows the offline gear-usability, class training
# and binding benchmark (make offline-benchmark, internal/paths/offline_usability_benchmark_test.go) compares: a spread of
# worn armor and weapons (one item per item class, subclass and slot, every item a class mask restricts, items that list no
# required level, shields and off-hand weapons), shirts, tabards, bags (plain, soul, herb and enchanting), quivers, ammo pouches and ammo, the crafted items of each binding with their
# recipes, the loot and vendor rows of those items, every class spell, the trainer spells, the talent trees and nodes, the weapon skills
# and the Weapon Masters. Whole rows of the dataset; read-only on it. Needs duckdb. Usage: testdata/usability_snapshot/build.sh <dataset dir> [output dir]
set -eu
src="${1:?usage: build.sh <dataset dir> [output dir]}"
out="${2:-$(dirname "$0")}"
mkdir -p "$out"
rm -f "$out"/*.parquet
duckdb -no-agent -c "
create temp table gear as select * from '$src/items.parquet' where slot > 0 and class_id in (2, 4) and quality >= 1
  and not regexp_matches(name, '(?i)deprecated|test|monster|^old|\(|pvp|wedding|dress|hidden|transmog');
create temp table worn as select * from '$src/items.parquet' where slot in (4, 18, 19) and class_id in (1, 4, 11) and quality >= 1
  and not regexp_matches(name, '(?i)deprecated|test|monster|^old|\(|pvp|transmog|hidden');
create temp table want as
  select id from (select id, row_number() over (partition by class_id, subclass_id, slot order by id) n from gear) where n <= 2
  union select id from (select id, row_number() over (partition by allowable_class_mask order by id) n from gear where allowable_class_mask not in (-1, 32767, 1535)) where n <= 3
  union select id from (select id, row_number() over (order by id) n from gear where required_level = 0 and quality >= 2 and item_level > 5) where n <= 8
  union select id from (select id, row_number() over (partition by slot order by id) n from gear where slot in (14, 22, 23, 21, 17, 13)) where n <= 4
  union select id from (select id, row_number() over (partition by class_id, slot order by id) n from worn) where n <= 2
  union select id from (select i.id, row_number() over (partition by i.bonding order by i.id) n from '$src/items.parquet' i
    where i.quality >= 2 and i.slot > 0 and i.class_id in (2, 4) and i.id in (select creates_item_id from '$src/recipes.parquet')) where n <= 2
  union select id from (select i.id, row_number() over (partition by i.bonding order by i.id) n from '$src/items.parquet' i
    where i.quality >= 2 and i.slot > 0 and i.class_id in (2, 4) and i.id in (select item_id from '$src/loot_sources.parquet')) where n <= 2
  union select id from (select i.id, row_number() over (order by i.id) n from '$src/items.parquet' i
    where i.bonding = 3 and i.id in (select creates_item_id from '$src/recipes.parquet')) where n <= 2
  union select id from '$src/items.parquet' where bonding = 3 and class_id = 0 and id < 6000
  union select id from '$src/items.parquet' where id in (3030, 10579, 274387, 8067, 8068, 8069, 10512, 10513, 15997, 274388,
    7278, 7371, 8217, 7279, 7372, 8218, 4238, 10050, 14046, 14156, 21340, 21341, 22246, 22251);
create temp table rec as select * from '$src/recipes.parquet' where creates_item_id in (select id from want);
create temp table reag as select * from '$src/recipe_reagents.parquet' where spell_id in (select spell_id from rec);
create temp table want2 as select id from want union select reagent_item_id from reag;
create temp table ls as select * from '$src/loot_sources.parquet' where item_id in (select id from want2);
create temp table vs as select * from '$src/vendor_sources.parquet' where item_id in (select id from want2);
copy (select * from '$src/items.parquet' where id in (select id from want2)) to '$out/items.parquet' (format parquet, compression zstd);
copy ls to '$out/loot_sources.parquet' (format parquet, compression zstd);
copy vs to '$out/vendor_sources.parquet' (format parquet, compression zstd);
copy rec to '$out/recipes.parquet' (format parquet, compression zstd);
copy reag to '$out/recipe_reagents.parquet' (format parquet, compression zstd);
copy (select * from '$src/npcs.parquet' where id in (select source_id from ls where source_kind = 'creature') or id in (select npc_id from vs)) to '$out/npcs.parquet' (format parquet, compression zstd);
copy (select * from '$src/class_spells.parquet') to '$out/class_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/trainer_spells.parquet') to '$out/trainer_spells.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_trees.parquet') to '$out/talent_trees.parquet' (format parquet, compression zstd);
copy (select * from '$src/talent_nodes.parquet') to '$out/talent_nodes.parquet' (format parquet, compression zstd);
copy (select * from '$src/weapon_skills.parquet') to '$out/weapon_skills.parquet' (format parquet, compression zstd);
copy (select * from '$src/weapon_masters.parquet') to '$out/weapon_masters.parquet' (format parquet, compression zstd);
"
