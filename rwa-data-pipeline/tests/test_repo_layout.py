"""The pipeline and the site live in one repository; these checks keep the wiring between them honest."""

from rwa_pipeline.config import DEFAULT_FRONTEND_DIR, PROJECT_ROOT
from rwa_pipeline.export import EXPORT_RELATIVE_PATH


def test_default_frontend_dir_is_the_site_folder_of_this_repository():
    # `build` writes public/data/rwa.json under this folder; a stale name makes it skip the site without failing.
    assert DEFAULT_FRONTEND_DIR.parent == PROJECT_ROOT.parent
    assert (DEFAULT_FRONTEND_DIR / "package.json").is_file()
    assert (DEFAULT_FRONTEND_DIR / EXPORT_RELATIVE_PATH).parent.is_dir()
