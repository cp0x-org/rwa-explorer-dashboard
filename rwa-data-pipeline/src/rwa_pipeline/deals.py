"""Public cp0x deals and their last good disk cache."""

from collections.abc import Callable
from datetime import UTC, datetime
from pathlib import Path
from urllib.parse import urlsplit

import httpx

from rwa_pipeline.config import write_json

DEALS_URL = "https://cp0x.com/api/v1/deals"
DEALS_FILE = "cp0x_deals.json"


class DealsError(Exception):
    pass


def clean_https_url(url) -> bool:
    """Only absolute HTTPS URLs without credentials or unsafe characters."""
    if not isinstance(url, str) or not url.startswith("https://"):
        return False
    if any(c.isspace() or ord(c) < 32 or ord(c) == 127 or c in '\\"<>`{}' for c in url):
        return False
    try:
        parts = urlsplit(url)
        parts.port  # Reject malformed ports before publishing the URL.
        return bool(parts.hostname) and parts.username is None and parts.password is None
    except ValueError:
        return False


def clean_deal_page_url(url, slug: str) -> bool:
    """Publish only the exact cp0x page URL for this deal's slug."""
    return clean_https_url(url) and url == f"https://cp0x.com/deals/{slug}"


def parse_deals(body) -> tuple[dict, list[str]]:
    """Keep named deals with clean HTTPS links; report unusable named entries."""
    if not isinstance(body, dict) or not isinstance(body.get("deals"), list):
        raise DealsError("invalid deals response: expected a deals array")
    deals, skipped = {}, []
    for item in body["deals"]:
        if not isinstance(item, dict) or not isinstance(item.get("slug"), str) or not item["slug"]:
            continue
        slug = item["slug"]
        if not clean_https_url(item.get("link")):
            skipped.append(slug)
            continue
        deals[slug] = {
            "title": item.get("title"),
            "link": item["link"],
            "bonus": item.get("bonus"),
            "page_url": item.get("page_url") if clean_deal_page_url(item.get("page_url"), slug) else None,
        }
    return deals, skipped


def fetch_deals(client: httpx.Client | None = None) -> dict:
    """Fetch the public response; transport, status and JSON failures are DealsError."""
    try:
        if client is None:
            with httpx.Client(timeout=30) as client:
                return fetch_deals(client)
        response = client.get(DEALS_URL)
        if response.status_code != 200:
            raise DealsError(f"HTTP {response.status_code}")
        return response.json()
    except (httpx.HTTPError, ValueError) as e:
        raise DealsError(str(e)) from e


def refresh_deals(
    data_dir: Path, fetch: Callable | None = None, now: datetime | None = None,
    keep: set[str] | None = None,
) -> tuple[dict | None, str | None]:
    """Replace the cache after a successful fetch, optionally keeping only named slugs."""
    try:
        deals, skipped = parse_deals((fetch if fetch is not None else fetch_deals)())
    except DealsError as e:
        return None, f"deals: {e}"
    if keep is not None:
        deals = {slug: deal for slug, deal in deals.items() if slug in keep}
        skipped = [slug for slug in skipped if slug in keep]
    cache = {"fetched_at": (now or datetime.now(UTC)).astimezone(UTC).strftime("%Y-%m-%dT%H:%M:%SZ"), "deals": deals}
    write_json(data_dir / DEALS_FILE, cache)
    warning = f"deals: skipped unusable links for {', '.join(skipped)}" if skipped else None
    return cache, warning
