"""Reviewed holder rights apply only to explicitly listed token IDs."""

import copy
import json
import re
from datetime import date
from pathlib import Path

from rwa_pipeline.config import CONFIG_DIR, ConfigError
from rwa_pipeline.deals import clean_https_url

FACTS = {
    "instrument": {"group": "what", "values": ["share", "beneficial_interest", "secured_debt", "unsecured_debt", "derivative", "contractual_claim", "no_claim"], "needs_who": []},
    "backing": {"group": "what", "values": ["full_1to1", "full", "partial", "none"], "needs_who": []},
    "asset_protection": {"group": "what", "values": ["pledged_to_holders", "trust_or_segregated", "none"], "needs_who": []},
    "primary_access": {"group": "who", "values": ["available"], "needs_who": ["available"]},
    "redemption": {"group": "who", "values": ["available", "at_events_only", "not_redeemable"], "needs_who": ["available", "at_events_only"]},
    "claim_scope": {"group": "who", "values": ["has_claim", "no_claim"], "needs_who": ["has_claim"]},
    "restricted": {"group": "who", "values": ["none_stated", "listed"], "needs_who": []},
    "transfer": {"group": "who", "values": ["free", "blocklist", "allowlist"], "needs_who": []},
    "freeze": {"group": "issuer_powers", "values": ["yes", "no"], "needs_who": []},
    "cancellation": {"group": "issuer_powers", "values": ["yes", "no"], "needs_who": []},
    "pause": {"group": "issuer_powers", "values": ["yes", "no"], "needs_who": []},
    "upgrade": {"group": "issuer_powers", "values": ["yes", "no"], "needs_who": []},
    "forced_redemption": {"group": "issuer_powers", "values": ["yes", "no"], "needs_who": []},
    "loss_exploit": {"group": "loss", "values": ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], "needs_who": ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]},
    "loss_lost_tokens": {"group": "loss", "values": ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], "needs_who": ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]},
    "loss_issuer_insolvency": {"group": "loss", "values": ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], "needs_who": ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]},
    "loss_custodian_failure": {"group": "loss", "values": ["holder_bears", "replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim", "not_addressed"], "needs_who": ["replacement_possible", "compensation_stated", "claim_on_assets", "unsecured_claim"]},
    "dividends": {"group": "shareholder", "values": ["paid_out", "reinvested", "none"], "needs_who": []},
    "voting": {"group": "shareholder", "values": ["none", "pass_through", "discretionary"], "needs_who": ["discretionary"]},
    "reserves_proof": {"group": "other", "values": ["published", "none"], "needs_who": []},
    "governing_law": {"group": "other", "values": ["stated"], "needs_who": []},
}
WHO = ["anyone", "verified", "professional", "institutional", "platform_clients", "eligible_holders", "nobody"]


def load_rights_profiles(path: Path = CONFIG_DIR / "rights_profiles.json") -> dict | None:
    """Reject file-wide errors; individual profiles are checked when attached."""
    try:
        config = json.loads(path.read_text())
    except FileNotFoundError:
        return None
    except (ValueError, OSError) as e:
        raise ConfigError(f"{path}: invalid rights config: {e}") from e

    def require(condition, message):
        if not condition:
            raise ConfigError(f"{path}: {message}")

    require(isinstance(config, dict), "expected an object")
    require(type(config.get("schema_version")) is int and config["schema_version"] == 1, "schema_version must be 1")
    require(isinstance(config.get("profiles"), list), "profiles must be a list")
    if "review_valid_days" in config:
        days = config["review_valid_days"]
        require(type(days) is int and days > 0, "review_valid_days must be a positive integer")
    ids, tokens = set(), set()
    for profile in config["profiles"]:
        if not isinstance(profile, dict):
            continue
        profile_id = profile.get("id")
        if isinstance(profile_id, str):
            require(profile_id not in ids, f"duplicate profile id {profile_id!r}")
            ids.add(profile_id)
        token_ids = profile.get("token_ids")
        if isinstance(token_ids, list):
            listed = {token_id for token_id in token_ids if type(token_id) is int}
            for token_id in listed:
                require(token_id not in tokens, f"token {token_id} belongs to two profiles")
            tokens.update(listed)
    return config


def _require(condition, location: str, expected: str) -> None:
    if not condition:
        raise ValueError(f"{location}: expected {expected}")


def _object(value, keys, location: str) -> None:
    """Every object has a closed set of keys."""
    _require(isinstance(value, dict), location, "an object")
    for key in value:
        _require(key in keys, location, f"known keys only (unexpected {key!r})")


def _text(value, location: str) -> None:
    _require(isinstance(value, str) and bool(value.strip()), location, "a nonempty string")


def _country(value, location: str) -> None:
    _require(isinstance(value, str) and re.fullmatch(r"[A-Z]{2}", value), location, "a two-letter uppercase country code")


def _date(value, location: str, today: date) -> date:
    """Review and retrieval dates are ISO calendar dates no later than today."""
    _require(isinstance(value, str) and re.fullmatch(r"[0-9]{4}-[0-9]{2}-[0-9]{2}", value), location, "an ISO date")
    try:
        parsed = date.fromisoformat(value)
    except ValueError as e:
        raise ValueError(f"{location}: expected an ISO date") from e
    _require(parsed <= today, location, "a date no later than today")
    return parsed


def _note(value, location: str) -> None:
    _object(value, {"en", "zh"}, location)
    for language in ("en", "zh"):
        _text(value.get(language), f"{location}.{language}")


def _evidence(value, location: str, today: date) -> None:
    """Every cited fact has a short verbatim quote and a clean HTTPS source."""
    _require(isinstance(value, list) and bool(value), location, "a nonempty evidence list")
    for index, citation in enumerate(value):
        at = f"{location}[{index}]"
        _object(citation, {"quote", "url", "title", "doc_date", "retrieved_at"}, at)
        _text(citation.get("quote"), f"{at}.quote")
        quote = " ".join(citation["quote"].split())
        _require(len(quote) <= 600, f"{at}.quote", "at most 600 characters")
        _require(len(quote.split()) >= 6, f"{at}.quote", "at least six words")
        _require(clean_https_url(citation.get("url")), f"{at}.url", "a clean HTTPS URL")
        for key in ("title", "doc_date"):
            _text(citation.get(key), f"{at}.{key}")
        _date(citation.get("retrieved_at"), f"{at}.retrieved_at", today)


def _fact(key: str, value, today: date) -> None:
    at = f"facts.{key}"
    _object(value, {"value", "who", "countries", "zones", "us_persons", "note", "evidence", "checked"}, at)
    spec = FACTS[key]
    choice = value.get("value")
    _require(choice in [*spec["values"], "unknown"], f"{at}.value", "an allowed value or unknown")
    if choice == "unknown":
        _require("evidence" not in value, at, "no evidence for unknown")
        checked = value.get("checked")
        _require(isinstance(checked, list) and bool(checked), f"{at}.checked", "a nonempty list of clean HTTPS URLs")
        for url in checked:
            _require(clean_https_url(url), f"{at}.checked", "clean HTTPS URLs")
    else:
        _require("checked" not in value, at, "checked only for unknown")
        _evidence(value.get("evidence"), f"{at}.evidence", today)
    if choice in spec["needs_who"]:
        _require(value.get("who") in WHO, f"{at}.who", "an allowed holder category")
    else:
        _require("who" not in value, at, "who only on values that need it")
    if "countries" in value:
        _require(isinstance(value["countries"], list), f"{at}.countries", "a list")
        for country in value["countries"]:
            _country(country, f"{at}.countries")
    if "zones" in value:
        _require(isinstance(value["zones"], list), f"{at}.zones", "a list")
        for zone in value["zones"]:
            _text(zone, f"{at}.zones")
            _require(len(zone) <= 40, f"{at}.zones", "strings of at most 40 characters")
    if "us_persons" in value:
        _require(key == "restricted" and isinstance(value["us_persons"], bool), f"{at}.us_persons", "a boolean on restricted only")
    if key == "restricted" and choice == "listed":
        _require(bool(value.get("countries")) or bool(value.get("zones")) or value.get("us_persons") is True, at, "countries, zones or us_persons: true")
    if key == "restricted" and choice == "none_stated":
        _require("countries" not in value and "zones" not in value and value.get("us_persons") is not True, at, "no restriction details for none_stated")
    if key == "governing_law" and choice == "stated":
        _require(bool(value.get("countries")) or bool(value.get("zones")), at, "countries or zones")
    if "note" in value:
        _note(value["note"], f"{at}.note")


def _profile(profile, issuers: dict, tokens: dict, today: date) -> date:
    """A profile is accepted only after all its fields and bindings pass."""
    _object(profile, {
        "id", "issuer_id", "cmc_label", "product", "token_ids", "scope_basis", "scope_evidence",
        "reviewed_at", "legal_issuer", "facts", "documents",
    }, "profile")
    profile_id = profile.get("id")
    _require(isinstance(profile_id, str) and re.fullmatch(r"[a-z0-9-]+", profile_id), "id", "lowercase letters, digits and hyphens")
    for key in ("issuer_id", "cmc_label", "product"):
        _text(profile.get(key), key)
    token_ids = profile.get("token_ids")
    _require(isinstance(token_ids, list) and bool(token_ids), "token_ids", "a nonempty list of integers")
    _require(all(type(token_id) is int for token_id in token_ids), "token_ids", "integers without booleans")
    _require(len(token_ids) == len(set(token_ids)), "token_ids", "unique token IDs")
    basis = profile.get("scope_basis")
    _require(basis in ["per_token_documents", "programme_documents"], "scope_basis", "per_token_documents or programme_documents")
    if basis == "per_token_documents":
        _require(len(token_ids) == 1, "token_ids", "exactly one token for per_token_documents")
    if basis == "programme_documents" or "scope_evidence" in profile:
        _evidence(profile.get("scope_evidence"), "scope_evidence", today)
    reviewed_at = _date(profile.get("reviewed_at"), "reviewed_at", today)
    issuer = issuers.get(profile["issuer_id"])
    if issuer is None:
        _require(not any(token_id in tokens for token_id in token_ids), "issuer_id", "an issuer present in the export")
    else:
        _require(issuer.get("cmc_label") == profile["cmc_label"], "cmc_label", "the export issuer label")
    for token_id in token_ids:
        token = tokens.get(token_id)
        _require(token is None or token.get("issuer_id") == profile["issuer_id"], f"token_ids[{token_id}]", "a token of this issuer")
    legal = profile.get("legal_issuer")
    _object(legal, {"name", "country", "zone", "note", "evidence"}, "legal_issuer")
    _text(legal.get("name"), "legal_issuer.name")
    _country(legal.get("country"), "legal_issuer.country")
    if "zone" in legal:
        _text(legal["zone"], "legal_issuer.zone")
    if "note" in legal:
        _note(legal["note"], "legal_issuer.note")
    _evidence(legal.get("evidence"), "legal_issuer.evidence", today)
    facts = profile.get("facts")
    _object(facts, FACTS, "facts")
    for key in FACTS:
        _fact(key, facts.get(key), today)
    documents = profile.get("documents")
    _require(isinstance(documents, list) and bool(documents), "documents", "a nonempty document list")
    for index, document in enumerate(documents):
        at = f"documents[{index}]"
        _object(document, {"title", "url", "type", "doc_date"}, at)
        for key in ("title", "doc_date"):
            _text(document.get(key), f"{at}.{key}")
        _require(clean_https_url(document.get("url")), f"{at}.url", "a clean HTTPS URL")
        _require(document.get("type") in ["terms", "prospectus", "risk", "faq", "register", "other"], f"{at}.type", "an allowed document type")
    return reviewed_at


def attach_rights(export: dict, config: dict | None, today: date) -> list[str]:
    """Publish valid profiles as copies; excluded profiles never bind tokens."""
    warnings = []
    profiles = export["rights_profiles"] = []
    export["rights_schema"] = copy.deepcopy({"unknown_value": "unknown", "who": WHO, "facts": [{"key": key, **spec} for key, spec in FACTS.items()]})
    issuers = {issuer["id"]: issuer for issuer in export["issuers"]}
    tokens = {token["id"]: token for token in export["tokens"]}
    for token in export["tokens"]:
        token["rights_profile_id"] = None
    for issuer in export["issuers"]:
        issuer["rights_profile_ids"] = []
        issuer["rights_reviewed_tokens_count"] = 0
    valid_days = config.get("review_valid_days", 180) if config is not None else 180
    excluded = 0
    for index, profile in enumerate(config["profiles"] if config is not None else [], start=1):
        name = profile.get("id", f"#{index}") if isinstance(profile, dict) else f"#{index}"
        try:
            reviewed_at = _profile(profile, issuers, tokens, today)
        except ValueError as e:
            warnings.append(f"rights profile {name}: excluded: {e}")
            excluded += 1
            continue
        present = [token_id for token_id in profile["token_ids"] if token_id in tokens]
        missing = list(dict.fromkeys(token_id for token_id in profile["token_ids"] if token_id not in tokens))
        if not present:
            warnings.append(f"rights profile {name}: outside the export; absent tokens: {', '.join(map(str, missing))}")
            continue
        if missing:
            warnings.append(f"rights profile {name}: tokens {', '.join(map(str, missing))} are absent from the export")
        published = copy.deepcopy(profile)
        published["token_ids"] = present
        citations = published.get("scope_evidence", []) + published["legal_issuer"]["evidence"]
        for fact in published["facts"].values():
            citations.extend(fact.get("evidence", []))
        for citation in citations:
            citation["quote"] = " ".join(citation["quote"].split())
        published["stale"] = (today - reviewed_at).days > valid_days
        profiles.append(published)
        if published["stale"]:
            warnings.append(f"rights profile {name}: review is stale (older than {valid_days} days)")
        issuer = issuers[profile["issuer_id"]]
        issuer["rights_profile_ids"].append(profile["id"])
        for token_id in dict.fromkeys(present):
            tokens[token_id]["rights_profile_id"] = profile["id"]
    for token in export["tokens"]:
        if token["rights_profile_id"] is not None:
            issuers[token["issuer_id"]]["rights_reviewed_tokens_count"] += 1
    export["summary"].update({
        "rights_profiles_count": len(profiles),
        "rights_reviewed_tokens_count": sum(token["rights_profile_id"] is not None for token in export["tokens"]),
        "rights_review_valid_days": valid_days,
        "rights_excluded_profiles_count": excluded,
        "rights_warnings_count": len(warnings),
    })
    return warnings
