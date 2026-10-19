"""`sync`: fetch CMC + Portals data per underlying and save one snapshot file per underlying.

A snapshot is written only after everything for that underlying was fetched; on failure the previous one stays."""

import time
from collections.abc import Callable
from datetime import UTC, datetime
from pathlib import Path

from rwa_pipeline import cmc, portals
from rwa_pipeline.analyze import EXCLUDED_KINDS, classify_token
from rwa_pipeline.api import ApiError, AuthError
from rwa_pipeline.config import Asset, ConfigError, load_api_keys, now_iso, read_json, write_json


def snapshot_path(data_dir: Path, asset: Asset) -> Path:
    return data_dir / "snapshots" / f"{asset.storage_id}.json"


def select_assets(assets: list[Asset], symbols: list[str] | None) -> list[Asset]:
    enabled = [a for a in assets if a.enabled]
    if not symbols:
        return enabled
    wanted = {s.strip().upper() for s in symbols if s.strip()}
    unknown = wanted - {a.ticker for a in enabled}
    if unknown:
        raise ConfigError(f"unknown or disabled symbols: {', '.join(sorted(unknown))}")
    return [a for a in enabled if a.ticker in wanted]


class Progress:
    """Prefixes every line with the time since the run started; `log=None` keeps the pipeline silent (tests)."""

    def __init__(self, log: Callable[[str], None] | None):
        self.log = log or (lambda msg: None)
        self.started = time.monotonic()

    def elapsed(self) -> float:
        return time.monotonic() - self.started

    def __call__(self, msg: str) -> None:
        self.log(f"[{_fmt_seconds(self.elapsed()):>6}] {msg}")


def _fmt_seconds(seconds: float) -> str:
    return f"{seconds:.0f}s" if seconds < 90 else f"{seconds / 60:.1f}m"


def _short_params(params: dict) -> str:
    text = " ".join(f"{k}={v}" for k, v in params.items())
    return text if len(text) <= 80 else text[:77] + "..."


def sync(
    assets: list[Asset],
    settings: dict,
    data_dir: Path,
    symbols: list[str] | None,
    refresh: bool,
    log: Callable[[str], None] | None = None,
    verbose: bool = False,
) -> tuple[int, dict | None]:
    selected = select_assets(assets, symbols)
    todo = [a for a in selected if refresh or not snapshot_path(data_dir, a).exists()]
    if not todo:
        return 0, None  # nothing_to_add: no requests, no new export

    say = Progress(log)
    run_id = datetime.now(UTC).strftime("%Y%m%dT%H%M%SZ")
    raw_dir = data_dir / "raw" / run_id
    cmc_key, portals_key = load_api_keys()
    cmc_api = cmc.make_client(cmc_key, settings, raw_dir)
    portals_api = portals.make_client(portals_key, settings, raw_dir)
    if verbose:
        cmc_api.on_request = portals_api.on_request = lambda provider, path, params, status, sec: say(
            f"      {provider} GET {path} {_short_params(params)} -> {status} ({sec:.1f}s)"
        )
    say(
        f"run {run_id} ({'refresh' if refresh else 'add'}): {len(todo)} of {len(selected)} selected underlyings to fetch"
        + (f", {len(selected) - len(todo)} already have a snapshot" if len(todo) < len(selected) else "")
    )
    say(f"      {', '.join(a.ticker for a in todo)}")
    say(f"      raw responses: {raw_dir}")
    report = {
        "run_id": run_id,
        "started_at": now_iso(),
        "mode": "refresh" if refresh else "add",
        "selected": [a.ticker for a in selected],
        "skipped_existing": [a.ticker for a in selected if a not in todo],
        "completed": [],
        "failed": {},
        "warnings": [],
    }

    try:
        say(f"[1/5] CMC: tokenized assets (RWA quotes) for {len(todo)} tickers, one request")
        quotes = cmc.fetch_rwa_quotes(cmc_api, [a.ticker for a in todo])
        resolved = {}
        for a in todo:
            previous = read_json(snapshot_path(data_dir, a))
            known_rwa_id = ((previous or {}).get("cmc") or {}).get("rwa", {}).get("rwa_id")
            resolved[a.id] = cmc.pick_rwa_asset(quotes.get(a.ticker, []), a, known_rwa_id)
        found = [a for a in todo if resolved[a.id][0]]
        missing = [f"{a.ticker} ({resolved[a.id][1]})" for a in todo if not resolved[a.id][0]]
        say(f"      {len(found)} tickers tracked by CMC" + (f"; not resolved: {', '.join(missing)}" if missing else ""))

        ids = [t["crypto_id"] for rwa, _ in resolved.values() if rwa for t in rwa.get("tokens") or [] if t.get("crypto_id")]
        say(f"[2/5] CMC: contract addresses (cryptocurrency/info) for {len(set(ids))} tokens, batches of {cmc.INFO_BATCH}")
        info, invalid_ids = cmc.fetch_crypto_info(cmc_api, ids)
        if invalid_ids:
            report["warnings"].append(f"CMC rejected ids for /v2/cryptocurrency/info: {invalid_ids}")
            say(f"      warning: CMC rejected ids {invalid_ids}")

        issuers_path = data_dir / "issuers.json"
        issuers = read_json(issuers_path, {})
        issuer_ids = {t["issuer_id"] for rwa, _ in resolved.values() if rwa for t in rwa.get("tokens") or [] if t.get("issuer_id")}
        to_fetch = sorted(i for i in issuer_ids if refresh or i not in issuers)
        say(f"[3/5] CMC: issuers, {len(to_fetch)} of {len(issuer_ids)} to fetch (one request each)")
        for issuer_id in to_fetch:
            try:
                issuers[issuer_id] = cmc.fetch_issuer(cmc_api, issuer_id)
                say(f"      issuer {issuers[issuer_id].get('name') or issuer_id}: {issuers[issuer_id].get('roster_size')} tokens tracked")
            except AuthError:
                raise
            except ApiError as e:
                report["warnings"].append(f"issuer {issuer_id}: {e}")
                say(f"      warning: issuer {issuer_id}: {e}")
        write_json(issuers_path, issuers)

        rwa_ids = [rwa["rwa_id"] for rwa, _ in resolved.values() if rwa and rwa.get("rwa_id") is not None]
        say(f"[4/5] CMC: RWA metadata (website, logo) for {len(rwa_ids)} underlyings")
        if warning := update_rwa_info(cmc_api, data_dir, rwa_ids, refresh):
            report["warnings"].append(warning)
            say(f"      warning: {warning}")
        say(f"      CMC done: {cmc_api.requests} requests, {cmc_api.credits} credits")

        say("[5/5] Portals: supported networks and platform names")
        networks = portals.fetch_networks(portals_api)
        platforms_path = data_dir / "portals_platforms.json"
        if refresh or not platforms_path.exists():
            write_json(platforms_path, portals.fetch_platform_names(portals_api))
        say(f"      {len(networks)} networks: {', '.join(networks)}")
    except ApiError as e:
        report["fatal"] = str(e)
        say(f"fatal: {e}")
        return _finish(report, data_dir, cmc_api, portals_api, 1)

    say(f"Portals per underlying: exact lookup of contracts, then text search (all pages) per term; {len(todo)} to go")
    asset_started = time.monotonic()
    for n, asset in enumerate(todo, 1):
        rwa, error = resolved[asset.id]
        prefix = f"[{n:>{len(str(len(todo)))}}/{len(todo)}] {asset.ticker}"
        t0 = time.monotonic()
        try:
            snapshot = collect_asset(asset, rwa, error, info, invalid_ids, networks, portals_api, settings, run_id, say, prefix)
        except AuthError as e:
            report["failed"][asset.ticker] = str(e)
            report["fatal"] = str(e)
            say(f"{prefix}: fatal {e}")
            break
        except ApiError as e:
            report["failed"][asset.ticker] = str(e)
            say(f"{prefix}: failed, previous snapshot kept: {e}")
            continue
        write_json(snapshot_path(data_dir, asset), snapshot)
        report["completed"].append({"ticker": asset.ticker, "status": snapshot["status"]})
        per_asset = (time.monotonic() - asset_started) / n
        eta = per_asset * (len(todo) - n)
        say(
            f"{prefix}: saved ({snapshot['status']}) in {_fmt_seconds(time.monotonic() - t0)}"
            + (f", ~{_fmt_seconds(eta)} left" if n < len(todo) else "")
        )

    code = 1 if "fatal" in report and not report["completed"] else (2 if report["failed"] else 0)
    say(
        f"done: {len(report['completed'])} saved, {len(report['failed'])} failed; "
        f"requests cmc={cmc_api.requests} portals={portals_api.requests}, cmc credits={cmc_api.credits}"
    )
    return _finish(report, data_dir, cmc_api, portals_api, code)


def update_rwa_info(cmc_api, data_dir: Path, rwa_ids: list[int], refresh: bool) -> str | None:
    """Caches CMC RWA metadata (official website, logo) in data/rwa_info.json. Returns a warning on failure."""
    path = data_dir / "rwa_info.json"
    cache = read_json(path, {})
    need = sorted({i for i in rwa_ids if refresh or str(i) not in cache})
    if not need:
        return None
    try:
        cache.update(cmc.fetch_rwa_info(cmc_api, need))
    except AuthError:
        raise
    except ApiError as e:
        return f"rwa info: {e}"
    write_json(path, cache)
    return None


def fill_missing_rwa_info(assets: list[Asset], settings: dict, data_dir: Path) -> str | None:
    """For `logos`: metadata of already collected underlyings that predate the rwa_info cache."""
    rwa_ids = []
    for asset in assets:
        snap = read_json(snapshot_path(data_dir, asset)) or {}
        rwa_id = ((snap.get("cmc") or {}).get("rwa") or {}).get("rwa_id")
        if asset.enabled and rwa_id is not None:
            rwa_ids.append(rwa_id)
    cache = read_json(data_dir / "rwa_info.json", {})
    if all(str(i) in cache for i in rwa_ids):
        return None
    cmc_key, _ = load_api_keys()
    return update_rwa_info(cmc.make_client(cmc_key, settings), data_dir, rwa_ids, refresh=False)


def collect_asset(asset, rwa, error, info, invalid_ids, networks, portals_api, settings, run_id, say=None, prefix="") -> dict:
    say = say or (lambda msg: None)
    prefix = prefix or asset.ticker
    snapshot = {
        "schema_version": 1,
        "underlying_id": asset.id,
        "ticker": asset.ticker,
        "name": asset.name,
        "isin": asset.isin,
        "asset_type": asset.asset_type,
        "run_id": run_id,
        "fetched_at": now_iso(),
    }
    if rwa is None:
        say(f"{prefix}: {error}, nothing to fetch from Portals")
        return {**snapshot, "status": error, "cmc": None, "portals": None}

    tokens = [t for t in rwa.get("tokens") or [] if t.get("crypto_id")]
    token_info = {str(t["crypto_id"]): _info_subset(info[t["crypto_id"]]) for t in tokens if t["crypto_id"] in info}
    cmc_part = {"rwa": rwa, "info": token_info, "invalid_ids": [i for i in invalid_ids if str(i) in {str(t["crypto_id"]) for t in tokens}]}
    if not tokens:
        say(f"{prefix}: CMC tracks no tokenized assets")
        return {**snapshot, "status": "no_tracked_tokens", "cmc": cmc_part, "portals": None}

    keys = [
        d["key"]
        for t in tokens
        for d in cmc.parse_deployments(token_info.get(str(t["crypto_id"])), settings["chain_mapping"])
        if d["key"] and d["network"] in networks
    ]
    limit = settings["portals_page_limit"]
    included = [t for t in tokens if classify_token(t, asset.ticker)[0] not in EXCLUDED_KINDS]
    say(
        f"{prefix}: {len(tokens)} tokens in CMC ({len(included)} tokenized: {', '.join(t['symbol'] for t in included if t.get('symbol'))}), "
        f"{len(set(keys))} contracts on Portals networks"
    )
    exact = portals.lookup(portals_api, keys, limit) if keys else {}
    if keys:
        say(f"{prefix}: lookup -> {len(exact)} of {len(set(keys))} contracts indexed by Portals")

    terms = portals.minimal_terms([asset.ticker] + [t["symbol"] for t in included if t.get("symbol")])
    records, searches = {}, {}
    for term in terms:
        t0 = time.monotonic()
        found, meta = portals.search(portals_api, term, limit)
        records.update(found)
        searches[term] = meta
        say(
            f"{prefix}: search '{term}' -> {len(found)} positions, {meta.get('pages', '?')} page(s) in {_fmt_seconds(time.monotonic() - t0)}"
            + ("" if meta.get("complete") else " (incomplete)")
        )

    snapshot["fetched_at"] = now_iso()
    return {
        **snapshot,
        "status": "ok",
        "cmc": cmc_part,
        "portals": {"networks": networks, "looked_up": sorted(set(keys)), "exact": exact, "searches": searches, "records": records},
    }


def _info_subset(item: dict) -> dict:
    return {k: item.get(k) for k in ("id", "symbol", "name", "slug", "logo", "date_added", "platform", "contract_address")}


def _finish(report, data_dir, cmc_api, portals_api, code) -> tuple[int, dict]:
    report["finished_at"] = now_iso()
    report["requests"] = {"cmc": cmc_api.requests, "portals": portals_api.requests}
    report["cmc_credits"] = cmc_api.credits
    report["exit_code"] = code
    write_json(data_dir / "runs" / f"{report['run_id']}.json", report)
    return code, report
