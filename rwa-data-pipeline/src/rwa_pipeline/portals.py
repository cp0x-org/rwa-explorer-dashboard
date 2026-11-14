"""Portals: supported networks, exact token lookup, paginated text search."""

from rwa_pipeline.api import Api, ApiError

PORTALS_BASE_URL = "https://api.portals.fi"
LOOKUP_BATCH = 25
MAX_PAGES = 100


def make_client(api_key: str, settings: dict, raw_dir=None) -> Api:
    return Api("portals", PORTALS_BASE_URL, {"Authorization": f"Bearer {api_key}"}, settings, raw_dir)


def fetch_networks(api: Api) -> list[str]:
    body = api.get("/v1/networks", {})
    if not isinstance(body, list) or not body or not all(isinstance(n, str) and n.strip() for n in body):
        raise ApiError("portals", None, "invalid networks response: expected a nonempty list of nonempty strings")
    return body


def fetch_platform_names(api: Api) -> dict[str, str]:
    body = api.get("/v2/platforms", {})
    items = body if isinstance(body, list) else []
    return {p["platform"]: p.get("name") or p["platform"] for p in items if isinstance(p, dict) and p.get("platform")}


def _paginate(api: Api, params: dict, limit: int) -> tuple[dict[str, dict], dict]:
    """Zero-based pages with a constant limit until more=false. Dedupes by key."""
    items: dict[str, dict] = {}
    page, pages, total, more = 0, 0, None, True
    while more and pages < MAX_PAGES:
        body = api.get("/v2/tokens", {**params, "limit": limit, "page": page})
        pages += 1
        for token in body.get("tokens") or []:
            if isinstance(token, dict) and token.get("key"):
                items[token["key"]] = token
        total = body.get("totalItems")
        more = bool(body.get("more"))
        page += 1
    meta = {"total_items": total, "pages": pages, "received": len(items), "complete": not more}
    if total is not None and total != len(items):
        meta["count_mismatch"] = True
    return items, meta


def lookup(api: Api, keys: list[str], limit: int) -> dict[str, dict]:
    """Exact records for network:address keys. Returns only what Portals indexes."""
    found: dict[str, dict] = {}
    unique = sorted(set(keys))
    for start in range(0, len(unique), LOOKUP_BATCH):
        items, meta = _paginate(api, {"ids": ",".join(unique[start : start + LOOKUP_BATCH])}, limit)
        received, total = meta["received"], meta["total_items"]
        if total is not None and received < total:
            raise ApiError("portals", None, f"incomplete lookup (count_mismatch): received {received}, reported {total}")
        if not meta["complete"]:
            raise ApiError("portals", None, f"incomplete lookup (MAX_PAGES reached): received {received}, reported {total}")
        found.update(items)
    return found


def search(api: Api, term: str, limit: int) -> tuple[dict[str, dict], dict]:
    return _paginate(api, {"search": term}, limit)


def minimal_terms(terms: list[str]) -> list[str]:
    """Portals search matches substrings of symbol/name, so a term containing a shorter term adds nothing."""
    unique = sorted({t.strip() for t in terms if t and t.strip()}, key=lambda s: (len(s), s.upper(), s))
    out: list[str] = []
    for term in unique:
        if not any(o.upper() in term.upper() for o in out):
            out.append(term)
    return out
