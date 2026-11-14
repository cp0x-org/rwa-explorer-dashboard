"""A refresh never trades a snapshot with tokens for one without, a run that saved nothing fails, CMC errors keep their
code and message, and a version found through an alias counts as searched only once its symbol was searched."""

import copy
import hashlib
import json
from pathlib import Path

import pytest

from rwa_pipeline import cmc, collect, portals
from rwa_pipeline.analyze import build
from rwa_pipeline.api import ApiError
from rwa_pipeline.config import Asset, load_settings, write_json

FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "nvda_snapshot.json").read_text())
NVDA = Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True)
AAPL = Asset("AAPL", "Apple", "stock", "US0378331005", True)
AAPL_RWA = {"rwa_id": 3, "symbol": "AAPL", "asset_type": "stock", "tokens": []}
SETTINGS = load_settings()


class FakeApi:
    def __init__(self, responses):
        self.responses = list(responses)
        self.calls = []
        self.credits = 0
        self.requests = 0

    def get(self, path, params):
        self.calls.append((path, params))
        self.requests += 1
        return self.responses.pop(0)


def _digest(path: Path) -> str:
    return hashlib.sha256(path.read_bytes()).hexdigest()


def _patch_network(monkeypatch, quotes, fail_search=False):
    monkeypatch.setattr(collect, "load_api_keys", lambda: ("k1", "k2"))

    def search(api, term, limit):
        if fail_search:
            raise ApiError("portals", 503, "down")
        return {}, {"complete": True}

    monkeypatch.setattr(cmc, "fetch_rwa_quotes", lambda api, tickers: copy.deepcopy(quotes))
    monkeypatch.setattr(cmc, "fetch_crypto_info", lambda api, ids: ({int(k): v for k, v in FIXTURE["cmc"]["info"].items()}, []))
    monkeypatch.setattr(cmc, "fetch_issuer", lambda api, i: {"issuer_id": i, "name": "x"})
    monkeypatch.setattr(cmc, "fetch_rwa_info", lambda api, ids: {str(i): {"rwa_id": i, "website": None, "logo": None} for i in ids})
    monkeypatch.setattr(portals, "fetch_networks", lambda api: FIXTURE["portals"]["networks"])
    monkeypatch.setattr(portals, "fetch_platform_names", lambda api: {})
    monkeypatch.setattr(portals, "lookup", lambda api, keys, limit: {})
    monkeypatch.setattr(portals, "search", search)


def _nvda_with_tokens(tmp_path) -> Path:
    path = collect.snapshot_path(tmp_path, NVDA)
    write_json(path, copy.deepcopy(FIXTURE))
    return path


# ---------- a snapshot with tokens is never replaced by one without ----------


def test_an_empty_cmc_roster_does_not_replace_a_snapshot_with_tokens(tmp_path, monkeypatch):
    path = _nvda_with_tokens(tmp_path)
    before = _digest(path)
    _patch_network(monkeypatch, quotes={})  # CMC answered, but lists nothing at all
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert _digest(path) == before
    assert "NVDA" in report["failed"] and report["completed"] == []
    assert code == 1  # nothing was saved: the run failed and there is nothing to publish


def test_a_ticker_gone_from_the_roster_keeps_its_snapshot_while_the_rest_of_the_run_goes_on(tmp_path, monkeypatch):
    nvda = _nvda_with_tokens(tmp_path)
    before = _digest(nvda)
    _patch_network(monkeypatch, quotes={"AAPL": [AAPL_RWA]})  # NVDA is missing from this answer
    code, report = collect.sync([NVDA, AAPL], SETTINGS, tmp_path, None, refresh=True)
    assert _digest(nvda) == before
    assert "NVDA" in report["failed"]
    assert [c["ticker"] for c in report["completed"]] == ["AAPL"]
    assert code == 2


def test_a_roster_emptied_to_no_tokens_keeps_the_snapshot(tmp_path, monkeypatch):
    path = _nvda_with_tokens(tmp_path)
    before = _digest(path)
    emptied = {**copy.deepcopy(FIXTURE["cmc"]["rwa"]), "tokens": []}
    _patch_network(monkeypatch, quotes={"NVDA": [emptied]})
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert _digest(path) == before
    assert "NVDA" in report["failed"] and code == 1


def test_a_snapshot_without_tokens_may_still_be_replaced(tmp_path, monkeypatch):
    # nothing is lost: a ticker that had no tokens may turn "not in CMC"
    path = collect.snapshot_path(tmp_path, AAPL)
    write_json(path, {"underlying_id": AAPL.id, "status": "no_tracked_tokens", "cmc": None, "portals": None})
    _patch_network(monkeypatch, quotes={})
    code, report = collect.sync([AAPL], SETTINGS, tmp_path, None, refresh=True)
    assert code == 0 and report["failed"] == {}
    assert json.loads(path.read_text())["status"] == "not_in_cmc"


def test_a_refresh_with_the_roster_in_place_still_replaces_the_snapshot(tmp_path, monkeypatch):
    path = _nvda_with_tokens(tmp_path)
    before = _digest(path)
    _patch_network(monkeypatch, quotes={"NVDA": [FIXTURE["cmc"]["rwa"]]})
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert code == 0 and report["failed"] == {}
    assert _digest(path) != before and json.loads(path.read_text())["status"] == "ok"


# ---------- a run that saved nothing fails ----------


def test_a_run_where_every_selected_underlying_failed_exits_1(tmp_path, monkeypatch):
    _nvda_with_tokens(tmp_path)
    _patch_network(monkeypatch, quotes={"NVDA": [FIXTURE["cmc"]["rwa"]]}, fail_search=True)
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert code == 1 and report["completed"] == [] and "NVDA" in report["failed"]


def test_a_partial_run_still_exits_2(tmp_path, monkeypatch):
    _nvda_with_tokens(tmp_path)
    # AAPL has no tokens and needs no Portals request, so it completes while every search fails
    _patch_network(monkeypatch, quotes={"NVDA": [FIXTURE["cmc"]["rwa"]], "AAPL": [AAPL_RWA]}, fail_search=True)
    code, report = collect.sync([NVDA, AAPL], SETTINGS, tmp_path, None, refresh=True)
    assert code == 2 and "NVDA" in report["failed"]
    assert [c["ticker"] for c in report["completed"]] == ["AAPL"]


# ---------- CMC errors keep their code and message ----------


def test_a_cmc_error_inside_a_200_answer_keeps_its_code_and_message():
    with pytest.raises(ApiError) as error:
        cmc.fetch_rwa_quotes(FakeApi([{"status": {"error_code": 1008, "error_message": "You've exceeded your API Key's HTTP request rate limit."}}]), ["NVDA"])
    assert error.value.status == 1008
    assert error.value.message == "You've exceeded your API Key's HTTP request rate limit."


def test_an_invalid_id_reported_inside_a_200_answer_is_skipped_and_the_batch_retried():
    api = FakeApi([
        {"status": {"error_code": 400, "error_message": "Invalid value for 'id': '39839'"}},
        {"status": {"error_code": 0}, "data": {"1": {"id": 1, "contract_address": []}}},
    ])
    info, invalid = cmc.fetch_crypto_info(api, [1, 39839])
    assert invalid == [39839] and list(info) == [1]
    assert api.calls[1][1]["id"] == "1"


def test_a_successful_status_still_needs_data():
    with pytest.raises(ApiError):
        cmc.fetch_rwa_quotes(FakeApi([{"status": {"error_code": 0, "credit_count": 1}}]), ["NVDA"])


# ---------- a version found through an alias counts as searched once its symbol was searched ----------


def _aliased_snapshot(searched_alias: bool):
    snap = copy.deepcopy(FIXTURE)
    token = next(t for t in snap["cmc"]["rwa"]["tokens"] if t["symbol"] == "NVDAc")
    token["symbol"] = "NVIDIAC"  # its contracts are indexed, but the symbol lacks the ticker: only an alias keeps it
    if searched_alias:
        snap["portals"]["searches"]["NVIDIA"] = {"complete": True, "pages": 1, "received": 0, "total_items": 0}
    return snap, token["crypto_id"]


def test_a_version_found_through_an_alias_is_not_search_coverage_until_its_symbol_is_searched():
    snap, cid = _aliased_snapshot(searched_alias=False)
    result = build([snap], {}, SETTINGS, {}, symbol_aliases={"NVDA": ("NVIDIA",)})
    token = next(t for t in result["tokens"] if t["id"] == cid)
    assert token["excluded_reason"] is None  # the alias keeps it as a version
    assert token["coverage"] == "partial"  # no search could have found its positions
    assert result["underlyings"][0]["search_complete"] is False


def test_once_its_symbol_was_searched_the_aliased_version_counts_as_searched():
    snap, cid = _aliased_snapshot(searched_alias=True)
    result = build([snap], {}, SETTINGS, {}, symbol_aliases={"NVDA": ("NVIDIA",)})
    token = next(t for t in result["tokens"] if t["id"] == cid)
    assert token["coverage"] == "search"
    assert result["underlyings"][0]["search_complete"] is True


def test_versions_whose_symbol_holds_a_searched_term_keep_their_coverage():
    result = build([copy.deepcopy(FIXTURE)], {}, SETTINGS, {})
    coverage = {t["symbol"]: t["coverage"] for t in result["tokens"] if t["excluded_reason"] is None}
    assert coverage == {"NVDAX": "search", "NVDAon": "search", "WNVDAX": "partial", "NVDAc": "search"}
    assert result["underlyings"][0]["search_complete"] is True


# ---------- guards from the negative controls ----------

SPCX_FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "spcx_snapshot.json").read_text())


def test_a_version_portals_cannot_index_keeps_its_coverage_though_its_symbol_was_not_searched():
    # SPACEX and tSpaceX live on Solana only: unsearched, but there is nothing to search for them on Portals
    result = build([copy.deepcopy(SPCX_FIXTURE)], {}, SETTINGS, {}, symbol_aliases={"SPCX": ("SPACEX",)})
    coverage = {t["symbol"]: t["coverage"] for t in result["tokens"] if t["excluded_reason"] is None}
    assert coverage["SPACEX"] == coverage["tSpaceX"] == "unsupported_by_portals"
    assert result["underlyings"][0]["search_complete"] is False  # their symbols were never searched


def test_a_searched_term_covers_a_symbol_in_any_letter_case():
    snap, cid = _aliased_snapshot(searched_alias=False)
    snap["portals"]["searches"]["Nvidia"] = {"complete": True, "pages": 1, "received": 0, "total_items": 0}
    result = build([snap], {}, SETTINGS, {}, symbol_aliases={"NVDA": ("NVIDIA",)})
    assert next(t for t in result["tokens"] if t["id"] == cid)["coverage"] == "search"
    assert result["underlyings"][0]["search_complete"] is True
