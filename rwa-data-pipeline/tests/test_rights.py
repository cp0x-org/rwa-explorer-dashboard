"""Holder-rights profiles: reviewed facts with verbatim quotes, bound to explicitly listed tokens."""

import copy
import json
from datetime import date
from pathlib import Path

import pytest
from typer.testing import CliRunner

from rwa_pipeline import cli, collect
from rwa_pipeline.config import CONFIG_DIR, DEFAULT_DATA_DIR, Asset, ConfigError, load_settings, read_json, write_json
from rwa_pipeline.export import build_export_with_warnings
from rwa_pipeline.rights import FACTS, WHO, attach_rights, load_rights_profiles

FIXTURE = json.loads((Path(__file__).parent / "fixtures" / "nvda_snapshot.json").read_text())
NVDA = Asset("NVDA", "NVIDIA", "stock", "US67066G1040", True)
TODAY = date(2026, 11, 13)
ISSUER, OTHER_ISSUER = "issuer-1", "issuer-2"
URL = "https://example.org/terms.pdf"

# The published contract: order, groups, allowed values and the values that must say who they apply to.
SPEC = {
    "instrument": ("what", ["share", "beneficial_interest", "secured_debt", "unsecured_debt", "derivative", "contractual_claim", "no_claim"], []),
    "backing": ("what", ["full_1to1", "full", "partial", "none"], []),
    "asset_protection": ("what", ["pledged_to_holders", "trust_or_segregated", "none"], []),
    "primary_access": ("who", ["available"], ["available"]),
    "redemption": ("who", ["available", "at_events_only", "not_redeemable"], ["available", "at_events_only"]),
    "claim_scope": ("who", ["has_claim", "no_claim"], ["has_claim"]),
    "restricted": ("who", ["none_stated", "listed"], []),
    "transfer": ("who", ["free", "blocklist", "allowlist"], []),
    "freeze": ("issuer_powers", ["yes", "no"], []),
    "cancellation": ("issuer_powers", ["yes", "no"], []),
    "pause": ("issuer_powers", ["yes", "no"], []),
    "upgrade": ("issuer_powers", ["yes", "no"], []),
    "forced_redemption": ("issuer_powers", ["yes", "no"], []),
    "loss_exploit": ("loss", ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]),
    "loss_lost_tokens": ("loss", ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]),
    "loss_issuer_insolvency": ("loss", ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]),
    "loss_custodian_failure": ("loss", ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]),
    "dividends": ("shareholder", ["paid_out", "reinvested", "none"], []),
    "voting": ("shareholder", ["none", "pass_through", "discretionary"], ["discretionary"]),
    "reserves_proof": ("other", ["published", "none"], []),
    "governing_law": ("other", ["stated"], []),
}
SPEC_WHO = ["anyone", "verified", "professional", "institutional", "platform_clients", "eligible_holders", "nobody"]


def evidence(quote="The holder has an individual entitlement."):
    return {"quote": quote, "url": URL, "title": "Terms and Conditions", "doc_date": "2026-06-11", "retrieved_at": "2026-11-13"}


def fact(key):
    _, values, needs_who = SPEC[key]
    out = {"value": values[0], "evidence": [evidence()]}
    if values[0] in needs_who:
        out["who"] = "verified"
    if key == "governing_law":
        out["countries"] = ["AE"]
        out["zones"] = ["ADGM"]
    return out


def profile(profile_id="acme-line", token_ids=(1,), **overrides):
    out = {
        "id": profile_id,
        "issuer_id": ISSUER,
        "cmc_label": "Acme Assets",
        "product": "Acme Stocks",
        "token_ids": list(token_ids),
        "scope_basis": "per_token_documents" if len(token_ids) == 1 else "programme_documents",
        "reviewed_at": "2026-11-13",
        "legal_issuer": {"name": "Acme Holdings Ltd", "country": "AE", "zone": "ADGM", "evidence": [evidence("The Issuer is Acme Holdings Ltd, a company incorporated in the ADGM.")]},
        "facts": {key: fact(key) for key in SPEC},
        "documents": [{"title": "Terms and Conditions", "url": URL, "type": "terms", "doc_date": "2026-06-11"}],
    }
    if len(token_ids) > 1:
        out["scope_evidence"] = [evidence("These Terms apply to every Token issued under the programme.")]
    out.update(overrides)
    return out


def config(*profiles, **top):
    return {"schema_version": 1, "review_valid_days": 180, "profiles": list(profiles), **top}


def export_of(tokens=((1, ISSUER), (2, ISSUER), (3, OTHER_ISSUER))):
    return {
        "summary": {},
        "tokens": [{"id": token_id, "symbol": f"T{token_id}", "issuer_id": issuer_id, "kind": "tokenized"} for token_id, issuer_id in tokens],
        "issuers": [{"id": ISSUER, "name": "Acme", "cmc_label": "Acme Assets"}, {"id": OTHER_ISSUER, "name": "Other", "cmc_label": "Other Assets"}],
    }


def attach(cfg, export=None, today=TODAY):
    export = export_of() if export is None else export
    warnings = attach_rights(export, cfg, today)
    return export, warnings


# ---------- the contract table ----------


def test_fact_table_is_the_published_contract():
    assert list(FACTS) == list(SPEC)  # the order is the order of the site
    for key, (group, values, needs_who) in SPEC.items():
        assert FACTS[key] == {"group": group, "values": values, "needs_who": needs_who}
    assert WHO == SPEC_WHO


# ---------- a valid profile ----------


def test_valid_profile_binds_only_its_listed_tokens():
    export, warnings = attach(config(profile()))
    assert warnings == []
    by_id = {t["id"]: t["rights_profile_id"] for t in export["tokens"]}
    assert by_id == {1: "acme-line", 2: None, 3: None}  # token 2 is the same issuer, but nobody reviewed it
    assert [p["id"] for p in export["rights_profiles"]] == ["acme-line"]
    published = export["rights_profiles"][0]
    assert published["stale"] is False
    assert published["facts"] == profile()["facts"] and published["legal_issuer"] == profile()["legal_issuer"]
    acme, other = export["issuers"]
    assert acme["rights_profile_ids"] == ["acme-line"] and acme["rights_reviewed_tokens_count"] == 1
    assert other["rights_profile_ids"] == [] and other["rights_reviewed_tokens_count"] == 0
    summary = export["summary"]
    assert summary["rights_profiles_count"] == 1
    assert summary["rights_reviewed_tokens_count"] == 1
    assert summary["rights_review_valid_days"] == 180
    assert summary["rights_warnings_count"] == 0 and summary["rights_excluded_profiles_count"] == 0


def test_the_schema_travels_with_the_export():
    export, _ = attach(config(profile()))
    schema = export["rights_schema"]
    assert schema["who"] == SPEC_WHO
    assert [f["key"] for f in schema["facts"]] == list(SPEC)
    assert schema["facts"][3] == {"key": "primary_access", "group": "who", "values": ["available"], "needs_who": ["available"]}
    # every fact may also be "the documents do not say": the schema names that value instead of leaving it to be guessed
    assert schema["unknown_value"] == "unknown"
    assert all("unknown" not in f["values"] for f in schema["facts"])


def test_without_profiles_every_field_is_present_and_empty():
    for cfg in (None, config()):
        export, warnings = attach(cfg)
        assert warnings == []
        assert export["rights_profiles"] == []
        assert all(t["rights_profile_id"] is None for t in export["tokens"])
        assert all(i["rights_profile_ids"] == [] and i["rights_reviewed_tokens_count"] == 0 for i in export["issuers"])
        assert export["summary"]["rights_profiles_count"] == 0
        assert [f["key"] for f in export["rights_schema"]["facts"]] == list(SPEC)


def test_programme_profile_covers_several_tokens_and_two_profiles_coexist():
    export, warnings = attach(config(profile("acme-programme", (1, 2)), profile("other-line", (3,), issuer_id=OTHER_ISSUER, cmc_label="Other Assets")))
    assert warnings == []
    assert {t["id"]: t["rights_profile_id"] for t in export["tokens"]} == {1: "acme-programme", 2: "acme-programme", 3: "other-line"}
    assert export["summary"]["rights_reviewed_tokens_count"] == 3


def test_token_missing_from_the_export_is_a_warning_not_an_exclusion():
    export, warnings = attach(config(profile("acme-programme", (1, 2, 999))))
    assert len(warnings) == 1 and "acme-programme" in warnings[0] and "999" in warnings[0]
    assert {t["id"]: t["rights_profile_id"] for t in export["tokens"]} == {1: "acme-programme", 2: "acme-programme", 3: None}
    assert export["issuers"][0]["rights_reviewed_tokens_count"] == 2
    assert export["summary"]["rights_excluded_profiles_count"] == 0
    assert export["rights_profiles"][0]["token_ids"] == [1, 2]  # the published list names the tokens of this export


@pytest.mark.parametrize("key,value", [("redemption", "at_events_only"), ("loss_issuer_insolvency", "unsecured_claim"), ("loss_exploit", "unsecured_claim"), ("voting", "discretionary")])
def test_the_conditional_outcomes_have_their_own_values_and_say_to_whom(key, value):
    # a payout only at maturity is not "redemption available"; an unsecured creditor has no "claim on the backing assets";
    # an issuer that may follow instructions does not "pass voting through"
    stated = profile(facts={**{k: fact(k) for k in SPEC}, key: {"value": value, "who": "platform_clients", "evidence": [evidence()]}})
    export, warnings = attach(config(stated))
    assert warnings == []
    assert export["rights_profiles"][0]["facts"][key] == {"value": value, "who": "platform_clients", "evidence": [evidence()]}
    nobody = profile(facts={**{k: fact(k) for k in SPEC}, key: {"value": value, "evidence": [evidence()]}})
    export, warnings = attach(config(nobody))
    assert export["rights_profiles"] == [] and len(warnings) == 1


def test_a_token_listed_twice_makes_the_profile_invalid():
    export, warnings = attach(config(profile("acme-programme", (1, 2, 1))))
    assert export["rights_profiles"] == [] and export["summary"]["rights_excluded_profiles_count"] == 1
    assert len(warnings) == 1 and "acme-programme" in warnings[0]
    assert all(t["rights_profile_id"] is None for t in export["tokens"])


def test_a_restriction_may_list_only_zones():
    # a product closed to a region, not to a whole country and not to US persons
    zones_only = profile(facts={**{key: fact(key) for key in SPEC}, "restricted": {"value": "listed", "zones": ["Crimea"], "evidence": [evidence()]}})
    export, warnings = attach(config(zones_only))
    assert warnings == []
    assert export["rights_profiles"][0]["facts"]["restricted"] == {"value": "listed", "zones": ["Crimea"], "evidence": [evidence()]}


def test_a_profile_outside_the_basket_is_absent_not_broken():
    # a build from a part of the snapshots: neither the issuer nor any token of the profile is in the export
    cfg = config(profile(), profile("elsewhere-line", (501, 502), issuer_id="issuer-elsewhere", cmc_label="Elsewhere Assets"))
    before = copy.deepcopy(cfg)
    export, warnings = attach(cfg)
    assert [p["id"] for p in export["rights_profiles"]] == ["acme-line"]
    assert len(warnings) == 1 and "elsewhere-line" in warnings[0]
    assert export["summary"]["rights_excluded_profiles_count"] == 0  # nothing is wrong with the profile itself
    assert export["summary"]["rights_warnings_count"] == 1 and export["summary"]["rights_profiles_count"] == 1
    assert cfg == before


def test_a_profile_with_no_token_in_the_export_is_not_published():
    export, warnings = attach(config(profile("acme-line", (999,))))
    assert export["rights_profiles"] == []
    assert export["issuers"][0]["rights_profile_ids"] == [] and export["issuers"][0]["rights_reviewed_tokens_count"] == 0
    assert len(warnings) == 1 and "acme-line" in warnings[0] and "999" in warnings[0]
    assert export["summary"]["rights_excluded_profiles_count"] == 0


# ---------- review age ----------


def test_review_older_than_the_valid_period_is_published_as_stale_with_a_warning():
    fresh, warnings = attach(config(profile(reviewed_at="2026-05-17")))  # exactly 180 days before TODAY
    assert fresh["rights_profiles"][0]["stale"] is False and warnings == []

    old, warnings = attach(config(profile(reviewed_at="2026-05-16")))
    assert old["rights_profiles"][0]["stale"] is True
    assert old["tokens"][0]["rights_profile_id"] == "acme-line"  # still shown, marked as old by the site
    assert len(warnings) == 1 and "acme-line" in warnings[0]
    assert old["summary"]["rights_excluded_profiles_count"] == 0


# ---------- a broken profile is left out, the rest is published ----------


def _fact(key, **changes):
    def mutate(p):
        p["facts"][key] = {**p["facts"][key], **changes}

    return mutate


def _drop(*path):
    def mutate(p):
        node = p
        for key in path[:-1]:
            node = node[key]
        del node[path[-1]]

    return mutate


def _put(*path_and_value):
    *path, value = path_and_value

    def mutate(p):
        node = p
        for key in path[:-1]:
            node = node[key]
        node[path[-1]] = value

    return mutate


BROKEN = {
    "unknown top-level key": _put("rating", "A+"),
    "unknown fact": _put("facts", "safety_score", {"value": "yes", "evidence": [evidence()]}),
    "missing fact": _drop("facts", "dividends"),
    "value outside the list": _fact("voting", value="maybe"),
    "value without who": _drop("facts", "redemption", "who"),
    "who outside the list": _fact("redemption", who="residents_only"),
    "who on a value that takes none": _fact("voting", who="anyone"),
    "fact without evidence": _fact("freeze", evidence=[]),
    "empty quote": _fact("freeze", evidence=[{**evidence(), "quote": "  "}]),
    "quote longer than 600 characters": _fact("freeze", evidence=[{**evidence(), "quote": "word " * 121}]),
    "quote is a fragment": _fact("freeze", evidence=[{**evidence(), "quote": "an admin-burn"}]),
    "quote is a bare label of five words": _fact("freeze", evidence=[{**evidence(), "quote": "Daily Proof of Collateral here"}]),
    "issuer quote is only a name": _put("legal_issuer", "evidence", [evidence("Acme Holdings Ltd")]),
    "evidence url is not https": _fact("freeze", evidence=[{**evidence(), "url": "http://example.org/terms.pdf"}]),
    "evidence url is a script": _fact("freeze", evidence=[{**evidence(), "url": "javascript:alert(1)"}]),
    "evidence without a title": _fact("freeze", evidence=[{**evidence(), "title": ""}]),
    "evidence without a document date": _fact("freeze", evidence=[{k: v for k, v in evidence().items() if k != "doc_date"}]),
    "retrieved in the future": _fact("freeze", evidence=[{**evidence(), "retrieved_at": "2026-11-14"}]),
    "retrieved date is not a date": _fact("freeze", evidence=[{**evidence(), "retrieved_at": "13.11.2026"}]),
    "unknown key in evidence": _fact("freeze", evidence=[{**evidence(), "page": 12}]),
    "unknown key in a fact": _fact("freeze", confidence="high"),
    "unknown without checked": _put("facts", "pause", {"value": "unknown"}),
    "unknown with evidence": _put("facts", "pause", {"value": "unknown", "checked": [URL], "evidence": [evidence()]}),
    "unknown checked is not https": _put("facts", "pause", {"value": "unknown", "checked": ["ftp://example.org"]}),
    "note in one language": _fact("freeze", note={"en": "Only English."}),
    "note with an extra language": _fact("freeze", note={"en": "a", "zh": "b", "ru": "c"}),
    "country is not an ISO code": _fact("governing_law", countries=["UAE"]),
    "governing law without a place": _put("facts", "governing_law", {"value": "stated", "evidence": [evidence()]}),
    "restricted listed without countries or us_persons": _put("facts", "restricted", {"value": "listed", "evidence": [evidence()]}),
    "us_persons on another fact": _fact("freeze", us_persons=True),
    "us_persons is not a boolean": _put("facts", "restricted", {"value": "listed", "us_persons": "yes", "evidence": [evidence()]}),
    "reviewed in the future": _put("reviewed_at", "2026-11-14"),
    "reviewed_at is not a date": _put("reviewed_at", "yesterday"),
    "legal issuer without evidence": _put("legal_issuer", {"name": "Acme Holdings Ltd", "country": "AE"}),
    "legal issuer without a name": _put("legal_issuer", "name", ""),
    "legal issuer country is not an ISO code": _put("legal_issuer", "country", "Abu Dhabi"),
    "legal issuer note in one language": _put("legal_issuer", "note", {"zh": "x"}),
    "unknown key in the legal issuer": _put("legal_issuer", "licence", "FSRA"),
    "no documents": _put("documents", []),
    "document type outside the list": _put("documents", [{"title": "T", "url": URL, "type": "tweet", "doc_date": "2026"}]),
    "document url is not https": _put("documents", [{"title": "T", "url": "//example.org", "type": "terms", "doc_date": "2026"}]),
    "scope basis outside the list": _put("scope_basis", "all_tokens"),
    "empty token list": _put("token_ids", []),
    "token ids are not integers": _put("token_ids", ["1"]),
    "token id is a boolean": _put("token_ids", [True]),
    "empty product": _put("product", ""),
    "id with spaces": _put("id", "Acme Line"),
    "label differs from the export": _put("cmc_label", "Acme"),
    "issuer is not in the export": _put("issuer_id", "issuer-404"),  # its token is here and belongs to somebody else
    "no restrictions stated, yet countries are listed": _put("facts", "restricted", {"value": "none_stated", "countries": ["US"], "evidence": [evidence()]}),
    "no restrictions stated, yet zones are listed": _put("facts", "restricted", {"value": "none_stated", "zones": ["Crimea"], "evidence": [evidence()]}),
    "no restrictions stated, yet US persons are excluded": _put("facts", "restricted", {"value": "none_stated", "us_persons": True, "evidence": [evidence()]}),
    "token of another issuer": _put("token_ids", [3]),
}


@pytest.mark.parametrize("name", BROKEN)
def test_broken_profile_is_excluded_and_the_other_one_is_published(name):
    broken = profile()
    BROKEN[name](broken)
    good = profile("other-line", (3,), issuer_id=OTHER_ISSUER, cmc_label="Other Assets")
    export, warnings = attach(config(broken, good))
    assert [p["id"] for p in export["rights_profiles"]] == ["other-line"]
    assert {t["id"]: t["rights_profile_id"] for t in export["tokens"]} == {1: None, 2: None, 3: "other-line"}
    assert len(warnings) == 1, warnings  # one line per excluded profile, however many problems it has
    assert export["summary"]["rights_excluded_profiles_count"] == 1
    assert export["summary"]["rights_warnings_count"] == 1


def test_per_token_documents_cover_exactly_one_token():
    export, warnings = attach(config(profile("acme-line", (1, 2), scope_basis="per_token_documents")))
    assert export["rights_profiles"] == [] and len(warnings) == 1


def test_programme_documents_need_a_quote_that_they_cover_the_line():
    broken = profile("acme-programme", (1, 2))
    del broken["scope_evidence"]
    export, warnings = attach(config(broken))
    assert export["rights_profiles"] == [] and len(warnings) == 1


def test_a_profile_that_is_not_an_object_is_excluded_without_a_crash():
    export, warnings = attach(config("acme-line", None, profile("other-line", (3,), issuer_id=OTHER_ISSUER, cmc_label="Other Assets")))
    assert [p["id"] for p in export["rights_profiles"]] == ["other-line"]
    assert export["summary"]["rights_excluded_profiles_count"] == 2 and len(warnings) == 2


def test_attach_does_not_modify_the_config():
    cfg = config(profile())
    before = copy.deepcopy(cfg)
    attach(cfg)
    assert cfg == before


def test_quotes_are_published_without_the_line_breaks_of_a_pdf_text_layer():
    ragged = "The Issuer   holds its interest\n      in the Underlying as trustee\tfor the benefit of holders."
    cfg = config(profile(facts={**{key: fact(key) for key in SPEC}, "freeze": {"value": "yes", "evidence": [evidence(ragged)]}}))
    before = copy.deepcopy(cfg)
    export, warnings = attach(cfg)
    assert warnings == []
    assert export["rights_profiles"][0]["facts"]["freeze"]["evidence"][0]["quote"] == "The Issuer holds its interest in the Underlying as trustee for the benefit of holders."
    assert cfg == before  # the file keeps the text exactly as it was copied


def test_the_length_limit_counts_the_published_text_not_the_padding_of_the_source():
    sentence = "The Issuer holds its interest in the Underlying as trustee for the benefit of holders."
    padded = sentence.replace(" ", " " * 60)  # a column-padded PDF line: far over 600 raw characters, 86 once collapsed
    assert len(padded) > 600
    export, warnings = attach(config(profile(facts={**{key: fact(key) for key in SPEC}, "freeze": {"value": "yes", "evidence": [evidence(padded)]}})))
    assert warnings == [] and export["rights_profiles"][0]["facts"]["freeze"]["evidence"][0]["quote"] == sentence
    # ...and a text that is still too long after collapsing stays refused
    long = " ".join(["word"] * 130)
    assert len(long) > 600
    export, warnings = attach(config(profile(facts={**{key: fact(key) for key in SPEC}, "freeze": {"value": "yes", "evidence": [evidence(long)]}})))
    assert export["rights_profiles"] == [] and len(warnings) == 1


def test_tables_carry_the_rights(tmp_path):
    from rwa_pipeline.tables import build_tables

    token, prof = _fixture_profile(tmp_path)
    prof["legal_issuer"]["note"] = {"en": "Sole issuer of the line.", "zh": "该产品线的唯一发行人。"}
    prof["legal_issuer"]["evidence"].append({**evidence("Acme Holdings Ltd is licensed in the ADGM and is the sole issuer of the Tokens."), "url": "https://example.org/register"})
    prof["facts"]["pause"] = {"value": "unknown", "checked": [URL, "https://example.org/faq"]}
    prof["facts"]["redemption"] = {"value": "available", "who": "platform_clients", "countries": ["AE"], "zones": ["ADGM"], "evidence": [evidence(), evidence("Redemption Orders may be submitted through the platform only.")]}
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof))
    export, _ = build_export_with_warnings([NVDA], load_settings(), tmp_path / "data", config_dir=tmp_path / "config")
    tables = {t.name: t for t in build_tables(export)}

    facts = tables["rights_facts"].rows
    # one row per quote; a fact the documents are silent on is one row with the list of what was read
    assert len(facts) == sum(max(1, len(f.get("evidence", []))) for f in prof["facts"].values())
    redemption = [row for row in facts if row["fact"] == "redemption"]
    assert len(redemption) == 2
    assert redemption[0]["profile_id"] == "fixture-line" and redemption[0]["group"] == "who"
    assert redemption[0]["value"] == "available" and redemption[0]["who"] == "platform_clients"
    assert redemption[0]["countries"] == "AE" and redemption[0]["zones"] == "ADGM"
    assert redemption[1]["quote"] == "Redemption Orders may be submitted through the platform only." and redemption[1]["url"] == URL
    assert redemption[0]["reviewed_at"] == "2026-11-13" and redemption[0]["stale"] is False
    (pause,) = [row for row in facts if row["fact"] == "pause"]
    assert pause["value"] == "unknown" and pause["quote"] is None and pause["checked"] == f"{URL}; https://example.org/faq"

    tokens = {row["token_id"]: row for row in tables["rights_tokens"].rows}
    assert len(tokens) == len(export["tokens"])  # every token is listed: an empty profile column means "not reviewed"
    assert tokens[token["id"]]["rights_profile_id"] == "fixture-line" and tokens[token["id"]]["reviewed_at"] == "2026-11-13"
    assert sum(row["rights_profile_id"] is None for row in tokens.values()) == len(export["tokens"]) - 1

    # who actually owes the holder: the legal issuer of the profile, with the words that name it
    assert tables["rights_profiles"].columns == [
        "profile_id", "issuer_id", "issuer_name", "product", "scope_basis", "tokens_count", "legal_issuer_name", "legal_issuer_country",
        "legal_issuer_zone", "legal_issuer_note_en", "legal_issuer_quotes", "legal_issuer_urls", "reviewed_at", "stale",
    ]
    (line,) = tables["rights_profiles"].rows
    assert line["profile_id"] == "fixture-line" and line["issuer_id"] == prof["issuer_id"] and line["issuer_name"] == token["issuer_name"]
    assert line["product"] == "Acme Stocks" and line["scope_basis"] == "per_token_documents" and line["tokens_count"] == 1
    assert (line["legal_issuer_name"], line["legal_issuer_country"], line["legal_issuer_zone"]) == ("Acme Holdings Ltd", "AE", "ADGM")
    assert line["legal_issuer_note_en"] == "Sole issuer of the line."
    assert line["legal_issuer_quotes"] == (
        "The Issuer is Acme Holdings Ltd, a company incorporated in the ADGM. | Acme Holdings Ltd is licensed in the ADGM and is the sole issuer of the Tokens."
    )
    assert line["legal_issuer_urls"] == f"{URL}; https://example.org/register"
    assert line["reviewed_at"] == "2026-11-13" and line["stale"] is False

    # an export from before the rights layer has the table and no rows
    old = {key: value for key, value in export.items() if not key.startswith("rights_")}
    assert {t.name: t for t in build_tables(old)}["rights_profiles"].rows == []


# ---------- the file as a whole ----------


def test_load_absent_is_none_and_a_broken_file_is_a_config_error(tmp_path):
    path = tmp_path / "rights_profiles.json"
    assert load_rights_profiles(path) is None

    path.write_text("{not json")
    with pytest.raises(ConfigError):
        load_rights_profiles(path)
    for broken in (
        [],
        {"schema_version": 2, "profiles": []},
        {"schema_version": 1, "profiles": {}},
        config(profile("same-id", (1,)), profile("same-id", (2,))),  # duplicate id
        config(profile("acme-a", (1,)), profile("acme-b", (1,))),  # one token under two profiles
        config(profile(), review_valid_days=0),
        config(profile(), review_valid_days="180"),
        config(profile(), review_valid_days=True),
    ):
        write_json(path, broken)
        with pytest.raises(ConfigError):
            load_rights_profiles(path)

    write_json(path, config(profile()))
    assert load_rights_profiles(path) == config(profile())


def test_review_valid_days_defaults_to_180(tmp_path):
    cfg = config(profile())
    del cfg["review_valid_days"]
    export, _ = attach(cfg)
    assert export["summary"]["rights_review_valid_days"] == 180


def test_the_shipped_profiles_are_valid_for_the_shipped_export():
    """config/rights_profiles.json is edited through pull requests: a slip there must fail here, not on the site."""
    cfg = load_rights_profiles(CONFIG_DIR / "rights_profiles.json")
    assert cfg is not None and cfg["profiles"], "the repository ships reviewed profiles"
    export = read_json(DEFAULT_DATA_DIR / "exports" / "rwa.json")
    reviewed = max(date.fromisoformat(p["reviewed_at"]) for p in cfg["profiles"])
    warnings = attach_rights(export, cfg, reviewed)
    assert export["summary"]["rights_excluded_profiles_count"] == 0, warnings
    assert len(export["rights_profiles"]) == len(cfg["profiles"])


# ---------- build wiring ----------


def _data_dir(tmp_path):
    data_dir = tmp_path / "data"
    write_json(collect.snapshot_path(data_dir, NVDA), copy.deepcopy(FIXTURE))
    return data_dir


def _fixture_profile(tmp_path, **overrides):
    """A profile for the first tokenized NVDA token of the fixture, whatever its ids are."""
    export, _ = build_export_with_warnings([NVDA], load_settings(), _data_dir(tmp_path), config_dir=tmp_path / "no-config")
    token = next(t for t in export["tokens"] if t["kind"] == "tokenized" and t["issuer_id"])
    issuer = next(i for i in export["issuers"] if i["id"] == token["issuer_id"])
    return token, profile("fixture-line", (token["id"],), issuer_id=issuer["id"], cmc_label=issuer["cmc_label"], **overrides)


def test_build_export_attaches_rights_from_the_config_dir(tmp_path):
    token, prof = _fixture_profile(tmp_path)
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof))
    export, warnings = build_export_with_warnings([NVDA], load_settings(), tmp_path / "data", config_dir=tmp_path / "config")
    assert warnings == []
    assert next(t for t in export["tokens"] if t["id"] == token["id"])["rights_profile_id"] == "fixture-line"
    assert export["summary"]["rights_profiles_count"] == 1
    assert all("link" in o for o in export["opportunities"])  # the links layer is still attached


def _cli(tmp_path, command="build"):
    args = [command, "--data-dir", str(tmp_path / "data"), "--frontend-dir", str(tmp_path / "no-site"), "--config-dir", str(tmp_path / "config")]
    return CliRunner().invoke(cli.app, args)


def test_cli_build_exits_2_when_a_profile_was_excluded_and_still_writes_the_export(tmp_path):
    _, prof = _fixture_profile(tmp_path, reviewed_at="2031-01-01")
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof))
    result = _cli(tmp_path)
    assert result.exit_code == 2, result.output
    assert any("warning" in line and "fixture-line" in line for line in result.output.splitlines())
    written = json.loads((tmp_path / "data" / "exports" / "rwa.json").read_text())
    assert written["rights_profiles"] == [] and written["summary"]["rights_excluded_profiles_count"] == 1


def test_cli_build_exits_0_with_valid_or_merely_old_profiles(tmp_path):
    _, prof = _fixture_profile(tmp_path, reviewed_at="2020-01-01")
    for row in prof["facts"].values():
        for e in row.get("evidence", []):
            e["retrieved_at"] = "2020-01-01"
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof))
    result = _cli(tmp_path)
    assert result.exit_code == 0, result.output
    written = json.loads((tmp_path / "data" / "exports" / "rwa.json").read_text())
    assert written["rights_profiles"][0]["stale"] is True


def test_cli_build_exits_0_when_a_profile_is_outside_the_basket(tmp_path):
    _, prof = _fixture_profile(tmp_path)
    elsewhere = profile("elsewhere-line", (987654321,), issuer_id="issuer-elsewhere", cmc_label="Elsewhere Assets")
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof, elsewhere))
    result = _cli(tmp_path)
    assert result.exit_code == 0, result.output
    assert any("warning" in line and "elsewhere-line" in line for line in result.output.splitlines())
    written = json.loads((tmp_path / "data" / "exports" / "rwa.json").read_text())
    assert [p["id"] for p in written["rights_profiles"]] == ["fixture-line"]


def _cli_export(tmp_path):
    args = ["export", "--format", "csv", "--out", str(tmp_path / "tables"), "--data-dir", str(tmp_path / "data"), "--config-dir", str(tmp_path / "config")]
    return CliRunner().invoke(cli.app, args)


def test_cli_export_reads_the_same_config_dir_and_reports_what_the_build_reports(tmp_path):
    import csv

    token, prof = _fixture_profile(tmp_path)
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof))
    result = _cli_export(tmp_path)
    assert result.exit_code == 0, result.output
    with (tmp_path / "tables" / "rights_tokens.csv").open(newline="", encoding="utf-8") as f:
        rows = {row["token_id"]: row for row in csv.DictReader(f)}
    assert rows[str(token["id"])]["rights_profile_id"] == "fixture-line"  # the tables and the JSON come from one config

    # a profile the build would exclude: the tables are written, the warning is printed, the exit code says so
    _, broken = _fixture_profile(tmp_path, reviewed_at="2031-01-01")
    write_json(tmp_path / "config" / "rights_profiles.json", config(broken))
    result = _cli_export(tmp_path)
    assert result.exit_code == 2, result.output
    assert any("warning" in line and "fixture-line" in line for line in result.output.splitlines())
    with (tmp_path / "tables" / "rights_tokens.csv").open(newline="", encoding="utf-8") as f:
        assert all(row["rights_profile_id"] == "" for row in csv.DictReader(f))

    # a broken config is a config error here as well
    (tmp_path / "config" / "rights_profiles.json").write_text("{not json")
    assert _cli_export(tmp_path).exit_code == 1


def test_review_dates_are_dates_in_the_workbook(tmp_path):
    from datetime import datetime

    from openpyxl import load_workbook

    from rwa_pipeline.tables import write_tables

    _, prof = _fixture_profile(tmp_path)
    write_json(tmp_path / "config" / "rights_profiles.json", config(prof))
    export, _ = build_export_with_warnings([NVDA], load_settings(), tmp_path / "data", config_dir=tmp_path / "config")
    (xlsx,) = write_tables(export, "xlsx", tmp_path / "out.xlsx", tmp_path)
    wb = load_workbook(xlsx)
    for sheet, columns in (("rights_tokens", ["reviewed_at"]), ("rights_facts", ["reviewed_at", "retrieved_at"])):
        ws = wb[sheet]
        header = [cell.value for cell in ws[1]]
        for column in columns:
            cells = [row[header.index(column)] for row in ws.iter_rows(min_row=2) if row[header.index(column)].value is not None]
            assert cells, (sheet, column)
            for cell in cells:
                assert isinstance(cell.value, datetime) and cell.value == datetime(2026, 11, 13), (sheet, column, cell.value)
                assert cell.number_format == "yyyy-mm-dd", (sheet, column, cell.number_format)
    # the date printed on a document is free text ("June 2026", "undated") and stays text
    ws = wb["rights_facts"]
    header = [cell.value for cell in ws[1]]
    assert {type(row[header.index("doc_date")].value) for row in ws.iter_rows(min_row=2) if row[header.index("doc_date")].value is not None} == {str}


def test_cli_build_with_a_broken_rights_file_is_a_config_error(tmp_path):
    _data_dir(tmp_path)
    (tmp_path / "config").mkdir()
    (tmp_path / "config" / "rights_profiles.json").write_text("{not json")
    result = _cli(tmp_path)
    assert result.exit_code == 1
    assert not (tmp_path / "data" / "exports" / "rwa.json").exists()
