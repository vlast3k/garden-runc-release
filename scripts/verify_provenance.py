#!/usr/bin/env python3

import argparse
import hashlib
import json
import os
from pathlib import Path
import re
import subprocess
import sys
from typing import Any


SHA1 = re.compile(r"^[0-9a-f]{40}$")
SHA256 = re.compile(r"^[0-9a-f]{64}$")
IDENTIFIER = re.compile(r"^[a-z0-9][a-z0-9._-]*$")
HTTPS_REPOSITORY = re.compile(r"^https://[A-Za-z0-9.-]+(?:/[A-Za-z0-9._-]+)+\.git$")
GO_ARCHIVE = re.compile(r"^go[0-9]+\.[0-9]+\.[0-9]+\.linux-amd64\.tar\.gz$")
GO_PACKAGE = re.compile(r"^golang-[0-9]+\.[0-9]+-linux$")
RELEASE_REPOSITORY = "https://github.tools.sap/cp-runtime/garden-runc-release.git"
GUARDIAN_REPOSITORY = "https://github.tools.sap/cp-runtime/guardian.git"
GUARDIAN_BRANCH = "main"


class MetadataError(ValueError):
    pass


def reject_duplicate_keys(pairs: list[tuple[str, Any]]) -> dict[str, Any]:
    result = {}
    for key, value in pairs:
        if key in result:
            raise MetadataError(f"duplicate JSON key: {key}")
        result[key] = value
    return result


def require_object(value: Any, location: str, keys: set[str]) -> dict[str, Any]:
    if type(value) is not dict:
        raise MetadataError(f"{location} must be an object")
    missing = keys - value.keys()
    extra = value.keys() - keys
    if missing:
        raise MetadataError(f"{location} is missing: {', '.join(sorted(missing))}")
    if extra:
        raise MetadataError(f"{location} has unsupported fields: {', '.join(sorted(extra))}")
    return value


def require_string(value: Any, location: str, pattern: re.Pattern[str] | None = None) -> str:
    if type(value) is not str or not value:
        raise MetadataError(f"{location} must be a non-empty string")
    if pattern and not pattern.fullmatch(value):
        raise MetadataError(f"{location} has invalid format")
    return value


def require_path(value: Any, location: str) -> str:
    path = require_string(value, location)
    components = path.split("/")
    if path == "." or "\\" in path or any(component in {"", ".", ".."} for component in components):
        raise MetadataError(f"{location} must be an unambiguous repository-relative path")
    candidate = Path(path)
    if candidate.is_absolute():
        raise MetadataError(f"{location} must be an unambiguous repository-relative path")
    return path


def validate_anchor(value: Any, location: str) -> None:
    anchor = require_object(value, location, {"kind", "path", "selector"})
    require_string(anchor["kind"], f"{location}.kind", IDENTIFIER)
    require_path(anchor["path"], f"{location}.path")
    require_string(anchor["selector"], f"{location}.selector")


def validate_metadata(value: Any) -> dict[str, Any]:
    root = require_object(
        value,
        "metadata",
        {"schema_version", "release", "toolchain", "guardian", "downstream_patches", "behavior_gates"},
    )
    if type(root["schema_version"]) is not int or root["schema_version"] != 1:
        raise MetadataError("schema_version must be the supported integer 1")

    release = require_object(root["release"], "release", {"name", "repository", "final_config"})
    require_string(release["name"], "release.name", IDENTIFIER)
    require_string(release["repository"], "release.repository", HTTPS_REPOSITORY)
    if release["repository"] != RELEASE_REPOSITORY:
        raise MetadataError(f"release.repository must be {RELEASE_REPOSITORY}")
    require_path(release["final_config"], "release.final_config")

    toolchain = require_object(
        root["toolchain"],
        "toolchain",
        {"package", "spec", "version_file", "packaging", "archive", "blob_manifest", "blob_root", "size", "sha256"},
    )
    require_string(toolchain["package"], "toolchain.package", GO_PACKAGE)
    require_path(toolchain["spec"], "toolchain.spec")
    require_path(toolchain["version_file"], "toolchain.version_file")
    require_path(toolchain["packaging"], "toolchain.packaging")
    require_string(toolchain["archive"], "toolchain.archive", GO_ARCHIVE)
    require_path(toolchain["blob_manifest"], "toolchain.blob_manifest")
    require_path(toolchain["blob_root"], "toolchain.blob_root")
    if toolchain["blob_root"] != "blobs":
        raise MetadataError("toolchain.blob_root must be the approved blobs directory")
    if type(toolchain["size"]) is not int or toolchain["size"] <= 0:
        raise MetadataError("toolchain.size must be a positive integer")
    require_string(toolchain["sha256"], "toolchain.sha256", SHA256)

    guardian = require_object(root["guardian"], "guardian", {"path", "revision", "repository", "branch"})
    require_path(guardian["path"], "guardian.path")
    require_string(guardian["revision"], "guardian.revision", SHA1)
    require_string(guardian["repository"], "guardian.repository", HTTPS_REPOSITORY)
    require_string(guardian["branch"], "guardian.branch", IDENTIFIER)
    if guardian["repository"] != GUARDIAN_REPOSITORY:
        raise MetadataError(f"guardian.repository must be {GUARDIAN_REPOSITORY}")
    if guardian["branch"] != GUARDIAN_BRANCH:
        raise MetadataError(f"guardian.branch must be {GUARDIAN_BRANCH}")

    patches = root["downstream_patches"]
    if type(patches) is not list:
        raise MetadataError("downstream_patches must be an array")
    patch_ids = set()
    for index, value in enumerate(patches):
        location = f"downstream_patches[{index}]"
        patch = require_object(
            value,
            location,
            {"id", "owner", "revision", "status", "application_check", "test_anchors"},
        )
        patch_id = require_string(patch["id"], f"{location}.id", IDENTIFIER)
        if patch_id in patch_ids:
            raise MetadataError(f"duplicate downstream patch id: {patch_id}")
        patch_ids.add(patch_id)
        owner = require_string(patch["owner"], f"{location}.owner", IDENTIFIER)
        if owner != "guardian":
            raise MetadataError(f"{location}.owner must be guardian")
        require_string(patch["revision"], f"{location}.revision", SHA1)
        patch_status = require_string(patch["status"], f"{location}.status", IDENTIFIER)
        if patch_status not in {"required", "superseded"}:
            raise MetadataError(f"{location}.status must be required or superseded")
        require_string(patch["application_check"], f"{location}.application_check")
        if type(patch["test_anchors"]) is not list or not patch["test_anchors"]:
            raise MetadataError(f"{location}.test_anchors must be a non-empty array")
        for anchor_index, anchor in enumerate(patch["test_anchors"]):
            validate_anchor(anchor, f"{location}.test_anchors[{anchor_index}]")
    if "guardian-created-task-cleanup" not in patch_ids:
        raise MetadataError("downstream_patches must retain guardian-created-task-cleanup")
    cleanup_patch = next(patch for patch in patches if patch["id"] == "guardian-created-task-cleanup")
    if cleanup_patch["status"] != "required":
        raise MetadataError("guardian-created-task-cleanup downstream patch must remain required")

    gates = root["behavior_gates"]
    if type(gates) is not list or not gates:
        raise MetadataError("behavior_gates must be a non-empty array")
    gate_ids = set()
    for index, value in enumerate(gates):
        location = f"behavior_gates[{index}]"
        gate = require_object(value, location, {"id", "owner", "status", "test_anchors", "limitation"})
        gate_id = require_string(gate["id"], f"{location}.id", IDENTIFIER)
        if gate_id in gate_ids:
            raise MetadataError(f"duplicate behavior gate id: {gate_id}")
        gate_ids.add(gate_id)
        owner = require_string(gate["owner"], f"{location}.owner", IDENTIFIER)
        if owner != "guardian":
            raise MetadataError(f"{location}.owner must be guardian")
        gate_status = require_string(gate["status"], f"{location}.status", IDENTIFIER)
        if gate_status not in {"required", "blocked"}:
            raise MetadataError(f"{location}.status must be required or blocked; metadata cannot assert passed")
        require_string(gate["limitation"], f"{location}.limitation")
        if type(gate["test_anchors"]) is not list or not gate["test_anchors"]:
            raise MetadataError(f"{location}.test_anchors must be a non-empty array")
        for anchor_index, anchor in enumerate(gate["test_anchors"]):
            validate_anchor(anchor, f"{location}.test_anchors[{anchor_index}]")
    if "guardian-created-task-cleanup" not in gate_ids:
        raise MetadataError("behavior_gates must retain guardian-created-task-cleanup")
    cleanup_gate = next(gate for gate in gates if gate["id"] == "guardian-created-task-cleanup")
    if cleanup_gate["status"] != "required":
        raise MetadataError("guardian-created-task-cleanup must remain required")
    return root


def run_git(root: Path, *args: str, ceiling: Path | None = None) -> subprocess.CompletedProcess[str]:
    env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
    env["GIT_TERMINAL_PROMPT"] = "0"
    env["GIT_OPTIONAL_LOCKS"] = "0"
    env["GIT_NO_REPLACE_OBJECTS"] = "1"
    if ceiling is not None:
        env["GIT_CEILING_DIRECTORIES"] = str(ceiling)
    return subprocess.run(
        ["git", "-C", str(root), *args],
        check=False,
        capture_output=True,
        text=True,
        env=env,
    )


def git_value(root: Path, failures: list[str], description: str, *args: str, ceiling: Path | None = None) -> str | None:
    result = run_git(root, *args, ceiling=ceiling)
    if result.returncode != 0:
        detail = result.stderr.strip() or result.stdout.strip() or f"exit {result.returncode}"
        failures.append(f"cannot determine {description}: {detail}")
        return None
    return result.stdout.strip()


def safe_repository_file(root: Path, relative: str) -> Path:
    root = root.resolve()
    current = root
    for component in relative.split("/"):
        current = current / component
        if current.is_symlink():
            raise OSError(f"symlink is not allowed: {relative}")
    resolved = current.resolve(strict=True)
    if not resolved.is_relative_to(root):
        raise OSError(f"path escapes repository root: {relative}")
    if not resolved.is_file():
        raise OSError(f"not a regular file: {relative}")
    return resolved


def read_text(root: Path, relative: str, failures: list[str], description: str) -> str | None:
    try:
        return safe_repository_file(root, relative).read_bytes().decode("utf-8")
    except (OSError, UnicodeError) as error:
        failures.append(f"cannot read {description} at {relative}: {error}")
        return None


def read_yaml(root: Path, relative: str, failures: list[str], description: str) -> dict[str, Any] | None:
    source = read_text(root, relative, failures, description)
    if source is None:
        return None
    try:
        result = subprocess.run(
            ["ruby", str(Path(__file__).with_name("yaml_to_json.rb"))],
            input=source, capture_output=True, text=True, check=False,
        )
        if result.returncode != 0:
            raise ValueError(result.stderr.strip() or f"Ruby exited {result.returncode}")
        return json.loads(result.stdout, object_pairs_hook=reject_duplicate_keys)
    except (OSError, ValueError) as error:
        failures.append(f"cannot parse {description} at {relative}: {error}")
        return None


def packaging_commands(archive: str) -> list[str]:
    return [
        "set -e -x -u",
        f"tar xzf {archive}",
        "cp -R go/* ${BOSH_INSTALL_TARGET}",
        "mkdir ${BOSH_INSTALL_TARGET}/bosh",
        'export PACKAGE_NAME="$(basename ${BOSH_INSTALL_TARGET})"',
        r'sed "s#\${PACKAGE_NAME}#${PACKAGE_NAME}#" compile.env.unix > "${BOSH_INSTALL_TARGET}/bosh/compile.env"',
        r'sed "s#\${PACKAGE_NAME}#${PACKAGE_NAME}#" runtime.env.unix > "${BOSH_INSTALL_TARGET}/bosh/runtime.env"',
    ]


def verify(root: Path, metadata: dict[str, Any]) -> list[str]:
    failures: list[str] = []
    release = metadata["release"]
    toolchain = metadata["toolchain"]
    guardian_metadata = metadata["guardian"]

    for source, label in ((root, "release"), (root / guardian_metadata["path"], "Guardian")):
        flags = git_value(source, failures, f"{label} index flags", "ls-files", "-v", "-z", ceiling=source.parent)
        if flags and any(entry[0].islower() or entry[0] == "S" for entry in flags.split("\0") if entry):
            failures.append(f"{label} source has hidden index flags; clear assume-unchanged and skip-worktree")

    release_root = git_value(root, failures, "release worktree root", "rev-parse", "--show-toplevel")
    if release_root is not None and (not release_root or Path(release_root).resolve() != root):
        failures.append(f"release worktree root is {release_root}, expected {root}")
    release_git_dir = git_value(root, failures, "release Git directory", "rev-parse", "--absolute-git-dir")
    release_remote = git_value(root, failures, "release origin", "config", "--get", "remote.origin.url")
    if release_remote != release["repository"]:
        failures.append(f"release origin is {release_remote}, expected {release['repository']}")
    release_status = git_value(root, failures, "release source status", "status", "--porcelain", "--untracked-files=all")
    if release_status:
        failures.append("release source is dirty; commit or remove all tracked and untracked changes")

    final_config = read_yaml(root, release["final_config"], failures, "release final config")
    if final_config is not None:
        actual = final_config.get("final_name")
        if actual != release["name"]:
            failures.append(f"release name is {actual}, expected {release['name']}")

    spec = read_yaml(root, toolchain["spec"], failures, "Go toolchain package spec")
    if spec is not None:
        actual = spec.get("name")
        if actual != toolchain["package"]:
            failures.append(f"Go toolchain package is {actual}, expected {toolchain['package']}")
        files = spec.get("files")
        valid_files = type(files) is list and all(type(entry) is str for entry in files)
        archives = [entry for entry in files if GO_ARCHIVE.fullmatch(entry)] if valid_files else []
        if not valid_files or archives != [toolchain["archive"]] or len(files) != len(set(files)):
            failures.append(f"Go toolchain package archive is {archives!r}, expected [{toolchain['archive']!r}]")

    expected_version = re.fullmatch(r"go([0-9]+\.[0-9]+\.[0-9]+)\.linux-amd64\.tar\.gz", toolchain["archive"])
    version = read_text(root, toolchain["version_file"], failures, "Go toolchain version")
    if version is not None and expected_version and version.strip() != expected_version.group(1):
        failures.append(f"Go toolchain version is {version.strip()}, expected {expected_version.group(1)}")
    packaging = read_text(root, toolchain["packaging"], failures, "Go toolchain packaging script")
    if packaging is not None:
        if any(character not in "\t\n" and not " " <= character <= "~" for character in packaging):
            failures.append("Go toolchain packaging script contains an unsupported packaging character; only printable ASCII, TAB and LF are allowed")
        else:
            lines = [line.strip(" \t") for line in packaging.split("\n")]
            commands = [line for line in lines if line and not line.startswith("#")]
            if commands != packaging_commands(toolchain["archive"]):
                failures.append(
                    f"Go toolchain packaging archive is not consumed by the supported straight-line command sequence: {toolchain['archive']}"
                )

    blobs = read_yaml(root, toolchain["blob_manifest"], failures, "blob manifest")
    if blobs is not None:
        block = blobs.get(toolchain["archive"])
        if (
            type(block) is not dict
            or type(block.get("size")) is not int
            or type(block.get("sha")) is not str
            or not re.fullmatch(r"sha256:[0-9a-f]{64}", block["sha"])
        ):
            failures.append(f"Go toolchain archive {toolchain['archive']} is missing or malformed in {toolchain['blob_manifest']}")
        else:
            actual_size = block["size"]
            actual_sha = block["sha"].removeprefix("sha256:")
            if actual_size != toolchain["size"]:
                failures.append(f"Go toolchain size is {actual_size}, expected {toolchain['size']}")
            if actual_sha != toolchain["sha256"]:
                failures.append(f"Go toolchain SHA-256 is {actual_sha}, expected {toolchain['sha256']}")

    hydrated_relative = f"{toolchain['blob_root']}/{toolchain['archive']}"
    try:
        hydrated = safe_repository_file(root, hydrated_relative)
    except OSError as error:
        failures.append(f"hydrated Go toolchain archive is unavailable or unsafe at {hydrated_relative}: {error}")
    else:
        actual_size = hydrated.stat().st_size
        digest = hashlib.sha256()
        with hydrated.open("rb") as archive_file:
            for chunk in iter(lambda: archive_file.read(1024 * 1024), b""):
                digest.update(chunk)
        actual_sha = digest.hexdigest()
        if actual_size != toolchain["size"]:
            failures.append(f"hydrated Go toolchain archive size is {actual_size}, expected {toolchain['size']}")
        if actual_sha != toolchain["sha256"]:
            failures.append(f"hydrated Go toolchain archive SHA-256 is {actual_sha}, expected {toolchain['sha256']}")

    path = guardian_metadata["path"]
    modules = read_text(root, ".gitmodules", failures, "submodule configuration")
    if modules is not None:
        recorded_remote = git_value(
            root,
            failures,
            "recorded Guardian submodule URL",
            "config",
            "-f",
            ".gitmodules",
            "--get",
            f"submodule.{path}.url",
        )
        if recorded_remote != guardian_metadata["repository"]:
            failures.append(f"recorded Guardian submodule URL is {recorded_remote}, expected {guardian_metadata['repository']}")
        recorded_branch = git_value(
            root,
            failures,
            "recorded Guardian submodule branch",
            "config",
            "-f",
            ".gitmodules",
            "--get",
            f"submodule.{path}.branch",
        )
        if recorded_branch != guardian_metadata["branch"]:
            failures.append(f"recorded Guardian submodule branch is {recorded_branch}, expected {guardian_metadata['branch']}")
    gitlink = git_value(root, failures, "Guardian gitlink", "ls-tree", "HEAD", "--", path)
    if gitlink is not None:
        fields = gitlink.split()
        if not fields:
            failures.append(f"Guardian gitlink is missing at {path}")
        elif len(fields) < 3 or fields[0] != "160000" or fields[1] != "commit":
            failures.append(f"Guardian entry at {path} is not a gitlink")
        elif fields[2] != guardian_metadata["revision"]:
            failures.append(f"Guardian gitlink is {fields[2]}, expected {guardian_metadata['revision']}")

    guardian = root / path
    if not guardian.is_dir():
        failures.append(f"Guardian submodule is not initialized at {path}; run git submodule update --init {path}")
        return failures
    if guardian.is_symlink():
        failures.append(f"Guardian worktree path {path} is a symlink; expected an initialized worktree at that exact root")
        return failures
    guardian_root_result = run_git(guardian, "rev-parse", "--show-toplevel", ceiling=guardian.parent)
    if guardian_root_result.returncode != 0:
        failures.append(f"Guardian submodule is not initialized as a separate Git worktree at {path}; run git submodule update --init {path}")
        return failures
    guardian_root = Path(guardian_root_result.stdout.strip()).resolve()
    if guardian_root != guardian:
        failures.append(f"Guardian worktree root is {guardian_root}, expected {guardian}; refusing parent or unrelated repository")
        return failures
    guardian_git_dir = git_value(
        guardian,
        failures,
        "Guardian Git directory",
        "rev-parse",
        "--absolute-git-dir",
        ceiling=guardian.parent,
    )
    if guardian_git_dir and release_git_dir and Path(guardian_git_dir).resolve() == Path(release_git_dir).resolve():
        failures.append("Guardian uses the release Git directory; refusing parent repository identity")
        return failures

    revision = guardian_metadata["revision"]
    exists = run_git(guardian, "cat-file", "-e", f"{revision}^{{commit}}", ceiling=guardian.parent)
    if exists.returncode != 0:
        failures.append(f"declared Guardian revision {revision} does not exist in the initialized repository")
    guardian_head = git_value(guardian, failures, "Guardian HEAD", "rev-parse", "HEAD", ceiling=guardian.parent)
    if guardian_head != revision:
        failures.append(f"Guardian HEAD is {guardian_head}, expected {revision}")
    guardian_remote = git_value(
        guardian,
        failures,
        "Guardian origin",
        "config",
        "--get",
        "remote.origin.url",
        ceiling=guardian.parent,
    )
    if guardian_remote != guardian_metadata["repository"]:
        failures.append(f"Guardian origin is {guardian_remote}, expected {guardian_metadata['repository']}")

    guardian_status = git_value(
        guardian,
        failures,
        "Guardian source status",
        "status",
        "--porcelain",
        "--untracked-files=all",
        ceiling=guardian.parent,
    )
    if guardian_status:
        failures.append("Guardian source is dirty; restore the exact committed source before verification")
    for record_type in ("downstream_patches", "behavior_gates"):
        for record in metadata[record_type]:
            if record_type == "downstream_patches":
                patch_exists = run_git(
                    guardian,
                    "cat-file",
                    "-e",
                    f"{record['revision']}^{{commit}}",
                    ceiling=guardian.parent,
                )
                if patch_exists.returncode != 0:
                    failures.append(
                        f"declared downstream patch revision {record['revision']} does not exist for {record['id']}"
                    )
            for anchor in record["test_anchors"]:
                try:
                    anchor_path = safe_repository_file(guardian, anchor["path"])
                    anchor_source = anchor_path.read_text(encoding="utf-8")
                except (OSError, UnicodeError) as error:
                    failures.append(f"cannot read Guardian test anchor for {record['id']} at {anchor['path']}: {error}")
                    continue
                if anchor["selector"] not in anchor_source:
                    failures.append(
                        f"Guardian test anchor selector is missing for {record['id']}: "
                        f"{anchor['selector']} in {anchor['path']}"
                    )
    return failures


def main() -> int:
    parser = argparse.ArgumentParser(description="Verify the Garden release-local contract consistency")
    parser.add_argument("--root", type=Path, default=Path(__file__).resolve().parent.parent)
    args = parser.parse_args()
    root = args.root.resolve()
    try:
        raw = safe_repository_file(root, "PROVENANCE.json").read_text(encoding="utf-8")
        metadata = validate_metadata(json.loads(raw, object_pairs_hook=reject_duplicate_keys))
    except (OSError, UnicodeError, json.JSONDecodeError, MetadataError) as error:
        print(f"provenance metadata invalid: {error}", file=sys.stderr)
        return 2

    failures = verify(root, metadata)
    if failures:
        print("provenance verification failed:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1
    print("Verified release-local contract consistency for the Go toolchain, Guardian source, and behavior gates")
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
