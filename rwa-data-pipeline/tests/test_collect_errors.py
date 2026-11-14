"""A provider answer that is not usable keeps the last good data instead of being read as "nothing found"."""

import copy
import json
from pathlib import Path

import httpx
import pytest

from rwa_pipeline import cmc, collect, portals
from rwa_pipeline.api import Api, ApiError
from rwa_pipeline.config import Asset, load_settings, write_json

FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "nvda_snapshot.json").read_text())
NVDA = Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True)
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
        r = self.responses.pop(0)
        if isinstance(r, Exception):
            raise r
        return r


def _http_api(provider, handler, raw_dir=None):
    api = Api(provider, "https://api.example", {}, SETTINGS, raw_dir)
    api.client = httpx.Client(base_url="https://api.example", transport=httpx.MockTransport(handler))
    api.min_interval = 0
    return api


# ---------- one answer ----------


@pytest.mark.parametrize("text", ["", "   ", "<html><body>maintenance</body></html>"])
def test_a_successful_status_with_a_body_that_is_not_json_is_an_error(text):
    api = _http_api("cmc", lambda request: httpx.Response(200, text=text))
    with pytest.raises(ApiError) as error:
        api.get("/v5/real-world-assets/quotes/latest", {"symbol": "NVDA"})
    assert error.value.status == 200


def test_json_answers_and_error_statuses_keep_their_meaning():
    assert _http_api("portals", lambda r: httpx.Response(200, json=["base", "ethereum"])).get("/v1/networks", {}) == ["base", "ethereum"]
    assert _http_api("cmc", lambda r: httpx.Response(200, json={"status": {}, "data": {}})).get("/x", {}) == {"status": {}, "data": {}}
    with pytest.raises(ApiError) as error:
        _http_api("cmc", lambda r: httpx.Response(404, text="not found")).get("/x", {})
    assert error.value.status == 404


@pytest.mark.parametrize(
    "body",
    [
        {},
        {"_non_json_body": ""},
        {"status": {"error_code": "0", "credit_count": 1}},  # no data at all
        {"data": {"rwa_assets": []}},  # no status block
        {"status": "ok", "data": {"rwa_assets": []}},  # status is not an object
    ],
)
def test_a_cmc_answer_without_its_status_and_data_is_an_error(body):
    with pytest.raises(ApiError):
        cmc.fetch_rwa_quotes(FakeApi([body]), ["NVDA"])


def test_a_complete_cmc_answer_with_nothing_found_is_still_a_valid_answer():
    api = FakeApi([{"status": {"error_code": "0", "credit_count": 1}, "data": {"rwa_assets": []}}])
    assert cmc.fetch_rwa_quotes(api, ["NVDA"]) == {}
    assert api.credits == 1


def test_a_portals_lookup_with_fewer_tokens_than_it_reports_is_an_error():
    api = FakeApi([{"tokens": [{"key": "base:0x1"}], "more": False, "totalItems": 2}])
    with pytest.raises(ApiError) as error:
        portals.lookup(api, ["base:0x1", "base:0x2"], 10)
    assert "1" in str(error.value) and "2" in str(error.value)  # says how many came and how many were reported


def test_a_portals_lookup_cut_by_the_page_limit_is_an_error():
    pages = [{"tokens": [{"key": f"base:0x{i}"}], "more": True, "totalItems": 999} for i in range(portals.MAX_PAGES)]
    with pytest.raises(ApiError):
        portals.lookup(FakeApi(pages), ["base:0x1"], 1)


@pytest.mark.parametrize("total", [None, "matching"])
def test_a_portals_lookup_cut_by_the_page_limit_is_an_error_even_when_the_counts_agree(total):
    # no totalItems at all, or a totalItems equal to what came: only `more` says the answer is cut
    pages = [{"tokens": [{"key": f"base:0x{i}"}], "more": True} for i in range(portals.MAX_PAGES)]
    if total == "matching":
        for page in pages:
            page["totalItems"] = portals.MAX_PAGES
    with pytest.raises(ApiError):
        portals.lookup(FakeApi(pages), ["base:0x1"], 1)


def test_a_portals_lookup_with_more_tokens_than_it_reports_is_not_cut():
    # only a shortfall means a truncated answer; extra tokens lose nothing
    api = FakeApi([{"tokens": [{"key": "base:0x1"}, {"key": "base:0x2"}], "more": False, "totalItems": 1}])
    assert set(portals.lookup(api, ["base:0x1", "base:0x2"], 10)) == {"base:0x1", "base:0x2"}


def test_a_complete_portals_lookup_returns_what_portals_indexes():
    # asking for two contracts and getting one is fine when Portals says it has exactly one: the other is not indexed
    api = FakeApi([{"tokens": [{"key": "base:0x1"}], "more": False, "totalItems": 1}])
    assert set(portals.lookup(api, ["base:0x1", "base:0x2"], 10)) == {"base:0x1"}


def test_a_portals_search_still_reports_an_incomplete_answer_without_failing():
    api = FakeApi([{"tokens": [{"key": "base:0x1"}], "more": False, "totalItems": 2}])
    items, meta = portals.search(api, "NVDA", 10)
    assert set(items) == {"base:0x1"} and meta.get("count_mismatch") is True


@pytest.mark.parametrize("body", [[], {}, {"networks": ["base"]}, ["", None], ["base", None], ["base", "  "]])
def test_portals_networks_must_be_a_nonempty_list_of_names(body):
    with pytest.raises(ApiError):
        portals.fetch_networks(FakeApi([body]))
    assert portals.fetch_networks(FakeApi([["base", "ethereum"]])) == ["base", "ethereum"]


# ---------- a whole run ----------


def _good_snapshot(tmp_path):
    path = collect.snapshot_path(tmp_path, NVDA)
    write_json(path, copy.deepcopy(FIXTURE))
    return path, path.read_bytes()


@pytest.mark.parametrize("answer", [httpx.Response(200, text=""), httpx.Response(200, json={}), httpx.Response(200, json={"status": {"error_code": "0"}})])
def test_an_unusable_cmc_answer_keeps_every_snapshot_and_fails_the_run(tmp_path, monkeypatch, answer):
    path, before = _good_snapshot(tmp_path)
    monkeypatch.setattr(collect, "load_api_keys", lambda: ("k1", "k2"))
    monkeypatch.setattr(cmc, "make_client", lambda key, settings, raw_dir=None: _http_api("cmc", lambda r: answer, raw_dir))
    monkeypatch.setattr(portals, "make_client", lambda key, settings, raw_dir=None: FakeApi([]))
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert code != 0, report
    assert path.read_bytes() == before  # the good snapshot is not replaced by "not tracked"
    assert report["completed"] == []


def _patch_cmc(monkeypatch):
    monkeypatch.setattr(collect, "load_api_keys", lambda: ("k1", "k2"))
    monkeypatch.setattr(cmc, "make_client", lambda key, settings, raw_dir=None: FakeApi([]))
    monkeypatch.setattr(cmc, "fetch_rwa_quotes", lambda api, tickers: {"NVDA": [FIXTURE["cmc"]["rwa"]]})
    monkeypatch.setattr(cmc, "fetch_crypto_info", lambda api, ids: ({int(k): v for k, v in FIXTURE["cmc"]["info"].items()}, []))
    monkeypatch.setattr(cmc, "fetch_issuer", lambda api, i: {"issuer_id": i, "name": "x"})
    monkeypatch.setattr(cmc, "fetch_rwa_info", lambda api, ids: {str(i): {"rwa_id": i, "website": None, "logo": None} for i in ids})
    monkeypatch.setattr(portals, "fetch_platform_names", lambda api: {})
    monkeypatch.setattr(portals, "search", lambda api, term, limit: ({}, {"complete": True}))


def test_a_truncated_portals_lookup_keeps_the_previous_snapshot(tmp_path, monkeypatch):
    path, before = _good_snapshot(tmp_path)
    _patch_cmc(monkeypatch)
    monkeypatch.setattr(portals, "fetch_networks", lambda api: FIXTURE["portals"]["networks"])
    truncated = {"tokens": [{"key": "base:0x0000000000000000000000000000000000000001"}], "more": False, "totalItems": 5}
    monkeypatch.setattr(portals, "make_client", lambda key, settings, raw_dir=None: FakeApi([truncated] * 20))
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert code == 1, report  # the only selected underlying failed: nothing was saved
    assert "NVDA" in report["failed"]
    assert path.read_bytes() == before


def test_an_empty_portals_network_list_stops_the_run_before_any_snapshot_is_written(tmp_path, monkeypatch):
    path, before = _good_snapshot(tmp_path)
    _patch_cmc(monkeypatch)
    monkeypatch.setattr(portals, "make_client", lambda key, settings, raw_dir=None: FakeApi([[]] * 5))
    code, report = collect.sync([NVDA], SETTINGS, tmp_path, None, refresh=True)
    assert code != 0, report
    assert path.read_bytes() == before
