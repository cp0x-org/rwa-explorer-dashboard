"""`build`: recompute analytics from saved snapshots (no network) and write the frontend JSON."""

import shutil
from pathlib import Path

from rwa_pipeline.analyze import build
from rwa_pipeline.collect import snapshot_path
from rwa_pipeline.config import Asset, now_iso, read_json, storage_id, write_json
from rwa_pipeline.logos import find_logo

EXPORT_RELATIVE_PATH = Path("public/data/rwa.json")
PUBLIC_DIR = Path("public")
LOGOS_PUBLIC_PATH = Path("logos")  # served as /logos/<kind>/<file>


def build_export(assets: list[Asset], settings: dict, data_dir: Path) -> dict:
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
    return {"schema_version": 1, "generated_at": now_iso(), **result}


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
