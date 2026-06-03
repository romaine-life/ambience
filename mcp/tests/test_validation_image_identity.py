from __future__ import annotations

import sys
from pathlib import Path
from typing import Any

import pytest

REPO_ROOT = Path(__file__).resolve().parents[1]
sys.path.insert(0, str(REPO_ROOT))

from ambience_preview import ops  # noqa: E402


def test_build_preview_image_rejects_mutable_slot_tag() -> None:
    with pytest.raises(ValueError, match="mutable slot-scoped validation image tags are retired"):
        ops.build_preview_image(image_tag="ambience-slot-2")


def test_build_preview_image_requires_tag_matching_source_revision(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    monkeypatch.setattr(ops, "run_command", lambda *_args, **_kwargs: pytest.fail("should not build"))
    with pytest.raises(ValueError, match="image_tag must be git-"):
        ops.build_preview_image(
            image_tag=f"git-{'a' * 40}",
            source_revision="b" * 40,
        )


def test_build_preview_image_reports_source_revision_when_existing_tag_is_reused(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    revision = "a" * 40
    image_tag = f"git-{revision}"

    monkeypatch.setattr(ops, "acr_repository_tag", lambda **_kwargs: image_tag)
    monkeypatch.setattr(ops, "run_command", lambda *_args, **_kwargs: pytest.fail("should not build"))

    result = ops.build_preview_image(image_tag=image_tag, source_revision=revision)

    assert result["image"] == f"romainecr.azurecr.io/ambience:{image_tag}"
    assert result["image_tag"] == image_tag
    assert result["source_revision"] == revision
    assert result["skipped_build"] is True


def test_rebuild_validation_image_rejects_mutable_rebuild_tag() -> None:
    with pytest.raises(ValueError, match="mutable slot-scoped validation image tags are retired"):
        ops.rebuild_validation_image(
            namespace="ambience-slot-2",
            branch="issue-170-run-4.1",
            image_tag="ambience-slot-2-r2",
            source_revision="a" * 40,
        )


def test_rebuild_validation_image_builds_exact_source_revision(
    monkeypatch: pytest.MonkeyPatch,
) -> None:
    revision = "b" * 40
    image_tag = f"git-{revision}"
    commands: list[list[str]] = []

    monkeypatch.setattr(ops, "acr_repository_tag", lambda **_kwargs: "")

    def record_command(command: list[str], *args: Any, **kwargs: Any) -> str:
        commands.append(command)
        return ""

    monkeypatch.setattr(ops, "run_command", record_command)

    result = ops.rebuild_validation_image(
        namespace="ambience-slot-2",
        branch="issue-170-run-4.1",
        image_tag=image_tag,
        source_revision=revision,
        repo_slug="romaine-life/ambience",
    )

    assert result["source_revision"] == revision
    assert result["image_tag"] == image_tag
    assert commands[0][-1] == f"https://github.com/romaine-life/ambience.git#{revision}"
    assert not any("issue-170-run-4.1" in part for command in commands for part in command)
