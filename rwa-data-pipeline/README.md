# rwa-data-pipeline

Collects tokenized assets (on-chain tokens of stocks/ETFs) from CoinMarketCap, finds their DeFi usage in Portals,
computes ratings for tokenized assets and issuers, and publishes one JSON file for `../rwa-interface`.
The same data can be written as flat CSV/XLSX tables for manual analysis.

```
config/assets.json ──► sync ──► data/snapshots/<ISIN>.json ──► build ──► rwa-interface/public/data/rwa.json
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
uv run rwa-pipeline deals                         # refresh the public cp0x deals cache, then rebuild the export
uv run rwa-pipeline export                        # flat tables for manual analysis: data/exports/rwa.xlsx (no network)
uv run rwa-pipeline export --format csv           # one CSV per table into data/exports/csv/
uv run rwa-pipeline export -f xlsx --out ~/Desktop/rwa.xlsx   # xlsx: --out is the file; csv: --out is the directory
uv run rwa-pipeline build --format all            # sync/build also take --format csv|xlsx|all to write tables after the JSON
uv run pytest                                     # tests
```

Options: `--data-dir` (default `data/`), `--frontend-dir` (default `../rwa-interface`), `--format/-f` (`csv`, `xlsx`,
`all`; `sync` and `build` write tables only when it is given, `export` defaults to `xlsx`).
`sync`, `build`, `logos`, `deals` and `export` also accept `--config-dir` for the directory containing `links.json`, `issuers.json` and `rights_profiles.json`; assets and settings
are still read from their default paths.
Exit codes: `0` success or nothing to add, `2` a partial run (some underlyings saved, some failed; previous snapshots
of failed underlyings are kept) or rights profiles were excluded, `1` a failed collection where nothing was saved
(including fatal auth/CMC errors), or a config error.

- **add** (default) never touches existing snapshots. A repeated add with nothing new prints `nothing_to_add`, makes no
  requests and no new export. An underlying whose CMC roster is empty is saved as `no_tracked_tokens` and is not retried.
- **refresh** replaces the snapshot of each selected underlying only after all its requests succeeded; a failed
  Portals/CMC call keeps the last good snapshot. This includes 2xx responses with a non-JSON body, CMC responses
  missing `status`/`data` or with a non-object `status`, truncated Portals contract lookups (`count_mismatch` or
  `MAX_PAGES` reached with `more` still true), and empty or invalid network lists.
  A snapshot with CMC tokens is never replaced by one without tokens, including when the ticker disappears,
  its RWA ID changes, its CMC match becomes ambiguous, or its token roster becomes empty. The underlying is marked
  failed and the run continues. To remove an asset from the basket, set `enabled: false`.
  Discovering a new issuer or network of a known ticker = refresh.
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
Optional `symbol_aliases` is a list of nonempty strings for versions with a different symbol, such as
`"symbol_aliases": ["SPACEX"]` for SPCX. Aliases are trimmed, uppercased and deduplicated in their original order;
the default is an empty list. A symbol may contain the normalized ticker or any normalized alias.

## Data

```
data/
  snapshots/<ISIN>.json        one per underlying: CMC roster + contract metadata + Portals exact lookups and search results
  issuers.json                 CMC issuer metadata cache (name, website, roster)
  rwa_info.json                CMC RWA metadata cache per rwa_id (official website, logo)
  portals_platforms.json       Portals platform slug -> name
  cp0x_deals.json              public cp0x deals cache with fetched_at and deals keyed by slug
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

Logos are downloaded once and published to `rwa-interface/public/logos/` by `build`; the site loads only local files.
An existing file is never overwritten, so to set a logo by hand (e.g. an issuer or an ETF without logo and website in
CMC) put it at `data/logos/issuers/<issuer_id>.png` or `data/logos/underlyings/<ISIN>.png` and run `build`.

The export (`public/data/rwa.json`) contains `summary`, `settings`, `underlyings`, `tokens`, `issuers`, `opportunities`.
Money/APY/liquidity are decimal strings, `null` means "not reported" (never zero). It is written atomically as one
file, so the site never mixes data of two builds.
Display names (token `name` and `issuer_name`, position, issuer and underlying `name`) have whitespace runs,
including line breaks and tabs, collapsed to single spaces and their edges trimmed. Saved snapshots stay unchanged.

Optional `config/issuers.json` supplies issuer websites missing from CMC:

```json
{"schema_version": 1, "issuers": {"6a43600c282e7240b76c8bbb": {"website": "https://realityfinance.xyz"}}}
```

The top level must be an object with `schema_version: 1` and an `issuers` object keyed by issuer ID. Each entry must
contain only `website`, a clean absolute HTTPS URL without credentials or unsafe characters. Invalid config stops
the command with exit code `1`. A CMC website always takes precedence; a missing config leaves issuer data unchanged.
This file is read from `--config-dir`, alongside `links.json`, and is separate from the CMC cache `data/issuers.json`.

## Outbound links and cp0x deals

`config/links.json` holds platform home URLs and position routes, issuer apps, swap templates, allowed `hosts`,
optional cp0x deal slugs and `deals_max_age_days`. A route is used only for a network with a `verified` object
containing `at` (a valid ISO `YYYY-MM-DD` date) and `sample` (a nonempty string). Open the template in a browser
on a live position of that network before recording the date and sample key, for example
`"verified": {"at": "2026-11-13", "sample": "base:0xsample"}`. Swap templates require the same verification.
Omit `verified` until checked; booleans, strings and incomplete records are config errors. Unverified or unresolved
position routes fall back to the platform home. URLs must be clean HTTPS links on a host allowed by that config entry.

Routes can use `{network}` (the route's network slug), `{address}` and `{meta.field}` from the raw Portals metadata.
Substitutions accept plain identifiers only; metadata comes from the newest snapshot by `fetched_at`, regardless
of asset order. A deployment gets a swap link only when its exact `key` is among the components of a direct eligible
DEX pool on that network and one of the template's platforms. Another contract on the same network does not qualify.

`uv run rwa-pipeline deals` fetches `https://cp0x.com/api/v1/deals`, atomically updates `data/cp0x_deals.json`, then
rebuilds the export. `cp0x_deals.json` stores only the deals named by `links.json`.
`sync` refreshes this cache after logos and before building. A failed source prints a warning
and keeps the last good cache; `deals` still builds and exits successfully. `build` reads the cache without network
requests. Set `deal` to a nonempty slug string, such as `"deal": "venus"`, or omit it; other values are config errors.
Missing, unreadable, stale or unusable deals hide the referral and produce a warning once per affected action.
A cache dated more than one hour into the future is unusable; up to one hour of clock skew is accepted.
Deal pages must equal `https://cp0x.com/deals/<slug>` literally for that deal's own slug, including case;
extra path segments, `..`, a trailing slash, queries, fragments and another deal's slug are rejected.
An invalid page becomes `null`
without hiding an otherwise valid referral URL, including when the cache was edited by hand. `sync` appends deal,
link and rights warnings to the saved run report, preserving collection warnings; an absent report is allowed.

The export adds `opportunities[].link` (`url`, `kind`: `position` or `home`) and `opportunities[].referral`,
`issuers[].app` (`url`, `label`) and `issuers[].referral`, and `tokens[].links` (`kind`: `swap`, `label`, `url`,
`network`). A referral contains `url`, `deal`, `bonus`, `page_url` and `as_of`. The referral link is a separate
action; the direct link never depends on it. The summary carries `links_deals_fetched_at`,
`links_deals_max_age_days` and `links_warnings_count`, so the site can hide referrals that age after a build.
Without `links.json`, all these actions are present as `null` or empty lists, with no link warnings.
Flat tables include `opportunities.link_url`, `opportunities.link_kind`, `opportunities.referral_url`,
`issuers.app_url`, `issuers.referral_url` and `tokens.swap_urls` (URLs joined with `; `; empty means unreported).
`logos --config-dir <directory>` rebuilds these links and rights using the selected config directory.

## Holder rights profiles

`config/rights_profiles.json` holds manually reviewed legal facts from issuer documents. It contains
`schema_version: 1`, a `profiles` list and optional `review_valid_days` (a positive integer, default 180).
Each profile identifies its product, legal issuer, CMC issuer and explicit `token_ids`, with a review date,
source documents and every fact defined in `rwa_pipeline/rights.py`. Facts use closed lists of values;
the exported schema supplies their display order, groups, allowed values and holder categories (`who`).

- A profile applies only to its listed token IDs. Another token from the same issuer stays unreviewed.
  `per_token_documents` covers exactly one token; `programme_documents` requires `scope_evidence` quoting
  the documents that establish coverage of the programme. Duplicate IDs in `token_ids` are forbidden and exclude the profile.
- A broken profile is excluded with one warning; valid profiles and the rest of the export are still published.
  An unreadable file or invalid overall structure stops the build, including duplicate profile IDs or a token
  assigned to two profiles. A missing file simply leaves all tokens unreviewed.
- A profile with none of its tokens in the current basket is outside the export, even if its issuer is present.
  It is not published and produces one warning naming the profile and absent tokens; it does not count as excluded
  or change the exit code. An absent issuer is an error if any listed token is present in the export.
- `unknown` means the documents were reviewed but did not answer that fact: `checked` lists the HTTPS documents
  examined, and there is no `evidence`. No profile means nobody has reviewed that token. These are distinct states.

To add or correct a profile, edit `config/rights_profiles.json` through a pull request. Check the issuer's
documents and the exact token IDs. Support each known value with verbatim quotes and links to those documents;
each evidence entry records `quote`, clean HTTPS `url`, `title`, `doc_date` and `retrieved_at`. Quotes in `evidence`
and `scope_evidence` must contain at least six whitespace-separated words and at most 600 characters after whitespace
normalization. The export collapses whitespace runs (including PDF line breaks and tabs) to single spaces and trims
the edges; the source config is unchanged. A shorter fragment excludes the profile. Record the legal issuer with
evidence, list the documents consulted, and update `reviewed_at`.
Use the required `who` for values that depend on the holder, and supply both `en` and `zh` for any optional note.

- `redemption: at_events_only` means payout only at maturity or a specified event, with no holder-demand redemption; requires `who`.
- `unsecured_claim` on `loss_exploit`, `loss_lost_tokens`, `loss_issuer_insolvency` or `loss_custodian_failure` means an unsecured claim against the issuer, unlike `claim_on_assets`, a claim on backing assets; requires `who`.
- `voting: discretionary` means the issuer may follow holder instructions but is not obliged to; requires `who`.

For `restricted: listed`, supply a nonempty `countries` or `zones` list, or set `us_persons: true`.
For `restricted: none_stated`, omit `countries` and `zones` and do not set `us_persons: true`;
restriction details would contradict that value and exclude the profile.
Run the pipeline tests and `uv run rwa-pipeline build` before submitting the pull request.

A review becomes `stale` only when its age exceeds `review_valid_days`; the exact boundary remains fresh.
Stale profiles stay published with a warning and do not change the exit code. If only some listed tokens are missing,
one warning lists the absent tokens; the profile stays published with only exported `token_ids` in their original
order, without changing the config. After writing the export, commands that rebuild
the frontend JSON return exit code `2` if any profile was excluded: the export is usable, but those profiles
need correction. A file-wide config error returns exit code `1` and writes no export.

The export adds:

- `rights_profiles`: accepted profiles in file order, copied with a `stale` boolean.
- `rights_schema`: `unknown_value: "unknown"`, `who` and ordered `facts` entries with `key`, `group`, `values`
  and `needs_who`. The shared `unknown_value` is not repeated in individual facts' `values` lists.
- `tokens[].rights_profile_id`: the reviewed profile ID, or `null` for an unreviewed token.
- `issuers[].rights_profile_ids` and `issuers[].rights_reviewed_tokens_count`: profiles and covered tokens.
- Summary fields `rights_profiles_count`, `rights_reviewed_tokens_count`, `rights_review_valid_days`,
  `rights_excluded_profiles_count` and `rights_warnings_count`.

Without profiles, the schema is still present, profile lists are empty, token references are `null` and counts
are zero. CMC issuer labels remain separate from the documented legal issuer in each profile.

## Tables for manual analysis (`export`)

`uv run rwa-pipeline export` flattens the export into eleven tables; they hold everything the site shows, including the
figures the site computes on the fly (average APY, tokenized assets "in DeFi", protocol diversity):

| table           | one row per                              | notable columns                                                                      |
|-----------------|------------------------------------------|--------------------------------------------------------------------------------------|
| `summary`       | metric                                   | basket totals, settings, `generated_at`                                              |
| `underlyings`   | underlying stock/ETF                     | tokenized market cap/volume, versions, issuers, `tokens_in_defi_count`, eligible/raw counts, yield (max/average/median APY), `eligible_platforms` |
| `tokens`        | tokenized asset (CMC token)              | `kind`/`excluded_reason`, issuer, price/market cap/volume, `usefulness_score`, `rank_in_underlying`, counts, deployments, eligible networks/platforms/categories, yield profile, `cmc_url`, `swap_urls` |
| `issuers`       | issuer                                   | score, relative basket score, `tokens_in_defi_count` and share, protocols, yield, pools liquidity, basket market cap, `app_url`, `referral_url` |
| `opportunities` | DeFi position (Portals)                  | protocol, network, category, liquidity, APY (total/base/reward/borrow), volumes, eligibility, flags, `linked_tokens`, `link_url`, `link_kind`, `referral_url` |
| `relations`     | token × position link                    | `link` = direct / indirect (via LP → vault, with `path`) / unconfirmed, `status`, plus the position's columns |
| `deployments`   | token contract on a network              | CMC network, Portals slug, address, `status` (indexed / not_indexed / unsupported …)  |
| `breakdowns`    | entity × dimension × key                 | raw/eligible counts of tokens and issuers by network, category, platform, underlying |
| `rights_profiles` | published rights profile              | `profile_id`, `issuer_id`, `issuer_name`, `product`, `scope_basis`, `tokens_count`, `legal_issuer_name`, `legal_issuer_country`, `legal_issuer_zone`, `legal_issuer_note_en`, `legal_issuer_quotes`, `legal_issuer_urls`, `reviewed_at`, `stale` |
| `rights_facts`  | fact citation (one row for `unknown`)     | `profile_id`, `issuer_id`, `product`, `fact`, `group`, `value`, `who`, `countries`, `zones`, `us_persons`, `note_en`, `quote`, `url`, `title`, `doc_date`, `retrieved_at`, `checked`, `reviewed_at`, `stale` |
| `rights_tokens` | every exported token                     | `token_id`, `symbol`, `ticker`, `issuer_name`, `rights_profile_id`, `reviewed_at`, `stale`; empty profile means unreviewed |

In `opportunities`, an otherwise `eligible` position with `linked_tokens_count` = 0 has `eligibility` = `unconfirmed`:
only a symbol match exists, with no confirmed basket token link. Other eligibility values are copied unchanged.
This table-only status does not change the site's JSON, counts or ratings.

In `rights_facts`, `countries`, `zones` and `checked` are joined with `; `; `checked` is populated only for `unknown`.
In `rights_profiles`, legal issuer quotes are joined with ` | ` and their URLs with `; `, deduplicated in source order.
Exports without rights fields yield empty `rights_profiles` and `rights_facts` tables and `rights_tokens` rows without profiles.
All three rights tables are included in CSV and XLSX, with descriptions on the workbook's README sheet.

Values follow the JSON: USD, percent and counts are numbers, an empty cell is "not reported" (never zero), times are
UTC. Lists are joined with `; `. In XLSX the header row is frozen with an autofilter. Strict ISO `YYYY-MM-DD`
values in `reviewed_at` and `retrieved_at` become real Excel dates formatted as `yyyy-mm-dd`;
`doc_date` remains free text. Timestamps remain Excel dates with times.
CSV keeps decimal strings and ISO dates and timestamps as text. `export` reads the saved snapshots (like `build`) and does not
touch the frontend JSON; `build --format xlsx` / `sync --format xlsx` write both.
Use `export --config-dir <directory>` to select the same links and rights configs as `build`.
It prints link and rights build warnings as `  warning ...` and returns `0` on success, including profiles
outside the export and stale reviews; `2` after writing tables if any rights profile was excluded;
or `1` on a config error, before writing tables.

## How the numbers are computed

1. **Tokenized assets** — every token in CMC `quotes/latest` for the ticker. `(Derivatives)` entries, tokens without
   symbol and tokens whose normalized symbol contains neither the ticker nor a configured `symbol_aliases` entry
   (e.g. `GOOGon` listed under GOOGL) are kept but excluded
   from ratings. Names starting with "Wrapped" are marked as wrappers.
2. **Deployments** — all `contract_address[]` from CMC `cryptocurrency/info`, one per network+address. CMC network
   names map to Portals slugs via `chain_mapping` in settings: `null` = known but not in Portals
   (`unsupported_network`), including Solana, TON, Mantle and Monad; missing = `unmapped_network`. Portals exact lookup marks each supported deployment
   `indexed` or `not_indexed`.
3. **Positions** — Portals `/v2/tokens?search=` over the ticker and the symbols of its tokenized assets (substring
   search, so redundant terms are dropped), all pages. A position is **confirmed** only if its `tokens[]` contains the
   address of a tokenized asset (directly, or through nested positions: root → LP → vault, up to `max_graph_depth`).
   Name matches without an address link (e.g. Morpho markets that list only the loan asset) are stored as
   *unconfirmed* and never counted. Tracked tokens themselves (root, other tokenized assets, wrappers tracked by CMC)
   are not positions.
   A version found through a configured alias counts as covered by search only after a search term matching a
   substring of its symbol (case-insensitively) was searched. Otherwise, `search` coverage becomes `partial`;
   an underlying's search is complete only when all searches are complete and every included version's symbol
   contains a searched term.
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
- Only networks indexed by Portals are covered (Solana, TON, Mantle, Monad, X Layer, Ink, Hyperliquid core are not). Unsupported
  means unknown usage, not zero.
- Portals does not expose lending collateral parameters; collateral use is never counted as confirmed.
- Issuer names are CMC labels, not verified legal entities.
