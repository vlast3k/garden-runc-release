#!/usr/bin/env python3

import hashlib
import json
import os
from pathlib import Path
import shutil
import shlex
import subprocess
import sys
import tempfile
import unittest


VERIFIER = Path(__file__).resolve().parents[1] / "verify_provenance.py"
RELEASE_REMOTE = "https://github.tools.sap/cp-runtime/garden-runc-release.git"
GUARDIAN_REMOTE = "https://github.tools.sap/cp-runtime/guardian.git"
GUARDIAN_REVISION = "2f4941f8ed804d966c6962075b2a84d74a4f88c9"
CLEANUP_PATCH_REVISION = "186b414477c98ed9fab5707a9a8c79d8e55ae80b"
CLEANUP_SELECTOR = "deletes a created alternative-runtime task using the required shim cleanup"
FIXTURE_ARCHIVE = b"fixture-go-archive\n"
FIXTURE_ARCHIVE_SIZE = 19
FIXTURE_ARCHIVE_SHA = "cb9f4682c40eafd858310d12b3fd7d1ecec19f2387fb28e4c181f0fa9c3b98f4"
FIXTURE_PACKAGING = '''set -e -x -u
tar xzf go1.26.3.linux-amd64.tar.gz
cp -R go/* ${BOSH_INSTALL_TARGET}
mkdir ${BOSH_INSTALL_TARGET}/bosh
export PACKAGE_NAME="$(basename ${BOSH_INSTALL_TARGET})"
sed "s#\\${PACKAGE_NAME}#${PACKAGE_NAME}#" compile.env.unix > "${BOSH_INSTALL_TARGET}/bosh/compile.env"
sed "s#\\${PACKAGE_NAME}#${PACKAGE_NAME}#" runtime.env.unix > "${BOSH_INSTALL_TARGET}/bosh/runtime.env"
'''


def git(root: Path, *args: str) -> str:
    result = subprocess.run(["git", "-C", str(root), *args], check=True, capture_output=True, text=True)
    return result.stdout.strip()


def write(path: Path, value: str) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    path.write_text(value, encoding="utf-8")


class Fixture:
    def __init__(self, parent: Path):
        self.root = parent / "release"
        self.guardian = self.root / "src" / "guardian"
        self.guardian_seed = parent / "guardian-seed"
        self.guardian_seed.mkdir()
        git(self.guardian_seed, "init", "-b", "main")
        git(self.guardian_seed, "config", "user.email", "fixture@example.invalid")
        git(self.guardian_seed, "config", "user.name", "Fixture")
        write(
            self.guardian_seed / "rundmc/runcontainerd/nerd/nerd_linux_test.go",
            f'package nerd_test\n\nvar _ = It("{CLEANUP_SELECTOR}", func() {{}})\n',
        )
        git(self.guardian_seed, "add", ".")
        git(self.guardian_seed, "commit", "-m", "fixture Guardian")
        self.guardian_remote = parent / "guardian-origin.git"
        subprocess.run(
            ["git", "clone", "--bare", str(self.guardian_seed), str(self.guardian_remote)],
            check=True,
            capture_output=True,
        )
        self.guardian.parent.mkdir(parents=True)
        subprocess.run(
            ["git", "clone", str(self.guardian_remote), str(self.guardian)],
            check=True,
            capture_output=True,
        )
        git(self.guardian, "config", "user.email", "fixture@example.invalid")
        git(self.guardian, "config", "user.name", "Fixture")
        self.revision = git(self.guardian, "rev-parse", "HEAD")
        git(self.guardian, "remote", "set-url", "origin", GUARDIAN_REMOTE)
        self.verifier_env = os.environ.copy()
        self.git_wrapper_dir = parent / "git-wrapper"
        self.git_wrapper_dir.mkdir()
        self.real_git = shutil.which("git")
        if self.real_git is None:
            raise RuntimeError("git is unavailable")
        self.verifier_env["PATH"] = str(self.git_wrapper_dir) + os.pathsep + self.verifier_env["PATH"]
        self.set_live_remote(self.guardian_remote)

        git(self.root, "init", "-b", "main")
        git(self.root, "config", "user.email", "fixture@example.invalid")
        git(self.root, "config", "user.name", "Fixture")
        git(self.root, "remote", "add", "origin", RELEASE_REMOTE)
        write(self.root / ".gitignore", "blobs/\n")
        write(self.root / "config/final.yml", "---\nfinal_name: garden-runc\n")
        write(
            self.root / "config/blobs.yml",
            "go1.26.3.linux-amd64.tar.gz:\n"
            f"  size: {FIXTURE_ARCHIVE_SIZE}\n"
            f"  sha: sha256:{FIXTURE_ARCHIVE_SHA}\n",
        )
        write(
            self.root / "packages/golang-1.26-linux/spec",
            "---\nname: golang-1.26-linux\nfiles:\n- go1.26.3.linux-amd64.tar.gz\n",
        )
        write(self.root / "packages/golang-1.26-linux/version", "1.26.3\n")
        write(self.root / "packages/golang-1.26-linux/packaging", FIXTURE_PACKAGING)
        self.archive = self.root / "blobs/go1.26.3.linux-amd64.tar.gz"
        self.archive.parent.mkdir(parents=True)
        self.archive.write_bytes(FIXTURE_ARCHIVE)
        write(
            self.root / ".gitmodules",
            '[submodule "src/guardian"]\n'
            "\tpath = src/guardian\n"
            f"\turl = {GUARDIAN_REMOTE}\n"
            "\tbranch = main\n",
        )
        self.metadata = {
            "schema_version": 1,
            "release": {
                "name": "garden-runc",
                "repository": RELEASE_REMOTE,
                "final_config": "config/final.yml",
            },
            "toolchain": {
                "package": "golang-1.26-linux",
                "spec": "packages/golang-1.26-linux/spec",
                "version_file": "packages/golang-1.26-linux/version",
                "packaging": "packages/golang-1.26-linux/packaging",
                "archive": "go1.26.3.linux-amd64.tar.gz",
                "blob_manifest": "config/blobs.yml",
                "blob_root": "blobs",
                "size": FIXTURE_ARCHIVE_SIZE,
                "sha256": FIXTURE_ARCHIVE_SHA,
            },
            "guardian": {
                "path": "src/guardian",
                "revision": self.revision,
                "repository": GUARDIAN_REMOTE,
                "branch": "main",
            },
            "downstream_patches": [
                {
                    "id": "guardian-created-task-cleanup",
                    "owner": "guardian",
                    "revision": self.revision,
                    "status": "required",
                    "application_check": "Verify exact source, then run the anchored test.",
                    "test_anchors": [
                        {
                            "kind": "unit",
                            "path": "rundmc/runcontainerd/nerd/nerd_linux_test.go",
                            "selector": CLEANUP_SELECTOR,
                        }
                    ],
                }
            ],
            "behavior_gates": [
                {
                    "id": "guardian-created-task-cleanup",
                    "owner": "guardian",
                    "status": "required",
                    "test_anchors": [
                        {
                            "kind": "unit",
                            "path": "rundmc/runcontainerd/nerd/nerd_linux_test.go",
                            "selector": CLEANUP_SELECTOR,
                        }
                    ],
                    "limitation": "An anchor does not prove execution.",
                }
            ],
        }
        self.write_metadata()
        git(self.root, "add", ".gitignore", ".gitmodules", "PROVENANCE.json", "config", "packages")
        git(self.root, "update-index", "--add", "--cacheinfo", f"160000,{self.revision},src/guardian")
        git(self.root, "commit", "-m", "fixture release")

    def write_metadata(self) -> None:
        write(self.root / "PROVENANCE.json", json.dumps(self.metadata, indent=2) + "\n")

    def set_live_remote(self, remote: Path) -> None:
        wrapper = self.git_wrapper_dir / "git"
        write(
            wrapper,
            "#!/bin/sh\n"
            f"if [ \"$#\" -eq 4 ] && [ \"$1\" = ls-remote ] && [ \"$2\" = --exit-code ] && "
            f"[ \"$3\" = {shlex.quote(GUARDIAN_REMOTE)} ] && [ \"$4\" = refs/heads/main ]; then\n"
            '  [ "$PWD" = / ] && [ "$GIT_CONFIG_NOSYSTEM" = 1 ] && '
            '[ "$GIT_CONFIG_SYSTEM" = /dev/null ] && [ "$GIT_CONFIG_GLOBAL" = /dev/null ] && '
            '[ "$GIT_TERMINAL_PROMPT" = 0 ] && [ "$HOME" = /dev/null ] && '
            '[ "$XDG_CONFIG_HOME" = /dev/null ] && [ -z "${GIT_CONFIG_COUNT+x}" ] || exit 97\n'
            f"  exec {shlex.quote(self.real_git)} ls-remote --exit-code {shlex.quote(str(remote))} refs/heads/main\n"
            "fi\n"
            f"exec {shlex.quote(self.real_git)} \"$@\"\n",
        )
        wrapper.chmod(0o755)

    def commit_release(self, message: str = "fixture mutation") -> None:
        git(self.root, "add", "-A")
        git(self.root, "commit", "-m", message)

    def run(self) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(VERIFIER), "--root", str(self.root)],
            check=False,
            capture_output=True,
            text=True,
            env=self.verifier_env,
        )

    def source_snapshot(self) -> dict[str, str]:
        snapshot = {}
        for path in sorted(self.root.rglob("*")):
            if path.is_file():
                snapshot[str(path.relative_to(self.root))] = hashlib.sha256(path.read_bytes()).hexdigest()
        return snapshot


class VerifyProvenanceTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.fixture = Fixture(Path(self.temp.name))

    def tearDown(self) -> None:
        self.temp.cleanup()

    def assert_result(self, code: int, diagnostic: str) -> subprocess.CompletedProcess[str]:
        before = self.fixture.source_snapshot()
        result = self.fixture.run()
        self.assertEqual(result.returncode, code, result.stdout + result.stderr)
        self.assertIn(diagnostic, result.stdout + result.stderr)
        if code == 0:
            self.assertEqual(
                result.stdout,
                "Verified release-local contract consistency for the Go toolchain, Guardian source, and behavior gates\n",
            )
            self.assertEqual(result.stderr, "")
        self.assertEqual(self.fixture.source_snapshot(), before, "verifier mutated fixture source")
        return result

    def test_valid_control_does_not_mutate_fixture(self) -> None:
        before = self.fixture.source_snapshot()
        status_before = git(self.fixture.root, "status", "--porcelain", "--untracked-files=all")
        self.assert_result(0, "Verified release-local contract consistency")
        self.assertEqual(self.fixture.source_snapshot(), before)
        self.assertEqual(git(self.fixture.root, "status", "--porcelain", "--untracked-files=all"), status_before)

    def test_duplicate_quoted_final_name_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        write(self.fixture.root / "config/final.yml", 'final_name: garden-runc\n"final_name": wrong\n')
        self.fixture.commit_release()
        self.assert_result(1, "duplicate YAML key: final_name")

    def test_archive_in_unused_package_field_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        write(
            self.fixture.root / "packages/golang-1.26-linux/spec",
            "name: golang-1.26-linux\nfiles:\n- wrong.tar.gz\nunused:\n- go1.26.3.linux-amd64.tar.gz\n",
        )
        self.fixture.commit_release()
        self.assert_result(1, "Go toolchain package archive is []")

    def test_duplicate_blob_archive_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        blobs = self.fixture.root / "config/blobs.yml"
        write(blobs, blobs.read_text() + '"go1.26.3.linux-amd64.tar.gz":\n  size: 1\n  sha: sha256:' + "0" * 64 + "\n")
        self.fixture.commit_release()
        self.assert_result(1, "duplicate YAML key: go1.26.3.linux-amd64.tar.gz")

    def test_dead_extraction_is_rejected_and_bash_mock_proves_executed_archive(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        packaging = self.fixture.root / "packages/golang-1.26-linux/packaging"
        install = self.fixture.root.parent / "install"
        (install / "bosh").mkdir(parents=True)
        env = self.fixture.verifier_env.copy()
        env["BOSH_INSTALL_TARGET"] = str(install)
        mock = '''tar() { printf '%s\\n' "$@"; }
cp() { :; }
mkdir() { :; }
sed() { :; }
source "$1"
'''
        control = subprocess.run(
            ["bash", "-c", mock, "packaging-mock", str(packaging)],
            env=env, capture_output=True, text=True, check=False,
        )
        self.assertEqual(control.returncode, 0, control.stderr)
        self.assertEqual(control.stdout.splitlines(), ["xzf", "go1.26.3.linux-amd64.tar.gz"])
        write(
            packaging,
            "if false; then\ntar xzf go1.26.3.linux-amd64.tar.gz\nfi\ntar -xzf wrong.tar.gz\n",
        )
        self.fixture.commit_release()
        self.assert_result(1, "supported straight-line command sequence")
        invalid = subprocess.run(
            ["bash", "-c", mock, "packaging-mock", str(packaging)],
            env=env, capture_output=True, text=True, check=False,
        )
        self.assertEqual(invalid.returncode, 0, invalid.stderr)
        self.assertEqual(invalid.stdout.splitlines(), ["-xzf", "wrong.tar.gz"])

    def test_empty_release_origin_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        git(self.fixture.root, "config", "remote.origin.url", "")
        self.assert_result(1, "release origin is , expected")

    def test_empty_recorded_guardian_url_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        git(self.fixture.root, "config", "-f", ".gitmodules", "submodule.src/guardian.url", "")
        self.fixture.commit_release()
        self.assert_result(1, "recorded Guardian submodule URL is , expected")

    def test_empty_recorded_guardian_branch_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        git(self.fixture.root, "config", "-f", ".gitmodules", "submodule.src/guardian.branch", "")
        self.fixture.commit_release()
        self.assert_result(1, "recorded Guardian submodule branch is , expected")

    def test_empty_guardian_origin_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        git(self.fixture.guardian, "config", "remote.origin.url", "")
        self.assert_result(1, "Guardian origin is , expected")

    def test_quoted_yaml_values_are_parsed_like_ruby(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        write(self.fixture.root / "config/final.yml", '"final_name": "garden-runc"\n')
        write(
            self.fixture.root / "packages/golang-1.26-linux/spec",
            '"name": "golang-1.26-linux"\n"files": ["go1.26.3.linux-amd64.tar.gz"]\n',
        )
        write(
            self.fixture.root / "config/blobs.yml",
            f'"go1.26.3.linux-amd64.tar.gz": {{size: {FIXTURE_ARCHIVE_SIZE}, sha: "sha256:{FIXTURE_ARCHIVE_SHA}"}}\n',
        )
        self.fixture.commit_release()
        self.assert_result(0, "Verified release-local contract consistency")

    def test_multiple_yaml_documents_are_rejected_for_each_input(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        for relative in ("config/final.yml", "packages/golang-1.26-linux/spec", "config/blobs.yml"):
            with self.subTest(relative=relative):
                path = self.fixture.root / relative
                original = path.read_text()
                write(path, original + "---\nignored: wrong\n")
                self.fixture.commit_release()
                self.assert_result(1, "expected exactly one YAML document")
                write(path, original)
                self.fixture.commit_release("restore valid YAML")

    def test_duplicate_quoted_package_files_is_rejected(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        spec = self.fixture.root / "packages/golang-1.26-linux/spec"
        write(spec, spec.read_text() + '"files": [wrong.tar.gz]\n')
        self.fixture.commit_release()
        self.assert_result(1, "duplicate YAML key: files")

    def test_malformed_yaml_and_files_fail_without_traceback(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        spec = self.fixture.root / "packages/golang-1.26-linux/spec"
        for files in ("[", "null", "{}", "[go1.26.3.linux-amd64.tar.gz, {}]", "[go1.26.3.linux-amd64.tar.gz, false]"):
            with self.subTest(files=files):
                write(spec, f"name: golang-1.26-linux\nfiles: {files}\n")
                self.fixture.commit_release()
                result = self.assert_result(1, "provenance verification failed")
                self.assertNotIn("Traceback", result.stderr)

    def test_missing_git_identities_fail_closed_on_none(self) -> None:
        self.assert_result(0, "Verified release-local contract consistency")
        git(self.fixture.root, "config", "--unset", "remote.origin.url")
        git(self.fixture.guardian, "config", "--unset", "remote.origin.url")
        git(self.fixture.root, "config", "-f", ".gitmodules", "--unset", "submodule.src/guardian.url")
        git(self.fixture.root, "config", "-f", ".gitmodules", "--unset", "submodule.src/guardian.branch")
        self.fixture.commit_release()
        result = self.assert_result(1, "cannot determine release origin")
        for diagnostic in (
            "cannot determine Guardian origin",
            "cannot determine recorded Guardian submodule URL",
            "cannot determine recorded Guardian submodule branch",
        ):
            self.assertIn(diagnostic, result.stderr)

    def test_malformed_metadata(self) -> None:
        write(self.fixture.root / "PROVENANCE.json", "{\n")
        self.assert_result(2, "provenance metadata invalid")

    def test_duplicate_metadata_key(self) -> None:
        raw = json.dumps(self.fixture.metadata).replace('"schema_version": 1', '"schema_version": 1, "schema_version": 1')
        write(self.fixture.root / "PROVENANCE.json", raw)
        self.assert_result(2, "duplicate JSON key: schema_version")

    def test_unsupported_schema_and_field(self) -> None:
        self.fixture.metadata["schema_version"] = 2
        self.fixture.write_metadata()
        self.assert_result(2, "supported integer 1")
        self.fixture.metadata["schema_version"] = 1
        self.fixture.metadata["unknown"] = True
        self.fixture.write_metadata()
        self.assert_result(2, "unsupported fields: unknown")

    def test_invalid_metadata_type_and_format(self) -> None:
        self.fixture.metadata["toolchain"]["size"] = True
        self.fixture.write_metadata()
        self.assert_result(2, "positive integer")
        self.fixture.metadata["toolchain"]["size"] = 1
        self.fixture.metadata["guardian"]["revision"] = "not-a-revision"
        self.fixture.write_metadata()
        self.assert_result(2, "guardian.revision has invalid format")

    def test_wrong_and_missing_gitlink(self) -> None:
        git(self.fixture.root, "update-index", "--cacheinfo", f"160000,{'1' * 40},src/guardian")
        git(self.fixture.root, "commit", "-m", "wrong pin")
        self.assert_result(1, "Guardian gitlink is 1111111111111111111111111111111111111111")
        git(self.fixture.root, "rm", "--cached", "src/guardian")
        git(self.fixture.root, "commit", "-m", "missing pin")
        self.assert_result(1, "Guardian gitlink is missing")

    def test_uninitialized_submodule_does_not_walk_parent(self) -> None:
        shutil.rmtree(self.fixture.guardian / ".git")
        self.assert_result(1, "not initialized as a separate Git worktree")

    def test_wrong_guardian_worktree_root(self) -> None:
        shutil.rmtree(self.fixture.guardian / ".git")
        write(self.fixture.guardian / ".git", "gitdir: ../../.git\n")
        self.assert_result(1, "refusing parent repository identity")

    def test_symlinked_guardian_wrong_root(self) -> None:
        unrelated = self.fixture.root.parent / "unrelated-guardian"
        shutil.move(self.fixture.guardian, unrelated)
        self.fixture.guardian.symlink_to(unrelated, target_is_directory=True)
        self.assert_result(1, "is a symlink; expected an initialized worktree at that exact root")

    def test_wrong_guardian_remote(self) -> None:
        git(self.fixture.guardian, "remote", "set-url", "origin", "https://example.invalid/wrong.git")
        self.assert_result(1, "Guardian origin is https://example.invalid/wrong.git")

    def test_wrong_recorded_guardian_remote(self) -> None:
        modules = self.fixture.root / ".gitmodules"
        write(modules, modules.read_text().replace(GUARDIAN_REMOTE, "https://example.invalid/wrong.git"))
        self.fixture.commit_release()
        self.assert_result(1, "recorded Guardian submodule URL is https://example.invalid/wrong.git")

    def test_wrong_recorded_guardian_branch(self) -> None:
        modules = self.fixture.root / ".gitmodules"
        write(modules, modules.read_text().replace("branch = main", "branch = legacy"))
        self.fixture.commit_release()
        self.assert_result(1, "recorded Guardian submodule branch is legacy, expected main")

    def test_missing_anchor_selector(self) -> None:
        self.fixture.metadata["behavior_gates"][0]["test_anchors"][0]["selector"] = "TestDoesNotExist"
        self.fixture.write_metadata()
        self.fixture.commit_release()
        self.assert_result(1, "Guardian test anchor selector is missing")

    def test_old_generic_cleanup_selector_is_rejected(self) -> None:
        self.fixture.metadata["behavior_gates"][0]["test_anchors"][0]["selector"] = (
            "deletes the containerd task by container id"
        )
        self.fixture.write_metadata()
        self.fixture.commit_release()
        self.assert_result(1, "Guardian test anchor selector is missing")

    def test_anchor_symlink_escape(self) -> None:
        anchor = self.fixture.guardian / "rundmc/runcontainerd/nerd/nerd_linux_test.go"
        outside = self.fixture.root.parent / "outside-anchor"
        write(outside, anchor.read_text())
        anchor.unlink()
        anchor.symlink_to(outside)
        self.assert_result(1, "cannot read Guardian test anchor")

    def test_gate_and_patch_records_are_guardian_owned_and_not_self_passed(self) -> None:
        self.fixture.metadata["downstream_patches"][0]["owner"] = "release"
        self.fixture.write_metadata()
        self.assert_result(2, "owner must be guardian")
        self.fixture.metadata["downstream_patches"][0]["owner"] = "guardian"
        self.fixture.metadata["behavior_gates"][0]["status"] = "passed"
        self.fixture.write_metadata()
        self.assert_result(2, "metadata cannot assert passed")

    def test_cleanup_patch_cannot_be_superseded(self) -> None:
        self.fixture.metadata["downstream_patches"][0]["status"] = "superseded"
        self.fixture.write_metadata()
        self.assert_result(2, "cleanup downstream patch must remain required")

    def test_release_repository_identity_is_allowlisted(self) -> None:
        repository = "https://example.invalid/release.git"
        self.fixture.metadata["release"]["repository"] = repository
        self.fixture.write_metadata()
        git(self.fixture.root, "remote", "set-url", "origin", repository)
        self.assert_result(2, f"release.repository must be {RELEASE_REMOTE}")

    def test_guardian_repository_identity_is_allowlisted(self) -> None:
        repository = "https://example.invalid/guardian.git"
        self.fixture.metadata["guardian"]["repository"] = repository
        self.fixture.write_metadata()
        git(self.fixture.guardian, "remote", "set-url", "origin", repository)
        self.assert_result(2, f"guardian.repository must be {GUARDIAN_REMOTE}")

    def test_guardian_branch_is_allowlisted(self) -> None:
        self.fixture.metadata["guardian"]["branch"] = "forged"
        self.fixture.write_metadata()
        modules = self.fixture.root / ".gitmodules"
        write(modules, modules.read_text().replace("branch = main", "branch = forged"))
        self.assert_result(2, "guardian.branch must be main")

    def test_ambiguous_repository_paths_are_rejected(self) -> None:
        for path in (".", "../guardian", "src//guardian", "src/./guardian", "src\\guardian"):
            with self.subTest(path=path):
                self.fixture.metadata["guardian"]["path"] = path
                self.fixture.write_metadata()
                self.assert_result(2, "must be an unambiguous repository-relative path")

    def test_dirty_guardian_and_release_sources(self) -> None:
        write(self.fixture.guardian / "untracked", "dirty\n")
        self.assert_result(1, "Guardian source is dirty")
        (self.fixture.guardian / "untracked").unlink()
        write(self.fixture.root / "untracked", "dirty\n")
        self.assert_result(1, "release source is dirty")

    def test_nonexistent_declared_revision(self) -> None:
        revision = "f" * 40
        self.fixture.metadata["guardian"]["revision"] = revision
        self.fixture.write_metadata()
        git(self.fixture.root, "add", "PROVENANCE.json")
        git(self.fixture.root, "update-index", "--cacheinfo", f"160000,{revision},src/guardian")
        git(self.fixture.root, "commit", "-m", "nonexistent revision")
        self.assert_result(1, f"declared Guardian revision {revision} does not exist")

    def test_hidden_index_flags_are_rejected(self) -> None:
        for root, path in (
            (self.fixture.root, "config/final.yml"),
            (self.fixture.guardian, "rundmc/runcontainerd/nerd/nerd_linux_test.go"),
        ):
            for flag in ("assume-unchanged", "skip-worktree"):
                with self.subTest(root=root, flag=flag):
                    git(root, "update-index", f"--{flag}", path)
                    try:
                        self.assert_result(1, "hidden index flags")
                    finally:
                        git(root, "update-index", f"--no-{flag}", path)

    def test_wrong_toolchain_identity(self) -> None:
        blobs = self.fixture.root / "config/blobs.yml"
        write(blobs, blobs.read_text().replace(FIXTURE_ARCHIVE_SHA, "0" * 64))
        self.fixture.commit_release()
        self.assert_result(1, "Go toolchain SHA-256 is")

    def test_wrong_toolchain_version(self) -> None:
        write(self.fixture.root / "packages/golang-1.26-linux/version", "1.26.2\n")
        self.fixture.commit_release()
        self.assert_result(1, "Go toolchain version is 1.26.2, expected 1.26.3")

    def test_wildcard_toolchain_packaging_is_rejected(self) -> None:
        write(self.fixture.root / "packages/golang-1.26-linux/packaging", "tar xzf go*.linux-amd64.tar.gz\n")
        self.fixture.commit_release()
        self.assert_result(1, "Go toolchain packaging archive is")

    def test_missing_hydrated_toolchain_archive(self) -> None:
        self.fixture.archive.unlink()
        self.assert_result(1, "hydrated Go toolchain archive is unavailable")

    def test_one_byte_toolchain_archive_tampering(self) -> None:
        self.fixture.archive.write_bytes(FIXTURE_ARCHIVE + b"!")
        self.assert_result(1, "hydrated Go toolchain archive size is 20, expected 19")

    def test_same_size_toolchain_archive_tampering(self) -> None:
        self.fixture.archive.write_bytes(b"X" + FIXTURE_ARCHIVE[1:])
        self.assert_result(1, "hydrated Go toolchain archive SHA-256 is")

    def test_hydrated_toolchain_symlink_escape(self) -> None:
        outside = self.fixture.root.parent / "outside-archive"
        outside.write_bytes(FIXTURE_ARCHIVE)
        self.fixture.archive.unlink()
        self.fixture.archive.symlink_to(outside)
        self.assert_result(1, "symlink is not allowed")

    def test_hydrated_toolchain_root_must_be_approved_and_relative(self) -> None:
        for blob_root in ("../blobs", "/tmp/blobs", "blob\\root"):
            with self.subTest(blob_root=blob_root):
                self.fixture.metadata["toolchain"]["blob_root"] = blob_root
                self.fixture.write_metadata()
                self.assert_result(2, "toolchain.blob_root")

    def test_wrong_release_identity(self) -> None:
        write(self.fixture.root / "config/final.yml", "---\nfinal_name: not-garden\n")
        self.fixture.commit_release()
        self.assert_result(1, "release name is not-garden, expected garden-runc")

    def test_explicit_pin_does_not_require_branch_ancestry(self) -> None:
        write(self.fixture.guardian / "local-only", "not on origin main\n")
        git(self.fixture.guardian, "add", "local-only")
        git(self.fixture.guardian, "commit", "-m", "local-only Guardian")
        revision = git(self.fixture.guardian, "rev-parse", "HEAD")
        self.fixture.metadata["guardian"]["revision"] = revision
        self.fixture.metadata["downstream_patches"][0]["revision"] = revision
        self.fixture.write_metadata()
        git(self.fixture.root, "add", "PROVENANCE.json")
        git(self.fixture.root, "update-index", "--cacheinfo", f"160000,{revision},src/guardian")
        git(self.fixture.root, "commit", "-m", "local-only pin")
        self.assert_result(0, "Verified release-local contract consistency")

    def test_unavailable_live_remote_does_not_block_offline_verification(self) -> None:
        self.fixture.set_live_remote(self.fixture.root.parent / "missing-origin.git")
        self.assert_result(0, "Verified release-local contract consistency")

    def test_absent_live_branch_does_not_block_offline_verification(self) -> None:
        empty_remote = self.fixture.root.parent / "empty-origin.git"
        empty_remote.mkdir()
        git(empty_remote, "init", "--bare", "-b", "main")
        self.fixture.set_live_remote(empty_remote)
        self.assert_result(0, "Verified release-local contract consistency")

    def test_ambiguous_live_branch_does_not_block_offline_verification(self) -> None:
        wrapper_dir = self.fixture.root.parent / "ambiguous-git-wrapper"
        wrapper_dir.mkdir()
        wrapper = wrapper_dir / "git"
        real_git = shutil.which("git")
        self.assertIsNotNone(real_git)
        write(
            wrapper,
            "#!/bin/sh\n"
            f'if [ "$#" -eq 4 ] && [ "$1" = "ls-remote" ] && [ "$2" = "--exit-code" ] && '
            f'[ "$3" = "{GUARDIAN_REMOTE}" ] && [ "$4" = "refs/heads/main" ]; then\n'
            f"  printf '%s\\t%s\\n%s\\t%s\\n' '{self.fixture.revision}' 'refs/heads/main' "
            f"'{self.fixture.revision}' 'refs/heads/main'\n"
            "  exit 0\n"
            "fi\n"
            f"exec {shlex.quote(real_git or 'git')} \"$@\"\n",
        )
        wrapper.chmod(0o755)
        self.fixture.verifier_env["PATH"] = str(wrapper_dir) + os.pathsep + self.fixture.verifier_env["PATH"]
        self.assert_result(0, "Verified release-local contract consistency")

    def test_inherited_url_rewrite_does_not_change_live_query(self) -> None:
        empty_remote = self.fixture.root.parent / "rewrite-target.git"
        empty_remote.mkdir()
        git(empty_remote, "init", "--bare", "-b", "main")
        self.fixture.verifier_env["GIT_CONFIG_COUNT"] = "1"
        self.fixture.verifier_env["GIT_CONFIG_KEY_0"] = f"url.{empty_remote}.insteadOf"
        self.fixture.verifier_env["GIT_CONFIG_VALUE_0"] = GUARDIAN_REMOTE
        self.assert_result(0, "Verified release-local contract consistency")

    def test_forged_local_origin_main_inconsistent_with_live_remote(self) -> None:
        write(self.fixture.guardian / "forged", "local only\n")
        git(self.fixture.guardian, "add", "forged")
        git(self.fixture.guardian, "commit", "-m", "forged local remote head")
        forged = git(self.fixture.guardian, "rev-parse", "HEAD")
        git(self.fixture.guardian, "update-ref", "refs/remotes/origin/main", forged)
        self.assert_result(1, f"Guardian HEAD is {forged}, expected {self.fixture.revision}")

    def test_equivalent_independent_remote_main_history_is_accepted(self) -> None:
        independent_seed = self.fixture.root.parent / "independent-seed"
        independent_seed.mkdir()
        git(independent_seed, "init", "-b", "main")
        git(independent_seed, "config", "user.email", "fixture@example.invalid")
        git(independent_seed, "config", "user.name", "Fixture")
        write(
            independent_seed / "rundmc/runcontainerd/nerd/nerd_linux_test.go",
            f'package nerd_test\n\nvar _ = It("{CLEANUP_SELECTOR}", func() {{}})\n',
        )
        git(independent_seed, "add", ".")
        git(independent_seed, "commit", "-m", "independent equivalent Guardian")
        independent_remote = self.fixture.root.parent / "independent-origin.git"
        subprocess.run(
            ["git", "clone", "--bare", str(independent_seed), str(independent_remote)],
            check=True,
            capture_output=True,
        )
        shutil.rmtree(self.fixture.guardian)
        subprocess.run(
            ["git", "clone", str(independent_remote), str(self.fixture.guardian)],
            check=True,
            capture_output=True,
        )
        git(self.fixture.guardian, "remote", "set-url", "origin", GUARDIAN_REMOTE)
        self.fixture.set_live_remote(independent_remote)
        revision = git(self.fixture.guardian, "rev-parse", "HEAD")
        self.fixture.metadata["guardian"]["revision"] = revision
        self.fixture.metadata["downstream_patches"][0]["revision"] = revision
        self.fixture.write_metadata()
        git(self.fixture.root, "add", "PROVENANCE.json")
        git(self.fixture.root, "update-index", "--cacheinfo", f"160000,{revision},src/guardian")
        git(self.fixture.root, "commit", "-m", "accept independent remote main history")
        self.assert_result(0, "Verified release-local contract consistency")

    def test_production_contract_records_reviewed_guardian_identities(self) -> None:
        root = VERIFIER.parent.parent
        metadata = json.loads((root / "PROVENANCE.json").read_text(encoding="utf-8"))
        self.assertEqual(metadata["guardian"]["revision"], GUARDIAN_REVISION)
        self.assertEqual(metadata["downstream_patches"][0]["revision"], CLEANUP_PATCH_REVISION)
        self.assertEqual(metadata["behavior_gates"][0]["test_anchors"][0]["selector"], CLEANUP_SELECTOR)
        self.assertEqual(git(root / "src/guardian", "rev-parse", "HEAD"), GUARDIAN_REVISION)

    def test_patch_revision_need_not_be_ancestor_of_delivered_head(self) -> None:
        delivered = self.fixture.revision
        git(self.fixture.guardian, "checkout", "--orphan", "unrelated-patch")
        write(self.fixture.guardian / "unrelated", "patch\n")
        git(self.fixture.guardian, "add", ".")
        git(self.fixture.guardian, "commit", "-m", "unrelated patch")
        patch_revision = git(self.fixture.guardian, "rev-parse", "HEAD")
        git(self.fixture.guardian, "checkout", "main")
        self.assertEqual(git(self.fixture.guardian, "rev-parse", "HEAD"), delivered)
        self.fixture.metadata["downstream_patches"][0]["revision"] = patch_revision
        self.fixture.write_metadata()
        self.fixture.commit_release()
        self.assert_result(0, "Verified release-local contract consistency")

    def test_missing_patch_revision_is_rejected(self) -> None:
        self.fixture.metadata["downstream_patches"][0]["revision"] = "f" * 40
        self.fixture.write_metadata()
        self.fixture.commit_release()
        self.assert_result(1, "declared downstream patch revision ffffffffffffffffffffffffffffffffffffffff does not exist")

    def test_inherited_git_directory_cannot_redirect_identity(self) -> None:
        self.fixture.verifier_env["GIT_DIR"] = str(self.fixture.guardian / ".git")
        self.fixture.verifier_env["GIT_WORK_TREE"] = str(self.fixture.guardian)
        self.assert_result(0, "Verified release-local contract consistency")

    def test_missing_cleanup_gate_is_rejected(self) -> None:
        self.fixture.metadata["behavior_gates"][0]["id"] = "unrelated-gate"
        self.fixture.write_metadata()
        self.assert_result(2, "behavior_gates must retain guardian-created-task-cleanup")

    def test_missing_revision_is_schema_error(self) -> None:
        del self.fixture.metadata["guardian"]["revision"]
        self.fixture.write_metadata()
        self.assert_result(2, "guardian is missing: revision")


if __name__ == "__main__":
    unittest.main()
