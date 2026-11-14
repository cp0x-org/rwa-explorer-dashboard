"""rwa-interface/build.sh pushes the tag that docker-compose runs by default."""

import os
import subprocess
from pathlib import Path

SITE = Path(__file__).resolve().parents[2] / "rwa-interface"


def _run(tmp_path, *args):
    bin_dir = tmp_path / "bin"
    bin_dir.mkdir(exist_ok=True)
    log = tmp_path / "docker.log"
    fake = bin_dir / "docker"
    fake.write_text(f'#!/bin/sh\nprintf "%s\\n" "$*" >> "{log}"\n')
    fake.chmod(0o755)
    env = {**os.environ, "PATH": f"{bin_dir}{os.pathsep}{os.environ.get('PATH', '')}"}
    result = subprocess.run(["bash", "build.sh", *args], cwd=SITE, env=env, capture_output=True, text=True, timeout=60)
    calls = log.read_text().splitlines() if log.exists() else []
    return result, calls


def _tags(words):
    tags = []
    for i, word in enumerate(words):
        if word in ("-t", "--tag") and i + 1 < len(words):
            tags.append(words[i + 1])
        elif word.startswith("--tag="):
            tags.append(word.split("=", 1)[1])
    return tags


def test_the_image_is_pushed_with_its_version_and_latest(tmp_path):
    result, calls = _run(tmp_path, "1.2.3")
    assert result.returncode == 0, result.stderr
    assert len(calls) == 1, calls  # one multi-arch build that pushes both tags
    words = calls[0].split()
    assert words[:2] == ["buildx", "build"]
    assert sorted(_tags(words)) == ["cp0x/pi-rwa-interface:1.2.3", "cp0x/pi-rwa-interface:latest"]
    assert "--push" in words
    assert "linux/amd64,linux/arm64" in words


def test_docker_compose_runs_latest_unless_a_version_is_given():
    compose = (SITE / "docker-compose.yml").read_text()
    assert "image: cp0x/pi-rwa-interface:${VERSION:-latest}" in compose


def test_the_image_build_still_requires_a_version(tmp_path):
    result, calls = _run(tmp_path)
    assert result.returncode == 1
    assert "Usage" in result.stdout
    assert calls == []
