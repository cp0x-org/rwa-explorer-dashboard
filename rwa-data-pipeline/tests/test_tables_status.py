"""In the flat tables, an eligible position that no token is confirmed to use is marked as such; the site export keeps its meaning."""

import copy
import csv
import json
from pathlib import Path

from rwa_pipeline import collect
from rwa_pipeline.config import Asset, load_settings, write_json
from rwa_pipeline.export import build_export_with_warnings
from rwa_pipeline.tables import build_tables, write_tables

FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "nvda_snapshot.json").read_text())
NVDA = Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True)


def _export(tmp_path):
    data_dir = tmp_path / "data"
    write_json(collect.snapshot_path(data_dir, NVDA), copy.deepcopy(FIXTURE))
    export, _ = build_export_with_warnings([NVDA], load_settings(), data_dir, config_dir=tmp_path / "no-config")
    return export


def _unlink(export):
    """Take two eligible positions used by basket tokens and remove every token link to the first one."""
    basket = [t for t in export["tokens"] if t["excluded_reason"] is None]
    linked = {r["key"] for t in basket for r in t["relations"]}
    eligible = [o for o in export["opportunities"] if o["eligibility"] == "eligible" and o["key"] in linked]
    assert len(eligible) >= 2, "the fixture has eligible positions used by tokens"
    target, kept = eligible[0], eligible[1]
    for t in export["tokens"]:
        t["relations"] = [r for r in t["relations"] if r["key"] != target["key"]]
    return target, kept


def test_an_eligible_position_without_a_confirmed_token_link_is_unconfirmed_in_the_tables(tmp_path):
    export = _export(tmp_path)
    target, kept = _unlink(export)
    before = copy.deepcopy(export["opportunities"])
    tables = {t.name: t for t in build_tables(export)}
    rows = {r["opportunity_key"]: r for r in tables["opportunities"].rows}
    assert rows[target["key"]]["linked_tokens_count"] == 0
    assert rows[target["key"]]["eligibility"] == "unconfirmed"
    assert rows[kept["key"]]["eligibility"] == "eligible"
    # every other status is copied as it is
    for o in export["opportunities"]:
        if o["key"] not in (target["key"], kept["key"]) and o["eligibility"] != "eligible":
            assert rows[o["key"]]["eligibility"] == o["eligibility"]
    # the JSON the site reads is not touched: its counts and ratings keep their meaning
    assert export["opportunities"] == before
    assert "unconfirmed" in tables["opportunities"].description


def test_the_unconfirmed_status_reaches_the_csv(tmp_path):
    export = _export(tmp_path)
    target, _ = _unlink(export)
    paths = write_tables(export, "csv", tmp_path / "csv", tmp_path)
    with next(p for p in paths if p.name == "opportunities.csv").open(newline="", encoding="utf-8") as f:
        rows = {r["opportunity_key"]: r for r in csv.DictReader(f)}
    assert rows[target["key"]]["eligibility"] == "unconfirmed"


def test_a_position_that_is_not_eligible_keeps_its_status_without_token_links(tmp_path):
    # guard from the negative controls: only an eligible position becomes unconfirmed
    export = _export(tmp_path)
    other = [o for o in export["opportunities"] if o["eligibility"] != "eligible"]
    assert other, "the fixture has positions that are not eligible"
    target = other[0]
    for t in export["tokens"]:
        t["relations"] = [r for r in t["relations"] if r["key"] != target["key"]]
    rows = {r["opportunity_key"]: r for r in {t.name: t for t in build_tables(export)}["opportunities"].rows}
    assert rows[target["key"]]["linked_tokens_count"] == 0
    assert rows[target["key"]]["eligibility"] == target["eligibility"]
