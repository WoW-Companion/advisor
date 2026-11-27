# Product principles

## What we are

WoW Companion is a gameplay logistics app. It plans one route for a WoW Forever character: a numbered
list of stops and the legs between them, over everything the player has opted into. The goal is to do
more with each trip and waste less of the player's time.

## Commitments (opt-in)

The route covers only what the player has chosen:

- quests in the log;
- profession goals;
- class training;
- gear upgrades;
- restocking consumables (food, ammo, poisons and their reagents);
- bank and mail moves between their characters;
- repairs and selling junk;
- seasonal events, only when opted in.

Hidden items stay hidden.

## Costs we minimise, constraints we respect

- **Costs:** travel time (flights, boats, zeppelins, the hearthstone and its cooldown, mounts), dead
  time, bag pressure and gold.
- **Constraints:** level reach, elite or group needs, faction, known flight paths and reputation.

## Principles

1. **One numbered route**, shared by the map and the panel.
2. **Live re-order** from where the player stands.
3. **"While you're here":** bundle errands into stops the route already visits.
4. **Every row starts with a verb, and every quest has one home** (one stop).
5. **Never guess:** look it up, or say what data is missing.
6. **Read-only in game:** we advise; the player acts.
7. **The player decides:** hide, reach and opt-ins are theirs to set.
8. **Compact, calm UI.**

## Measures of success

- fewer back-trips;
- ready turn-ins never forgotten;
- no wasted flights;
- fewer "now what?" moments.

## Using this document

Missions and prompts cite it. A change that adds a feature should say which commitment it serves and
which cost it cuts. The AI answer prompt carries a short form of it (`LogisticsFraming` in
`internal/ask/ask.go`); keep the two in step.
