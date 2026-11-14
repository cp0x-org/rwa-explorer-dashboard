"""`build`: recompute analytics from saved snapshots (no network) and write the frontend JSON."""

import json
import shutil
from datetime import UTC, datetime
from pathlib import Path

from rwa_pipeline.analyze import build, norm_key
from rwa_pipeline.collect import snapshot_path
from rwa_pipeline.config import CONFIG_DIR, Asset, now_iso, read_json, storage_id, write_json
from rwa_pipeline.curated import attach_links, load_links
from rwa_pipeline.deals import DEALS_FILE
from rwa_pipeline.logos import find_logo
from rwa_pipeline.rights import attach_rights, load_rights_profiles

EXPORT_RELATIVE_PATH = Path("public/data/rwa.json")
PUBLIC_DIR = Path("public")
LOGOS_PUBLIC_PATH = Path("logos")  # served as /logos/<kind>/<file>


def raw_portals_records(snapshots: list[dict]) -> dict:
    """Match analytics: normalize keys and let the newest snapshot win."""
    records = {}
    for snapshot in sorted(snapshots, key=lambda s: s.get("fetched_at") or ""):
        portals = snapshot.get("portals") or {}
        for section in ("records", "exact"):
            for key, record in portals.get(section, {}).items():
                records[norm_key(key)] = record
    return records


def build_export(assets: list[Asset], settings: dict, data_dir: Path, config_dir: Path = CONFIG_DIR) -> dict:
    """Build the frontend export from local snapshots and caches only."""
    return build_export_with_warnings(assets, settings, data_dir, config_dir)[0]


def build_export_with_warnings(assets: list[Asset], settings: dict, data_dir: Path, config_dir: Path = CONFIG_DIR) -> tuple[dict, list[str]]:
    """Build the export and report unusable curated actions and rights profiles."""
    links = load_links(config_dir / "links.json")
    rights = load_rights_profiles(config_dir / "rights_profiles.json")
    # Only enabled assets form the active basket; snapshots of disabled ones stay on disk as history.
    snapshots = [s for a in assets if a.enabled and (s := read_json(snapshot_path(data_dir, a)))]
    result = build(
        snapshots,
        read_json(data_dir / "issuers.json", {}),
        settings,
        read_json(data_dir / "portals_platforms.json", {}),
    )
    # Logos point to local files only; the site never loads images from CMC or issuer websites.
    logos_dir = data_dir / "logos"
    rwa_info = read_json(data_dir / "rwa_info.json", {})
    for u in result["underlyings"]:
        u["website"] = rwa_info.get(str(u["cmc_rwa_id"]), {}).get("website")
    for kind, items in (("tokens", result["tokens"]), ("issuers", result["issuers"]), ("underlyings", result["underlyings"])):
        for item in items:
            entity_id = storage_id(item["id"]) if kind == "underlyings" else str(item["id"])
            path = find_logo(logos_dir, kind, entity_id)
            item["logo"] = (LOGOS_PUBLIC_PATH / kind / path.name).as_posix() if path else None
    export = {"schema_version": 1, "generated_at": now_iso(), **result}
    raw_records = raw_portals_records(snapshots)
    try:
        deals = read_json(data_dir / DEALS_FILE)
    except (json.JSONDecodeError, UnicodeError, OSError):
        deals = None
    now = datetime.now(UTC)
    warnings = attach_links(export, links, deals, raw_records, now)
    warnings.extend(attach_rights(export, rights, now.date()))
    return export, warnings


def write_export(export: dict, data_dir: Path, frontend_dir: Path | None) -> list[Path]:
    targets = [data_dir / "exports" / "rwa.json"]
    if frontend_dir is not None:
        # Logo files first, then the JSON that references them.
        for item in export["tokens"] + export["issuers"] + export["underlyings"]:
            if item["logo"]:
                src = data_dir / item["logo"]
                dst = frontend_dir / PUBLIC_DIR / item["logo"]
                if not dst.exists() or dst.read_bytes() != src.read_bytes():
                    dst.parent.mkdir(parents=True, exist_ok=True)
                    shutil.copyfile(src, dst)
        targets.append(frontend_dir / EXPORT_RELATIVE_PATH)
    # One file -> the frontend never mixes tokens and issuers from different builds.
    for target in targets:
        write_json(target, export, compact=True)
    return targets
