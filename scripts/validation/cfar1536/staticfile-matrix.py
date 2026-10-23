import argparse
import hashlib
import json
from pathlib import Path
import subprocess
import time
import urllib.error
import urllib.request


def command(*arguments, deadline=40):
    return subprocess.run(["cf", *arguments], stdin=subprocess.DEVNULL,
                          capture_output=True, text=True, timeout=deadline)


def checked(*arguments):
    result = command(*arguments)
    assert result.returncode == 0, (arguments, result.returncode, result.stderr)
    return result.stdout


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--output-root", type=Path, required=True)
    arguments = parser.parse_args()
    root = arguments.output_root
    root.mkdir(mode=0o700)
    apps = {"stock": "0f521273-8c1d-427a-a3c3-3701a314e259",
            "gvisor": "f1ca1ea0-4e03-4495-beb8-7fa6d3df58a0"}
    payload_sha = "80097eecb2a798755b10bc1d4b0056e27654d9aa980205e0e80a5ada497006a2"
    config_sha = "0ee37c3cb10517063751e0278378f33ab1d0f563f1bcf86280e29674d4ca4ed4"
    profile_sha = "10ea72d238ef6be86adeb13d7e8b52dc34e478ecf5f835ae2bb145a0a17a0a43"
    with (root / "matrix.jsonl").open("x", buffering=1) as output:
        def record(row):
            encoded = json.dumps(row, sort_keys=True)
            output.write(encoded + "\n")
            print(encoded, flush=True)

        record({"event": "contract", "apps": apps, "restarts_per_backend": 3,
                "restart_seconds": 120, "ssh_seconds": 40, "http_seconds": 5,
                "config_sha256": config_sha, "profile_sha256": profile_sha,
                "garden": "1.103.0-cfar1536.2a7cf010"})
        for backend, app in apps.items():
            checked("target", "-o", "cfar-1499-gvisor", "-s", "cfar1536-" + backend)
            target = checked("target")
            assert "api.cf.lod-cfk8s-t09a.cfrt-sof.sapcloud.io" in target
            assert checked("app", "cfar1536-static", "--guid").strip() == app
            processes = json.loads(checked("curl", "/v3/apps/" + app + "/processes"))["resources"]
            process = next(row for row in processes if row["type"] == "web")
            assert process["memory_in_mb"] == 192 and process["disk_in_mb"] == 512
            assert process["health_check"]["type"] == "http"
            sidecars = json.loads(checked("curl", "/v3/apps/" + app + "/sidecars"))["resources"]
            assert len(sidecars) == 1 and sidecars[0]["name"] == "cfar1536-audit"
            assert sidecars[0]["memory_in_mb"] == 32 and sidecars[0]["process_types"] == ["web"]
            route = "https://cfar1536-static-" + backend + ".cfapps.lod-cfk8s-t09a.cfrt-sof.sapcloud.io/"
            for cycle in range(3):
                start = time.monotonic()
                restart = command("restart", "cfar1536-static", deadline=120)
                elapsed = time.monotonic() - start
                (root / f"{backend}-restart-{cycle}.json").write_text(json.dumps({
                    "exit": restart.returncode, "stdout": restart.stdout,
                    "stderr": restart.stderr, "elapsed": elapsed}))
                assert restart.returncode == 0 and elapsed < 120, restart
                stats = json.loads(checked("curl", "/v3/processes/" + process["guid"] + "/stats"))["resources"]
                assert len(stats) == 1 and stats[0]["state"] == "RUNNING"
                assert stats[0]["usage"]["mem"] > 0 and stats[0]["usage"]["disk"] > 0
                with urllib.request.urlopen(route, timeout=5) as response:
                    assert response.status == 200 and hashlib.sha256(response.read()).hexdigest() == payload_sha
                try:
                    urllib.request.urlopen(route + "cfar1536-missing", timeout=5)
                except urllib.error.HTTPError as error:
                    assert error.code == 404
                else:
                    raise AssertionError("missing path was not rejected")
                hashes = checked("ssh", "cfar1536-static", "--disable-pseudo-tty", "-i", "0", "-c",
                                 "sha256sum /home/vcap/app/nginx/conf/nginx.conf /home/vcap/profile.d/0_staticfile.sh")
                assert [line.split()[0] for line in hashes.splitlines()] == [config_sha, profile_sha]
                shell = "/bin/sh -c 'set -eu; test \"$(cat /tmp/cfar1536-sidecar)\" = cfar1536-sidecar; printf \"cfar1536-stdout\\n\"; printf \"cfar1536-stderr\\n\" >&2; exit 23'"
                ssh = command("ssh", "cfar1536-static", "--disable-pseudo-tty", "-i", "0", "-c", shell)
                assert (ssh.returncode, ssh.stdout, ssh.stderr) == (23, "cfar1536-stdout\n", "cfar1536-stderr\n"), ssh
                record({"event": "cycle_pass", "backend": backend, "cycle": cycle,
                        "restart_seconds": elapsed, "route_exact": True, "missing_path": 404,
                        "ssh_exit": 23, "streams_exact": True, "sidecar": True,
                        "config_sha256": config_sha, "profile_sha256": profile_sha,
                        "stats": stats})
        record({"event": "PASS", "cycles": 6})


if __name__ == "__main__":
    main()