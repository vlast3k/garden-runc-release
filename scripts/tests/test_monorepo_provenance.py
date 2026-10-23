import copy
import hashlib
import importlib.util
import io
import json
from pathlib import Path
import shutil
import subprocess
import tarfile
import tempfile
import unittest


SPEC = importlib.util.spec_from_file_location("provenance", Path(__file__).resolve().parents[1] / "verify_provenance.py")
PROVENANCE = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(PROVENANCE)
CLEANUP = "deletes a created alternative-runtime task using the required shim cleanup"


def archive_bytes(files):
    buffer = io.BytesIO()
    with tarfile.open(fileobj=buffer, mode="w:gz") as archive:
        root = tarfile.TarInfo(".")
        root.type = tarfile.DIRTYPE
        archive.addfile(root)
        for name, content in files.items():
            member = tarfile.TarInfo(name)
            member.size = len(content)
            archive.addfile(member, io.BytesIO(content))
    return buffer.getvalue()


class MonorepoProvenanceTest(unittest.TestCase):
    def setUp(self):
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.root = Path(self.temp.name) / "release"
        self.root.mkdir()
        self.git("init", "-b", "main")
        self.git("config", "user.name", "Fixture")
        self.git("config", "user.email", "fixture@example.invalid")
        self.git("remote", "add", "origin", PROVENANCE.RELEASE_REPOSITORY)
        self.path = "src/code.cloudfoundry.org/guardian"
        self.write(f"{self.path}/go.mod", "module code.cloudfoundry.org/guardian\n")
        self.write(f"{self.path}/cleanup_test.go", CLEANUP)
        self.write("config/final.yml", "final_name: garden-runc\n")
        self.fingerprint = "a" * 64
        self.package_name = "golang-1.27-linux"
        self.inner = archive_bytes({"go/VERSION": b"go1.27.3\ntime fixture\n"})
        self.package = Path(self.temp.name) / "go-package.tgz"
        self.package.write_bytes(archive_bytes({
            "./packaging": b"set -eu\n",
            "./compile.env.unix": b"GOTOOLCHAIN=local\n",
            "./runtime.env.unix": b"",
            "./go1.27.3.linux-amd64.tar.gz": self.inner,
        }))
        self.package_sha = hashlib.sha256(self.package.read_bytes()).hexdigest()
        self.write(f"packages/{self.package_name}/spec.lock", f"name: {self.package_name}\nfingerprint: {self.fingerprint}\n")
        self.index_path = f".final_builds/packages/{self.package_name}/index.yml"
        self.write(self.index_path, json.dumps({"builds": {self.fingerprint: {"version": self.fingerprint, "sha1": f"sha256:{self.package_sha}"}}}))
        self.commit()
        anchor = {"kind": "unit", "path": "cleanup_test.go", "selector": CLEANUP}
        self.metadata = {
            "schema_version": 2,
            "release": {"name": "garden-runc", "repository": PROVENANCE.RELEASE_REPOSITORY, "final_config": "config/final.yml"},
            "guardian": {"path": self.path, "tree": self.git("rev-parse", f"HEAD:{self.path}"), "upstream_revision": self.git("rev-parse", "HEAD"), "ported_revision": "b" * 40, "ported_repository": PROVENANCE.GUARDIAN_REPOSITORY},
            "toolchain": {"package": self.package_name, "fingerprint": self.fingerprint, "package_sha256": self.package_sha, "archive": "go1.27.3.linux-amd64.tar.gz", "archive_sha256": hashlib.sha256(self.inner).hexdigest()},
            "downstream_patches": [{"id": "guardian-created-task-cleanup", "owner": "guardian", "revision": "b" * 40, "status": "required", "application_check": "Source reference only; execute the gate.", "test_anchors": [anchor]}],
            "behavior_gates": [{"id": "guardian-created-task-cleanup", "owner": "guardian", "status": "required", "test_anchors": [anchor], "limitation": "Anchor presence is not execution proof."}],
        }

    def git(self, *arguments):
        return subprocess.run(["git", "-C", str(self.root), *arguments], check=True, capture_output=True, text=True).stdout.strip()

    def write(self, path, content):
        target = self.root / path
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content)

    def commit(self):
        self.git("add", "-f", ".")
        self.git("commit", "-m", "fixture")

    def check(self, expected=""):
        metadata = PROVENANCE.validate_metadata(self.metadata)
        failures = PROVENANCE.verify(self.root, metadata, self.package)
        if expected:
            self.assertIn(expected, "\n".join(failures))
        else:
            self.assertEqual(failures, [])

    def test_valid_inline_source_and_hydrated_package(self):
        self.check()

    def test_changed_tree_is_rejected(self):
        self.metadata["guardian"]["tree"] = "f" * 40
        self.check("inline Guardian tree does not match")

    def test_dirty_source_is_rejected(self):
        self.write(f"{self.path}/go.mod", "changed")
        self.check("release source is dirty")

    def test_hidden_index_flag_is_rejected(self):
        self.git("update-index", "--assume-unchanged", f"{self.path}/go.mod")
        self.check("hidden index flags")

    def test_changed_origin_is_rejected(self):
        self.git("remote", "set-url", "origin", "https://example.invalid/release.git")
        self.check("release origin does not match")

    def test_missing_toolchain_bytes_are_rejected(self):
        self.package = None
        self.check("--toolchain-package is required")

    def test_changed_toolchain_bytes_are_rejected(self):
        self.package.write_bytes(self.package.read_bytes() + b"changed")
        self.check("hydrated Go package SHA-256")

    def test_wrong_inner_hash_is_rejected(self):
        self.metadata["toolchain"]["archive_sha256"] = "f" * 64
        self.check("inner Go archive SHA-256")

    def test_wrong_version_is_rejected_even_with_matching_hashes(self):
        self.metadata["toolchain"]["archive"] = "go1.27.4.linux-amd64.tar.gz"
        self.package.write_bytes(archive_bytes({"packaging": b"", "compile.env.unix": b"", "runtime.env.unix": b"", "go1.27.4.linux-amd64.tar.gz": self.inner}))
        self.package_sha = hashlib.sha256(self.package.read_bytes()).hexdigest()
        self.metadata["toolchain"]["package_sha256"] = self.package_sha
        self.write(self.index_path, json.dumps({"builds": {self.fingerprint: {"version": self.fingerprint, "sha1": f"sha256:{self.package_sha}"}}}))
        self.commit()
        self.check("inner Go VERSION does not match")

    def test_malformed_index_is_rejected(self):
        self.write(self.index_path, "builds: []\n")
        self.commit()
        self.check("vendored Go package index does not match")

    def test_nested_guardian_git_is_rejected(self):
        subprocess.run(["git", "init", str(self.root / self.path)], check=True, capture_output=True)
        self.check("Guardian must be inline source")

    def test_missing_inline_module_is_rejected_without_traceback(self):
        shutil.rmtree(self.root / self.path)
        self.commit()
        self.check("unsafe inline Guardian source")

    def test_cleanup_cannot_be_waived_in_metadata(self):
        self.metadata["behavior_gates"][0]["status"] = "blocked"
        with self.assertRaisesRegex(PROVENANCE.MetadataError, "must remain required"):
            PROVENANCE.validate_metadata(self.metadata)

    def test_unknown_field_is_rejected(self):
        metadata = copy.deepcopy(self.metadata)
        metadata["guardian"]["skip_checks"] = True
        with self.assertRaisesRegex(PROVENANCE.MetadataError, "unsupported fields"):
            PROVENANCE.validate_metadata(metadata)


if __name__ == "__main__":
    unittest.main()