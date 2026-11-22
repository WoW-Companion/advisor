# Hosted, knowledge-only mode

`wowc-advisor serve --http --knowledge-only --dataset-url <base URL>` serves game-data questions over
streamable HTTP at `/mcp`, reading the dataset from any static file host with HTTP range requests. No
database server, no character files, no capture.

```
wowc-advisor serve --http --knowledge-only --dataset-url https://data.example.com/wowc/1.60.1.71037 --host 0.0.0.0 --port 8000
```

## What the host must serve

The base URL must serve, with `Range` and `HEAD` support (object storage and CDNs do):

- `manifest.json`, as the data pipeline writes it;
- `<table>.parquet` for every table the manifest lists, at the same level. The server needs `items`,
  `quests`, `npcs`, `loot_sources`, `instances`, `instance_bosses`, `recipes`, `recipe_reagents` and
  `quest_lines`, and refuses to start without them.

At startup the server fetches and checks the manifest, reads the footer and one column of every
required table (which proves range support and that each file is Parquet). It does **not** re-hash the tables: that would download all of them, which is what range reads
avoid. Serve the dataset over HTTPS from a bucket only you can write.

## Tools

| Tool | Answers |
| --- | --- |
| `server_info` | version, dataset URL (credentials stripped), game build, limits |
| `item_lookup` | one item by `id` or `name` (a name matching several items lists them with ids) |
| `quest_lookup` | one quest by `id`, with its quest line |
| `npc_lookup` | a creature by `id` or its `name` (exact, then the start of it) |
| `where_drops` | an item's sources by `id` or `name`, best drop rate first, with the creature's zone |
| `dungeon_info` | a dungeon or raid by `map_id` or its `name` (exact, then the start of it), with its bosses |
| `recipe_lookup` | a recipe by `spell_id` or its `name` (exact, then the start of it), with reagents |

The character tools (`list_characters`, `character_brief`, `whats_next`, `upgrade_finder`,
`profession_plan`, `talent_plan`, `dungeon_plan`, `farm_spots`, `quest_chain`, `quest_plan`, `inventory_review`, `update_digest`, `doctor`) are not
listed. Calling one returns an error saying it is "not available in knowledge-only mode". The mode
refuses `--dataset`, `--savedvariables` and the stdio transport, and never reads `$WOWC_DATASET` or
`$WOWC_SAVEDVARIABLES`.

## Name index

Item, creature and recipe names are found through the dataset's `names.parquet` (columns `norm`, `name`,
`table`, `id`, sorted by `norm` with a Bloom filter on it): the server normalises the name (lower case,
accents folded, apostrophes dropped), reads only the row groups whose `norm` range and Bloom filter
admit it, and matches exactly, else by prefix. Nothing is built at startup. A name in the middle of a
longer one (`bandage` for `Heavy Linen Bandage`) is not found. A dataset published without
`names.parquet` is searched by scanning the table's `name` column instead, matching any part of the
name, and the server logs that it did.

## Limits

| Flag | Default | Bounds |
| --- | --- | --- |
| `--timeout` | 10s | each request to the dataset host, headers and body |
| `--max-response` | 8388608 | bytes of one response body; a larger one fails the request |
| `--max-concurrent` | 8 | requests to the dataset host in flight at once; the rest wait |

A lookup that hits a limit returns a tool error ("Lookup failed: ...") and the server carries on. Lists
are cut to 10 candidates, 15 drop sources and 20 related rows.
These limits protect the dataset host and the process; they are not a rate limit on callers. Put the
server behind a proxy that rate-limits per client.

## Cost in requests

Measured by `TestRequestsPerLookup` against an `httptest` server with 20 ms injected latency per request
and a fixture of 3,000 items and one row group per table. Each lookup is a sequence of range reads (a
`HEAD` for a table's size, then reads of its footer and index, then the page holding the id and the row),
and the open files are kept, so the cost after the first lookup of a table is what is shown.

| Step | Requests | Time at 20 ms |
| --- | --- | --- |
| startup, whole fixture | 65 | about 1.4 s |
| `item_lookup` by id | 8 | about 0.18 s |
| `item_lookup` by name (index) | 18 | about 0.4 s |
| `where_drops` by name (3 sources) | 22 | about 0.48 s |
| `dungeon_info` by name | 8 | about 0.17 s |
| `npc_lookup` by name (index, then the row) | 12 | about 0.26 s |

Requests are sequential per lookup. Real tables have more row groups and pages, so expect more
requests, and a `where_drops` over a common item more again; re-run the test with `-v` against a
larger fixture to measure. The footer and page-index reads are repeated for a table's first lookup only.
Put a CDN in front so repeated range reads are served from cache.

## What deploying needs from the Operator

This repository holds no deploy files. To run it somewhere an Operator must provide:

- an account with a host for the dataset files (an object-storage bucket) and the dataset published
  there: `manifest.json` and the Parquet tables, public read, range requests enabled;
- an account with somewhere to run the server process (any host that runs a Go binary and exposes
  port 8000, behind HTTPS);
- the spend for storage, egress and the compute, and a decision on rate limiting and access.
