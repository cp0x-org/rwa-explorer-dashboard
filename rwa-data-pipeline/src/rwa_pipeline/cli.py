"""CLI: `sync` collects data (then builds), `build` recomputes and exports without network, `export` writes CSV/XLSX tables."""

from enum import Enum
from pathlib import Path
from typing import Annotated

import typer

from rwa_pipeline.collect import fill_missing_rwa_info
from rwa_pipeline.collect import sync as run_sync
from rwa_pipeline.config import DEFAULT_DATA_DIR, DEFAULT_FRONTEND_DIR, ConfigError, load_assets, load_settings
from rwa_pipeline.export import build_export, write_export
from rwa_pipeline.logos import download_logos
from rwa_pipeline.tables import write_tables

app = typer.Typer(add_completion=False, no_args_is_help=True)


class TablesFormat(str, Enum):
    csv = "csv"
    xlsx = "xlsx"
    all = "all"


DataDir = Annotated[Path, typer.Option(help="Where snapshots, raw responses and run reports live")]
FrontendDir = Annotated[Path, typer.Option(help="pi-rwa-interface root; JSON goes to public/data/rwa.json")]
Verbose = Annotated[bool, typer.Option("--verbose", "-v", help="Log every HTTP request (provider, path, params, status, time)")]
Format = Annotated[
    TablesFormat | None,
    typer.Option("--format", "-f", help="Also write flat tables for manual analysis: csv (data/exports/csv/*.csv), xlsx (data/exports/rwa.xlsx) or all"),
]


@app.command()
def sync(
    symbols: Annotated[str | None, typer.Option(help="Comma-separated tickers from config/assets.json")] = None,
    refresh: Annotated[bool, typer.Option(help="Re-fetch selected underlyings even if a snapshot exists")] = False,
    data_dir: DataDir = DEFAULT_DATA_DIR,
    frontend_dir: FrontendDir = DEFAULT_FRONTEND_DIR,
    format: Format = None,
    verbose: Verbose = False,
) -> None:
    """Fetch new underlyings (default) or refresh selected ones, then rebuild the export."""
    try:
        assets, settings = load_assets(), load_settings()
        code, report = run_sync(assets, settings, data_dir, symbols.split(",") if symbols else None, refresh, log=typer.echo, verbose=verbose)
    except ConfigError as e:
        typer.echo(f"config error: {e}", err=True)
        raise typer.Exit(1)

    if report is None:
        typer.echo("nothing_to_add: every selected underlying already has a snapshot (use --refresh to update)")
        raise typer.Exit(0)

    typer.echo(f"run {report['run_id']} ({report['mode']}) report: {data_dir / 'runs' / (report['run_id'] + '.json')}")
    for item in report["completed"]:
        typer.echo(f"  ok      {item['ticker']:<6} {item['status']}")
    for ticker, error in report["failed"].items():
        typer.echo(f"  failed  {ticker:<6} {error}")
    for warning in report["warnings"]:
        typer.echo(f"  warning {warning}")
    if "fatal" in report:
        typer.echo(f"  fatal   {report['fatal']}")
    typer.echo(f"  requests cmc={report['requests']['cmc']} portals={report['requests']['portals']} cmc_credits={report['cmc_credits']}")

    if report["completed"]:
        _logos(data_dir)
        _build(assets, settings, data_dir, frontend_dir, format)
    else:
        typer.echo("nothing saved, export not rebuilt")
    raise typer.Exit(code)


@app.command()
def logos(data_dir: DataDir = DEFAULT_DATA_DIR, frontend_dir: FrontendDir = DEFAULT_FRONTEND_DIR) -> None:
    """Download missing token/issuer/underlying logos into data/logos/, then rebuild the export."""
    try:
        assets, settings = load_assets(), load_settings()
    except ConfigError as e:
        typer.echo(f"config error: {e}", err=True)
        raise typer.Exit(1)
    if warning := fill_missing_rwa_info(assets, settings, data_dir):
        typer.echo(f"  warning {warning}")
    _logos(data_dir)
    _build(assets, settings, data_dir, frontend_dir)


def _logos(data_dir: Path) -> None:
    report = download_logos(data_dir, log=typer.echo)
    typer.echo(f"logos: {len(report['downloaded'])} downloaded, {len(report['failed'])} failed, {len(report['missing_source'])} without source")
    for item in report["failed"] + report["missing_source"]:
        typer.echo(f"  no logo: {item} (put a file at data/logos/{item}.png by hand)")


@app.command()
def build(data_dir: DataDir = DEFAULT_DATA_DIR, frontend_dir: FrontendDir = DEFAULT_FRONTEND_DIR, format: Format = None) -> None:
    """Recompute analytics from saved snapshots and write the frontend JSON. No network."""
    try:
        _build(load_assets(), load_settings(), data_dir, frontend_dir, format)
    except ConfigError as e:
        typer.echo(f"config error: {e}", err=True)
        raise typer.Exit(1)


@app.command()
def export(
    format: Annotated[TablesFormat, typer.Option("--format", "-f", help="csv (one file per table), xlsx (one workbook) or all")] = TablesFormat.xlsx,
    out: Annotated[Path | None, typer.Option(help="Target: the .xlsx file for xlsx, the directory for csv (default under data/exports/)")] = None,
    data_dir: DataDir = DEFAULT_DATA_DIR,
) -> None:
    """Write the current data as flat CSV/XLSX tables for manual analysis. No network, the frontend JSON is not touched."""
    try:
        export = build_export(load_assets(), load_settings(), data_dir)
    except ConfigError as e:
        typer.echo(f"config error: {e}", err=True)
        raise typer.Exit(1)
    _tables(export, format, out, data_dir)


def _tables(export: dict, format: TablesFormat, out: Path | None, data_dir: Path) -> None:
    paths = write_tables(export, format.value, out, data_dir)
    typer.echo(f"tables ({format.value}): {len(export['tokens'])} tokens, {len(export['opportunities'])} opportunities")
    for path in paths:
        typer.echo(f"  wrote {path}")


def _build(assets, settings, data_dir: Path, frontend_dir: Path, format: TablesFormat | None = None) -> None:
    export = build_export(assets, settings, data_dir)
    if not frontend_dir.exists():
        typer.echo(f"  frontend dir {frontend_dir} not found, writing only to {data_dir / 'exports'}")
    paths = write_export(export, data_dir, frontend_dir if frontend_dir.exists() else None)
    s = export["summary"]
    typer.echo(
        f"build: {s['underlyings_count']} underlyings, {s['tokens_count']} tokens, {s['issuers_count']} issuers, "
        f"{s['raw_confirmed_opportunities_count']} confirmed / {s['eligible_opportunities_count']} eligible opportunities"
    )
    if frontend_dir.exists():
        typer.echo("  the frontend reads this JSON directly: `pnpm start` picks it up on reload, `pnpm build` bakes it into dist/")
    for path in paths:
        typer.echo(f"  wrote {path}")
    if format is not None:
        _tables(export, format, None, data_dir)


if __name__ == "__main__":
    app()
