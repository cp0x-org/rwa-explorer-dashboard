"""HTTP client shared by CMC and Portals: rate limit, retries, raw response capture."""

import random
import re
import time
from pathlib import Path

import httpx

from rwa_pipeline.config import now_iso, write_json

RETRY_STATUSES = {429, 500, 502, 503, 504}


class ApiError(Exception):
    def __init__(self, provider: str, status: int | None, message: str):
        super().__init__(f"{provider}: HTTP {status}: {message}")
        self.provider = provider
        self.status = status
        self.message = message


class AuthError(ApiError):
    pass


class Api:
    def __init__(
        self,
        provider: str,
        base_url: str,
        headers: dict,
        settings: dict,
        raw_dir: Path | None = None,
    ):
        self.provider = provider
        self.client = httpx.Client(base_url=base_url, headers=headers, timeout=settings["request_timeout_seconds"])
        self.min_interval = 60.0 / settings["requests_per_minute"][provider]
        self.max_retries = settings["max_retries"]
        self.raw_dir = raw_dir
        self.requests = 0
        self.credits = 0
        self._last_request = 0.0
        # Optional observer (provider, path, params, status, seconds) called after every answered request (`--verbose`).
        self.on_request = None

    def get(self, path: str, params: dict) -> dict:
        attempt = 0
        while True:
            wait = self._last_request + self.min_interval - time.monotonic()
            if wait > 0:
                time.sleep(wait)
            self._last_request = time.monotonic()
            self.requests += 1
            started = time.monotonic()
            try:
                resp = self.client.get(path, params=params)
            except httpx.TransportError as e:
                if attempt >= self.max_retries:
                    raise ApiError(self.provider, None, f"network error: {e}") from e
                self._backoff(attempt, None)
                attempt += 1
                continue

            if resp.status_code in RETRY_STATUSES and attempt < self.max_retries:
                self._backoff(attempt, resp.headers.get("Retry-After"))
                attempt += 1
                continue

            try:
                body = resp.json()
            except ValueError:
                body = {"_non_json_body": resp.text[:2000]}
            self._save_raw(path, params, resp.status_code, body)
            if self.on_request:
                self.on_request(self.provider, path, params, resp.status_code, time.monotonic() - started)

            if resp.status_code in (401, 403):
                raise AuthError(self.provider, resp.status_code, _error_message(body))
            if resp.status_code >= 400:
                raise ApiError(self.provider, resp.status_code, _error_message(body))
            return body

    def _backoff(self, attempt: int, retry_after: str | None) -> None:
        delay = min(60.0, 2.0**attempt) + random.uniform(0, 1)
        if retry_after and retry_after.isdigit():
            delay = max(delay, float(retry_after))
        time.sleep(delay)

    def _save_raw(self, path: str, params: dict, status: int, body) -> None:
        if self.raw_dir is None:
            return
        name = f"{self.requests:04d}_{re.sub(r'[^A-Za-z0-9]+', '_', path).strip('_')}.json"
        write_json(
            self.raw_dir / self.provider / name,
            # No headers are stored: auth lives only in headers.
            {"url": str(self.client.base_url) + path, "params": params, "status": status, "fetched_at": now_iso(), "body": body},
        )


def _error_message(body) -> str:
    if isinstance(body, dict):
        status = body.get("status")
        if isinstance(status, dict) and status.get("error_message"):
            return str(status["error_message"])
        for key in ("message", "error", "_non_json_body"):
            if body.get(key):
                return str(body[key])[:500]
    return str(body)[:500]
