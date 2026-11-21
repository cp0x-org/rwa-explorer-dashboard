"""Curated outbound actions with verified routes and explicit host allowlists."""

import json
import re
from datetime import date, datetime, timedelta
from pathlib import Path
from urllib.parse import urlsplit

from rwa_pipeline.config import CONFIG_DIR, ConfigError
from rwa_pipeline.deals import clean_deal_page_url, clean_https_url


def _verified(value) -> bool:
    """Verification records identify both the calendar date and the tested sample."""
    if not isinstance(value, dict):
        return False
    at, sample = value.get("at"), value.get("sample")
    if not isinstance(at, str) or not re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}", at):
        return False
    try:
        date.fromisoformat(at)
    except ValueError:
        return False
    return isinstance(sample, str) and bool(sample.strip())


def load_links(path: Path = CONFIG_DIR / "links.json") -> dict | None:
    """A missing config disables links; an invalid config is an error."""
    try:
        links = json.loads(path.read_text())
    except FileNotFoundError:
        return None
    except (ValueError, OSError) as e:
        raise ConfigError(f"{path}: invalid links config: {e}") from e
    def require(condition, location, expected):
        if not condition:
            raise ConfigError(f"{path}: {location}: expected {expected}")

    def check_entry(entry, location):
        require(isinstance(entry, dict), location, "an object")
        if "verified" in entry:
            require(_verified(entry["verified"]), f"{location}.verified", "an object with an ISO date at and a nonempty sample")
        if "deal" in entry:
            require(isinstance(entry["deal"], str) and bool(entry["deal"].strip()), f"{location}.deal", "a nonempty string")
        if "hosts" in entry:
            hosts = entry["hosts"]
            require(isinstance(hosts, list), f"{location}.hosts", "a list of nonempty strings")
            for index, host in enumerate(hosts):
                require(isinstance(host, str) and bool(host), f"{location}.hosts[{index}]", "a nonempty string")

    require(isinstance(links, dict), "links", "an object")
    require(links.get("schema_version") == 1, "schema_version", "1")
    require(isinstance(links.get("platforms"), dict), "platforms", "an object")
    for section in ("issuers", "swap"):
        if section in links:
            require(isinstance(links[section], dict), section, "an object")
    if "deals_max_age_days" in links:
        age = links["deals_max_age_days"]
        require(
            isinstance(age, (int, float)) and not isinstance(age, bool) and age >= 0,
            "deals_max_age_days", "a nonnegative number",
        )
    for name, entry in links["platforms"].items():
        location = f"platforms.{name}"
        check_entry(entry, location)
        routes = entry.get("routes", [])
        require(isinstance(routes, list), f"{location}.routes", "a list")
        for index, route in enumerate(routes):
            route_location = f"{location}.routes[{index}]"
            require(isinstance(route, dict), route_location, "an object")
            require(isinstance(route.get("when", {}), dict), f"{route_location}.when", "an object")
            networks = route.get("networks", {})
            require(isinstance(networks, dict), f"{route_location}.networks", "an object")
            for network, settings in networks.items():
                check_entry(settings, f"{route_location}.networks.{network}")
    for name, entry in links.get("issuers", {}).items():
        check_entry(entry, f"issuers.{name}")
    for network, entries in links.get("swap", {}).items():
        location = f"swap.{network}"
        require(isinstance(entries, list), location, "a list")
        for index, entry in enumerate(entries):
            entry_location = f"{location}[{index}]"
            check_entry(entry, entry_location)
            platforms = entry.get("platforms", [])
            require(isinstance(platforms, list), f"{entry_location}.platforms", "a list of strings")
            for platform_index, platform in enumerate(platforms):
                require(isinstance(platform, str), f"{entry_location}.platforms[{platform_index}]", "a string")
    return links


def referenced_deals(links: dict | None) -> set[str]:
    """Return the deal slugs named by platform and issuer actions."""
    if links is None:
        return set()
    return {
        entry["deal"]
        for section in ("platforms", "issuers")
        for entry in links.get(section, {}).values()
        if "deal" in entry
    }


def _allowed(url, entry: dict) -> bool:
    hosts = entry.get("hosts", [])
    if not isinstance(hosts, list) or not clean_https_url(url):
        return False
    hostname = urlsplit(url).hostname
    return any(isinstance(host, str) and host == hostname for host in hosts)


def _render(template, values: dict) -> str | None:
    """Every placeholder must resolve to a plain identifier."""
    if not isinstance(template, str):
        return None

    def replace(match):
        value = values.get(match[1])
        if not isinstance(value, str) or not re.fullmatch(r"[A-Za-z0-9_.-]+", value):
            raise ValueError("unresolved identifier")
        return value

    try:
        result = re.sub(r"\{([^{}]+)\}", replace, template)
    except ValueError:
        return None
    return result if "{" not in result and "}" not in result else None


def _parse_timestamp(value) -> datetime | None:
    """Parse a timestamp only when it is a string with a UTC offset."""
    if not isinstance(value, str):
        return None
    try:
        timestamp = datetime.fromisoformat(value)
        return timestamp if timestamp.utcoffset() is not None else None
    except (ValueError, OverflowError):
        return None


def _fresh(deals, max_age_days, now: datetime) -> bool:
    if not isinstance(deals, dict) or not isinstance(deals.get("deals"), dict):
        return False
    fetched_at = _parse_timestamp(deals.get("fetched_at"))
    if fetched_at is None:
        return False
    try:
        return -timedelta(hours=1) <= now - fetched_at <= timedelta(days=max_age_days)
    except (TypeError, ValueError, OverflowError):
        return False


def attach_links(export: dict, links: dict | None, deals: dict | None, raw_records: dict, now: datetime) -> list[str]:
    """Attach independent direct, issuer, swap and referral actions in place."""
    warnings = []

    def warn(message):
        if message not in warnings:
            warnings.append(message)

    max_age_days = links.get("deals_max_age_days", 14) if links is not None else None
    fresh = links is not None and _fresh(deals, max_age_days, now)

    def referral(entry, name):
        if "deal" not in entry:
            return None
        slug = entry.get("deal")
        if not isinstance(slug, str) or not slug.strip():
            warn(f"{name}: invalid deals referral slug")
            return None
        deal = deals["deals"].get(slug) if fresh else None
        if not isinstance(deal, dict) or not _allowed(deal.get("link"), entry):
            warn(f"{name}: deals referral {slug} unavailable, stale or invalid")
            return None
        return {
            "url": deal["link"],
            "deal": slug,
            "bonus": deal.get("bonus"),
            "page_url": deal.get("page_url") if clean_deal_page_url(deal.get("page_url"), slug) else None,
            "as_of": deals["fetched_at"],
        }

    for position in export["opportunities"]:
        position["link"], position["referral"] = None, None
        entry = links["platforms"].get(position["platform"]) if links is not None else None
        if not entry:
            continue
        name = f"platform {position['platform']}"
        raw = raw_records.get(position["key"], {})
        metadata = raw.get("metadata") or {}
        values = {"address": position.get("address"), **{f"meta.{k}": v for k, v in metadata.items()}}
        for route in entry.get("routes", []):
            if any(position.get(k) != v for k, v in route.get("when", {}).items()):
                continue
            network = route.get("networks", {}).get(position["network"])
            if not network or not _verified(network.get("verified")):
                continue
            url = _render(route.get("url"), {**values, "network": network.get("slug")})
            if url is None:
                continue
            if not _allowed(url, entry):
                warn(f"{name}: invalid route URL")
                continue
            position["link"] = {"url": url, "kind": "position"}
            break
        if position["link"] is None:
            if _allowed(entry.get("home"), entry):
                position["link"] = {"url": entry["home"], "kind": "home"}
            else:
                warn(f"{name}: invalid home URL")
        position["referral"] = referral(entry, name)

    for issuer in export["issuers"]:
        issuer["app"], issuer["referral"] = None, None
        entry = links.get("issuers", {}).get(issuer["id"]) if links is not None else None
        if not entry:
            continue
        name = f"issuer {issuer['id']}"
        if _allowed(entry.get("app"), entry):
            issuer["app"] = {"url": entry["app"], "label": entry.get("label")}
        else:
            warn(f"{name}: invalid app URL")
        issuer["referral"] = referral(entry, name)

    opportunities = {o["key"]: o for o in export["opportunities"]}
    for token in export["tokens"]:
        token["links"] = []
        if links is None:
            continue
        pools = [
            o for relation in token["relations"]
            if relation["depth"] == 1 and (o := opportunities.get(relation["key"]))
            and o["category"] == "dex_lp" and o["eligibility"] == "eligible"
        ]
        for deployment in token["deployments"]:
            network = deployment.get("network")
            for entry in links.get("swap", {}).get(network, []):
                if not _verified(entry.get("verified")) or not deployment.get("key") or not any(
                    o["network"] == network and o["platform"] in entry.get("platforms", [])
                    and any(component.get("key") == deployment["key"] for component in o["components"])
                    for o in pools
                ):
                    continue
                url = _render(entry.get("url"), {"address": deployment.get("address"), "network": network})
                if url is None:
                    continue
                if not _allowed(url, entry):
                    for platform in entry.get("platforms", []):
                        warn(f"platform {platform}: invalid swap URL on {network}")
                    continue
                link = {"kind": "swap", "label": entry.get("label"), "url": url, "network": network}
                if link not in token["links"]:
                    token["links"].append(link)

    fetched_at = deals.get("fetched_at") if isinstance(deals, dict) else None
    export["summary"].update({
        "links_deals_fetched_at": fetched_at if _parse_timestamp(fetched_at) is not None else None,
        "links_deals_max_age_days": max_age_days,
        "links_warnings_count": len(warnings),
    })
    return warnings
