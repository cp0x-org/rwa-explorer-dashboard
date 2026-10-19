"""Logos of tokens, issuers and underlyings, downloaded once into data/logos/ and published with the export.

Sources, first that works: the CMC `logo` (token info, issuer, RWA metadata); otherwise, for the official website listed
in CMC, Google's favicon service (corporate sites often block scripts) and then the icon declared by the site itself.
Existing files are never re-downloaded, so a manually placed data/logos/<kind>/<id>.<ext> overrides everything."""

import glob
import re
from pathlib import Path
from urllib.parse import urljoin, urlparse

import httpx

from rwa_pipeline.config import read_json

EXTENSIONS = {"image/png": ".png", "image/jpeg": ".jpg", "image/svg+xml": ".svg", "image/webp": ".webp", "image/x-icon": ".ico",
              "image/vnd.microsoft.icon": ".ico", "image/gif": ".gif"}
HEADERS = {"User-Agent": "Mozilla/5.0 (Macintosh; Intel Mac OS X 14_0) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/126 Safari/537.36"}
MAX_BYTES = 2_000_000
FAVICON_SERVICE = "https://www.google.com/s2/favicons?sz=128&domain={domain}"
LINK_TAG = re.compile(r"<link\b[^>]*>", re.I)
ATTR = re.compile(r'([a-zA-Z-]+)\s*=\s*("[^"]*"|\'[^\']*\'|[^\s>]+)')


def find_logo(logos_dir: Path, kind: str, entity_id: str) -> Path | None:
    matches = sorted((logos_dir / kind).glob(f"{glob.escape(entity_id)}.*"))
    return matches[0] if matches else None


def website_icon_url(client: httpx.Client, website: str) -> str | None:
    """Best icon declared in the page <head>: the biggest declared size wins."""
    resp = client.get(website)
    if resp.status_code != 200:
        return None
    best, best_size = None, -1
    for tag in LINK_TAG.findall(resp.text[:300_000]):
        attrs = {k.lower(): v.strip("\"'") for k, v in ATTR.findall(tag)}
        rel = attrs.get("rel", "").lower()
        if "icon" not in rel or not attrs.get("href"):
            continue
        sizes = attrs.get("sizes", "")
        if sizes == "any":
            size = 512
        elif re.match(r"\d+x\d+", sizes):
            size = int(sizes.split("x")[0])
        else:
            size = 57 if "apple-touch-icon" in rel else 32  # undeclared sizes rank below declared ones
        if size > best_size:
            best, best_size = urljoin(str(resp.url), attrs["href"]), size
    return best or urljoin(str(resp.url), "/favicon.ico")


def _download(client: httpx.Client, url: str, target_stem: Path) -> Path | None:
    resp = client.get(url)
    content_type = resp.headers.get("content-type", "").split(";")[0].strip().lower()
    if resp.status_code != 200 or not content_type.startswith("image/") or not 0 < len(resp.content) <= MAX_BYTES:
        return None
    ext = EXTENSIONS.get(content_type) or Path(urlparse(url).path).suffix.lower() or ".png"
    target = target_stem.with_suffix(ext)
    target.parent.mkdir(parents=True, exist_ok=True)
    tmp = target.with_suffix(ext + ".tmp")
    tmp.write_bytes(resp.content)
    tmp.replace(target)
    return target


def _fetch_logo(client: httpx.Client, target_stem: Path, logo: str | None, website: str | None) -> Path | None:
    attempts = []
    if logo:
        attempts.append(lambda: logo)
    if website:
        domain = urlparse(website).hostname
        if domain:
            attempts.append(lambda: FAVICON_SERVICE.format(domain=domain))
        attempts.append(lambda: website_icon_url(client, website))
    for attempt in attempts:
        try:
            url = attempt()
            saved = _download(client, url, target_stem) if url else None
        except httpx.HTTPError:
            saved = None
        if saved:
            return saved
    return None


def download_logos(data_dir: Path, timeout: float = 20, log=None) -> dict:
    """Downloads missing logos. Network errors only skip that logo."""
    log = log or (lambda msg: None)
    logos_dir = data_dir / "logos"
    rwa_info = read_json(data_dir / "rwa_info.json", {})
    jobs: list[tuple[str, str, str | None, str | None]] = []  # kind, id, logo url, website
    for path in sorted((data_dir / "snapshots").glob("*.json")):
        cmc = (read_json(path) or {}).get("cmc") or {}
        for cid, info in (cmc.get("info") or {}).items():
            jobs.append(("tokens", str(cid), info.get("logo"), None))
        meta = rwa_info.get(str((cmc.get("rwa") or {}).get("rwa_id")), {})
        if cmc.get("rwa"):
            jobs.append(("underlyings", path.stem, meta.get("logo"), meta.get("website")))
    for issuer_id, meta in read_json(data_dir / "issuers.json", {}).items():
        jobs.append(("issuers", issuer_id, meta.get("logo"), meta.get("website")))

    report = {"downloaded": [], "failed": [], "missing_source": []}
    pending = [j for j in jobs if not find_logo(logos_dir, j[0], j[1])]
    log(f"logos: {len(jobs) - len(pending)} already on disk, {len(pending)} to download")
    with httpx.Client(timeout=timeout, follow_redirects=True, headers=HEADERS) as client:
        for n, (kind, entity_id, logo, website) in enumerate(pending, 1):
            if not logo and not website:
                report["missing_source"].append(f"{kind}/{entity_id}")
                log(f"  [{n}/{len(pending)}] {kind}/{entity_id}: no logo or website in CMC, skipped")
                continue
            saved = _fetch_logo(client, logos_dir / kind / entity_id, logo, website)
            (report["downloaded"] if saved else report["failed"]).append(f"{kind}/{entity_id}")
            log(f"  [{n}/{len(pending)}] {kind}/{entity_id}: {'saved ' + saved.name if saved else 'failed'}")
    return report
