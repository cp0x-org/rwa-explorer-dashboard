"""Display names without stray whitespace, versions whose symbol differs from the ticker, networks and issuer details."""

import copy
import json
from pathlib import Path

import pytest
from typer.testing import CliRunner

from rwa_pipeline import cli, cmc, collect, portals
from rwa_pipeline.analyze import classify_token
from rwa_pipeline.config import CONFIG_DIR, DEFAULT_DATA_DIR, Asset, ConfigError, load_assets, load_settings, write_json
from rwa_pipeline.export import build_export_with_warnings
from rwa_pipeline.logos import find_logo

FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "nvda_snapshot.json").read_text())
NVDA = Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True)
BSTOCKS, REALITY = "6a2aed5097c45356b1a5f710", "6a43600c282e7240b76c8bbb"
# a fixed copy of the SpaceX snapshot of 13.11.2026 without its positions: the daily refresh may change the live one
SPCX_FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "spcx_snapshot.json").read_text())


def _data_dir(tmp_path, snapshot=None):
    data_dir = tmp_path / "data"
    write_json(collect.snapshot_path(data_dir, NVDA), snapshot if snapshot is not None else copy.deepcopy(FIXTURE))
    return data_dir


def _build(tmp_path, snapshot=None, config_dir=None):
    data_dir = _data_dir(tmp_path, snapshot)
    return build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=config_dir or tmp_path / "no-config")


# ---------- names ----------


def test_display_names_carry_no_line_breaks_or_double_spaces(tmp_path):
    snap = copy.deepcopy(FIXTURE)
    token = snap["cmc"]["rwa"]["tokens"][0]
    token["name"] = "  NVIDIA  Tokenized\nStock "
    token["issuer_name"] = "Acme \n Assets"
    record_key = next(iter(snap["portals"]["records"]))
    snap["portals"]["records"][record_key]["name"] = "Uniswap  V3\tNVDA /  USDC "
    export, _ = _build(tmp_path, snap)

    built = next(t for t in export["tokens"] if t["id"] == token["crypto_id"])
    assert built["name"] == "NVIDIA Tokenized Stock"
    assert built["issuer_name"] == "Acme Assets"
    opportunity = next(o for o in export["opportunities"] if o["key"].lower() == record_key.lower())
    assert opportunity["name"] == "Uniswap V3 NVDA / USDC"
    for kind in ("tokens", "opportunities", "issuers", "underlyings"):
        for item in export[kind]:
            for field in ("name", "issuer_name"):
                value = item.get(field)
                if isinstance(value, str):
                    assert value == " ".join(value.split()), (kind, field, value)


def _spcx_export(tmp_path):
    spcx = next(a for a in load_assets() if a.ticker == "SPCX")
    data_dir = tmp_path / "data"
    write_json(collect.snapshot_path(data_dir, spcx), copy.deepcopy(SPCX_FIXTURE))
    export, _ = build_export_with_warnings([spcx], load_settings(), data_dir, config_dir=tmp_path / "no-config")
    return export


def test_the_republic_token_name_is_on_one_line(tmp_path):
    names = {t["symbol"]: t["name"] for t in _spcx_export(tmp_path)["tokens"]}
    assert names["preSPCX"] == "SpaceX Pre-IPO stock (Republic)"


# ---------- versions whose symbol differs from the ticker ----------


def test_an_alias_keeps_a_real_version_whose_symbol_differs_from_the_ticker():
    tessera = {"symbol": "tSpaceX", "name": "T-SpaceX tokenized stock (Tessera)", "issuer_name": "Tessera"}
    prestocks = {"symbol": "SPACEX", "name": "SpaceX tokenized stock (PreStocks)", "issuer_name": "PreStocks"}
    for token in (tessera, prestocks):
        assert classify_token(token, "SPCX") == ("unclassified", "symbol_does_not_match_ticker")
        assert classify_token(token, "SPCX", ("SPACEX",)) == ("tokenized", None)
    # an alias changes nothing for derivatives and wrappers
    assert classify_token({"symbol": "SPACEX", "name": "SpaceX (Derivatives)"}, "SPCX", ("SPACEX",)) == ("derivative", "derivative")
    assert classify_token({"symbol": "wSPACEX", "name": "Wrapped SpaceX"}, "SPCX", ("SPACEX",)) == ("wrapper", None)
    # and a symbol that matches neither stays out
    assert classify_token({"symbol": "GOOGon", "name": "Alphabet Class C"}, "SPCX", ("SPACEX",)) == ("unclassified", "symbol_does_not_match_ticker")


def _assets_file(tmp_path, aliases):
    path = tmp_path / "assets.json"
    item = {"ticker": "SPCX", "name": "SpaceX", "asset_type": "stock"}
    if aliases is not None:
        item["symbol_aliases"] = aliases
    path.write_text(json.dumps({"schema_version": 1, "assets": [item]}))
    return path


def test_symbol_aliases_are_read_and_checked(tmp_path):
    assert load_assets(_assets_file(tmp_path, None))[0].symbol_aliases == ()
    assert load_assets(_assets_file(tmp_path, ["spacex", " SpaceX "]))[0].symbol_aliases == ("SPACEX",)
    for bad in ("SPACEX", [""], [1], [None], {"a": 1}):
        with pytest.raises(ConfigError):
            load_assets(_assets_file(tmp_path, bad))
    assert Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True).symbol_aliases == ()


def test_the_shipped_assets_give_spacex_its_alias():
    assets = {a.ticker: a for a in load_assets()}
    assert assets["SPCX"].symbol_aliases == ("SPACEX",)


def test_spacex_shows_all_its_versions(tmp_path):
    export = _spcx_export(tmp_path)
    kept = [t for t in export["tokens"] if t["ticker"] == "SPCX" and t["excluded_reason"] is None]
    assert {"SPACEX", "tSpaceX"} <= {t["symbol"] for t in kept}
    assert len(kept) == 11  # 12 in the CMC roster minus the derivative
    underlying = next(u for u in export["underlyings"] if u["ticker"] == "SPCX")
    assert underlying["versions_count"] == 11
    issuers = {i["name"] for i in export["issuers"]}
    assert {"PreStocks", "Tessera"} <= {name.split()[0] for name in issuers}


def test_other_share_classes_stay_out():
    # the symbols of another share class do not pass as versions of this one
    googon = {"symbol": "GOOGon", "name": "Alphabet Class C tokenized stock (Ondo)", "issuer_name": "Ondo Global Markets"}
    brk_a = {"symbol": "BRK.A.D", "name": "Berkshire Hathaway Class A", "issuer_name": "Dinari"}
    assert classify_token(googon, "GOOGL") == ("unclassified", "symbol_does_not_match_ticker")
    assert classify_token(brk_a, "BRK.B") == ("unclassified", "symbol_does_not_match_ticker")


# ---------- networks ----------


def test_mantle_and_monad_are_known_networks_that_portals_does_not_index():
    mapping = load_settings()["chain_mapping"]
    assert mapping["Mantle"] is None and mapping["Monad"] is None
    info = {"contract_address": [
        {"contract_address": "0x" + "1" * 40, "platform": {"name": "Mantle"}},
        {"contract_address": "0x" + "2" * 40, "platform": {"name": "Monad"}},
    ]}
    statuses = {d["cmc_network"]: d["status"] for d in cmc.parse_deployments(info, mapping)}
    assert statuses == {"Mantle": "unsupported_network", "Monad": "unsupported_network"}  # not "Unmapped network"


# ---------- issuer details that CMC does not give ----------


def _issuer_without_website(tmp_path):
    export, _ = _build(tmp_path / "probe")
    return [i for i in export["issuers"] if not i.get("website")]


def test_a_curated_issuer_website_fills_the_gap_left_by_cmc(tmp_path):
    target, other = _issuer_without_website(tmp_path)[:2]
    config_dir = tmp_path / "config"
    write_json(config_dir / "issuers.json", {"schema_version": 1, "issuers": {target["id"]: {"website": "https://issuer.example/tokens"}}})
    export, _ = _build(tmp_path, config_dir=config_dir)
    websites = {i["id"]: i.get("website") for i in export["issuers"]}
    assert websites[target["id"]] == "https://issuer.example/tokens"
    assert websites[other["id"]] is None


def test_the_cmc_website_wins_when_cmc_has_one(tmp_path):
    target = _issuer_without_website(tmp_path)[0]
    config_dir = tmp_path / "config"
    write_json(config_dir / "issuers.json", {"schema_version": 1, "issuers": {target["id"]: {"website": "https://issuer.example/tokens"}}})
    data_dir = _data_dir(tmp_path)
    write_json(data_dir / "issuers.json", {target["id"]: {"issuer_id": target["id"], "name": target["name"], "website": "https://cmc.example", "logo": None}})
    export, _ = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=config_dir)
    assert next(i for i in export["issuers"] if i["id"] == target["id"])["website"] == "https://cmc.example"


@pytest.mark.parametrize(
    "entry",
    [
        {"website": "http://issuer.example"}, {"website": "issuer.example"}, {"website": ""}, {"site": "https://issuer.example"},
        "https://issuer.example", {"website": "https://issuer.example", "logo": "https://issuer.example/logo.png"},
    ],
)
def test_a_broken_issuer_override_is_a_config_error(tmp_path, entry):
    config_dir = tmp_path / "config"
    write_json(config_dir / "issuers.json", {"schema_version": 1, "issuers": {"any-issuer": entry}})
    with pytest.raises(ConfigError):
        _build(tmp_path, config_dir=config_dir)


def test_the_shipped_issuer_overrides_name_bstocks_and_reality():
    issuers = json.loads((CONFIG_DIR / "issuers.json").read_text())["issuers"]
    assert issuers[BSTOCKS]["website"] == "https://www.binance.com/en-AE/support/faq/detail/f0c03cd6509a4085b4cce1636f16be38"
    assert issuers[REALITY]["website"] == "https://realityfinance.xyz"


def test_reality_has_a_logo_on_disk():
    assert find_logo(DEFAULT_DATA_DIR / "logos", "issuers", REALITY) is not None


# ---------- guards from the negative controls ----------


def test_underlying_and_issuer_names_from_every_source_are_on_one_line(tmp_path):
    snap = copy.deepcopy(FIXTURE)
    snap["name"] = " NVIDIA \n Corporation "
    snap["cmc"]["rwa"]["name"] = "Nvidia\tCorp  (tokenized)"
    token = snap["cmc"]["rwa"]["tokens"][0]
    data_dir = _data_dir(tmp_path, snap)
    write_json(data_dir / "issuers.json", {token["issuer_id"]: {"issuer_id": token["issuer_id"], "name": "Acme \n Assets  Ltd", "website": None, "logo": None}})
    export, _ = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=tmp_path / "no-config")
    underlying = export["underlyings"][0]
    assert underlying["name"] == "NVIDIA Corporation"
    assert underlying["cmc_name"] == "Nvidia Corp (tokenized)"  # the site shows cmc_name first
    assert next(i for i in export["issuers"] if i["id"] == token["issuer_id"])["name"] == "Acme Assets Ltd"


def test_collect_searches_portals_for_versions_found_through_an_alias(monkeypatch):
    spcx = Asset("SPCX", "SpaceX", "stock", "US84615Q1031", True, ("SPACEX",))
    rwa = {"tokens": [
        {"crypto_id": 1, "symbol": "SPACEX", "name": "SpaceX", "issuer_name": "PreStocks"},
        {"crypto_id": 2, "symbol": "ABCX", "name": "Another company", "issuer_name": "Other"},
    ]}
    terms = []

    def search(api, term, limit):
        terms.append(term)
        return {}, {"complete": True, "pages": 1}

    monkeypatch.setattr(portals, "search", search)
    snapshot = collect.collect_asset(spcx, rwa, None, {}, [], [], None, {"chain_mapping": {}, "portals_page_limit": 50}, "run")
    assert "SPACEX" in terms and "ABCX" not in terms
    assert set(snapshot["portals"]["searches"]) == set(terms)


@pytest.mark.parametrize(
    "text",
    ["{not json", json.dumps({"schema_version": 2, "issuers": {}}), json.dumps({"schema_version": 1}), json.dumps([])],
)
def test_a_broken_issuers_file_is_a_config_error(tmp_path, text):
    config_dir = tmp_path / "config"
    config_dir.mkdir()
    (config_dir / "issuers.json").write_text(text)
    with pytest.raises(ConfigError):
        _build(tmp_path, config_dir=config_dir)


def test_cli_sync_stops_on_a_broken_issuers_file_before_any_request(tmp_path, monkeypatch):
    (tmp_path / "config").mkdir()
    (tmp_path / "config" / "issuers.json").write_text("{not json")
    calls = []
    monkeypatch.setattr(cli, "run_sync", lambda *args, **kwargs: calls.append(args) or (0, {}))
    args = ["sync", "--data-dir", str(tmp_path / "data"), "--frontend-dir", str(tmp_path / "no-site"), "--config-dir", str(tmp_path / "config")]
    result = CliRunner().invoke(cli.app, args)
    assert result.exit_code == 1, result.output
    assert calls == []  # no CMC or Portals request is made with a broken config
