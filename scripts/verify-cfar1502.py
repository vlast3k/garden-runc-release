#!/usr/bin/env python3

import json
from pathlib import Path
import subprocess
import sys


REPOSITORY_ROOT = Path(__file__).resolve().parent.parent
PROVENANCE_PATH = REPOSITORY_ROOT / "CFAR-1502-PROVENANCE.json"


def git(*args: str, cwd: Path = REPOSITORY_ROOT) -> str:
    result = subprocess.run(
        ["git", *args],
        cwd=cwd,
        check=True,
        capture_output=True,
        text=True,
    )
    return result.stdout.strip()


def main() -> int:
    provenance = json.loads(PROVENANCE_PATH.read_text(encoding="utf-8"))
    failures = []
    guardian = REPOSITORY_ROOT / "src" / "guardian"
    expected_guardian = provenance["guardian"]

    if not guardian.is_dir():
        failures.append("Guardian submodule is not initialized")
    else:
        actual_commit = git("rev-parse", "HEAD", cwd=guardian)
        if actual_commit != expected_guardian["source_commit"]:
            failures.append(
                f"Guardian commit is {actual_commit}, expected "
                f"{expected_guardian['source_commit']}"
            )

        for commit in expected_guardian["required_commits"]:
            result = subprocess.run(
                ["git", "merge-base", "--is-ancestor", commit, actual_commit],
                cwd=guardian,
                check=False,
            )
            if result.returncode != 0:
                failures.append(f"Guardian commit {commit} is not retained")

        nerd_source = (
            guardian / "rundmc" / "runcontainerd" / "nerd" / "nerd.go"
        ).read_text(encoding="utf-8")
        for marker in (
            "status.Status == client.Created",
            "n.runtimeType != plugins.RuntimeRuncV2",
            "parentShimPID(task.Pid(), os.ReadFile)",
            "syscall.Kill(shimPID, syscall.SIGKILL)",
        ):
            if marker not in nerd_source:
                failures.append(f"cleanup7 marker is missing: {marker}")

    modules = (REPOSITORY_ROOT / ".gitmodules").read_text(encoding="utf-8")
    if expected_guardian["repository"] not in modules:
        failures.append("Guardian submodule URL does not match provenance")
    if f"branch = {expected_guardian['branch']}" not in modules:
        failures.append("Guardian submodule branch does not match provenance")

    garden_spec = (REPOSITORY_ROOT / "jobs" / "garden" / "spec").read_text(
        encoding="utf-8"
    )
    garden_template = (
        REPOSITORY_ROOT
        / "jobs"
        / "garden"
        / "templates"
        / "config"
        / "config.ini.erb"
    ).read_text(encoding="utf-8")
    for prop in (
        "garden.containerd_runtime_type",
        "garden.use_containerd_for_processes",
    ):
        if prop not in garden_spec or prop not in garden_template:
            failures.append(f"Garden property is not fully rendered: {prop}")

    if failures:
        print("CFAR-1502 provenance verification failed:", file=sys.stderr)
        for failure in failures:
            print(f"- {failure}", file=sys.stderr)
        return 1

    print(
        "Verified Guardian pin, 17 retained commits, cleanup7, and Garden "
        "runtime properties"
    )
    return 0


if __name__ == "__main__":
    raise SystemExit(main())