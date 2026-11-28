# Changelog

## 0.7.104

- **Fixed** The next-recipe list now shows green, not yellow, for a recipe the game shows green.
- **Fixed** A route band's colour now reads green where the game shows green.
- **Fixed** Changing a primary profession now says which one you keep and which you drop.
- **Fixed** Dropping a profession now says the game data does not state what it costs.
- **Fixed** Gather lines name up to three nodes per material and skip nodes of unknown skill.
- **Fixed** Ore a smelt needs now gets gather lines, and ore and stone count as miner's materials.
- Asking for a poison vendor now finds Sly Garrett in Booty Bay by the reagents he sells.
- Other profession words (tailoring, alchemy, cooking) find vendors of that craft's reagents.
- "Vendor in Booty Bay" works as one sentence, with no separate place.
- When no vendor is found the answer says so and adds nothing from general knowledge.
- Restock and profession shopping now take materials from your own bank first, before any purchase.
- A character on your realm and faction can mail what it spares, and gets a matching mail errand.
- Items that arrive by bank or mail count in the bag-space forecast.
- The game data now follows the newest character capture, so a new game build gets its data
- The Notes line says when game data is published but not downloaded yet, and why
- Finding a vendor also searches the shops your characters opened, so Sly Garrett is found.
- Those answers say "seen by your characters"; the dataset and QuestieDB win where they differ.

## 0.7.103

- **Added** The route forecasts your bag space and plans a sell, mail or bank stop before they fill.
- **Added** Such a stop says when the bags fill ("bags fill around step 9") and what it frees.

## 0.7.102

- **Fixed** Quest giver, turn-in, prerequisite and breadcrumb questions now say the data has none.
- **Fixed** A minimum level asked for a quest by its title now says the data has none.
- **Fixed** A quest in a quest line now shows its giver, turn-in and level from your Questie.
- **Added** The route trains your professions in a capital, naming the trainer to visit.
- **Added** Materials your next skill-up needs show as a buy line on the stop near the vendor.
- **Added** Herbalism and Mining trips name the node to gather; held craft batches go to a city.

## 0.7.101

- **Fixed** Disenchant questions now answer from the dataset's bands and say when it has none.
- **Fixed** Potion and elixir cooldown or stacking questions now say the data holds no such rule.
- **Fixed** Stacking a guardian elixir with a battle elixir is no longer answered as a stack size.
- **Fixed** Questions on learning to ride or a flight's cost now say the data has no answer.
- **Fixed** Hearthstone cooldown, bind and replacement questions now say the data holds none.
- **Fixed** Flight path and flight master questions now say the data holds no such rows.
- **Fixed** A soulbound mount with no sell price is no longer called sellable to a vendor.
- **Fixed** A class quest reward is no longer told "in Warlock" or offered to other classes.
- **Fixed** Questions on what Rebirth, Vanish or Reincarnation need now say the data has no row.
- **Fixed** Wands no longer send a mage, priest or warlock to a Weapon Master who does not teach them.
- **Fixed** Weapon questions saying "two-hander", "2h" or "1h" now get the weapon master facts.
- **Fixed** Questions on a pet, totem, stance or form quest now say the data has no class quest line.
- **Fixed** "Where should I level at level 25" now says the data holds no zone level range.
- **Fixed** Questions on a contested, hostile or one-side zone now say the data holds no zone control.
- **Fixed** "Nearest capital city" and "neutral auction house" now get the capital services line.
- **Added** The route can hearth to your bind once per plan when it is ready and saves travel time.
- **Fixed** Dungeons now show 5 players, and Blackrock Spire shows 5 in Lower and 10 in Upper.
- **Fixed** "How do I get attuned" and "what do I need for" a raid now bring up that raid's facts.
- **Fixed** Short names such as ZF, WC, SM, Uld, ST, VC and DME now name their dungeon.
- **Fixed** Questions on the gear level a dungeon needs now say the data holds no recommendation.
- **Fixed** Questions on where a rare or elite stands, its level or respawn now say the data has none.
- **Fixed** A creature asked for by name now shows what the data holds for it and what it drops.
- **Fixed** Rare items, rare herbs and elite quests no longer get the rare creature note.

## 0.7.100

- **Fixed** A quest turns grey at the game's level for your level, not a flat eight levels below.
- **Fixed** The "grey" mark on a quest in your log means grey in the game, not six levels below.
- **Added** Questions on quest XP, group size or minimum level now say the data has none.
- **Fixed** An item a few creatures drop often is no longer called a random drop not worth chasing.
- **Added** Questions on an item set's pieces now list them from the game data.
- **Added** Questions on set bonuses, rare spawns or drop odds now say the data has none.
- **Fixed** An item that starts a quest is now kept in your bags until you take the quest.
- **Added** Questions on escort quests, timed quests and race limits now say the data has none.
- **Added** Questions on a class trainer's or summoning stone's place now say what the data holds.
- **Fixed** Questions on which weapons or armor a class can use now answer for the class you name.
- **Fixed** "Locate…" exploration quest steps are now placed from Questie's end-of-quest area.
- **Added** `doctor quests` counts the steps placed from that area.
- **Added** The AI now thinks in routes and errands, per new product principles.
- **Added** Plans attach restock, repair, junk-sale and alt-mail errands to stops the route visits.
- **Added** The doctor says when durability or bag size is missing from the capture.
- **Fixed** Vendors, repairers and trainers are found with your game's own NPC flag bit layout.
- **Fixed** "Is there a poison vendor in Booty Bay?" finds Sly Garrett on Forever and Classic.
- **Changed** `doctor quests` now says which NPC flag layout it read.

## 0.7.99

- **Improved** Chat answers now show bold text, lists and small headings instead of raw asterisks.
- **New** Settings has a Text density choice (Compact or Comfortable), and answers have Copy answer.
- **Fixed** Profession advice names the trainer of the rank you need, not an earlier rank's trainer.
- **Added** The next profession rank shows the skill and level it needs and what training costs.
- **Fixed** Expert and Artisan First Aid, Cooking and Fishing say they come from a book or quest.
- **Fixed** A better item is no longer hidden because the item you wear has a higher item level.
- **Fixed** Gear advice says so when it does not know your worn item, instead of ranking upgrades.
- **Fixed** Profession pairings say your two slots are full and name the pair you hold.
- **Fixed** Rank quests and books for First Aid, Cooking and Fishing are now named with their giver.
- **Fixed** The stat check counts a worn item even when the capture gives it no name.
- **Fixed** A recipe an item teaches is no longer sent to a trainer when the item has no listing.
- **Fixed** A recipe that is a quest reward now names the quest players saw give it.
- **Fixed** A recipe's answer now names the profession rank and character level its skill needs.
- **Fixed** A recipe past your trained rank now says which rank to train and from what level.
- **Fixed** A part of a dungeon name that fits several, like Blackrock, now lists them, no guess.
- **Fixed** Dungeon questions understand short names such as BRD, UBRS, SFK, the Stockade, Onyxia.
- **Fixed** An item's boss no longer comes from a dungeon's alternate-mode encounters.
- **Fixed** A boss drop with no kill count now says its chance is not known.
- **Fixed** The dungeon plan words one level and one dungeon in the singular.
- **Fixed** A material only limited-stock vendors sell is now marked "limited stock" in a plan.
- **Fixed** A spec named in a few letters, like "shadow" or "feral", now finds its talent build.
- **Fixed** Below level 10 the talent plan says the build starts at level 10, not that it is done.
- **Fixed** A weapon that needs a skill a Weapon Master teaches only from level 20 now says so.
- **Fixed** A higher rank of a spell you know is now called a new rank, not a new spell.
- **Fixed** A mage is no longer told to train teleports and portals to the other side's cities.
- **Fixed** Mana Tide Totem, a shaman talent, is no longer listed as a level 1 spell to train.
- **Fixed** The Auction House search for armor now names the armor type your class wears.
- **Fixed** A quiver or ammo pouch your class cannot use is mailed to an alt who can, else sold.
- **Fixed** Ammo your class cannot use goes to an alt that can, and a class-only bag to that class.
- **Fixed** Gear upgrades now count Defense Rating for tanks and Hit and Crit Rating for damage.
- **Fixed** A healer's gear now counts Crit Rating and mana per five seconds.
- **Fixed** A resistance on a piece of gear no longer lowers its value for your build.
- **Fixed** A quest reward line now says when the quest is for a group or a dungeon.
- **Fixed** The quest chain says whether one or all of the earlier quests must be finished first.
- **Fixed** The vendor finder no longer says a vendor is open to both sides when no data says so.
- **Fixed** The class trainer line now names the zeppelin or boat when the city is overseas.
- **Fixed** A character in Dun Morogh or Mulgore is sent to Ironforge or Thunder Bluff to train.
- **Added** A reputation question now gets your standing and the points left to the next standing.
- **Added** A loot roll question for a linked item says Need or Greed and how the item binds.
- **Fixed** A grey quest item is no longer listed as vendor trash in the bag review.
- **Fixed** A dungeon the data gives no level says the level is not known.
- **Added** A linked item gets its item level, required level and trade rule in the answer facts.
- **Added** A bag, bank, stack, mail, auction, trade or macro limit question says the data has none.
- **Fixed** A quest item is no longer mailed to an alt: quest items cannot be mailed or traded.
- **Fixed** A quest item no quest needs is no longer listed as gear to bank in the bag review.
- **Fixed** A grey item is no longer called bound when picked up; grey items can be traded.
- **Added** An ore or herb node question gets the node's skill and zones from the game data.
- **Added** A skinning question gets the data's rule and the skill for each creature level named.
- **Added** A fishing question says the data has no fishing skill or pool; a zone lists its catches.
- **Added** A Cooking or First Aid recipe question gets the skill it needs ("about" if estimated).
- **Fixed** Gear and item advice no longer offers the other faction's PvP rank pieces.
- **Fixed** Test items named "PVP TEST" are no longer treated as real items.
- **Added** A battleground or honor question says the data holds no bracket, rank or honor cost.
- **Fixed** A plan no longer says to fly to another continent; it names the portal or boat.
- **Fixed** A route through the Deeprun Tram keeps the tram's name in capitals.
- **Added** A hearthstone cooldown, boat schedule or flight fare question says the data holds none.
- **Fixed** A riding or mount question says the data holds no riding trainer, cost or reputation.
- **Fixed** A class trainer price question says the data holds no class spell price.
- **Added** A Dual Wield question gives the level the class learns it at, from the class trainer.
- **Fixed** A profession trainer question no longer offers a trainer in the other side's capital.
- **Added** Racial trait, capital city service and faction leader questions say the data holds none.
- **Fixed** An event name no longer matches a zone or creature, such as Winterspring.
- **Added** Event, weather, day-night and zone level questions say the data holds none.
- **Fixed** A vendor price shows in gold, silver and copper; an Auction House sale is no longer 0g.
- **Added** Gold making, Auction House, gold cap, repair and respec questions say the data has none.
- **Added** Boss ability, raid lockout, heroic mode and group role questions say the data has none.
- **Fixed** A Feral Combat druid is offered first to a damage dealer as well as to a tank.
- **Added** Pet, tabard, achievement, title and tool bonus questions now say the data has none.
- **Fixed** The key ring size is no longer guessed; a creature's prop is not taken for your tool.
- **Added** Class quest chain, quiver size and mail rule questions now say the data has none.
- **Fixed** Dungeon keys are kept, not destroyed; ammo is kept only for the weapon you wield.
- **Added** Questions on spell reagents, soul shards, totems and rituals now say the data has none.
- **Fixed** A druid's seeds, a warlock's shards and stones and other class reagents are kept.
- **Fixed** A shaman's totem is kept or worn, not sent to the bank.
- **Added** Questions on a hunter pet's diet, abilities, ranks and taming now say the data has none.
- **Fixed** A hunter's pet food is kept, not sold on the Auction House or left unplaced.
- **Fixed** Pet food in another character's bags goes to a hunter alt by mail.
- **Added** Questions on a bandage's heal, lockpicking and food effects now say the data has none.
- **Fixed** Bandages, anti-venoms and a rogue's lockboxes are kept, not left unplaced.
- **Fixed** First Aid, Fishing and Cooking are found in more ways of asking to level them.
- **Fixed** "Can I make it yet?" now finds the skill a recipe needs.
- **Added** The plan lists step prerequisites, the zone's flight masters and the save position.
- **Fixed** An unplaced objective names its zone once, and "Locate" reads as an action.
- **Fixed** The overlay frame no longer has a blurred shadow cut off at the window edge.
- **Changed** The frame keeps a crisp 1px dark outline and the gold inner highlight.

## 0.7.98

- **Fixed** Picks, hammers, knives and poles for your professions are kept, not banked or sold.
- **Fixed** A bag or quiver in your bags is left alone, not banked or counted as a freed slot.
- **Fixed** A bag or tool you can't make yet says it is crafted, not "no source" or "not worth it".
- **Fixed** Asking about a Mageweave Bag no longer reads as a question about mages.
- **Fixed** Corpse, graveyard and repair questions are answered as game know-how, not your data.
- **Fixed** Engineering now asks for the Arclight Spanner when the character has none.
- **Fixed** The app now fetches the dataset matching your game build instead of an old one.
- **Changed** The overlay says "Updated: data <build> (matches your game)" after a dataset switch.
- **Changed** Notes say "No data for game build N yet: using M" until a matching dataset exists.
- **Fixed** Skill-up steps now say a green recipe gives a skill point, not only orange or yellow.
- **Added** The plan skips quests over 3 levels above you; you can change that reach or turn it off.
- **Added** Quests you hide go to Later as "Hidden by you", and elite quests wait for a group.
- **Added** Steps and later entries carry the quest's level and its difficulty colour.
- **Changed** A ready turn-in or delivery close to you now comes first in the plan.
- **Changed** Delivery steps read "Turn in <quest> to <NPC> (<place>)".
- **Fixed** Bag advice never says to sell, toss or mail an item a quest in your log needs.
- **Fixed** Items for a ready turn-in (Kravel's Parts, Glowing Shard) are kept, with the quest named.
- **Changed** Quest items no quest needs now read "safe to destroy", naming the completed quest.
- **Added** Ask "who sells poison?" or "which vendor is here?": answers name the NPC and the goods.
- **Added** Vendor, trainer and flight master lookups read your Questie subnames, like Shady Goods.
- **Changed** A recipe whose product you carry counts as likely known; open its window to confirm.
- **Added** `wowc-advisor doctor professions` shows which profession recipe lists were captured.

## 0.7.97

- **Changed** What's new always shows the release history, with a "New since you last looked" line.
- **Fixed** What's new no longer says "Nothing new" right after you update.
- **Fixed** Profession advice names a trainer and town for your side when the game data has one.
- **Fixed** A dungeon's boss list no longer repeats a boss the game data lists twice.
- **Changed** Profession advice says so when the game data names no trainer for your side.
- **Changed** What's new opens at the top with the newest release, and reads top to bottom.
- **Changed** The line now reads "You're caught up: everything below you've seen".
- **Fixed** Open quests use the level players reported when the game data has none.
- **Fixed** A recipe taught by a recipe item no longer points to a trainer when no seller is named.
- **Fixed** A vendor price is no longer lower than what you pay with no reputation standing.
- **Fixed** Skill-up advice calls a recipe green, one skill point in four crafts, instead of yellow.
- **Fixed** Routes to Outland name the Dark Portal; Mount Hyjal and Uldaman count as known places.
- **Fixed** "Train at" shows where to fly from when the save names your zone but no position.
- **Fixed** Crafted flasks, elixirs and food are no longer called Bind on Equip.
- **Fixed** A mount with no source in the game data says so, not "not worth chasing".
- **Fixed** Trainer advice reads "an Enchanting trainer" and "an Enchanter", not "a Enchanting".
- **Fixed** Librams, idols and totems are kept by paladins, druids and shamans, not sold as useless.
- **Fixed** Shirts, tabards and cosmetic armor are kept by every class, not called unusable.
- **Fixed** Gear above your level in the bag review reads "you can wear it from level N".
- **Fixed** Plate gear needing level 40 is kept for a warrior or paladin from level 40, not sold.
- **Fixed** Talents such as Frostbite or Combustion are not listed as spells a trainer teaches.
- **Fixed** Bags and quivers get no wear verdict in the bag review.
- **Fixed** A quest reward names its quest from what players saw, with its zone, level and side.
- **Fixed** A quest reward for the other side, or far above your level, is no longer offered.
- **Fixed** A session never ends at an inn or flight point of the other side.
- **Fixed** Tailoring advice trains Artisan at skill 200 and level 35, not at the 225 cap.
- **Fixed** A material a vendor sells is now sent to the vendor, not the Auction House.
- **Fixed** A gathered material names where to find it, and is not offered below the skill it needs.
- **Fixed** A craft now says how many of its materials sit in your bank.
- **Fixed** Bind on Equip gear no character can wear is sent to the Auction House, not a vendor.
- **Fixed** Session walking times now follow a typical zone's size and the speed of a first mount.
- **Fixed** Cooking and Alchemy recipes with "Goblin" in the name are no longer left out of plans.
- **Fixed** A quest can be found by its name when the game data only lists its title.
- **Fixed** A Cooking trainer is a city of your own side, not a zone of no known side.
- **Fixed** A dungeon's boss list no longer holds the bosses of its other game modes.
- **Fixed** A raid asked by name now lists its size, entrance and bosses, and its lockout as unknown.
- **Fixed** Zul'Farrak, Onyxia's Lair and other names with an apostrophe are found in a question.
- **Fixed** A boss drop that is very rare is no longer worded as a zero chance.
- **Fixed** An item the game data lists no source for is no longer called not worth chasing.
- **Fixed** Quest line steps are named from the titles players saw when the game tables have none.
- **Fixed** Quests that players tagged as group or dungeon quests are now marked as group quests.
- **Fixed** A quest that asks you to kill a boss outside a dungeon is flagged as group content.
- **Fixed** A recipe a trainer and a recipe item both teach now names the recipe item too.
- **Fixed** An estimated recipe skill is now worded as "about" instead of an exact number.
- **Fixed** Riverglades, Shen'dralas and the Darkspear Islands now count as zones for travel steps.
- **Fixed** The Feast of Winter Veil now starts on December 16, as in the game.
- **Fixed** A profession plan now shows the exact cost down to the copper, not rounded at 1 gold.
- **Fixed** An item set name now finds its set once, with its real id, without a name index.
- **Fixed** Fishing advice no longer sends you to the other faction's capital or starting zone.
- **Fixed** Asking where to fish an item now answers from catch records when Questie has no school.
- **Fixed** Expert and Artisan Fishing and Cooking now say they come from a book or a quest.
- **Fixed** The "before this step" line for a Fishing step no longer leaves out the step's name.
- **Fixed** A crafted pet nothing drops now says it is crafted instead of "also" crafted.
- **Fixed** A skill-up route now names the trainer for your next rank when the guide names one.
- **Fixed** The rank gate in a skill-up route now points at the step that needs the new rank.
- **Fixed** A skill-up route no longer offers an alternative you have passed, with a negative count.
- **Fixed** The missing-tool line names the Mining Pick, Skinning Knife or Hammer and its price.
- **Fixed** Fishing now says you need a Fishing Pole when you carry none.
- **Fixed** A bar or leather you can make with your own profession says so, not "buy it".
- **Fixed** An item with a sourceless material no longer says you lack a profession you have.
- **Fixed** A route names the zeppelin or boat even when your side is saved in lower case.
- **Fixed** The class-trainer flight point is found when the zone name has stray spaces around it.
- **Added** Your brief now says where your hearthstone is bound.
- **Added** The train-at line says when your hearthstone is bound in the class trainer's city.
- **Fixed** A class's trainer list no longer offers talents, stance passives or effect spells.
- **Fixed** The Warrior's Execute is offered at its real level, not at the start beside its effect.
- **Fixed** Quest XP is cut one level later for a quest you have out-levelled, as the game does.
- **Fixed** Asking where to learn riding no longer answers with the riding enchant recipe.
- **Fixed** Class training now picks a capital on your continent that has your trainer.
- **Fixed** Starting areas such as Deathknell and Valley of Trials are placed on their continent.
- **Fixed** An enemy capital no longer offers train and bank errands.
- **Fixed** A quest typed without its apostrophe, such as "A Rogues Deal", is found.
- **Fixed** A flask, elixir or food you can't make now points to the Auction House to buy it.
- **Fixed** A dungeon you ask about says its key or entry requirement is not known, not nothing.
- **Fixed** A raid that opens at level 60 is worded "level 60", not "levels 60-60".
- **Fixed** An item vendors sell but the data doesn't name says so, not "not worth chasing".
- **Fixed** A quest says when the game data doesn't name who gives it or takes it in.
- **Fixed** A dungeon far below your level says so when you ask about it by name.
- **Fixed** A chest that holds an ore or herb is no longer offered as a place to gather it.
- **Fixed** The dungeons offered for your level are listed lowest level range first.
- **Fixed** A quest you look up now says when players tagged it as a group or dungeon quest.
- **Fixed** A craft's vendor-sold materials say "each" on the price when you need more than one.
- **Fixed** Farming advice says what a vendor pays for the drop, and gives no gold per hour.
- **Fixed** Class questions list only abilities a class learns, each at its first rank's level.
- **Fixed** The level-up spell list no longer offers a talent whose ranks are all at level 1.
- **Fixed** The bag review no longer mails a Bind on Pickup piece to an alt; it can't be mailed.
- **Fixed** Leatherworking plans skip Cured Rugged Hide; its salt comes from a cooldown craft.
- **Fixed** "An hour and a half" or "forty five minutes" now sets the length of the session asked for.
- **Fixed** A top-level character's XP line says there is no XP to earn instead of an empty bar.
- **Fixed** Class trainer advice now picks the city a flight from your zone reaches, like Ironforge.
- **Fixed** A herb or ore too high for your skill now says what skill it needs, not "gather it".
- **Fixed** Herb and ore zones now include every spot the game data lists for it, like The Barrens.
- **Fixed** A fish with no other source now says it is a fishing catch and where to fish it.
- **Fixed** Fishing zones are credited to the game data's fishing reports, not a Questie database.
- **Improved** Trips are ordered as a travel route, so you cross between continents far less often.
- **Improved** Exits and trips name the real way: zeppelins, boats and your known flight paths.
- **Improved** Each trip shows an estimated travel time in minutes.
- **Fixed** The train and bank errands in a city now have a position on the map.
- **Fixed** Dungeon quests set aside now name the dungeon and its zone, not "another area".

## 0.7.96

- A finished quest is never "too high": it gets a trip to turn it in, or a Find line with its hint.
- Quests too high to do say when you can, such as "level 40 · doable around 37".
- A visit to a capital lists errands: train new ranks, open your bank, class quests, turn-ins.

## 0.7.95

- **Fixed** profession routes now list the materials each step needs, counted per craft.
- **Fixed** Herbalism, Mining, Skinning and Fishing routes now name where to gather and what.
- **Fixed** gear counts bonus healing and damage, extra armor, and armor for tanks.
- **Changed** the addon update notice now says a /reload is required to load the new addon.
- **Changed** when an addon update adds files, the notice says to fully exit and restart WoW.
- **Changed** Your plan now has one shared list: the run here, trips by zone, and parked quests.
- **Changed** The run here lists only stops on your map, ending with how to reach the next trip.
- **Changed** Every quest in your log appears once: in the run, in a trip, or parked with a reason.
- **Changed** New pick-ups are limited to 3, near your route or the hub, and fit your level.
- **Fixed** A turn-in that is not ready is a stop on the route, not an action at the spot now.
- **Added** every notice has a What's new button for all app and addon changes since you looked.
- **Fixed** the notice after an automatic addon update now says /reload or a full restart is needed.
- **Added** Settings has a What's new button that lists the same changes.

## 0.7.94

- **Fixed** objectives Questie places on the map but the plan missed now get real spots.
- **Changed** a stop with no known position is sent as x = -1, y = -1, marked unplaced, never 0, 0.
- **Changed** with no spawn data, subzones named in the quest text give approximate spots.
- **Fixed** Questie quest fields are read by the database's own key map.
- **Added** an objective with no known spot is placed from the game's quest map point.
- **Added** `wowc-advisor doctor quests` counts how your logged objectives are placed.
- **Fixed** objectives on enemy-faction NPCs now say "Kill", not "Talk to".
- **Changed** the verb comes from the quest log's own objective text ("slain" is Kill) when present.
- **Changed** without log text, an NPC is friendly only if its faction includes your side.
- **Fixed** a Horde Skyborne is no longer sent to Alliance quest givers; Skyborne has own race bits.
- **Fixed** an unknown race now falls back to the captured faction's races, and says so.
- **Changed** pick-ups skip quest givers who are friendly only to the other side.
- **Fixed** bag gear is compared by its captured stats, so a lower item-level blue is no upgrade.
- **Fixed** without stats, item level decides and quality only breaks ties ("likely better").

## 0.7.93

- **Changed** a quest handed in at another NPC, with nothing to do, now reads as a trip to that NPC.
- **Changed** a trip into a dungeon is marked "(dungeon)" and ranks after quick turn-ins.
- **Changed** a zone you are not in now gets one connected route that starts where you arrive.
- **Changed** the route starts at a flight master you know, such as Tharm at Sun Rock Retreat.
- **Changed** with no known flight point, the step says to walk or ride, never to fly.
- **Changed** across continents, the step names the zeppelin or boat first, then the flight.

## 0.7.92

- **Changed** talent advice now follows the Icy Veins leveling build, one next pick at a time.
- **Fixed** plans no longer say "map 310" or "map 279" for dungeons like Shadowfang Keep.
- **Changed** a travel step to a dungeon now names its entrance zone, e.g. Silverpine Forest.
- **Changed** a place the app cannot name now reads "an unknown place".

## 0.7.91

- After an addon update, the notice says to type /reload in game.
- **Added** a "Copy diagnostics" button in Settings that copies a private, plain-text summary.
- **Added** training lines now carry spell ids, prerequisites and the trainer's map position.

## 0.7.90

- Profession skill-up advice follows the efficient routes from your game data.
- Enchanting at 51 now says: Enchant Chest - Inferior Stamina to 70, then Journeyman.
- Trainers named are your profession's own trainers, never a class trainer.
- Material counts add bags and bank together and say where they are.
- The rod, pick or knife is mentioned only when you are missing it.
- With no route in your game data, the AI says the path is estimated and picks the cheaper recipe.
- Release notes now collect several changes per update.

## 0.7.89

- The app now records why it ended: quit, window close, update restart, go back, shutdown and crashes.
- After an unexpected end, the next start says "closed unexpectedly at <time>" with Open logs.
- The helper's output has its own log, and it restarts itself if it dies.
- Settings > Updates: Open logs folder and Copy diagnostics (a zip in Downloads, secrets left out).
- A missed class or racial spell quest, like Touch of Weakness, now gets a plan step.
- A trainer rank blocked by that quest says so, not listed as trainable.
- The training line in the digest names the quest that teaches the spell.
- The NEXT line names the item in your bags to equip, instead of only saying you have an upgrade.
- Rune abilities (engravings) are no longer listed as spells to train.
- Spell lists show each spell once, at its highest rank.
- With no trainer visit on record, training points you to your trainer's list.

## 0.7.88

- Nearby-quest picks skip holiday quests outside their event, or when holiday quests are off.
- Quests that need a higher level than yours are never picked.

## 0.7.87

- Weapon rules now come from your game data, not the AI's memory.
- Ask where to train Staves: the AI names your faction's Weapon Masters and the price.
- A weapon you can learn but haven't says "Needs the Staves skill" and where to train.
- A weapon your class can never use is not suggested.
- A big upgrade that needs a skill adds a "Train Staves" step, after your ready turn-ins.
- With older game data, the AI says it has no weapon-skill facts.

## 0.7.86

- The plan file lists where each quest in your log is handed in, for live turn-in hints.

## 0.7.85

- In town, a dungeon quest at your level (The Book of Ur) is a pick-up step, first on the route.
- A dungeon far above you, and holiday quests, stay under later.
- With a nearly full quest log, only as many pick-ups as free slots are planned.

## 0.7.84

- Holiday event quests are opt-in, off by default. Turn them on in Settings, or just ask the AI.
- While an event runs, the panel shows one line: "Midsummer Fire Festival is on (until July 5)".
- Opted in, a running event's quests join your plan as optional steps, after your level quests.
- When an event ends, its quests drop out. Your choice stays for the next one.

## 0.7.83

- Ready turn-ins where you stand now come first on the route, nearest first, before pick-ups.
- Ready turn-ins in a town you can fly to get one "Fly to <town>" step.
- Travel steps name the place ("Shadowfang Keep (Silverpine Forest)"), never a map number.

## 0.7.82

- Training lists only what your trainer shows as available.
- A spell that needs an earlier rank is noted, not listed as new.
- Gold needed counts only the spells you can train now.
- Talent picks skip ranks you already took, matched by spell.
- If it can't tell which talents you took, it says so.

## 0.7.81

- Unspent talent points come first: "Spend 1 talent point: Shadow Focus (Shadow)".
- If your picks can't be seen yet, it says to open your talents instead of naming a talent.
- "What next" answers mention unspent points first; the line clears once you spend them.

## 0.7.80

- Gear upgrades now weigh how much of each stat an item has, not only which stats.
- Spell Power counts for casters and healers, and Attack Power for melee, in upgrades.
- The stat check adds up the stat amounts of your worn gear.

## 0.7.79

- A turn-in is planned only for a quest that is done; objectives come first.
- Every step's first line is an action; the level fit goes below it.
- Quests now say "ready to turn in" or how many objectives are done.

## 0.7.78

- The route, the panel's Next quests and the answers now share one order.
- A class quest's objectives come early on the route, after anything at your current spot.
- Every quest in your log gets its open objectives as steps; with no known position, the zone only.
- Places your Questie database marks for objectives are used, and say so.

## 0.7.77

- Recorded recipes count as known; a plan you lack says "Needs the Plans", not "you can make it".
- Without a recipe list, crafting advice says "if you have the recipe".
- Skill-gap lines name the profession: "Your Blacksmithing is 35; it needs 45 (10 more points)."
- The update note names what updated: the app, the addon, or both.
- A class quest that teaches several spells lists them in a fixed order.

## 0.7.76

- Quest steps now name what is left to do and where: "Talk to" a friendly NPC, never "Kill".
- Pick-ups and turn-ins at a place you are already at come first on the route.
- The plan file now says, for each NPC stop, what you can do there and what each quest still needs.

## 0.7.75

- Training advice follows what your class trainer really sells and the spells you know.
- Spells a class quest teaches are named as quest rewards, not as things to buy.
- Without a trainer visit, the list says "usually" and "if you don't have it yet".

## 0.7.74

- The plan file now says what each quest needs once accepted, so the panel can move on live.
- The overlay follows the character you play; pick one by hand to keep it, or turn it off.

## 0.7.73

- Gear you already carry that beats what you wear now comes first, as "Equip these now".
- The panel, the NEXT line and gear answers list those bag items before anything to buy.
- Grouped Auction House lines say "Green mail gear" correctly and note that cloaks are cloth.

## 0.7.72

- Profession plans show vendor prices as a new character pays them.
- Profession plans send Alliance and Horde characters to a trainer of their own side.
- A trainer price the game data lacks is no longer shown as free.

## 0.7.71

- The app now offers, checks and installs new in-game addon versions.
- The addon follows the same delay and automatic-update settings as the app.
- Your Digest and Plan files are never touched during an addon update.
- Settings > Updates can go back to the last three addon versions.
- A pulled addon version goes back by itself.
- If the app needs a newer addon, the update notice says so.

## 0.7.70

- Training advice sends you to the nearest class trainer city, using flight paths.
- Gear advice lists only crafts your skill can make now, and says what skill the others need.

## 0.7.69

- See all changes now opens the release list.
- A link that can't be opened now says so in plain words.
- The update notice fits on one row: Install, What's new and Later.

## 0.7.68

- Update notes are now short bullets, one change each.
- The update notice shows the first few changes, with a See all changes link for the rest.

## 0.7.67

- Alchemy plans from skill 1 no longer open with a gap before the first elixirs.
- Profession plans no longer ask you to buy bolts the plan crafts itself.
- Stat advice now checks both stats of each suffix.

## 0.7.66

- Tailoring plans follow the proven path more closely, with cloth bolts and the right recipes.
- Recipes are no longer planned far too early for your skill.
- Cloth bolts you make for skill-ups now count for the recipes that use them later.
- Tailoring plans name your trainer, in Undercity or Stormwind, and when to visit.

## 0.7.65

- Profession plans need fewer crafts and match the Forever guides more closely.
- Plans now leave a recipe once it turns green, unless nothing else is better.
- Herbalism, Mining and Skinning tell you what to gather and where, not what to buy.
- Fishing shows its catches again.
- Gear advice no longer suggests suffixes with a stat your build ignores.
- A trainer the data has no name for is shown by its number, not "trainer trainer".

## 0.7.64

- Gear advice names the right item suffixes, like "of the Eagle" for survivability.
- The stat text no longer adds a line that contradicts the advice.
- The app knows your professions from your recipes, even if your skill levels were missed.
- "How do I level tailoring?" now gives your real plan.

## 0.7.63

- Stat advice shows what each point of Intellect and Stamina is worth for your character.
- The worth is shown only when it matches your own mana and crit.
- Stats from older addon versions now show the right value.

## 0.7.62

- New "Install updates automatically" switch in Settings > Updates, off by default.
- The update notice has an "Always update automatically" button.
- Updates install when the app starts or after a quiet while, never while an answer is written.
- If your version is pulled, the app goes back by itself and tells you why.
- A failed download is tried again at the next check.

## 0.7.61

- What's new now lists everything since the version you last ran, newest first.
- A See all changes button opens the full list.
- After going back to an older version, the app only says so.

## 0.7.60

- Stat advice now explains itself with your own character's numbers.
- It uses what the game addon records: your stats, health, mana and crit.
- It also shows what a point of Intellect, Agility or Spirit does for you.

## 0.7.59

- New plans start from where you are, with no pick-ups for quests already in your log.
- A quest you abandoned still gets its pick-up.
- The in-game upgrade list shows one line for many slots of the same green gear.

## 0.7.58

- Updates are clearer: newest first, with plain Update and Go back buttons.
- Settings says when an update is ready, with an Update now button.
- You get a note after each update, and What's new shows only that version's changes.

## 0.7.57

- Removed a leftover keyboard test from Settings.

## 0.7.56

- Stat and gear advice agree: your main stat first, survivability adds Stamina, with the right item names (like 'of the Eagle'). The stat check now leads with your main stat (Intellect for a Shadow Priest), says what comes next, and names the two item suffixes to look for. "More survivability" adds Stamina next to your main stat instead of replacing it, and gear advice follows the same list. When the game addon has recorded what a point of a stat does for you, the advice quotes it. In the overlay's What next, the same green gear for many slots is now one line instead of eight.

## 0.7.55

- If an update ever misbehaves, the app offers to go back, even from the tray icon. The tray menu now has Check for updates, Go back to the previous version (when one is saved) and Open logs folder, and they work even if the window, the advisor or your AI connection is broken. If a new version fails to start properly in its first ten minutes, the next start shows a plain screen that says "This update isn't working on your PC" with Go back, Keep trying and Open logs, and it never comes back for a version you chose to keep trying. Going back also restores the settings you had in that version. Your saved settings and profiles now carry a version number so older versions keep reading them. The addon check never offers an addon version that has been pulled, offers to reinstall the previous one if yours was pulled, and tells you whether the addon needs a /reload or a full game restart.

## 0.7.54

- Updates now show in the chat with one-click install; you can skip a version or go back to an earlier one. A new version is offered a day after it comes out (or right away if you turn on "Get updates right away" in Settings), with What's new, Install, Later and Skip this version. Install downloads the installer, checks it against the published checksum, saves a copy of your settings and replaces the app in place. If the version you are running is later found to have a problem, a red banner offers to go back, and Settings lists the last five versions with an Install this version button for each.

## 0.7.53

- Play style changes reach the game on the next /reload, no need to close Settings. Saving a play style, spec, stat check or focus (in Settings or through your AI client) now rewrites the in-game advice a moment later, so the panel's stat chip and advice are current as soon as you reload.

## 0.7.52

- Plans and answers use what your own game recorded: your recipes, your trainer's prices and your vendors. A recipe in your own recipe list is known, so a plan never tells you to learn it again, and its reagents and product come from your window ahead of the game data (a recipe only your list has is still planned, by the name your game showed). When you have opened your trainer, plans and the next-level spells say "your trainer teaches it" with the cost and level your trainer showed, not an estimate, and a spell your trainer shows as not yet available says what it needs. A vendor you opened sets the price and is named with where it stands (the nearest one to you first), and anything older than 30 days says its prices may have changed. Flight masters you opened are listed as flight points you know. Each of these says when it was seen.

## 0.7.51

- Tailoring plans no longer ask for mountains of cloth. When one recipe would take more than 600 of a cloth you don't gather (Mageweave, Silk, Runecloth), the plan now stops it short at 600 and moves to another recipe, and a recipe that needs a few bars still beats hundreds of extra cloth when you don't mine, because the bars are simply bought. Where no recipe avoids the pile, the plan says which material it still asks too much of. The Tailoring plan's Mageweave Cloth drops from 1,350 to 930, and Blacksmithing is unchanged.

## 0.7.50

- Blacksmithing plans now smelt your own bars and use sensible recipes. If you mine, every bar (Bronze, Iron, Steel, Mithril, Truesilver, Thorium and the rest) is smelted from ore, and the plan says which Mining skill each smelt needs; without Mining the bars are bought at the Auction House and no longer counted as rare. Pieces a recipe needs first, like a Shining Silver Breastplate, are crafted on the way and their skill-ups count. A rare material now costs the same for each unit you need, not once per kind, and gathered materials carry a small per-unit cost for the time spent farming. Items the game data has no name for show as "unnamed item N".

## 0.7.49

- Profession plans prefer recipes trainers are known to sell, and say when to double-check with your trainer. Each stretch of a plan now says whether its recipe is confirmed by trainer data, sold by a vendor or a quest reward, or only probably from your trainer ("check when you visit"), and a confirmed recipe wins over an unconfirmed one at a similar cost. Recipes whose pattern only drops inside a dungeon, like the Gordok Ogre Suit, are no longer treated as trainer recipes. Leatherworking no longer counts tens of thousands of materials by turning light leather into heavier grades. First Aid keeps Minor and Lesser Healing Potion, which the game lists under First Aid.

## 0.7.48

- Profession leveling plans now follow cheaper, common recipes all the way to 300, including vendor patterns, quest recipes and enchanting materials, and skip specialist or cooldown recipes. Where no trainer or vendor recipe covers a stretch of skill, the plan says so and names a pattern or quest to look for instead of stopping. Alchemy plans no longer come up empty, and Leatherworking no longer counts hundreds of thousands of materials.

## 0.7.47

- The app now reads the WoW Companion addon directly, so ForeverScribe is no longer needed, and its advice uses the trainers, vendors and gathering spots you've seen yourself. If you still have ForeverScribe, the app reads it too and, for each character, uses whichever capture is newer. The setup checklist and the doctor say where each character came from, and tell you once that you can remove ForeverScribe once WoW Companion 0.6.N+ is installed. What you see in the game now comes first, marked "seen by you": the trainer you visited, with its cost and the skill it asks for, answers "Who trains that?"; the vendors you opened give their stock and prices; the herbs, ore and fish you gathered give the places to gather and fish; and a quest giver you met fills in a giver the game data lacks.
## 0.7.46

- Tell it how long you can play and get a plan that fits; ask whether a quest is worth it, or what to drop when your log is full. "I've got 30 minutes" (or a quick session, an hour) gives a short loop from your quest route: quests ready to hand in first, then the nearby work that pays the most XP for the time, a trainer visit when you have spells to learn, and a finish at the closest flight point or inn on a short session. It says "about 25 minutes", the XP you should get and how much of your level that is, and it leaves out quests that need a group. Under a What next? answer there are 15 min, 30 min and 1 hour buttons. "Is Wolves at the Door worth it?" and "What should I abandon?" rate each quest in your log by XP for the time and mark it grey (little XP, finish only if it is on the way), far (its work is in another zone), group (elite or a dungeon: bring a group or skip it) or a chain starter (worth keeping); a quest that rewards a gear upgrade counts for more, and when the log is full it names the quests to drop first. Where the game data has no quest XP, the XP is estimated from the quest's level and says so.
## 0.7.45

- Ask what your next level brings: spells to train, what they cost, and the class quests coming up; the NEXT bar tells you when it's time to visit your trainer. "What do I get at level 12?" lists the new spells and ranks for your class (the ones you already know are left out), a plain line on what each does, which to train first for your spec and role, the training cost where the game data has one, and a total to bring ("bring about 1g 20s"); if you are short it says so and points you to the Bags review to sell something. Class quests your class can take now or within five levels show with who gives them and where, and the ones open now join What next?. Costs the game data does not have are shown as unknown, never guessed.

## 0.7.44

- Ask where an item is across all your characters, and press Bags for what to sell, keep, toss or mail to an alt. "Where's my Linen Cloth?" names each character holding it, with how many and whether it is in the bags or the bank, and says when a character's bank has not been seen yet. The new Bags button gives a short list: Sell (with the vendor value), Mail to your alts, Keep and Toss, each with its reason. Prices the game data does not have are shown as unknown, never guessed, and nothing is mailed or sold for you.

## 0.7.43

- Ask how to beat a dungeon, play your class or work a profession and get real tactics, built around the bosses, quests and drops in your game, even if you're not there yet; a Check Icy Veins button refines it from the WoW Forever guide. A dungeon you are too low for gets one line ("You're 10; it opens around 13.") and then the full plan, with its level range, entrance, bosses, quests to bring and the drops for your class. Under the answer a small line says what it is built from, and the button appears when your AI client can read web pages. Your game data is also re-checked each time the advisor starts, so a republished level range reaches you without opening the companion, and map rows that are not dungeons (such as Ruins of Lordaeron) are no longer suggested.

## 0.7.42

- Play Style changes save as you make them, with a Saved tick; Not set clears a choice. Set Role and Focus to Not set and the character's play style is cleared, so the next talent or stat question shows the play-style buttons again; set just one to Not set and only that one is asked for. Spec "Not sure yet" clears the spec. A small "Saved ✓" shows after each change, and if a save is refused, the reason appears under the section and the dropdowns go back to what is really saved. Done still just closes the panel. The AI client can now forget a single choice when you ask it to.

## 0.7.41

- Profession leveling plans now follow when each recipe can really be learned, stop at each trainer rank, and show the ore, cloth or leather behind every bar and bolt. No recipe is planned before the skill it is learned at, and a guessed learn skill is shown as "around skill N". A plan stops at 75, 150 and 225 and names the rank and character level you need to train next. By default only recipes from a trainer or vendor are planned; drop, quest and reputation patterns are used only if you already know them, and up to two are offered as "if you have the pattern". Bars, grinding stones, bolts and cured leather are followed back to smelting or crafting, with the skill-ups the crafts give, and ore, herbs, leather and cloth say where to gather or buy them instead of "rare".

## 0.7.40

- Spec buttons appear whenever no spec is chosen; gear advice includes quest rewards and goes by the level you need to wear it. Set Spec to "Not sure yet" in Settings, press Talents, and you get one button per spec instead of a question in text. The chip under an answer follows talents and a spec you just chose ("Following up on: Cruelty"). Gear advice names a quest reward with its quest, even one you have not picked up, ranks and labels by the level needed to wear it, puts anything above your level under "Soon (level N)" and leaves out items your spec can't use, such as a shield for a Fury warrior. A heads-up line closes only a What next? answer, and talent answers talk about your next pick and the one or two after it, not point totals.

## 0.7.39

- Every profession gets a named trainer near you, even when no one has reported one yet, from your Questie install. Ask "who trains that?" about a Blacksmithing recipe, or any other profession's, and you get a trainer of your faction with their rank (Journeyman, Expert, Artisan), the zone and the coordinates; one whose rank reaches your skill and the recipe comes first. Vendors that only sell supplies are never taken for trainers.

## 0.7.38

- With Questie installed, the companion knows where to mine, pick herbs and find fishing schools. Ask "where can I mine copper?" and you get the zones with the most copper veins, your own zone first, with a coordinate or two; ask "where should I fish at my level?" and you get the zones whose schools suit your Fishing skill. Profession plans use the same zones in their "gather here" lines for the ore, herbs and fish you still need. Places read from your Questie database are marked as coming from it.

## 0.7.37

- Crafting answers name the nearest trainer or vendor for your faction, with the place to find them. Ask "who trains that?" and you get the closest trainer for your faction in your zone first, then your continent, with the zone and coordinates; a place your Questie database knows is marked as coming from it. Recipes sold by a vendor or dropped name that source the same way. Recipes the trainer simply teaches no longer claim "needs skill 1": you're told the trainer will show whether you can learn it yet.

## 0.7.36

- For developers: `wowc-advisor profession-plan --profession NAME` prints a whole skill-up path (default 1 to the level's cap) for a fresh character with empty bags, with the recipe and crafts per band, material totals and vendor cost, recipe items to find, trainer costs per tier and an efficiency score; `--json` for tools. It needs only a dataset directory or URL, no game install.

## 0.7.35

- Follow-ups remember what the answer was about; choosing a spec is one tap and shows in Settings; answers no longer flash working notes or talk about their sources. The "Following up on" chip now names the quest, item or person the answer led with, not a side note. Ask about talents with no spec saved and you get one button per spec, with no AI call; a spec you pick, or one your AI client saves while chatting, appears in Settings → Play Style straight away. While an answer is on its way you see a plain line like "Checking your gear…" instead of the AI's working notes. Answers lead with the point and no longer mention data or sources, and gear you can't use until a higher level is listed after the main list under "Soon".

## 0.7.34

- Ask how to level any profession: a step-by-step path from your skill, with what to gather, fish or buy first. Each step says what to make and how many, which materials you already hold, where to get the rest, how to learn the recipe and which trainer to visit; asking "which professions should I pick?" suggests pairings for your class and play style.

## 0.7.33

- With Questie installed, dungeon advice knows each dungeon's entrance, what lives inside, what drops for you and which quests to bring. It reads your own Questie files on your PC and says so; without Questie nothing changes.

## 0.7.32

- Dungeon advice is clearer: when you're ready, where they are, which quests to grab first, and what drops for you. Too low for any dungeon? It names the first one, about which levels it opens at, and where its entrance is. In range, it lists up to three dungeons with their entrance zones, the quests to pick up first (and who gives them), and the boss drops that are upgrades for your class and spec. Dungeons from the other faction are left out.

## 0.7.31

- The quick buttons ask broader questions: Gear now reviews all the upgrades within reach (a short ranked list of at most five, each with where to get it), Talents asks which point to take next and why, and What next? asks what to do next and why. Specific questions stay on the cards.

## 0.7.30

- Follow-up questions stay cheap: the companion remembers what you're talking about (the quest, NPC or item, and your last question) instead of replaying whole earlier answers, and shows it as "Following up on: …" above the ask box. Click the × or Clear to start fresh; it also lapses after 30 minutes or when you switch character. Finished answers now scroll fully into view, including the usage line; if you had scrolled up to read, a "↓ New answer" button appears instead of moving you.

## 0.7.29

- Crafting advice now checks your profession skill and, when you're close, tells you what to craft to get there. It says whether you can make an item now, how many points short you are (with up to three recipes to level on, how many crafts each needs and the materials you're missing), or, if you lack the profession or are far short, points to the Auction House (when the item can be traded) or a crafter. It also says where the recipe comes from: a vendor and price, a drop, a quest or a trainer. Quest pick-ups in your plan now name the giver players have seen when the game data lacks one.

## 0.7.28

- Advice now names vendors who sell what you need, and knows more WoW Forever quest givers, from Everloot's player-reported data. Upgrade and crafting answers say "Buy from <vendor> in <zone> for <price>" (and "limited stock" when it is), and a cheap vendor offer is listed beside the Auction House ahead of farming. Quest answers fill in the level, giver, faction and category players have seen, marked "seen by players on Everloot (CC BY-SA)".

## 0.7.27

- Use your own AI key (Claude, OpenAI, OpenRouter) or a free model on your PC (Ollama) — Settings → AI. Choose how the companion answers: an AI app on this PC, an AI account with a key, or a model on this PC. Paste a key once (it is saved privately and never shown again), pick Fast and cheap, Balanced or Best, and press Test to see that it works. Ollama and LM Studio are found automatically. Under Advanced you can pick a backup to use if the first one fails.

## 0.7.26

- You can now ask through an AI provider with your own API key, not only through a command-line AI client. Anthropic (Claude), OpenAI and OpenRouter work with a key you save; Ollama and LM Studio run on your own PC and need no key, and any other OpenAI-compatible server works with its address. The AI uses the same advisor tools and the same answer settings as before, and the usage line shows tokens including cached ones. If a model can't use tools, the app says so instead of guessing.
- Keys are stored in Windows Credential Manager (a private file in your data folder on macOS and Linux), never in a settings file or a log.
- You can list several AI clients in order; if the first can't start, the next one answers. Setups that only use a command-line client work exactly as before.
- New page: [AI providers and API keys](docs/providers.md).

## 0.7.25

- The AI can now follow WoW Forever's own quest lines using your Questie database. Ask "What does The Skybreaker Bulwark lead to?" and it lists the quests that come before and after it, with who gives and takes in each one and what to do, instead of saying the quest has no data. When no follow-up is listed anywhere, it now says so plainly.

## 0.7.24

- Plan steps no longer double up verbs like "Collect Obtain". If the quest's own objective already starts with a verb (Obtain, Listen, Investigate and many more), the step uses it as written.

## 0.7.23

- You can type and edit in the ask box again: typing, Backspace, Delete, the arrow keys, Home, End, Ctrl+A and Ctrl+V all work. Enter sends and Shift+Enter adds a line.
- The Keyboard test now also tells you when a key was cancelled after it arrived ("cancelled later").

## 0.7.22

- Typing and editing keys work in the ask box again. The overlay no longer makes any native focus call when you click the box, so it takes keys exactly like the Keyboard test field.
- The Keyboard test now also shows the app's focus line after a key in the ask box, and logs `ask: focus` and `ask: blur`.

## 0.7.21

- Backspace, Delete, the arrow keys, Home and End now work in the overlay's ask box. The overlay no longer pulls keyboard focus away from the box when you click it, and if a key still gets lost, the box applies it itself.
- In Settings → Shortcuts, Backspace or Delete clears a shortcut box back to its default. Tab moves on, and Escape leaves the box unchanged.
- The Keyboard test now also watches the ask box: its lines start with `test:` or `ask:`, and say "no change" when an editing key did nothing.

## 0.7.20

- The overlay's ask box now takes the arrow keys, Backspace, Delete, Ctrl+V and the menu's Paste. The overlay now takes keyboard focus as soon as you click the box, turning click-through off puts the window back exactly as it was, and a shortcut you set for show/hide or click-through can no longer be an editing key or a bare Ctrl+V or Ctrl+C.
- Settings → Updates has a new Keyboard test field. Click it and press a key: it lists the last 8 key events the overlay gets and a line from the app on focus and window state, so if a key still does nothing you can see where it stops.

## 0.7.19

- A question copied in game now pastes into the overlay's ask box. The game puts plain text on the clipboard that Notepad understands but the overlay's own paste can miss, so the overlay now reads the clipboard directly whenever Ctrl+V or Shift+Insert pastes nothing. It pastes once, at the cursor, replacing any selected text.
- A small paste button sits inside the right end of the ask box (hover for "Paste from clipboard") and inserts the clipboard text at the cursor.

## 0.7.18

- The advisor now notices when the game data is republished for the same game build, for example when new talent builds are added. It checks when it starts and then once a day. If the data changed, it downloads the new copy, checks it and swaps it in. It leaves your current copy alone if you are offline or the download fails.
- When new data installs, the overlay shows a dim line, "Game data updated (talent builds and more)", and your in-game advice is rewritten from it. The "Game data" row in the setup checklist shows the date of the data you have.

## 0.7.17

- Talent advice now comes from the advisor's own curated builds, with no web lookup. Ask which talent to take next and the answer is "Take <talent> (<rank>): <why>", then at most two short bullets: the next picks in your level band, and why they fit your play style. It uses your chosen spec, or the spec that matches your role (damage, tank or healer) when none is set.
- The guide web address is gone from the play style questions, the settings and the help text, and the advisor never fetches a guide for talents. A guide address saved earlier is quietly ignored.
- For a class or spec with no curated build yet, the answer says so in one line and lists the talents you can take right now as options, not a recommendation.

## 0.7.16

- Each upgrade's first suggested question now follows the way we suggest getting it. A craft asks "How do I make Gemmed Copper Gauntlets?"; a quest asks how to get the item from that quest; an Auction House item or shopping list asks how to find it there; a dungeon drop asks how to get it from the boss; a creature drop asks where to farm it. The earlier follow-up questions come after it, three at most.
- The answer to that first question gives the steps rather than repeating the card: for a craft, where the trainer is if you don't know the recipe, the materials with what you have and still need and where to get the rest, and where to craft it. One answer line, then at most three steps.

## 0.7.15

- Suggested questions now follow what you clicked. Each one names the thing itself (the item, the quest, the NPC, the talent or the ability) and asks the natural next thing: whether a different suffix would do as well, whether a craft is worth making now or the materials should be saved, what a quest leads to, or why a talent suits your play style. They no longer ask what the card already shows, so "What should I search for on the Auction House?" is gone.
- A question pasted into the overlay on its own still makes sense, because it carries its subject in its own words.
- Answers to those questions skip what the card already said and cover only the new part.

## 0.7.14

- Cleaner upgrade cards. Each fact now shows once: the item's name, then its slot, when to wear it, how big the gain is, and the item it replaces (just its name). The same words no longer repeat in the title, the badges and the details, and a shopping entry no longer starts with "Cloak: Cloak:".
- Weapons and off-hands you can really use. You are only offered an off-hand weapon once you have learned Dual Wield (a level 10 warrior is not), a shield is only replaced by a better shield, and a two-hander is only suggested over a one-hand weapon and shield for a damage play style where it beats both together. The slot label always matches the item: One-hand, Main hand, Two-hand, Off hand, Shield or Held in off hand.
- Plainer crafting lines. "Learn it from a Blacksmithing trainer, then make it; have 2/4 Copper Bar", or "You can make this (Blacksmithing)" when you already know the recipe. The "recipe needs skill 1, you have 35" wording is gone and the line is no longer cut off.

## 0.7.13

- A suggestion now always gets a useful answer. Click a suggested question about a class quest and the companion starts from what your Next list already knows (who gives it, where, its level, whether it is a class quest and why it is listed) instead of searching by name and saying it found no match. When little is known it says so plainly and never makes up a reward.
- Spec is now a dropdown of your class's specs, such as Arms, Fury and Protection, plus "Not sure yet". A spec you typed before is kept when it matches one of them. Stat preference starts on Balanced.
- Updates now shows what's new: the last five versions in plain words, newest first, each one collapsible, with a link to the full list on GitHub. It works without a network connection.
- Questions are asked in the overlay only. The companion no longer watches for questions asked in game, the "You asked in game" card is gone, and Answers.lua is no longer written. In game, the question buttons copy the question so you can paste it into the overlay; anything already on disk is left alone.
- Tidier in-game advice. An Auction House upgrade now reads as one short line, such as "Cheap on the Auction House: search 'of Strength', level 7–10", with the full search steps one click away in its details. Nothing in the advice is cut off mid-sentence any more, and the old long-question field is gone.

## 0.7.12

- New stat check. Once you have chosen a play style, the companion reads your stats in plain words: what to raise and why it helps how you play ("Fury wants more Strength: more attack power, so every hit lands harder"), what is wasted ("2 items give Spirit, which a warrior barely uses: swap them first"), and what is already fine. At low levels it reminds you that quest gear matters more than fine-tuning. Ask about your stats, or "what should I look for?", and the new stat_check tool answers; with no play style it offers the choices first.
- Play style gains an optional Stat preference (Balanced, More damage, More survivability) in Settings.
- Upgrades and the Auction House list now follow the same stat priorities, so the advice agrees everywhere: a Fury warrior sees "of the Tiger" ahead of "of the Bear", a Protection warrior the reverse, and an upgrade can say "Adds the Strength you're short on". The in-game digest gains a short stats entry.
- The priorities are ballpark guidance, not a rule: players differ.

## 0.7.11

- Cheap world-drop gear is now an Auction House shopping list. When the best way to upgrade a slot is the Auction House, or no quest or craft helps, the upgrade reads like "Chest: any green mail chest, level 8-10, 'of the Bear' - cheap on the Auction House", with a few example base items and the stat suffixes that fit your class and play style ("of the Bear (Strength + Stamina)"). Ask "What should I search for on the Auction House?" and the companion gives the in-game search in plain steps: what to type, the level range, the armor type or slot filter.
- Any random world drop you can trade, and any Bind on Equip item, now counts as an Auction House way: "Common world drop. Usually cheap on the Auction House." It ranks after a quest in your log and a craft you can make now, and before quests you have not picked up, dungeons, crafts missing materials and named creatures.
- No Auction House prices are shown or used any more; the data behind them was too thin. Stat suffixes are listed only when the game data confirms them.

## 0.7.10

- Upgrades are now only gear you can actually get. Each one comes with its best way, in plain words: finish a quest and choose it, make it with your own profession (with the materials you have and still need), buy it on the Auction House, or kill a dungeon boss or a named creature that drops it often. Random world drops are left out.
- Each upgrade reads like a sentence: the slot ("Cloak", "Chest", "Wrists", "Two-hand weapon"), the item, "Wear now" or "From level 12", "Big upgrade", "Solid upgrade" or "Small upgrade", and what it replaces ("instead of your ..." or "fills an empty slot"). The "ilvl" and doubled slot text are gone, and the list's summary reads "N upgrades you can get".
- A quest that rewards an upgrade says so ("Rewards Traveler's Shawl, a big cloak upgrade"), and a material you need for a craftable upgrade shows "for <item>" in Keep materials.
- Ask "How do I get <item>?" or "Which upgrade first?" and the companion now looks the answer up with the new upgrade_paths tool instead of guessing; an item with no dependable way is called a random drop, not worth chasing, and the best gettable item for that slot is named instead.
- Auction House prices are shown only when the game data holds a real auction price, with the date it was seen; a vendor sell price is never shown as one.

## 0.7.9

- Settings is now a full-height page. Click the gear and every section shows at once with a single scroll bar for the whole page (and only when it is taller than the window), instead of a small box scrolling inside the window. Open a section such as Play style and the page grows; the version line sits at the bottom.
- The gear lights up in gold while settings are open, its tooltip reads "Close settings", and it shows a clear keyboard focus ring. Click the gear again, press Esc, or use the new Done button to close; your chat is exactly where you left it.

## 0.7.8

- The overlay picks up where you left off. It opens on your last chat for the character (the last 50 turns, kept on your PC only), with a dim line marking the gap, such as "Yesterday, 11:58 PM", and sends nothing by itself. Clear empties that character's chat.
- An in-game question that is more than 12 hours old is no longer answered on launch. It shows as one small card, "You asked in game last night: ...", with "Answer now" and "Dismiss". Newer in-game questions are answered as before.
- Calmer by default: the settings stay behind the gear button (and the version line lives there), the Next list shows one entry until opened, and at most one row of up to three suggestions appears when something is worth doing, such as choosing a play style, turning in a ready quest or updating an old plan. Hover a suggestion to see why; its x hides it for today.
- A slim quick bar above the ask box: What next?, Talents, Gear and Refresh. Refresh rebuilds your plan from your latest save, asks no AI, and answers in one sentence. Settings, Shortcuts can hide the bar.
- Each Next entry shows a short reason under its title. The plan list reads as plain steps with the coordinates dim at the end, and says "23 stops". The red "17h" pill is now a small dot that turns amber after 12 hours and red after 3 days; hover it for the last save time.
- Behind this: every entry carries a short "why", plan steps are plain sentences, the advisor ranks at most three suggested actions, a plan refresh endpoint rebuilds the plan from the newest save, and a ledger remembers answered in-game questions so none is answered twice.

## 0.7.7

- Ask without pasting. The overlay's Next section now lists your character's current entries (quests, talents, training, upgrades, notes, dungeons) one line each, in the same order as the in-game panel. Click an entry to see its questions as buttons, such as "Where should my talent point go?"; click one and just that question appears in the chat with a short answer. Nothing needs copying from the game.
- The ask box grows as you type or paste, up to six lines, then scrolls, and shrinks back after you send. Enter sends; Shift+Enter starts a new line.
- A talent question now asks how you like to play first: role and activity buttons (and an optional spec) appear right in the chat. Save stores them, the same as the Play style settings, and your question is answered automatically.
- Questions asked in the game show as your short question with a small "from game" mark. The overlay no longer tells you to `/reload`.
- Behind this: every in-game entry carries 1 to 3 natural questions and a hidden reference (digest schema 1.5), so a typed question that matches one is answered with that entry's facts; answers are shorter, and the advisor says honestly when a quest giver cannot be found.

## 0.7.6

- The prompts and notes on the in-game cards now read as plain sentences: correct plurals ("1 unspent talent point"), your saved play style and talent plan named in the question, a separate question for picking up, working on and turning in a quest, upgrade sources in words (a drop from a creature, a quest reward, crafted by a profession) and never an id. Card notes are short player-facing lines (who and where, what is left, what it unlocks, how the level fits) with no internal labels or cut-off text.

## 0.7.5

- The route plan now knows what turning a quest in unlocks. Follow-up quests (read from your own Questie, the dataset's quest lines, and unlocks you have seen in game) are routed after the turn-in that makes them available, marked locked, at most two links ahead and within a 25-step total. A turn-in's entry says "Unlocks: <quest> (from <NPC>, here)"; the overlay's Plan list shows locked steps dimmed with "after step N". `Plan.lua` schema 1.1 adds `locked`, `unlockedBy`, `depth`, `chain` and `unlocks`; older addons ignore them.
- The advisor reads the addon's bounded log of quests accepted, turned in and abandoned (`WoWCompanionDB.observed`), so a quest you pick up outside the plan is routed on the next refresh without a `/reload`. Unlocks it sees are kept in a small file on your PC only, and never sent anywhere.

## 0.7.4

- The in-game digest gains class-training entries: when your level unlocks trainer-taught abilities or ranks, `next_quests` gets a `training` entry listing them (new, or an upgrade of a rank), with the nearest class trainer only when your capture names one. Every next-quest, upgrade, dungeon, talent and training entry now carries 1 to 4 `details` lines and a ready, context-rich `prompt` (at most 500 characters) built from your character, play style and talent plan. Digest schema 1.4.
- A question you queue in game now appears in the overlay's Ask feed as soon as it is saved, marked as from the game, and its answer streams in there. An optional "Question from game" toast is off by default.

## 0.7.3

- The overlay's Next list now lays out each quest from the digest's structured fields, like the in-game panel: `[level] Title` coloured by difficulty with (Class) and (Elite) tags, objective rows with counts, "Ready to turn in" / "Turn in to NPC" or "Pick up from NPC", and "N unspent talent points". Nothing is cut off with "..."; long lines wrap. An older advisor's plain text still shows.

## 0.7.2

- The in-game digest lines now carry structured, untruncated fields (kind, full quest title, level, class and elite flags, NPC, zone, every objective with its count, unspent talent points, and an upgrade's slot, item level and level needed), so the addon can lay Next out like the quest tracker. The existing `text` is unchanged for older addons. Digest schema 1.3.

## 0.7.1

- Fixed a false "Advice may be out of date" warning in the addon after a /reload: `Digest.lua` and `Plan.lua` are now rewritten with a fresh time whenever ForeverScribe has saved a newer capture, even when the advice is unchanged.

## 0.7.0

- Class quests and unspent talent points now lead the advice: "Next", the digest and answers put class quests to pick up (read from your own Questie) and "You have N unspent talent points" (including profession talent points) ahead of routine quests. The route plan always includes class quests.
- New talent advisor: ask "What talents should I get?" and it asks about your play style, then builds a talent plan for your spec. Talent plans read the guide you choose (Icy Veins by default) at the time you ask, and nothing from it is stored in WoW Companion.
- The advisor remembers each character's talent choices and play style.
- The in-game `Plan.lua` and `Digest.lua` are now written as soon as the overlay starts, and missing data files are stubbed, so the addon no longer shows an "error loading Plan.lua" warning at login.
- Fixed quest route plans: a quest is never routed to its turn-in before its objectives are done, and item and kill-credit objectives are now resolved so they appear in the route.

## 0.6.0

- New quest route plan: pick-ups, objectives and turn-ins in an efficient order with map coordinates, computed by a distance optimiser from your own Questie. It shows as a numbered Plan under Next in the overlay, is available to the AI as `quest_plan`, and is written to `Plan.lua` beside the in-game digest. The map pins for the plan need WoW Companion addon 0.3.0; update the addon from the setup checklist.

## 0.5.1

- Fixed the upgrade check failing with a tool error in a question's answer.
- Quest progress now prints once per objective, and a Next line shows one objective plus "+N more".
- The setup checklist now reads Questie's version from its per-game TOC file ("Found Questie v12.11.1 (QuestieDB v1.1.3)").
- Quests not in your log are marked "Pick up: ..." in answers and the Next list; a failed tool is now named in the answer.

## 0.5.0

- Answers now use your saved quest progress, such as "3/8" on an objective, so the advisor knows what you still have left to do.
- The advisor can point you to nearby quest givers you have already met, with their positions.
- Optional: if you have Questie and QuestieDB installed, the advisor reads them on your own PC to suggest quests you have not picked up (with the giver's position) and to say where a logged quest's objective or turn-in is. Nothing from Questie is copied or shared. The setup checklist has a Questie row.

## 0.4.0

- Antigravity (`agy`) is now supported as an AI client. Gemini CLI still works but is now legacy.
- The Getting Started guide now has setup steps for each AI client.
- The setup checklist has a "Game data" row, and clearer messages appear when there is no game data yet.
- Settings are easier to use: links are readable, checkboxes line up, AI clients show friendly names, related settings are grouped, and the settings page scrolls.
- Each question now sends far fewer tokens: Claude runs with only the advisor's tools (no built-in tools, user settings or skills), and the character's facts are put in the prompt so most questions need no tool call.
- The usage line shows fresh input, output and cached tokens separately, with a tooltip on cached tokens.
- The advisor now suggests quests you have not picked up yet.

## 0.3.0

- The overlay now writes the in-game digest for you, so there is no separate step to run.
- Overlay-first answering: questions are answered in the overlay first.
- A setup checklist shows what is left to connect, and a Getting Started guide walks you through it.
- Fixed the launch of Codex as your AI client.
- The overlay shows its version, checks for updates on start, and upgrades in place.
  This one install may ask you to uninstall 0.2.0 first; later upgrades will not.
