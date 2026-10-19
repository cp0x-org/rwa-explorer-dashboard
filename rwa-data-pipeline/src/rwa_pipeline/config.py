"""Config loading, paths and small JSON storage helpers."""

import json
import os
import re
import tempfile
from dataclasses import dataclass
from datetime import UTC, datetime
from pathlib import Path

from dotenv import load_dotenv

PROJECT_ROOT = Path(__file__).resolve().parents[2]
CONFIG_DIR = PROJECT_ROOT / "config"
DEFAULT_DATA_DIR = PROJECT_ROOT / "data"
DEFAULT_FRONTEND_DIR = PROJECT_ROOT.parent / "pi-rwa-interface"

ASSET_TYPES = {"stock", "etf"}


class ConfigError(Exception):
    pass


@dataclass(frozen=True)
class Asset:
    ticker: str
    name: str
    asset_type: str
    isin: str | None
    enabled: bool

    @property
    def id(self) -> str:
        # ISIN identifies the underlying; assets without ISIN get a namespaced fallback, never a fake ISIN.
        return self.isin or f"{self.asset_type}:ticker:{self.ticker}"

    @property
    def storage_id(self) -> str:
        return storage_id(self.id)


def storage_id(underlying_id: str) -> str:
    """File-name-safe form of an underlying id (an ISIN stays as is)."""
    return re.sub(r"[^A-Za-z0-9_.-]", "_", underlying_id)


def isin_is_valid(isin: str) -> bool:
    if not re.fullmatch(r"[A-Z]{2}[A-Z0-9]{9}[0-9]", isin):
        return False
    digits = "".join(str(int(ch, 36)) for ch in isin[:-1])
    total = 0
    # Luhn over the expanded digit string, rightmost digit doubled first.
    for i, ch in enumerate(reversed(digits)):
        d = int(ch)
        if i % 2 == 0:
            d *= 2
            if d > 9:
                d -= 9
        total += d
    return (10 - total % 10) % 10 == int(isin[-1])


def load_assets(path: Path = CONFIG_DIR / "assets.json") -> list[Asset]:
    raw = json.loads(path.read_text())
    assets, seen_ids, seen_tickers = [], set(), set()
    for i, item in enumerate(raw.get("assets", [])):
        ticker = str(item.get("ticker", "")).strip().upper()
        isin = item.get("isin") or None
        asset_type = item.get("asset_type")
        if not ticker:
            raise ConfigError(f"assets[{i}]: ticker is required")
        if asset_type not in ASSET_TYPES:
            raise ConfigError(f"{ticker}: asset_type must be one of {sorted(ASSET_TYPES)}")
        if isin is not None and not isin_is_valid(isin):
            raise ConfigError(f"{ticker}: invalid ISIN {isin!r} (format or check digit)")
        asset = Asset(ticker, item.get("name") or ticker, asset_type, isin, bool(item.get("enabled", True)))
        if asset.id in seen_ids or ticker in seen_tickers:
            raise ConfigError(f"{ticker}: duplicate ticker or ISIN")
        seen_ids.add(asset.id)
        seen_tickers.add(ticker)
        assets.append(asset)
    return assets


def load_settings(path: Path = CONFIG_DIR / "settings.json") -> dict:
    return json.loads(path.read_text())


def load_api_keys() -> tuple[str, str]:
    load_dotenv(PROJECT_ROOT / ".env")
    cmc = os.environ.get("CMC_API_KEY", "").strip()
    portals = os.environ.get("PORTALS_API_KEY", "").strip()
    if portals.lower().startswith("bearer "):
        portals = portals[7:].strip()
    if not cmc or not portals:
        raise ConfigError("CMC_API_KEY and PORTALS_API_KEY must be set in rwa-data-pipeline/.env")
    return cmc, portals


def now_iso() -> str:
    return datetime.now(UTC).strftime("%Y-%m-%dT%H:%M:%SZ")


def read_json(path: Path, default=None):
    if not path.exists():
        return default
    return json.loads(path.read_text())


def write_json(path: Path, data, compact: bool = False) -> None:
    """Atomic write: temp file in the same directory, then replace."""
    path.parent.mkdir(parents=True, exist_ok=True)
    fd, tmp = tempfile.mkstemp(dir=path.parent, prefix=f".{path.name}.", suffix=".tmp")
    try:
        with os.fdopen(fd, "w") as f:
            json.dump(data, f, ensure_ascii=False, allow_nan=False, **({"separators": (",", ":")} if compact else {"indent": 1}))
        os.chmod(tmp, 0o644)  # mkstemp creates 0600; the export is served as a static file
        os.replace(tmp, path)
    except BaseException:
        Path(tmp).unlink(missing_ok=True)
        raise
