# rwa-data-pipeline

Collects tokenized assets (on-chain tokens of stocks/ETFs) from CoinMarketCap, finds their DeFi usage in Portals,
computes ratings for tokenized assets and issuers, and publishes one JSON file for `../pi-rwa-interface`.
The same data can be written as flat CSV/XLSX tables for manual analysis.

```
config/assets.json ──► sync ──► data/snapshots/<ISIN>.json ──► build ──► pi-rwa-interface/public/data/rwa.json
                       (CMC + Portals)   (raw collected data)    (no network)  └► export ──► data/exports/rwa.xlsx, csv/*.csv
```

Full flow with commands, from collection to the site build: [docs/workflow.md](../docs/workflow.md).

## Setup

```bash
brew install uv            # once; uv installs Python 3.12+ itself
cd rwa-data-pipeline
uv sync
cp .env.example .env       # put CMC_API_KEY and PORTALS_API_KEY (without "Bearer ") into .env
```

Keys are read only from `.env` / environment and are never written to raw responses, snapshots or the export.

## Commands

```bash
uv run rwa-pipeline sync                          # add: fetch only underlyings without a snapshot, then build
uv run rwa-pipeline sync --symbols AAPL,MSFT      # add, limited to these tickers
uv run rwa-pipeline sync --refresh                # update: re-fetch all enabled underlyings
uv run rwa-pipeline sync --refresh --symbols NVDA # update one underlying (also finds new tokenized assets/networks)
uv run rwa-pipeline sync --refresh -v             # --verbose: a line per HTTP request on top of the step/underlying progress
uv run rwa-pipeline build                         # recompute ratings from saved snapshots and re-export, no network
uv run rwa-pipeline logos                         # download missing token/issuer/underlying logos, then build (sync does it too)
uv run rwa-pipeline export                        # flat tables for manual analysis: data/exports/rwa.xlsx (no network)
uv run rwa-pipeline export --format csv           # one CSV per table into data/exports/csv/
uv run rwa-pipeline export -f xlsx --out ~/Desktop/rwa.xlsx   # xlsx: --out is the file; csv: --out is the directory
uv run rwa-pipeline build --format all            # sync/build also take --format csv|xlsx|all to write tables after the JSON
uv run pytest                                     # tests
```

Options: `--data-dir` (default `data/`), `--frontend-dir` (default `../pi-rwa-interface`), `--format/-f` (`csv`, `xlsx`,
`all`; `sync` and `build` write tables only when it is given, `export` defaults to `xlsx`).
Exit codes: `0` success or nothing to add, `2` some underlyings failed (their previous snapshots are kept), `1` fatal
(auth/config error, CMC unavailable).

- **add** (default) never touches existing snapshots. A repeated add with nothing new prints `nothing_to_add`, makes no
  requests and no new export. An underlying whose CMC roster is empty is saved as `no_tracked_tokens` and is not retried.
- **refresh** replaces the snapshot of each selected underlying only after all its requests succeeded; a failed
  Portals/CMC call keeps the last good snapshot. Discovering a new issuer or network of a known ticker = refresh.
- **build** is where all rules live, so changing `config/settings.json` (thresholds, categories, chain mapping) only needs
  `build`, not new requests.

## Adding an asset

Add an entry to `config/assets.json` and run `uv run rwa-pipeline sync`:

```json
{"ticker": "NFLX", "isin": "US64110L1061", "name": "Netflix", "asset_type": "stock", "enabled": true}
```

The ISIN (format and check digit are validated) identifies the underlying; CMC is queried by ticker and the resolved
`rwa_id` is checked on later refreshes. `enabled: false` removes the asset from the published basket but keeps its
snapshot. Being in the config does not guarantee CMC tracks tokenized assets of it.

## Data

```
data/
  snapshots/<ISIN>.json        one per underlying: CMC roster + contract metadata + Portals exact lookups and search results
  issuers.json                 CMC issuer metadata cache (name, website, roster)
  rwa_info.json                CMC RWA metadata cache per rwa_id (official website, logo)
  portals_platforms.json       Portals platform slug -> name
  logos/tokens/<cmc_id>.<ext>  token logos (CMC)
  logos/issuers/<id>.<ext>     issuer logos
  logos/underlyings/<ISIN>.<ext> underlying logos
                               (source order: CMC logo -> Google favicon service for the website listed in CMC
                                -> icon declared by that website)
  runs/<run-id>.json           run report: selected/skipped/failed, request counts, CMC credits
  raw/<run-id>/{cmc,portals}/  raw responses (URL, params, status, body; no headers) — debug only, git-ignored
  exports/rwa.json             copy of the published export
  exports/rwa.xlsx             `export`: one workbook, one sheet per table (README sheet lists them)
  exports/csv/<table>.csv      `export --format csv`: the same tables as CSV (UTF-8, header row)
```

Logos are downloaded once and published to `pi-rwa-interface/public/logos/` by `build`; the site loads only local files.
An existing file is never overwritten, so to set a logo by hand (e.g. an issuer or an ETF without logo and website in
CMC) put it at `data/logos/issuers/<issuer_id>.png` or `data/logos/underlyings/<ISIN>.png` and run `build`.

The export (`public/data/rwa.json`) contains `summary`, `settings`, `underlyings`, `tokens`, `issuers`, `opportunities`.
Money/APY/liquidity are decimal strings, `null` means "not reported" (never zero). It is written atomically as one
file, so the site never mixes data of two builds.

## Tables for manual analysis (`export`)

`uv run rwa-pipeline export` flattens the export into eight tables; they hold everything the site shows, including the
figures the site computes on the fly (average APY, tokenized assets "in DeFi", protocol diversity):

| table           | one row per                              | notable columns                                                                      |
|-----------------|------------------------------------------|--------------------------------------------------------------------------------------|
| `summary`       | metric                                   | basket totals, settings, `generated_at`                                              |
| `underlyings`   | underlying stock/ETF                     | tokenized market cap/volume, versions, issuers, `tokens_in_defi_count`, eligible/raw counts, yield (max/average/median APY), `eligible_platforms` |
| `tokens`        | tokenized asset (CMC token)              | `kind`/`excluded_reason`, issuer, price/market cap/volume, `usefulness_score`, `rank_in_underlying`, counts, deployments, eligible networks/platforms/categories, yield profile, `cmc_url` |
| `issuers`       | issuer                                   | score, relative basket score, `tokens_in_defi_count` and share, protocols, yield, pools liquidity, basket market cap |
| `opportunities` | DeFi position (Portals)                  | protocol, network, category, liquidity, APY (total/base/reward/borrow), volumes, eligibility, flags, `linked_tokens` |
| `relations`     | token × position link                    | `link` = direct / indirect (via LP → vault, with `path`) / unconfirmed, `status`, plus the position's columns |
| `deployments`   | token contract on a network              | CMC network, Portals slug, address, `status` (indexed / not_indexed / unsupported …)  |
| `breakdowns`    | entity × dimension × key                 | raw/eligible counts of tokens and issuers by network, category, platform, underlying |

Values follow the JSON: USD, percent and counts are numbers, an empty cell is "not reported" (never zero), times are
UTC. Lists are joined with `; `. In XLSX the header row is frozen with an autofilter, and dates are real Excel dates;
CSV keeps decimal strings and ISO timestamps as text. `export` reads the saved snapshots (like `build`) and does not
touch the frontend JSON; `build --format xlsx` / `sync --format xlsx` write both.

## How the numbers are computed

1. **Tokenized assets** — every token in CMC `quotes/latest` for the ticker. `(Derivatives)` entries, tokens without
   symbol and tokens whose symbol does not contain the ticker (e.g. `GOOGon` listed under GOOGL) are kept but excluded
   from ratings. Names starting with "Wrapped" are marked as wrappers.
2. **Deployments** — all `contract_address[]` from CMC `cryptocurrency/info`, one per network+address. CMC network
   names map to Portals slugs via `chain_mapping` in settings: `null` = known but not in Portals
   (`unsupported_network`), missing = `unmapped_network`. Portals exact lookup marks each supported deployment
   `indexed` or `not_indexed`.
3. **Positions** — Portals `/v2/tokens?search=` over the ticker and the symbols of its tokenized assets (substring
   search, so redundant terms are dropped), all pages. A position is **confirmed** only if its `tokens[]` contains the
   address of a tokenized asset (directly, or through nested positions: root → LP → vault, up to `max_graph_depth`).
   Name matches without an address link (e.g. Morpho markets that list only the loan asset) are stored as
   *unconfirmed* and never counted. Tracked tokens themselves (root, other tokenized assets, wrappers tracked by CMC)
   are not positions.
4. **Category** — `platform_categories` override, otherwise the first matching Portals tag from `tag_categories`
   (`#lending` → lending supply, `#yield-aggregator` → vault, `#clp`/`#pool` → DEX LP, ...). `wrapper`/`unknown` are
   never eligible.
5. **Eligible** — known use-case + source `updatedAt` not older than `max_position_age_hours` at collection time +
   liquidity ≥ `min_eligible_liquidity_usd`.
6. **Tokenized asset score** = eligible positions; ties: protocols, networks, direct positions, CMC id. Raw count and
   coverage are shown next to it. **Yield** is separate: max/average/median reported APY over eligible positions with
   positive APY (zero and missing APY never enter the average); APY above `yield_apy_review_threshold_percent` is
   flagged and excluded from yield stats but kept. Underlyings, issuers and the summary carry the same yield fields
   over the union of their positions, plus `tokens_in_defi_count` (tokenized assets with ≥ 1 eligible position) and
   `eligible_platforms` (distinct protocols behind eligible positions).
7. **Issuer score** = unique eligible positions over its tokenized assets in the basket (a pool shared by two of its
   tokens counts once). **Relative basket score**: for each asset with ≥ 2 issuers, issuer count / best issuer count ×
   100, averaged with equal weight per asset. Market caps are summed once per token; no issuer TVL is published, only
   liquidity of distinct direct pools.

## Limitations

- Coverage is **search-limited**: positions whose Portals name/symbol does not contain the ticker or the symbol of a
  tokenized asset are not found. A full-catalog discovery mode was intentionally left out to keep the pipeline simple.
- Only networks indexed by Portals are covered (Solana, TON, X Layer, Ink, Hyperliquid core are not). Unsupported
  means unknown usage, not zero.
- Portals does not expose lending collateral parameters; collateral use is never counted as confirmed.
- Issuer names are CMC labels, not verified legal entities.
