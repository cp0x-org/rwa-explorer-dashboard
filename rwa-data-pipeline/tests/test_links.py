"""Outbound links: where a position opens, where a token is obtained, and the separate cp0x referral action."""

import copy
import json
import re
from datetime import UTC, datetime
from pathlib import Path

import httpx
import pytest

from typer.testing import CliRunner

from rwa_pipeline import cli, collect, deals
from rwa_pipeline.config import Asset, ConfigError, load_settings, write_json
from rwa_pipeline.curated import attach_links, load_links
from rwa_pipeline.export import build_export, build_export_with_warnings

FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "nvda_snapshot.json").read_text())
NVDA = Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True)
NOW = datetime(2026, 11, 13, 12, 0, tzinfo=UTC)
VERIFIED = {"at": "2026-11-13", "sample": "robinhood:0xsample"}
BACKPACK = "issuer-backpack"
POOL_ID = "0x" + "ab" * 32
VAULT = "0x" + "cd" * 20


def links_config():
    return {
        "schema_version": 1,
        "deals_max_age_days": 14,
        "platforms": {
            "uniswap-v4": {
                "home": "https://app.uniswap.org",
                "hosts": ["app.uniswap.org"],
                "routes": [
                    {
                        "url": "https://app.uniswap.org/explore/pools/{network}/{address}",
                        "networks": {
                            "robinhood": {"slug": "robinhood", "verified": VERIFIED},
                            "bsc": {"slug": "bnb", "verified": VERIFIED},
                            "base": {"slug": "base"},  # listed but never opened in a browser: must not be used
                        },
                    }
                ],
            },
            "morpho": {
                "home": "https://app.morpho.org",
                "hosts": ["app.morpho.org"],
                "routes": [
                    {
                        "when": {"position_kind": "pool_id"},
                        "url": "https://app.morpho.org/{network}/market/{address}",
                        "networks": {"base": {"slug": "base", "verified": VERIFIED}},
                    },
                    {
                        "when": {"position_kind": "contract"},
                        "url": "https://app.morpho.org/{network}/vault/{address}",
                        "networks": {"base": {"slug": "base", "verified": VERIFIED}},
                    },
                ],
            },
            "beefy-clm": {
                "home": "https://app.beefy.com",
                "hosts": ["app.beefy.com"],
                "routes": [
                    {
                        "url": "https://app.beefy.com/vault/{meta.beefyClmId}",
                        "networks": {"robinhood": {"slug": "robinhood", "verified": VERIFIED}},
                    }
                ],
            },
            "venus": {"home": "https://app.venus.io", "hosts": ["app.venus.io"], "deal": "venus"},
        },
        "issuers": {BACKPACK: {"label": "Backpack", "app": "https://backpack.exchange", "hosts": ["backpack.exchange"], "deal": "backpack"}},
        "swap": {
            "base": [
                {
                    "platforms": ["uniswap-v4", "uniswap-v3"],
                    "label": "Uniswap",
                    "url": "https://app.uniswap.org/swap?chain=base&outputCurrency={address}",
                    "hosts": ["app.uniswap.org"],
                    "verified": VERIFIED,
                }
            ],
            "arbitrum": [
                {  # no `verified`: must not be used
                    "platforms": ["uniswap-v4", "uniswap-v3"],
                    "label": "Uniswap",
                    "url": "https://app.uniswap.org/swap?chain=arbitrum&outputCurrency={address}",
                    "hosts": ["app.uniswap.org"],
                }
            ],
        },
    }


def deals_cache(fetched_at="2026-11-12T12:00:00Z"):
    return {
        "fetched_at": fetched_at,
        "deals": {
            "backpack": {
                "title": "Backpack",
                "link": "https://backpack.exchange/refer/cp0x",
                "bonus": "+20% fee return",
                "page_url": "https://cp0x.com/deals/backpack",
            },
            "venus": {"title": "Venus", "link": "https://app.venus.io/?ref=cp0x", "bonus": "boost", "page_url": "https://cp0x.com/deals/venus"},
        },
    }


def opp(key, platform, kind="contract", category="dex_lp", eligibility="eligible", components=()):
    network, _, address = key.partition(":")
    return {
        "key": key,
        "network": network,
        "platform": platform,
        "address": address,
        "position_kind": kind,
        "category": category,
        "eligibility": eligibility,
        "components": [{"key": c, "symbol": None} for c in components],
    }


def export_of(*opps, tokens=(), issuers=()):
    return {"summary": {}, "opportunities": list(opps), "tokens": list(tokens), "issuers": list(issuers)}


def attach(export, links=None, cache="default", raw=None, now=NOW):
    return attach_links(export, links_config() if links is None else links, deals_cache() if cache == "default" else cache, raw or {}, now)


# ---------- position links ----------


def test_position_route_for_a_network_opened_in_a_browser():
    export = export_of(opp(f"robinhood:{POOL_ID}", "uniswap-v4", "pool_id"), opp(f"bsc:{POOL_ID}", "uniswap-v4", "pool_id"))
    assert attach(export) == []
    assert export["opportunities"][0]["link"] == {"url": f"https://app.uniswap.org/explore/pools/robinhood/{POOL_ID}", "kind": "position"}
    # the protocol's own name of the network goes into the URL, not ours
    assert export["opportunities"][1]["link"] == {"url": f"https://app.uniswap.org/explore/pools/bnb/{POOL_ID}", "kind": "position"}
    assert export["opportunities"][0]["referral"] is None


def test_unverified_or_unlisted_network_gets_the_home_link_only():
    export = export_of(opp(f"base:{POOL_ID}", "uniswap-v4", "pool_id"), opp(f"arbitrum:{POOL_ID}", "uniswap-v4", "pool_id"))
    assert attach(export) == []
    for o in export["opportunities"]:
        assert o["link"] == {"url": "https://app.uniswap.org", "kind": "home"}


def test_route_is_chosen_by_position_kind():
    export = export_of(opp(f"base:{POOL_ID}", "morpho", "pool_id"), opp(f"base:{VAULT}", "morpho", "contract"))
    assert attach(export) == []
    assert export["opportunities"][0]["link"]["url"] == f"https://app.morpho.org/base/market/{POOL_ID}"
    assert export["opportunities"][1]["link"]["url"] == f"https://app.morpho.org/base/vault/{VAULT}"


def test_meta_placeholder_is_read_from_the_raw_portals_record():
    key = f"robinhood:{VAULT}"
    export = export_of(opp(key, "beefy-clm"))
    raw = {key: {"key": key, "metadata": {"beefyClmId": "uniswap-cow-robinhood-usdg-gld"}}}
    assert attach(export, raw=raw) == []
    assert export["opportunities"][0]["link"] == {"url": "https://app.beefy.com/vault/uniswap-cow-robinhood-usdg-gld", "kind": "position"}

    # no raw record, or a record without the field: the placeholder cannot be resolved -> home, never a half-built URL
    for raw in ({}, {key: {"key": key, "metadata": {}}}, {key: {"key": key, "metadata": {"beefyClmId": None}}}):
        export = export_of(opp(key, "beefy-clm"))
        attach(export, raw=raw)
        assert export["opportunities"][0]["link"] == {"url": "https://app.beefy.com", "kind": "home"}


def test_platform_without_config_has_no_link_and_is_not_a_warning():
    export = export_of(opp(f"base:{VAULT}", "superform"))
    assert attach(export) == []
    assert export["opportunities"][0]["link"] is None
    assert export["opportunities"][0]["referral"] is None


def test_route_outside_declared_hosts_is_dropped_with_a_warning():
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["routes"][0]["url"] = "https://uniswap.example.org/pools/{network}/{address}"
    export = export_of(opp(f"robinhood:{POOL_ID}", "uniswap-v4", "pool_id"))
    warnings = attach(export, links=cfg)
    assert export["opportunities"][0]["link"] == {"url": "https://app.uniswap.org", "kind": "home"}
    assert len(warnings) == 1 and "uniswap-v4" in warnings[0]


@pytest.mark.parametrize("home", ["http://app.uniswap.org", "javascript:alert(1)", "https://app.uniswap.org/ x", "https://evil.example.org", "//app.uniswap.org", ""])
def test_home_that_is_not_a_clean_https_url_on_a_declared_host_gives_no_link(home):
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["home"] = home
    cfg["platforms"]["uniswap-v4"]["routes"] = []
    export = export_of(opp(f"robinhood:{POOL_ID}", "uniswap-v4", "pool_id"))
    warnings = attach(export, links=cfg)
    assert export["opportunities"][0]["link"] is None
    assert warnings and all("uniswap-v4" in w for w in warnings)


@pytest.mark.parametrize("address", ['0xabc" onclick="x', "0xabc def", "0xabc\n", "0xabc<script>", "0xabc/../../evil", "0xabc?next=https://evil.example.org", "0xabc#frag"])
def test_substituted_values_cannot_reshape_the_url(address):
    export = export_of(opp(f"robinhood:{address}", "uniswap-v4", "pool_id"))
    export["opportunities"][0]["address"] = address
    attach(export)
    # a value that is not a plain identifier never reaches a URL: the position link is not built
    assert export["opportunities"][0]["link"] == {"url": "https://app.uniswap.org", "kind": "home"}


# ---------- referral: a separate action, never instead of the direct link ----------


def test_referral_is_a_separate_object_and_the_direct_link_stays():
    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    assert attach(export) == []
    o = export["opportunities"][0]
    assert o["link"] == {"url": "https://app.venus.io", "kind": "home"}
    assert o["referral"] == {
        "url": "https://app.venus.io/?ref=cp0x",
        "deal": "venus",
        "bonus": "boost",
        "page_url": "https://cp0x.com/deals/venus",
        "as_of": "2026-11-12T12:00:00Z",
    }


def _venus_referral(cache):
    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    warnings = attach(export, cache=cache)
    return export["opportunities"][0], warnings


def test_referral_is_not_published_without_a_fresh_matching_deal():
    stale = deals_cache("2026-10-29T11:59:59Z")  # one second older than 14 days at NOW
    edge = deals_cache("2026-10-30T12:00:00Z")  # exactly 14 days: still valid
    missing = deals_cache()
    del missing["deals"]["venus"]
    foreign = deals_cache()
    foreign["deals"]["venus"]["link"] = "https://venus.example.org/?ref=cp0x"
    plain_http = deals_cache()
    plain_http["deals"]["venus"]["link"] = "http://app.venus.io/?ref=cp0x"
    no_scheme = deals_cache()
    no_scheme["deals"]["venus"]["link"] = "app.venus.io/?ref=cp0x"

    o, warnings = _venus_referral(edge)
    assert o["referral"] is not None and warnings == []

    for cache in (stale, missing, foreign, plain_http, no_scheme, None, {"fetched_at": "not a date", "deals": {}}):
        o, warnings = _venus_referral(cache)
        assert o["referral"] is None
        assert o["link"] == {"url": "https://app.venus.io", "kind": "home"}  # the direct link does not depend on the deal
        assert len(warnings) == 1 and "venus" in warnings[0]


def test_one_warning_per_platform_not_per_position():
    export = export_of(*(opp(f"bsc:0x{n:040x}", "venus") for n in range(1, 6)))
    warnings = attach(export, cache=None)
    assert len(warnings) == 1


def test_one_warning_per_broken_platform_entry_however_many_positions_use_it():
    five = [opp(f"robinhood:0x{n:064x}", "uniswap-v4", "pool_id") for n in range(1, 6)]

    bad_home = links_config()
    bad_home["platforms"]["uniswap-v4"]["home"] = "http://app.uniswap.org"
    bad_home["platforms"]["uniswap-v4"]["routes"] = []
    assert len(attach(export_of(*copy.deepcopy(five)), links=bad_home)) == 1

    bad_route = links_config()
    bad_route["platforms"]["uniswap-v4"]["routes"][0]["url"] = "https://uniswap.example.org/pools/{network}/{address}"
    assert len(attach(export_of(*copy.deepcopy(five)), links=bad_route)) == 1


# ---------- issuers and tokens ----------


def test_issuer_app_and_referral():
    export = export_of(issuers=[{"id": BACKPACK, "name": "Backpack"}, {"id": "issuer-other", "name": "Other"}])
    assert attach(export) == []
    backpack, other = export["issuers"]
    assert backpack["app"] == {"url": "https://backpack.exchange", "label": "Backpack"}
    assert backpack["referral"]["url"] == "https://backpack.exchange/refer/cp0x"
    assert backpack["referral"]["deal"] == "backpack"
    assert other["app"] is None and other["referral"] is None


def _token(token_id, deployments, relation_keys, depth=1):
    return {
        "id": token_id,
        "symbol": f"T{token_id}",
        "deployments": [{"network": n, "address": a, "key": f"{n}:{a}" if n else None, "status": "indexed"} for n, a in deployments],
        "relations": [{"key": k, "depth": depth, "path": []} for k in relation_keys],
    }


def test_swap_link_only_where_the_token_has_an_eligible_direct_pool_of_that_dex():
    base_pool, thin_pool, arb_pool, vault, cake_pool = (f"base:0x{n:040x}" for n in range(1, 6))
    arb_pool = arb_pool.replace("base:", "arbitrum:")
    here, there = f"base:{VAULT}", f"arbitrum:{VAULT}"
    opps = [
        opp(base_pool, "uniswap-v3", components=[here]),
        opp(thin_pool, "uniswap-v3", eligibility="low_liquidity", components=[here]),
        opp(arb_pool, "uniswap-v3", components=[there]),
        opp(vault, "beefy", category="vault", components=[here]),
        opp(cake_pool, "pancakeswap-v3", components=[here]),
    ]
    tokens = [
        _token(1, [("base", VAULT), ("arbitrum", VAULT), ("bsc", VAULT), (None, "SoLanaAddr111")], [base_pool, arb_pool]),
        _token(2, [("base", VAULT)], [thin_pool]),  # the only pool is below the liquidity bar
        _token(3, [("base", VAULT)], [vault]),  # a vault is not a place to swap
        _token(4, [("base", VAULT)], [cake_pool]),  # pool of another DEX: the Uniswap page would have no route
        _token(5, [("base", VAULT)], [base_pool], depth=2),  # reached only through another position
        _token(6, [], []),
    ]
    export = export_of(*opps, tokens=tokens)
    assert attach(export) == []
    assert export["tokens"][0]["links"] == [
        {"kind": "swap", "label": "Uniswap", "url": f"https://app.uniswap.org/swap?chain=base&outputCurrency={VAULT}", "network": "base"}
    ]  # arbitrum: template never opened in a browser; bsc: no template; Solana: no network
    for t in export["tokens"][1:]:
        assert t["links"] == []


def test_swap_link_belongs_to_the_contract_that_is_actually_in_the_pool():
    pool = f"base:0x{9:040x}"
    first, second = "0x" + "11" * 20, "0x" + "22" * 20
    export = export_of(
        opp(pool, "uniswap-v3", components=[f"base:{first}", "base:0x" + "ee" * 20]),
        tokens=[_token(1, [("base", first), ("base", second)], [pool])],
    )
    assert attach(export) == []
    # the token has two contracts on Base; only the one that sits in the pool gets a swap link
    assert [link["url"] for link in export["tokens"][0]["links"]] == [f"https://app.uniswap.org/swap?chain=base&outputCurrency={first}"]


def test_summary_carries_the_deals_timestamp_and_the_warnings_count():
    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    attach(export)
    assert export["summary"]["links_deals_fetched_at"] == "2026-11-12T12:00:00Z"
    assert export["summary"]["links_deals_max_age_days"] == 14  # the site hides a referral whose cache has aged since the build
    assert export["summary"]["links_warnings_count"] == 0

    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    attach(export, cache=None)
    assert export["summary"]["links_deals_fetched_at"] is None
    assert export["summary"]["links_warnings_count"] == 1


def test_without_a_links_config_every_field_is_present_and_empty():
    token = _token(1, [("base", VAULT)], [f"base:{POOL_ID}"])
    pools = [opp(f"robinhood:{POOL_ID}", "uniswap-v4", "pool_id"), opp(f"base:{POOL_ID}", "uniswap-v4", "pool_id", components=[f"base:{VAULT}"])]
    export = export_of(*pools, tokens=[token], issuers=[{"id": BACKPACK, "name": "Backpack"}])
    assert attach_links(export, None, deals_cache(), {}, NOW) == []
    assert export["opportunities"][0]["link"] is None and export["opportunities"][0]["referral"] is None
    assert export["tokens"][0]["links"] == []
    assert export["issuers"][0]["app"] is None and export["issuers"][0]["referral"] is None
    assert export["summary"]["links_warnings_count"] == 0
    assert export["summary"]["links_deals_max_age_days"] is None


# ---------- config file ----------


def test_load_links_absent_file_is_none_and_a_broken_file_is_a_config_error(tmp_path):
    assert load_links(tmp_path / "links.json") is None

    path = tmp_path / "links.json"
    path.write_text("{not json")
    with pytest.raises(ConfigError):
        load_links(path)
    for broken in ([], {"schema_version": 2, "platforms": {}}, {"schema_version": 1, "platforms": []}, {"schema_version": 1, "platforms": {}, "issuers": 5}, {"schema_version": 1, "platforms": {}, "swap": []}):
        write_json(path, broken)
        with pytest.raises(ConfigError):
            load_links(path)

    write_json(path, links_config())
    assert load_links(path) == links_config()


def test_the_shipped_links_config_loads():
    # config/links.json is part of the repository; a typo there must fail the tests, not the nightly build
    assert isinstance(load_links(), dict)


# ---------- build_export wiring ----------


def _data_dir(tmp_path):
    data_dir = tmp_path / "data"
    write_json(collect.snapshot_path(data_dir, NVDA), copy.deepcopy(FIXTURE))
    return data_dir


def test_build_export_attaches_links_from_the_config_dir(tmp_path):
    data_dir = _data_dir(tmp_path)
    config_dir = tmp_path / "config"
    cfg = links_config()
    cfg["platforms"]["uniswap-v3"] = {
        "home": "https://app.uniswap.org",
        "hosts": ["app.uniswap.org"],
        "routes": [{"url": "https://app.uniswap.org/explore/pools/{network}/{address}", "networks": {"base": {"slug": "base", "verified": VERIFIED}}}],
    }
    cfg["platforms"]["beefy-clm"]["routes"][0]["networks"]["base"] = {"slug": "base", "verified": VERIFIED}
    write_json(config_dir / "links.json", cfg)
    write_json(data_dir / deals.DEALS_FILE, deals_cache(datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")))

    export, warnings = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=config_dir)
    assert warnings == []

    def of(platform, network):
        found = [o for o in export["opportunities"] if o["platform"] == platform and o["network"] == network]
        assert found, f"the NVDA fixture has {platform} positions on {network}"
        return found

    for o in of("uniswap-v3", "base"):
        assert o["link"] == {"url": f"https://app.uniswap.org/explore/pools/base/{o['address']}", "kind": "position"}
    # the Beefy CLM id lives only in the raw Portals record of the snapshot
    for o in of("beefy-clm", "base"):
        assert o["link"]["kind"] == "position" and re.fullmatch(r"https://app\.beefy\.com/vault/[a-z0-9-]+", o["link"]["url"])
    # base is listed for uniswap-v4 in this config but was never opened in a browser; ethereum is not listed at all
    for o in of("uniswap-v4", "base") + of("uniswap-v4", "ethereum"):
        assert o["link"] == {"url": "https://app.uniswap.org", "kind": "home"}
    assert all(o["link"] is None for o in of("spectra", "base"))
    # NVDAc trades in a Uniswap V3 pool on Base above the liquidity bar -> one swap link
    nvdac = next(t for t in export["tokens"] if t["symbol"] == "NVDAc")
    assert [link["network"] for link in nvdac["links"]] == ["base"]
    assert all("links" in t for t in export["tokens"]) and all("app" in i and "referral" in i for i in export["issuers"])
    assert build_export([NVDA], load_settings(), data_dir, config_dir=config_dir)["opportunities"] == export["opportunities"]


def test_build_export_without_links_file_and_without_deals_cache(tmp_path):
    data_dir = _data_dir(tmp_path)
    export, warnings = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=tmp_path / "empty-config")
    assert warnings == []
    assert all(o["link"] is None and o["referral"] is None for o in export["opportunities"])
    assert export["summary"]["links_deals_fetched_at"] is None


# ---------- cp0x deals cache ----------


def test_parse_deals_keeps_only_well_formed_https_deals():
    body = {
        "deals": [
            {"slug": "hyperliquid", "title": "Hyperliquid", "link": "https://app.hyperliquid.xyz/join/CP0X", "bonus": "4% off", "page_url": "https://cp0x.com/deals/hyperliquid", "description": "long text"},
            {"slug": "backpack", "title": "Backpack", "link": "http://backpack.exchange/refer/cp0x", "bonus": "x", "page_url": "https://cp0x.com/deals/backpack"},
            {"slug": "lighter", "title": "Lighter", "link": "robinhoodchain.lighter.xyz/?referral=CP0XCOOL", "bonus": "x", "page_url": "https://cp0x.com/deals/lighter"},
            {"title": "No slug", "link": "https://example.org"},
            {"slug": "no-link", "title": "No link"},
            "junk",
        ]
    }
    parsed, skipped = deals.parse_deals(body)
    assert parsed == {
        "hyperliquid": {"title": "Hyperliquid", "link": "https://app.hyperliquid.xyz/join/CP0X", "bonus": "4% off", "page_url": "https://cp0x.com/deals/hyperliquid"}
    }
    assert sorted(skipped) == ["backpack", "lighter", "no-link"]  # named so the build can say which deals were unusable

    for shapeless in (None, [], {"deals": "x"}, {"other": []}):
        with pytest.raises(deals.DealsError):
            deals.parse_deals(shapeless)


def test_refresh_deals_writes_the_cache_and_keeps_the_old_one_on_failure(tmp_path):
    body = {"deals": [{"slug": "hyperliquid", "title": "Hyperliquid", "link": "https://app.hyperliquid.xyz/join/CP0X", "bonus": "4% off", "page_url": "https://cp0x.com/deals/hyperliquid"}]}
    cache, warning = deals.refresh_deals(tmp_path, fetch=lambda: body, now=NOW)
    assert warning is None
    path = tmp_path / deals.DEALS_FILE
    assert json.loads(path.read_text()) == cache
    assert cache["fetched_at"] == "2026-11-13T12:00:00Z" and list(cache["deals"]) == ["hyperliquid"]
    before = path.read_bytes()

    def failing():
        raise deals.DealsError("HTTP 503")

    for fetch in (failing, lambda: {"deals": "x"}, lambda: None):
        cache, warning = deals.refresh_deals(tmp_path, fetch=fetch, now=NOW)
        assert cache is None and warning and "deals" in warning
        assert path.read_bytes() == before  # the last good cache survives a bad answer


def test_the_links_config_names_the_deals_it_uses():
    from rwa_pipeline.curated import referenced_deals

    assert referenced_deals(links_config()) == {"venus", "backpack"}
    assert referenced_deals(None) == set()
    assert referenced_deals({"schema_version": 1, "platforms": {}, "issuers": {}}) == set()


def test_the_cache_keeps_only_the_deals_the_site_uses(tmp_path):
    # the source lists every cp0x deal; the repository needs the ones its links point to and nothing else
    body = {
        "deals": [
            {"slug": "hyperliquid", "title": "Hyperliquid", "link": "https://app.hyperliquid.xyz/join/CP0X", "bonus": "4% off", "page_url": "https://cp0x.com/deals/hyperliquid"},
            {"slug": "some-delegation", "title": "Delegate", "link": "https://vote.example.org/delegates/0xabc", "page_url": "https://cp0x.com/deals/some-delegation"},
            {"slug": "broken-unused", "title": "Broken", "link": "example.org/no-scheme"},
            {"slug": "broken-used", "title": "Broken", "link": "example.org/no-scheme"},
        ]
    }
    cache, warning = deals.refresh_deals(tmp_path, fetch=lambda: body, now=NOW, keep={"hyperliquid", "broken-used", "absent-from-the-source"})
    assert list(cache["deals"]) == ["hyperliquid"]
    text = (tmp_path / deals.DEALS_FILE).read_text()
    assert json.loads(text) == cache
    assert "some-delegation" not in text and "0xabc" not in text
    # a broken link is worth a warning only if the site would have used it
    assert warning and "broken-used" in warning and "broken-unused" not in warning

    cache, warning = deals.refresh_deals(tmp_path, fetch=lambda: body, now=NOW, keep=set())
    assert cache["deals"] == {} and warning is None and cache["fetched_at"] == "2026-11-13T12:00:00Z"

    # without a filter the function keeps every usable deal, as before
    cache, _ = deals.refresh_deals(tmp_path, fetch=lambda: body, now=NOW)
    assert set(cache["deals"]) == {"hyperliquid", "some-delegation"}


def test_fetch_deals_reads_json_and_turns_every_failure_into_a_deals_error():
    ok = httpx.Client(transport=httpx.MockTransport(lambda r: httpx.Response(200, json={"deals": []})))
    assert deals.fetch_deals(ok) == {"deals": []}
    assert deals.DEALS_URL == "https://cp0x.com/api/v1/deals"

    def boom(request):
        raise httpx.ConnectError("no route")

    for transport in (
        httpx.MockTransport(lambda r: httpx.Response(503, text="down")),
        httpx.MockTransport(lambda r: httpx.Response(200, text="<html>not json</html>")),
        httpx.MockTransport(boom),
    ):
        with pytest.raises(deals.DealsError):
            deals.fetch_deals(httpx.Client(transport=transport))


# ---------- hardening: shapes that must fail closed ----------


def test_first_applicable_route_wins():
    cfg = links_config()
    cfg["platforms"]["morpho"]["routes"].append(
        {"url": "https://app.morpho.org/{network}/any/{address}", "networks": {"base": {"slug": "base", "verified": VERIFIED}}}
    )
    export = export_of(opp(f"base:{POOL_ID}", "morpho", "pool_id"), opp(f"base:{VAULT}", "morpho", "contract"))
    assert attach(export, links=cfg) == []
    assert export["opportunities"][0]["link"]["url"] == f"https://app.morpho.org/base/market/{POOL_ID}"
    assert export["opportunities"][1]["link"]["url"] == f"https://app.morpho.org/base/vault/{VAULT}"


@pytest.mark.parametrize("hosts", ["app.uniswap.org", None, {"app.uniswap.org": True}, [], 5, [["uniswap.org"]], [None]])
def test_hosts_that_are_not_a_list_of_host_names_allow_nothing(hosts):
    # a bare string would turn "host in hosts" into a substring test: "uniswap.org" in "app.uniswap.org"
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["hosts"] = hosts
    cfg["platforms"]["uniswap-v4"]["home"] = "https://uniswap.org"
    cfg["platforms"]["uniswap-v4"]["routes"] = []
    export = export_of(opp(f"robinhood:{POOL_ID}", "uniswap-v4", "pool_id"))
    warnings = attach(export, links=cfg)
    assert export["opportunities"][0]["link"] is None
    assert warnings and all("uniswap-v4" in w for w in warnings)


def _set(path, value):
    def mutate(cfg):
        node = cfg
        for key in path[:-1]:
            node = node[key]
        node[path[-1]] = value

    return mutate


@pytest.mark.parametrize(
    "mutate",
    [
        _set(["platforms", "uniswap-v4"], "https://app.uniswap.org"),
        _set(["platforms", "uniswap-v4", "hosts"], "app.uniswap.org"),
        _set(["platforms", "uniswap-v4", "hosts"], [5]),
        _set(["platforms", "uniswap-v4", "routes"], {}),
        _set(["platforms", "uniswap-v4", "routes", 0], "https://app.uniswap.org/{address}"),
        _set(["platforms", "uniswap-v4", "routes", 0, "networks"], ["robinhood"]),
        _set(["platforms", "uniswap-v4", "routes", 0, "networks", "robinhood"], "robinhood"),
        _set(["platforms", "morpho", "routes", 0, "when"], "pool_id"),
        _set(["issuers", BACKPACK], ["https://backpack.exchange"]),
        _set(["swap", "base"], {"label": "Uniswap"}),
        _set(["swap", "base", 0], "https://app.uniswap.org/swap"),
        _set(["swap", "base", 0, "platforms"], "uniswap-v3"),
        _set(["deals_max_age_days"], "14"),
        _set(["deals_max_age_days"], -1),
        _set(["deals_max_age_days"], True),
    ],
)
def test_load_links_rejects_entries_of_the_wrong_shape(tmp_path, mutate):
    cfg = links_config()
    mutate(cfg)
    path = tmp_path / "links.json"
    write_json(path, cfg)
    with pytest.raises(ConfigError):
        load_links(path)


@pytest.mark.parametrize("fetched_at", ["not a date", "2026-11-12", 5, None, "2026-11-12T12:00:00"])
def test_summary_timestamp_is_published_only_when_it_is_a_utc_timestamp(fetched_at):
    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    attach(export, cache={"fetched_at": fetched_at, "deals": deals_cache()["deals"]})
    assert export["summary"]["links_deals_fetched_at"] is None
    assert export["opportunities"][0]["referral"] is None


def test_swap_address_that_is_not_an_identifier_is_skipped_without_a_warning():
    pool = f"base:0x{1:040x}"
    export = export_of(opp(pool, "uniswap-v3", components=["base:0xabc def", "base:None"]), tokens=[_token(1, [("base", "0xabc def"), ("base", None)], [pool])])
    assert attach(export) == []
    assert export["tokens"][0]["links"] == []


def test_broken_deal_records_in_the_cache_do_not_crash_the_build():
    cache = deals_cache()
    cache["deals"] = {"venus": "https://app.venus.io/?ref=cp0x", "backpack": None}
    export = export_of(opp(f"bsc:{VAULT}", "venus"), issuers=[{"id": BACKPACK, "name": "Backpack"}])
    warnings = attach(export, cache=cache)
    assert export["opportunities"][0]["referral"] is None and export["issuers"][0]["referral"] is None
    assert len(warnings) == 2

    cache["deals"] = ["venus"]
    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    attach(export, cache=cache)
    assert export["opportunities"][0]["referral"] is None


# ---------- review round: verification records, deal records, clocks ----------


@pytest.mark.parametrize("verified", ["false", True, 1, {}, [], {"at": "2026-11-13"}, {"sample": "robinhood:0x1"}, {"at": "yesterday", "sample": "robinhood:0x1"}, {"at": "2026-11-13", "sample": ""}])
def test_only_a_complete_verification_record_enables_a_route(tmp_path, verified):
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["routes"][0]["networks"]["robinhood"]["verified"] = verified
    export = export_of(opp(f"robinhood:{POOL_ID}", "uniswap-v4", "pool_id"))
    attach(export, links=cfg)
    assert export["opportunities"][0]["link"] == {"url": "https://app.uniswap.org", "kind": "home"}
    # and the file itself is refused: "verified" is either absent or a record of when and on what it was opened
    write_json(tmp_path / "links.json", cfg)
    with pytest.raises(ConfigError):
        load_links(tmp_path / "links.json")


@pytest.mark.parametrize("verified", ["false", True, {"at": "2026-11-13"}])
def test_only_a_complete_verification_record_enables_a_swap_link(tmp_path, verified):
    cfg = links_config()
    cfg["swap"]["base"][0]["verified"] = verified
    pool = f"base:0x{1:040x}"
    export = export_of(opp(pool, "uniswap-v3", components=[f"base:{VAULT}"]), tokens=[_token(1, [("base", VAULT)], [pool])])
    attach(export, links=cfg)
    assert export["tokens"][0]["links"] == []
    write_json(tmp_path / "links.json", cfg)
    with pytest.raises(ConfigError):
        load_links(tmp_path / "links.json")


@pytest.mark.parametrize("deal", [["venus"], {"slug": "venus"}, 5, True])
def test_a_deal_that_is_not_a_slug_is_refused_and_never_crashes_the_build(tmp_path, deal):
    cfg = links_config()
    cfg["platforms"]["venus"]["deal"] = deal
    export = export_of(opp(f"bsc:{VAULT}", "venus"))
    warnings = attach(export, links=cfg)
    assert export["opportunities"][0]["referral"] is None
    assert export["opportunities"][0]["link"] == {"url": "https://app.venus.io", "kind": "home"}
    assert len(warnings) == 1 and "venus" in warnings[0]
    write_json(tmp_path / "links.json", cfg)
    with pytest.raises(ConfigError):
        load_links(tmp_path / "links.json")


def test_a_deals_cache_from_the_future_is_not_fresh():
    slightly_ahead = deals_cache("2026-11-13T12:10:00Z")  # ten minutes of clock skew between writer and reader
    far_ahead = deals_cache("2026-11-13T14:00:01Z")  # written by a machine whose clock ran ahead
    o, warnings = _venus_referral(slightly_ahead)
    assert o["referral"] is not None and warnings == []
    o, warnings = _venus_referral(far_ahead)
    assert o["referral"] is None and len(warnings) == 1


def test_deal_page_must_be_a_cp0x_deal_page():
    body = {
        "deals": [
            {"slug": "a", "link": "https://a.example/ref", "page_url": "https://cp0x.com/deals/a"},
            {"slug": "b", "link": "https://b.example/ref", "page_url": "https://evil.example/deals/b"},
            {"slug": "c", "link": "https://c.example/ref", "page_url": "https://cp0x.com.evil.example/deals/c"},
            {"slug": "d", "link": "https://d.example/ref", "page_url": "https://cp0x.com/"},
            {"slug": "e", "link": "https://e.example/ref", "page_url": "https://cp0x.com/deals/e?next=https://evil.example"},
        ]
    }
    parsed, skipped = deals.parse_deals(body)
    assert skipped == []
    assert parsed["a"]["page_url"] == "https://cp0x.com/deals/a"
    assert [parsed[slug]["page_url"] for slug in "bcde"] == [None, None, None, None]

    # a cache edited by hand gets the same check when the referral is published
    cache = deals_cache()
    cache["deals"]["venus"]["page_url"] = "https://evil.example/deals/venus"
    o, warnings = _venus_referral(cache)
    assert o["referral"]["url"] == "https://app.venus.io/?ref=cp0x" and o["referral"]["page_url"] is None
    assert warnings == []


def test_deal_page_is_the_page_of_that_very_deal():
    body = {
        "deals": [
            {"slug": "a", "link": "https://a.example/ref", "page_url": "https://cp0x.com/deals/a"},
            {"slug": "b", "link": "https://b.example/ref", "page_url": "https://cp0x.com/deals/a"},  # somebody else's page
            {"slug": "c", "link": "https://c.example/ref", "page_url": "https://cp0x.com/deals/c/extra"},
            {"slug": "d", "link": "https://d.example/ref", "page_url": "https://cp0x.com/deals/../deals/d"},
            {"slug": "e", "link": "https://e.example/ref", "page_url": "https://cp0x.com/deals/e/"},
            {"slug": "f", "link": "https://f.example/ref", "page_url": "https://cp0x.com/deals/F"},
        ]
    }
    parsed, skipped = deals.parse_deals(body)
    assert skipped == []
    assert parsed["a"]["page_url"] == "https://cp0x.com/deals/a"
    assert [parsed[slug]["page_url"] for slug in "bcdef"] == [None, None, None, None, None]

    # the bonus of one deal is never published next to the page of another, whatever the cache says
    cache = deals_cache()
    cache["deals"]["venus"]["page_url"] = "https://cp0x.com/deals/backpack"
    o, warnings = _venus_referral(cache)
    assert o["referral"]["url"] == "https://app.venus.io/?ref=cp0x" and o["referral"]["page_url"] is None
    assert warnings == []
    o, _ = _venus_referral(deals_cache())
    assert o["referral"]["page_url"] == "https://cp0x.com/deals/venus"


def test_raw_records_come_from_the_newest_snapshot_whatever_the_asset_order():
    from rwa_pipeline.analyze import norm_key
    from rwa_pipeline.export import raw_portals_records

    mixed ="base:0x" + "AbCdEf0123" * 4  # a checksummed address: both spellings are one position
    key = mixed.lower()
    assert norm_key(mixed) == key
    old = {"fetched_at": "2026-10-01T00:00:00Z", "portals": {"records": {mixed: {"key": mixed, "metadata": {"beefyClmId": "old"}}}}}
    new = {"fetched_at": "2026-10-18T00:00:00Z", "portals": {"exact": {}, "records": {key: {"key": key, "metadata": {"beefyClmId": "new"}}}}}
    for order in ([old, new], [new, old]):
        records = raw_portals_records(order)
        assert list(records) == [key]
        assert records[key]["metadata"]["beefyClmId"] == "new"  # same rule as the analytics: newest wins
    empty = {"fetched_at": None, "portals": None}
    assert raw_portals_records([empty, new, {"fetched_at": "2026-10-19T00:00:00Z"}])[key]["metadata"]["beefyClmId"] == "new"


def test_an_unreadable_deals_cache_is_an_unavailable_cache(tmp_path):
    data_dir = _data_dir(tmp_path)
    (data_dir / deals.DEALS_FILE).mkdir()  # a directory where the cache file should be
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["deal"] = "venus"
    cfg["platforms"]["uniswap-v4"]["hosts"] = ["app.uniswap.org", "app.venus.io"]
    write_json(tmp_path / "config" / "links.json", cfg)
    export, warnings = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=tmp_path / "config")
    assert all(o["referral"] is None for o in export["opportunities"])
    assert len(warnings) == 1 and "uniswap-v4" in warnings[0]
    assert export["summary"]["links_deals_fetched_at"] is None


def test_tables_carry_the_links(tmp_path):
    from rwa_pipeline.tables import build_tables

    data_dir = _data_dir(tmp_path)
    cfg = links_config()
    cfg["platforms"]["uniswap-v3"] = {
        "home": "https://app.uniswap.org",
        "hosts": ["app.uniswap.org"],
        "routes": [{"url": "https://app.uniswap.org/explore/pools/{network}/{address}", "networks": {"base": {"slug": "base", "verified": VERIFIED}}}],
    }
    write_json(tmp_path / "config" / "links.json", cfg)
    export, _ = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=tmp_path / "config")
    tables = {t.name: t for t in build_tables(export)}

    by_key = {o["key"]: o for o in export["opportunities"]}
    rows = tables["opportunities"].rows
    assert len(rows) == len(by_key)
    for row in rows:
        o = by_key[row["opportunity_key"]]
        assert row["link_url"] == (o["link"] or {}).get("url")
        assert row["link_kind"] == (o["link"] or {}).get("kind")
        assert row["referral_url"] == (o["referral"] or {}).get("url")
    assert any(row["link_kind"] == "position" for row in rows) and any(row["link_url"] is None for row in rows)

    token_rows = {row["id"]: row for row in tables["tokens"].rows}
    for t in export["tokens"]:
        assert token_rows[t["id"]]["swap_urls"] == ("; ".join(link["url"] for link in t["links"]) or None)
    issuer_rows = {row["id"]: row for row in tables["issuers"].rows}
    for i in export["issuers"]:
        assert issuer_rows[i["id"]]["app_url"] == (i["app"] or {}).get("url")
        assert issuer_rows[i["id"]]["referral_url"] == (i["referral"] or {}).get("url")


# ---------- CLI wiring ----------

HYPERLIQUID_BODY = {
    "deals": [{"slug": "hyperliquid", "title": "Hyperliquid", "link": "https://app.hyperliquid.xyz/join/CP0X", "bonus": "4% off", "page_url": "https://cp0x.com/deals/hyperliquid"}]
}


def _cli(tmp_path, command):
    args = [command, "--data-dir", str(tmp_path / "data"), "--frontend-dir", str(tmp_path / "no-site"), "--config-dir", str(tmp_path / "config")]
    return CliRunner().invoke(cli.app, args)


def test_cli_build_prints_link_warnings_and_still_succeeds(tmp_path):
    _data_dir(tmp_path)
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["home"] = "http://app.uniswap.org"
    cfg["platforms"]["uniswap-v4"]["routes"] = []
    write_json(tmp_path / "config" / "links.json", cfg)

    result = _cli(tmp_path, "build")
    assert result.exit_code == 0, result.output
    assert any("warning" in line and "uniswap-v4" in line for line in result.output.splitlines())
    written = json.loads((tmp_path / "data" / "exports" / "rwa.json").read_text())
    assert written["summary"]["links_warnings_count"] >= 1
    assert all(o["link"] is None for o in written["opportunities"] if o["platform"] == "uniswap-v4")


def test_cli_build_with_a_broken_links_file_is_a_config_error(tmp_path):
    _data_dir(tmp_path)
    (tmp_path / "config").mkdir()
    (tmp_path / "config" / "links.json").write_text("{not json")
    result = _cli(tmp_path, "build")
    assert result.exit_code == 1
    assert not (tmp_path / "data" / "exports" / "rwa.json").exists()


def test_cli_deals_refreshes_the_cache_then_builds_and_survives_a_dead_source(tmp_path, monkeypatch):
    data_dir = _data_dir(tmp_path)
    cfg = links_config()
    cfg["issuers"] = {BACKPACK: {"label": "Hyperliquid", "app": "https://app.hyperliquid.xyz", "hosts": ["app.hyperliquid.xyz"], "deal": "hyperliquid"}}
    write_json(tmp_path / "config" / "links.json", cfg)
    source = {"deals": HYPERLIQUID_BODY["deals"] + [{"slug": "unused", "title": "Unused", "link": "https://unused.example/ref", "page_url": "https://cp0x.com/deals/unused"}]}
    # replaced on the module: the command must look the fetcher up at call time
    monkeypatch.setattr(deals, "fetch_deals", lambda *args, **kwargs: source)
    result = _cli(tmp_path, "deals")
    assert result.exit_code == 0, result.output
    cache_path = data_dir / deals.DEALS_FILE
    assert list(json.loads(cache_path.read_text())["deals"]) == ["hyperliquid"]  # only what the links config names
    assert "unused" not in cache_path.read_text()
    assert (data_dir / "exports" / "rwa.json").exists()
    before = cache_path.read_bytes()

    def down(*args, **kwargs):
        raise deals.DealsError("HTTP 503")

    monkeypatch.setattr(deals, "fetch_deals", down)
    result = _cli(tmp_path, "deals")
    assert result.exit_code == 0, result.output
    assert any("warning" in line and "deals" in line for line in result.output.splitlines())
    assert cache_path.read_bytes() == before


def test_cli_deals_without_a_links_config_stores_no_deals(tmp_path, monkeypatch):
    data_dir = _data_dir(tmp_path)
    monkeypatch.setattr(deals, "fetch_deals", lambda *args, **kwargs: HYPERLIQUID_BODY)
    result = _cli(tmp_path, "deals")
    assert result.exit_code == 0, result.output
    cache = json.loads((data_dir / deals.DEALS_FILE).read_text())
    assert cache["deals"] == {} and cache["fetched_at"]
    assert (data_dir / "exports" / "rwa.json").exists()


def test_cli_sync_stores_only_the_named_deals(tmp_path, monkeypatch):
    data_dir = _data_dir(tmp_path)
    cfg = links_config()
    cfg["issuers"] = {BACKPACK: {"label": "Hyperliquid", "app": "https://app.hyperliquid.xyz", "hosts": ["app.hyperliquid.xyz"], "deal": "hyperliquid"}}
    write_json(tmp_path / "config" / "links.json", cfg)
    source = {"deals": HYPERLIQUID_BODY["deals"] + [{"slug": "unused", "title": "Unused", "link": "https://unused.example/ref"}]}
    report = {"run_id": "20261113T120000Z", "mode": "refresh", "completed": [{"ticker": "NVDA", "status": "ok"}], "failed": {}, "warnings": [], "requests": {"cmc": 0, "portals": 0}, "cmc_credits": 0}
    monkeypatch.setattr(deals, "fetch_deals", lambda *args, **kwargs: source)
    monkeypatch.setattr(cli, "run_sync", lambda *args, **kwargs: (0, report))
    monkeypatch.setattr(cli, "_logos", lambda data_dir: None)
    result = _cli(tmp_path, "sync")
    assert result.exit_code == 0, result.output
    assert list(json.loads((data_dir / deals.DEALS_FILE).read_text())["deals"]) == ["hyperliquid"]


def test_cli_sync_refreshes_deals_once_before_the_build(tmp_path, monkeypatch):
    data_dir = _data_dir(tmp_path)
    calls = []

    def fetch(*args, **kwargs):
        calls.append("fetch")
        return HYPERLIQUID_BODY

    report = {
        "run_id": "20261113T120000Z",
        "mode": "refresh",
        "completed": [{"ticker": "NVDA", "status": "ok"}],
        "failed": {},
        "warnings": [],
        "requests": {"cmc": 0, "portals": 0},
        "cmc_credits": 0,
    }
    monkeypatch.setattr(deals, "fetch_deals", fetch)
    monkeypatch.setattr(cli, "run_sync", lambda *args, **kwargs: (0, report))
    monkeypatch.setattr(cli, "_logos", lambda data_dir: None)
    result = _cli(tmp_path, "sync")
    assert result.exit_code == 0, result.output
    assert calls == ["fetch"]
    written = json.loads((data_dir / "exports" / "rwa.json").read_text())
    assert written["summary"]["links_deals_fetched_at"] == json.loads((data_dir / deals.DEALS_FILE).read_text())["fetched_at"]


def test_cli_logos_rebuilds_with_the_selected_config_dir(tmp_path, monkeypatch):
    _data_dir(tmp_path)
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["home"] = "http://app.uniswap.org"
    cfg["platforms"]["uniswap-v4"]["routes"] = []
    write_json(tmp_path / "config" / "links.json", cfg)
    monkeypatch.setattr(cli, "_logos", lambda data_dir: None)
    monkeypatch.setattr(cli, "fill_missing_rwa_info", lambda *args, **kwargs: None)
    result = _cli(tmp_path, "logos")
    assert result.exit_code == 0, result.output
    assert any("warning" in line and "uniswap-v4" in line for line in result.output.splitlines())


def test_cli_sync_writes_deal_and_link_warnings_into_the_run_report(tmp_path, monkeypatch):
    data_dir = _data_dir(tmp_path)
    cfg = links_config()
    cfg["platforms"]["uniswap-v4"]["home"] = "http://app.uniswap.org"
    cfg["platforms"]["uniswap-v4"]["routes"] = []
    write_json(tmp_path / "config" / "links.json", cfg)
    report = {
        "run_id": "20261113T120000Z",
        "mode": "refresh",
        "completed": [{"ticker": "NVDA", "status": "ok"}],
        "failed": {},
        "warnings": ["a warning of the collection itself"],
        "requests": {"cmc": 0, "portals": 0},
        "cmc_credits": 0,
    }
    report_path = data_dir / "runs" / "20261113T120000Z.json"

    def run_sync(*args, **kwargs):
        write_json(report_path, report)  # the collection writes its report before the build starts
        return 0, copy.deepcopy(report)

    def down(*args, **kwargs):
        raise deals.DealsError("HTTP 503")

    monkeypatch.setattr(cli, "run_sync", run_sync)
    monkeypatch.setattr(cli, "_logos", lambda data_dir: None)
    monkeypatch.setattr(deals, "fetch_deals", down)
    result = _cli(tmp_path, "sync")
    assert result.exit_code == 0, result.output
    saved = json.loads(report_path.read_text())["warnings"]
    assert saved[0] == "a warning of the collection itself"
    assert any("deals" in w for w in saved[1:]) and any("uniswap-v4" in w for w in saved[1:])
