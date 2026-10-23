import argparse
import base64
import concurrent.futures
import hashlib
import json
import os
from pathlib import Path
import subprocess
import tempfile


def main():
    parser = argparse.ArgumentParser()
    parser.add_argument("--profile-evidence", type=Path, required=True)
    arguments = parser.parse_args()
    evidence = arguments.profile_evidence.read_text().splitlines()
    profile = base64.b64decode(evidence[1], validate=True)
    expected = "10ea72d238ef6be86adeb13d7e8b52dc34e478ecf5f835ae2bb145a0a17a0a43"
    assert hashlib.sha256(profile).hexdigest() == expected
    config = (Path(__file__).parent / "staticfile-fixture/nginx.conf").read_bytes()
    assert b"((" not in config and b"listen 8080;" in config
    assert b"((LISTEN_DIRECTIVE))" in profile and b"sed -i" in profile
    with tempfile.TemporaryDirectory(prefix="cfar1536-profile-control-") as temporary:
        root = Path(temporary)
        script = root / "actual-profile.sh"
        script.write_bytes(profile)
        target = root / "nginx/conf/nginx.conf"
        target.parent.mkdir(parents=True)
        (root / "nginx/logs").mkdir()
        environment = dict(os.environ, HOME=str(root), PORT="8080", ENABLE_HTTP2="", FORCE_HTTPS="")

        def run_profile():
            result = subprocess.run(["bash", str(script)], env=environment,
                                    capture_output=True, text=True, timeout=10)
            diagnostics = result.stderr.splitlines()
            expected_fifo_errors = all(line.startswith("mkfifo:") and "File exists" in line
                                       for line in diagnostics)
            assert result.returncode in (0, 1) and expected_fifo_errors, result

        target.write_bytes(b"((APP_ROOT))\n((PORT))\n((LISTEN_DIRECTIVE))\n((FORCE_HTTPS_DIRECTIVE))\n")
        run_profile()
        assert b"listen 8080;" in target.read_bytes() and b"((" not in target.read_bytes()
        target.write_bytes(config)
        for _ in range(30):
            run_profile()
            assert target.read_bytes() == config
        with concurrent.futures.ThreadPoolExecutor(max_workers=2) as executor:
            for _ in range(200):
                futures = [executor.submit(run_profile) for _ in range(2)]
                while not all(future.done() for future in futures):
                    assert target.read_bytes() == config
                for future in futures:
                    future.result()
                assert target.read_bytes() == config
    print(json.dumps({"result": "PASS", "profile_sha256": expected,
                      "config_sha256": hashlib.sha256(config).hexdigest(),
                      "serial_controls": 30, "concurrent_pairs": 200,
                      "original_token_substitution_control": True}))


if __name__ == "__main__":
    main()