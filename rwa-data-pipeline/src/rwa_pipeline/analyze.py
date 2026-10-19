"""Pure analytics over saved snapshots: composition graph, classification, eligibility, scores."""

import re
import statistics
from datetime import datetime, timedelta
from decimal import Decimal, InvalidOperation

from rwa_pipeline.cmc import normalize_address, parse_deployments

SCORE_VERSION = "eligible-count-v1"
SENTINEL_PRICE = Decimal("9e15")  # Portals uses 9007199254740990 as price for non-ERC-20 pool positions
USE_CATEGORIES = {"dex_lp", "lending_supply", "vault", "staking", "yield_market"}
LIQUIDITY_SEMANTICS = {
    "dex_lp": "pool_tvl",
    "yield_market": "pool_tvl",
    "lending_supply": "vault_tvl",
    "vault": "vault_tvl",
    "staking": "vault_tvl",
}
EXCLUDED_KINDS = {"derivative", "unclassified"}
COVERAGE_PRIORITY = ["search", "partial", "not_indexed", "unsupported_by_portals", "no_contracts", "data_unavailable"]


# ---------- numbers ----------


def dec(value) -> Decimal | None:
    if value is None or isinstance(value, bool):
        return None
    try:
        d = Decimal(str(value))
    except InvalidOperation:
        return None
    return d if d.is_finite() else None


def dstr(d: Decimal | None) -> str | None:
    return None if d is None else format(d, "f")


def parse_time(value) -> datetime | None:
    if not isinstance(value, str):
        return None
    try:
        return datetime.fromisoformat(value.replace("Z", "+00:00"))
    except ValueError:
        return None


def _norm(s: str) -> str:
    return re.sub(r"[^A-Z0-9]", "", s.upper())


# ---------- tokens ----------


def classify_token(token: dict, ticker: str) -> tuple[str, str | None]:
    """Returns (kind, excluded_reason). Derivatives and tokens of another share class stay out of rankings."""
    symbol = token.get("symbol")
    name = (token.get("name") or "").lower()
    issuer = (token.get("issuer_name") or "").lower()
    if not symbol:
        return "unclassified", "missing_symbol"
    if "(derivatives)" in name or "(derivatives)" in issuer:
        return "derivative", "derivative"
    if _norm(ticker) not in _norm(symbol):
        return "unclassified", "symbol_does_not_match_ticker"
    if name.startswith("wrapped"):
        return "wrapper", None
    return "tokenized", None


# ---------- positions ----------


def norm_key(key: str) -> str:
    network, _, address = key.partition(":")
    return f"{network}:{normalize_address(address)}"


def child_keys(record: dict) -> list[str]:
    """Composition from tokens[]: bare addresses inherit the position's network."""
    out = []
    for ref in record.get("tokens") or []:
        if isinstance(ref, str) and ref:
            out.append(norm_key(ref) if ":" in ref else f"{record.get('network')}:{normalize_address(ref)}")
    return out


def classify_position(record: dict, settings: dict) -> str:
    platform = record.get("platform")
    if platform in settings["platform_categories"]:
        return settings["platform_categories"][platform]
    tags = set((record.get("metadata") or {}).get("tags") or [])
    for tag, category in settings["tag_categories"]:
        if tag in tags:
            return category
    return "unknown"


def normalize_opportunity(record: dict, settings: dict, platform_names: dict) -> dict:
    metrics = record.get("metrics") or {}
    tags = (record.get("metadata") or {}).get("tags") or []
    category = classify_position(record, settings)
    apy = dec(metrics.get("apy"))
    liquidity = dec(record.get("liquidity"))
    price = dec(record.get("price"))
    address = record.get("address") or ""
    is_portable = record.get("isPortable")

    flags = []
    if price is not None and price >= SENTINEL_PRICE:
        flags.append("sentinel_price")
    if apy is None:
        flags.append("apy_missing")
    elif apy > Decimal(settings["yield_apy_review_threshold_percent"]):
        flags.append("apy_anomaly")
    elif apy < 0:
        flags.append("apy_negative")
    if any((dec(metrics.get(k)) or 0) < 0 for k in ("volumeUsd1d", "volumeUsd7d")):
        flags.append("negative_volume_net_flow")
    if category == "dex_lp" and "#clp" in tags:
        flags.append("range_details_unknown")
    if liquidity is None:
        flags.append("liquidity_missing")

    platform = record.get("platform")
    return {
        "key": norm_key(record["key"]),
        "network": record.get("network"),
        "platform": platform,
        "platform_name": platform_names.get(platform, platform),
        "symbol": record.get("symbol"),
        "name": record.get("name"),
        "address": address,
        # 32-byte V4/Infinity pool ids are not ERC-20 contracts.
        "position_kind": "pool_id" if len(address) == 66 else "contract",
        "receipt_token_key": norm_key(record["key"]) if is_portable and len(address) == 42 else None,
        "category": category,
        "tags": tags,
        "components": child_keys(record),
        "liquidity_usd": dstr(liquidity),
        "liquidity_semantics": LIQUIDITY_SEMANTICS.get(category, "unknown"),
        "apy_percent": dstr(apy),
        "base_apy_percent": dstr(dec(metrics.get("baseApy"))),
        "reward_apy_percent": dstr(dec(metrics.get("rewardApy"))),
        "borrow_apy_percent": dstr(dec(metrics.get("borrowApy"))),
        "volume_usd_1d": dstr(dec(metrics.get("volumeUsd1d"))),
        "volume_usd_7d": dstr(dec(metrics.get("volumeUsd7d"))),
        "is_portable": is_portable,
        "updated_at": record.get("updatedAt"),
        "flags": flags,
    }


def eligibility(opp: dict, fetched_at: str, settings: dict) -> str:
    """Freshness is the source age at collection time, so a saved snapshot does not decay on its own."""
    if opp["category"] not in USE_CATEGORIES:
        return "category_excluded"
    updated, fetched = parse_time(opp["updated_at"]), parse_time(fetched_at)
    if updated is None or fetched is None:
        return "unknown_freshness"
    if fetched - updated > timedelta(hours=settings["max_position_age_hours"]):
        return "stale"
    liquidity = dec(opp["liquidity_usd"])
    if liquidity is None or opp["liquidity_semantics"] == "unknown":
        return "unknown_liquidity"
    if liquidity < Decimal(settings["min_eligible_liquidity_usd"]):
        return "low_liquidity"
    return "eligible"


# ---------- graph ----------


def parent_index(records: dict[str, dict]) -> dict[str, set[str]]:
    index: dict[str, set[str]] = {}
    for key, record in records.items():
        for child in child_keys(record):
            index.setdefault(child, set()).add(norm_key(key))
    return index


def find_relations(roots: list[str], index: dict[str, set[str]], stop_keys: set[str], max_depth: int) -> dict[str, dict]:
    """BFS up the 'contains' graph: root -> LP -> vault. Min depth wins, cycles are cut by `visited`.

    Tracked tokens (stop_keys) are neither counted nor traversed: a wrapper or another version is a token, not a use.
    Swaps never appear here because only composition edges exist."""
    found: dict[str, dict] = {}
    for root in sorted(roots):
        visited = {root}
        frontier = [(root, [root])]
        for depth in range(1, max_depth + 1):
            nxt = []
            for node, path in frontier:
                for parent in sorted(index.get(node, ())):
                    if parent in visited or parent in stop_keys:
                        continue
                    visited.add(parent)
                    new_path = path + [parent]
                    if parent not in found or depth < found[parent]["depth"]:
                        found[parent] = {"key": parent, "depth": depth, "path": new_path}
                    nxt.append((parent, new_path))
            frontier = nxt
    return found


def symbol_candidates(symbol: str, networks: set[str], opps: dict[str, dict], exclude: set[str]) -> list[str]:
    """Positions whose symbol names the token on the same network but whose composition does not contain its address
    (e.g. Morpho markets expose only the loan token). Kept as unconfirmed, never counted."""
    target = _norm(symbol)
    out = []
    for key, opp in opps.items():
        if key in exclude or opp["network"] not in networks or opp["category"] not in USE_CATEGORIES:
            continue
        parts = {_norm(p) for p in re.split(r"[/\-\s_]+", opp.get("symbol") or "")}
        if target in parts:
            out.append(key)
    return sorted(out)


# ---------- aggregation ----------


def _breakdown(keys, opps, field) -> dict:
    out: dict[str, dict] = {}
    for k in keys:
        bucket = out.setdefault(opps[k][field] or "unknown", {"raw": 0, "eligible": 0})
        bucket["raw"] += 1
        bucket["eligible"] += opps[k]["eligibility"] == "eligible"
    return dict(sorted(out.items(), key=lambda kv: (-kv[1]["eligible"], -kv[1]["raw"], kv[0])))


def _mean(values: list[Decimal]) -> Decimal:
    return (sum(values, Decimal(0)) / len(values)).quantize(Decimal("0.0001"))


def _apy_stats(opps_list) -> dict:
    values = [dec(o["apy_percent"]) for o in opps_list]
    return {"count": len(values), "max": dstr(max(values)), "average": dstr(_mean(values)), "median": dstr(statistics.median(values))}


def yield_profile(keys, opps) -> dict:
    eligible = [opps[k] for k in keys if opps[k]["eligibility"] == "eligible"]
    with_apy = [
        o
        for o in eligible
        if o["apy_percent"] is not None and dec(o["apy_percent"]) > 0 and "apy_anomaly" not in o["flags"]
    ]
    with_apy.sort(key=lambda o: (-dec(o["apy_percent"]), o["key"]))
    by_category: dict[str, list] = {}
    for o in with_apy:
        by_category.setdefault(o["category"], []).append(o)
    profile = {
        "status": "reported" if with_apy else ("no_reported_yield_in_coverage" if eligible else "unknown"),
        "yield_opportunities_count": len(with_apy),
        "max_reported_apy_percent": with_apy[0]["apy_percent"] if with_apy else None,
        "max_reported_apy_key": with_apy[0]["key"] if with_apy else None,
        # Mean over positions with a positive reported APY (what the site's summary charts show), never over zeros.
        "average_reported_apy_percent": _apy_stats(with_apy)["average"] if with_apy else None,
        "median_reported_apy_percent": _apy_stats(with_apy)["median"] if with_apy else None,
        "top_keys": [o["key"] for o in with_apy[:5]],
        "by_category": {c: _apy_stats(v) for c, v in by_category.items()},
        "missing_apy_count": sum(1 for o in eligible if o["apy_percent"] is None),
        "anomaly_count": sum(1 for o in eligible if "apy_anomaly" in o["flags"]),
    }
    return profile


def yield_summary(relations: dict[str, dict], opps: dict[str, dict]) -> dict:
    """Flat yield fields of an aggregate (underlying, issuer, basket); tokens carry the full `yield` profile."""
    profile = yield_profile(relations, opps)
    return {
        "yield_opportunities_count": profile["yield_opportunities_count"],
        "max_reported_apy_percent": profile["max_reported_apy_percent"],
        "average_reported_apy_percent": profile["average_reported_apy_percent"],
        "median_reported_apy_percent": profile["median_reported_apy_percent"],
    }


def eligible_platforms(relations: dict[str, dict], opps: dict[str, dict], platform_names: dict) -> list[str]:
    """Distinct protocol names behind eligible positions (200 Uniswap V3 pools count once, V2 and V3 separately)."""
    slugs = {opps[k]["platform"] for k in relations if opps[k]["eligibility"] == "eligible"}
    return sorted(platform_names.get(p, p) or p for p in slugs)


def opportunity_counts(relations: dict[str, dict], opps: dict[str, dict]) -> dict:
    keys = list(relations)
    eligible = [k for k in keys if opps[k]["eligibility"] == "eligible"]
    return {
        "raw_confirmed_opportunities_count": len(keys),
        "eligible_opportunities_count": len(eligible),
        "unknown_liquidity_count": sum(1 for k in keys if opps[k]["eligibility"] == "unknown_liquidity"),
        "direct_eligible_count": sum(1 for k in eligible if relations[k]["depth"] == 1),
        "indirect_eligible_count": sum(1 for k in eligible if relations[k]["depth"] > 1),
        "eligible_protocols_count": len({opps[k]["platform"] for k in eligible}),
        "eligible_networks_count": len({opps[k]["network"] for k in eligible}),
    }


def rank_key(token: dict):
    c = token["counts"]
    return (
        -c["eligible_opportunities_count"],
        -c["eligible_protocols_count"],
        -c["eligible_networks_count"],
        -c["direct_eligible_count"],
        token["id"],
    )


def deployment_status(dep: dict, portals: dict | None) -> str:
    if dep["status"]:
        return dep["status"]
    if portals is None:
        return "data_unavailable"
    if dep["network"] not in portals["networks"]:
        return "unsupported_network"
    if dep["key"] not in portals["looked_up"]:
        return "not_checked"
    return "indexed" if dep["key"] in portals["exact"] else "not_indexed"


def token_coverage(deployments: list[dict], info_ok: bool) -> str:
    statuses = {d["status"] for d in deployments}
    if not info_ok:
        return "data_unavailable"
    if not deployments:
        return "no_contracts"
    if "indexed" in statuses:
        return "search"
    if "not_checked" in statuses or "data_unavailable" in statuses:
        return "partial"
    if "not_indexed" in statuses:
        return "not_indexed"
    return "unsupported_by_portals"


def build(snapshots: list[dict], issuers_cache: dict, settings: dict, platform_names: dict) -> dict:
    snapshots = sorted(snapshots, key=lambda s: s.get("fetched_at") or "")

    # 1. tokens and the basket-wide set of tracked deployment keys
    tokens: list[dict] = []
    for snap in snapshots:
        cmc = snap.get("cmc") or {}
        info = cmc.get("info") or {}
        for t in (cmc.get("rwa") or {}).get("tokens") or []:
            cid = t.get("crypto_id")
            if cid is None:
                continue
            token_info = info.get(str(cid))
            kind, excluded_reason = classify_token(t, snap["ticker"])
            deployments = parse_deployments(token_info, settings["chain_mapping"])
            for d in deployments:
                d["status"] = deployment_status(d, snap.get("portals"))
                exact = (snap.get("portals") or {}).get("exact", {}).get(d["key"]) if d["key"] else None
                d["portals_symbol"] = exact.get("symbol") if exact else None
                d["decimals"] = exact.get("decimals") if exact else None
                d["portals_platform"] = exact.get("platform") if exact else None
                d.pop("address_raw", None)
            tokens.append(
                {
                    "id": cid,
                    "symbol": t.get("symbol"),
                    "name": t.get("name"),
                    "kind": kind,
                    "excluded_reason": excluded_reason,
                    "underlying_id": snap["underlying_id"],
                    "ticker": snap["ticker"],
                    "issuer_id": t.get("issuer_id"),
                    "issuer_name": t.get("issuer_name"),
                    "price_usd": dstr(dec(t.get("price"))),
                    "market_cap_usd": dstr(dec(t.get("market_cap"))),
                    "volume_24h_usd": dstr(dec(t.get("volume_24h"))),
                    "logo": (token_info or {}).get("logo"),
                    "cmc_slug": (token_info or {}).get("slug"),
                    "deployments": deployments,
                    "coverage": token_coverage(deployments, token_info is not None),
                    "fetched_at": snap.get("fetched_at"),
                }
            )
    tracked_keys = {d["key"] for t in tokens for d in t["deployments"] if d["key"]}

    # 2. one normalized record per Portals key; the newest snapshot wins
    opps: dict[str, dict] = {}
    for snap in snapshots:
        portals = snap.get("portals") or {}
        for record in list(portals.get("records", {}).values()) + list(portals.get("exact", {}).values()):
            opp = normalize_opportunity(record, settings, platform_names)
            opp["eligibility"] = eligibility(opp, snap["fetched_at"], settings)
            opps[opp["key"]] = opp
    symbols_by_key = {k: o["symbol"] for k, o in opps.items()}

    # 3. relations per token
    by_snapshot = {s["underlying_id"]: s for s in snapshots}
    for token in tokens:
        snap = by_snapshot[token["underlying_id"]]
        records = (snap.get("portals") or {}).get("records", {})
        index = parent_index(records)
        roots = [d["key"] for d in token["deployments"] if d["status"] == "indexed"]
        relations = find_relations(roots, index, tracked_keys, settings["max_graph_depth"]) if roots else {}
        local_opps = {norm_key(k): opps[norm_key(k)] for k in records}
        unconfirmed = (
            symbol_candidates(
                token["symbol"],
                {d["network"] for d in token["deployments"] if d["network"]},
                local_opps,
                set(relations) | tracked_keys,
            )
            if token["kind"] not in EXCLUDED_KINDS
            else []
        )
        token["relations"] = sorted(relations.values(), key=lambda r: (r["depth"], r["key"]))
        token["unconfirmed_keys"] = unconfirmed
        token["counts"] = {**opportunity_counts(relations, opps), "unconfirmed_candidates_count": len(unconfirmed)}
        token["usefulness_score"] = token["counts"]["eligible_opportunities_count"]
        token["by_network"] = _breakdown(relations, opps, "network")
        token["by_category"] = _breakdown(relations, opps, "category")
        token["by_platform"] = _breakdown(relations, opps, "platform")
        token["yield"] = yield_profile(relations, opps)

    # 4. underlyings: rank versions inside each underlying
    underlyings = []
    for snap in snapshots:
        own = [t for t in tokens if t["underlying_id"] == snap["underlying_id"]]
        included = sorted((t for t in own if t["kind"] not in EXCLUDED_KINDS), key=rank_key)
        for rank, t in enumerate(included, 1):
            t["rank_in_underlying"] = rank
        rel = {}
        for t in included:
            for r in t["relations"]:
                if r["key"] not in rel or r["depth"] < rel[r["key"]]["depth"]:
                    rel[r["key"]] = r
        rwa = (snap.get("cmc") or {}).get("rwa") or {}
        coverages = {t["coverage"] for t in included}
        underlyings.append(
            {
                "id": snap["underlying_id"],
                "ticker": snap["ticker"],
                "name": snap["name"],
                "isin": snap.get("isin"),
                "asset_type": snap["asset_type"],
                "status": snap["status"],
                "fetched_at": snap.get("fetched_at"),
                "cmc_rwa_id": rwa.get("rwa_id"),
                "cmc_name": rwa.get("name"),
                "tokenized_market_cap_usd": dstr(dec(rwa.get("tokenized_market_cap"))),
                "tokenized_volume_24h_usd": dstr(dec(rwa.get("tokenized_volume_24h"))),
                "average_tokenized_price_usd": dstr(dec(rwa.get("average_tokenized_price"))),
                "token_ids": [t["id"] for t in included],
                "excluded_token_ids": [t["id"] for t in own if t["kind"] in EXCLUDED_KINDS],
                "versions_count": len(included),
                "issuers_count": len({t["issuer_id"] for t in included if t["issuer_id"]}),
                "tokens_in_defi_count": sum(1 for t in included if t["counts"]["eligible_opportunities_count"] > 0),
                "eligible_platforms": eligible_platforms(rel, opps, platform_names),
                "coverage": next((c for c in COVERAGE_PRIORITY if c in coverages), "no_tokens"),
                "search_terms": sorted(((snap.get("portals") or {}).get("searches") or {}).keys()),
                "search_complete": all(m.get("complete") for m in ((snap.get("portals") or {}).get("searches") or {}).values()),
                **opportunity_counts(rel, opps),
                **yield_summary(rel, opps),
            }
        )

    issuers = build_issuers(tokens, opps, issuers_cache, underlyings, platform_names)

    used = {r["key"] for t in tokens for r in t["relations"]} | {k for t in tokens for k in t["unconfirmed_keys"]}
    opportunities = []
    for key in sorted(used):
        o = dict(opps[key])
        o["components"] = [{"key": c, "symbol": symbols_by_key.get(c)} for c in o["components"]]
        opportunities.append(o)

    included_tokens = [t for t in tokens if t["kind"] not in EXCLUDED_KINDS]
    all_rel = {r["key"]: r for t in included_tokens for r in t["relations"]}
    fetched = [s["fetched_at"] for s in snapshots if s.get("fetched_at")]
    return {
        "score_version": SCORE_VERSION,
        "settings": {
            k: settings[k]
            for k in ("min_eligible_liquidity_usd", "max_position_age_hours", "max_graph_depth", "yield_apy_review_threshold_percent")
        },
        "summary": {
            "underlyings_count": len(underlyings),
            "tokens_count": len(included_tokens),
            "excluded_tokens_count": len(tokens) - len(included_tokens),
            "issuers_count": len(issuers),
            "deployments_count": sum(len(t["deployments"]) for t in included_tokens),
            "indexed_deployments_count": sum(1 for t in included_tokens for d in t["deployments"] if d["status"] == "indexed"),
            "tokens_in_defi_count": sum(1 for t in included_tokens if t["counts"]["eligible_opportunities_count"] > 0),
            **opportunity_counts(all_rel, opps),
            **yield_summary(all_rel, opps),
            "coverage": "search",
            "oldest_fetched_at": min(fetched) if fetched else None,
            "newest_fetched_at": max(fetched) if fetched else None,
        },
        "underlyings": underlyings,
        "tokens": tokens,
        "issuers": issuers,
        "opportunities": opportunities,
    }


def build_issuers(tokens: list[dict], opps: dict, issuers_cache: dict, underlyings: list[dict], platform_names: dict | None = None) -> list[dict]:
    platform_names = platform_names or {}
    groups: dict[str, list[dict]] = {}
    for t in tokens:
        if t["kind"] not in EXCLUDED_KINDS and t["issuer_id"]:
            groups.setdefault(t["issuer_id"], []).append(t)

    # per underlying: distinct eligible opportunities per issuer (for the relative basket score)
    per_underlying: dict[str, dict[str, set]] = {}
    for issuer_id, own in groups.items():
        for t in own:
            keys = {r["key"] for r in t["relations"] if opps[r["key"]]["eligibility"] == "eligible"}
            per_underlying.setdefault(t["underlying_id"], {}).setdefault(issuer_id, set()).update(keys)
    ticker_by_id = {u["id"]: u["ticker"] for u in underlyings}

    out = []
    for issuer_id, own in groups.items():
        rel: dict[str, dict] = {}
        for t in own:
            for r in t["relations"]:
                if r["key"] not in rel or r["depth"] < rel[r["key"]]["depth"]:
                    rel[r["key"]] = r
        eligible = [k for k in rel if opps[k]["eligibility"] == "eligible"]
        counts = opportunity_counts(rel, opps)
        underlying_ids = sorted({t["underlying_id"] for t in own})

        relative = []
        for uid in underlying_ids:
            scores = per_underlying.get(uid, {})
            best = max((len(v) for v in scores.values()), default=0)
            if len(scores) >= 2 and best > 0:
                relative.append(len(scores.get(issuer_id, ())) / best * 100)

        direct_pools = {k for k in eligible if rel[k]["depth"] == 1 and opps[k]["category"] == "dex_lp"}
        pools_liquidity = sum((dec(opps[k]["liquidity_usd"]) for k in direct_pools), Decimal(0))
        market_caps = [dec(t["market_cap_usd"]) for t in own if t["market_cap_usd"] is not None]
        yields = yield_profile(rel, opps)
        meta = issuers_cache.get(issuer_id) or {}
        fetched = [t["fetched_at"] for t in own if t.get("fetched_at")]
        out.append(
            {
                "id": issuer_id,
                "name": meta.get("name") or own[0]["issuer_name"],
                "cmc_label": own[0]["issuer_name"],
                "website": meta.get("website"),
                "logo": meta.get("logo"),
                "metadata_status": "cmc" if meta else "unknown",
                "roster_size": meta.get("roster_size"),
                "underlying_ids": underlying_ids,
                "token_ids": sorted(t["id"] for t in own),
                "basket_underlyings_count": len(underlying_ids),
                "basket_tokens_count": len(own),
                "tokens_in_defi_count": sum(1 for t in own if t["counts"]["eligible_opportunities_count"] > 0),
                "deployments_count": sum(len(t["deployments"]) for t in own),
                "supported_deployments_count": sum(1 for t in own for d in t["deployments"] if d["status"] in ("indexed", "not_indexed")),
                "unsupported_deployments_count": sum(
                    1 for t in own for d in t["deployments"] if d["status"] in ("unsupported_network", "unmapped_network")
                ),
                **counts,
                "issuer_usefulness_score": len(eligible),
                "eligible_platforms": eligible_platforms(rel, opps, platform_names),
                "yield_opportunities_count": yields["yield_opportunities_count"],
                "max_reported_apy_percent": yields["max_reported_apy_percent"],
                "average_reported_apy_percent": yields["average_reported_apy_percent"],
                "median_reported_apy_percent": yields["median_reported_apy_percent"],
                "best_reported_yield_keys": yields["top_keys"][:3],
                "by_network": _breakdown(rel, opps, "network"),
                "by_category": _breakdown(rel, opps, "category"),
                "by_underlying": {
                    ticker_by_id.get(uid, uid): len(per_underlying.get(uid, {}).get(issuer_id, ())) for uid in underlying_ids
                },
                "mean_opportunities_per_underlying": dstr(
                    (Decimal(len(eligible)) / len(underlying_ids)).quantize(Decimal("0.01"))
                ),
                "mean_opportunities_per_token": dstr((Decimal(len(eligible)) / len(own)).quantize(Decimal("0.01"))),
                "basket_relative_score": dstr(Decimal(str(statistics.mean(relative))).quantize(Decimal("0.1"))) if relative else None,
                "compared_underlyings_count": len(relative),
                "direct_pools_liquidity_usd": dstr(pools_liquidity.quantize(Decimal("0.01"))) if direct_pools else None,
                "basket_market_cap_usd": dstr(sum(market_caps, Decimal(0))) if market_caps else None,
                "coverage": "search",
                "oldest_fetched_at": min(fetched) if fetched else None,
            }
        )
    out.sort(key=lambda i: (-i["issuer_usefulness_score"], -i["eligible_protocols_count"], i["name"] or ""))
    return out
