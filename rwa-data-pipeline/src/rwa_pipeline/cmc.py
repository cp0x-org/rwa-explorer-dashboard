"""CoinMarketCap: RWA roster (versions), contract addresses per network, issuers."""

import re

from rwa_pipeline.api import Api, ApiError
from rwa_pipeline.config import Asset, now_iso

CMC_BASE_URL = "https://pro-api.coinmarketcap.com"
INFO_BATCH = 100

EVM_ADDRESS = re.compile(r"^0x[0-9a-fA-F]{40}$")


def make_client(api_key: str, settings: dict, raw_dir=None) -> Api:
    headers = {"X-CMC_PRO_API_KEY": api_key, "Accept": "application/json"}
    return Api("cmc", CMC_BASE_URL, headers, settings, raw_dir)


def _checked(api: Api, body: dict) -> dict:
    # CMC reports errors in status.error_code (sometimes as a string) even with HTTP 200.
    status = body.get("status") or {}
    api.credits += int(status.get("credit_count") or 0)
    if str(status.get("error_code", "0")) != "0":
        raise ApiError("cmc", status.get("error_code"), str(status.get("error_message")))
    return body


def fetch_rwa_quotes(api: Api, tickers: list[str]) -> dict[str, list[dict]]:
    """All tracked RWA assets per symbol; one request for the whole selection."""
    body = _checked(api, api.get("/v5/real-world-assets/quotes/latest", {"symbol": ",".join(tickers)}))
    out: dict[str, list[dict]] = {}
    for item in (body.get("data") or {}).get("rwa_assets") or []:
        out.setdefault(str(item.get("symbol") or "").upper(), []).append(item)
    return out


def pick_rwa_asset(candidates: list[dict], asset: Asset, known_rwa_id: int | None) -> tuple[dict | None, str | None]:
    matches = [c for c in candidates if c.get("asset_type") == asset.asset_type]
    if known_rwa_id is not None:
        same = [c for c in matches if c.get("rwa_id") == known_rwa_id]
        if not same:
            return None, "rwa_id_changed" if matches else "not_in_cmc"
        matches = same
    if not matches:
        return None, "not_in_cmc"
    if len(matches) > 1:
        return None, "ambiguous_cmc_match"
    return matches[0], None


def fetch_crypto_info(api: Api, ids: list[int]) -> tuple[dict[int, dict], list[int]]:
    """Metadata incl. contract_address[] per CMC id. Ids rejected by CMC are skipped and returned separately."""
    info: dict[int, dict] = {}
    invalid: list[int] = []
    unique = sorted(set(ids))
    for start in range(0, len(unique), INFO_BATCH):
        chunk = unique[start : start + INFO_BATCH]
        while chunk:
            try:
                body = _checked(api, api.get("/v2/cryptocurrency/info", {"id": ",".join(map(str, chunk))}))
            except ApiError as e:
                # One unknown id fails the whole batch: "Invalid value for 'id': '39839'".
                m = re.search(r"Invalid value for 'id': '([\d,]+)'", e.message or "")
                bad = {int(x) for x in m.group(1).split(",")} & set(chunk) if m else set()
                if not bad:
                    raise
                invalid.extend(sorted(bad))
                chunk = [i for i in chunk if i not in bad]
                continue
            for key, value in (body.get("data") or {}).items():
                info[int(key)] = value
            break
    return info, invalid


def fetch_issuer(api: Api, issuer_id: str) -> dict:
    body = _checked(api, api.get("/v5/real-world-assets/issuers", {"issuer_id": issuer_id}))
    data = body.get("data") or {}  # data is the issuer object itself
    roster = data.get("tokens") or []
    return {
        "issuer_id": issuer_id,
        "name": data.get("name"),
        "website": data.get("website"),
        "logo": data.get("logo"),
        "roster_size": data.get("num_tokens") if data.get("num_tokens") is not None else len(roster),
        "roster": [{"crypto_id": t.get("crypto_id"), "symbol": t.get("symbol"), "rwa_id": t.get("rwa_id")} for t in roster],
        "fetched_at": now_iso(),
    }


def fetch_rwa_info(api: Api, rwa_ids: list[int]) -> dict[str, dict]:
    """RWA metadata (official website, logo when CMC has one), keyed by rwa_id."""
    body = _checked(api, api.get("/v5/real-world-assets/info", {"rwa_id": ",".join(map(str, sorted(set(rwa_ids))))}))
    out = {}
    for item in (body.get("data") or {}).get("rwa_assets") or []:
        about = item.get("about") or {}
        out[str(item.get("rwa_id"))] = {
            "rwa_id": item.get("rwa_id"),
            "name": item.get("name"),
            "website": item.get("website") or about.get("website"),
            "logo": about.get("logo"),
            "industry": item.get("industry"),
            "fetched_at": now_iso(),
        }
    return out


def normalize_address(address: str) -> str:
    # EVM addresses are case-insensitive; Solana/TON and others are not.
    return address.lower() if EVM_ADDRESS.match(address) else address


def parse_deployments(info: dict | None, chain_mapping: dict) -> list[dict]:
    """One deployment per (CMC network, address). Same address on two networks = two deployments."""
    if not info:
        return []
    entries = list(info.get("contract_address") or [])
    platform = info.get("platform") or {}
    fallback = platform.get("token_address")
    if fallback and not any(normalize_address(str(e.get("contract_address") or "")) == normalize_address(fallback) for e in entries):
        entries.append({"contract_address": fallback, "platform": {"name": platform.get("name")}})

    out, seen = [], set()
    for entry in entries:
        raw_address = str(entry.get("contract_address") or "").strip()
        cmc_network = (entry.get("platform") or {}).get("name")
        if not raw_address or not cmc_network:
            continue
        address = normalize_address(raw_address)
        if (cmc_network, address) in seen:
            continue
        seen.add((cmc_network, address))
        network = chain_mapping.get(cmc_network)
        if cmc_network not in chain_mapping:
            status = "unmapped_network"
        elif network is None:
            status = "unsupported_network"
        else:
            status = None  # resolved after the Portals lookup
        out.append(
            {
                "cmc_network": cmc_network,
                "network": network,
                "address": address,
                "address_raw": raw_address,
                "key": f"{network}:{address}" if network else None,
                "status": status,
            }
        )
    return out
