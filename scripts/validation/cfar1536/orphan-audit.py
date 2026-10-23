import json
import os
from pathlib import Path
import subprocess
import tomllib


def owned(value):
    return any(part.startswith(("cfar1536-", "cfar1526-cfar1536-", "cfar1526-ctl-cfar1536-"))
               for part in value.replace("=", "/").split("/"))


ctr = ["/var/vcap/packages/containerd/bin/ctr", "--address",
       "/var/vcap/sys/run/containerd/containerd.sock", "--namespace", "garden"]
result = {"tasks": [], "metadata": [], "processes": [], "paths": [], "read_errors": []}
for command, key in ((["tasks", "list", "--quiet"], "tasks"),
                     (["containers", "list", "--quiet"], "metadata")):
    output = subprocess.run(ctr + command, check=True, capture_output=True, text=True, timeout=10)
    result[key] = [line for line in output.stdout.splitlines() if owned(line)]

for directory in Path("/proc").iterdir():
    if not directory.name.isdigit() or int(directory.name) == os.getpid():
        continue
    try:
        arguments = (directory / "cmdline").read_bytes().split(b"\0")
    except FileNotFoundError:
        continue
    except OSError as error:
        result["read_errors"].append({"path": str(directory), "error": str(error)})
        continue
    decoded = [argument.decode("utf-8", errors="replace") for argument in arguments]
    runtime_names = {"runsc", "runsc-sandbox", "runsc-gofer", "runsc-exec",
                     "gvisor_sentry", "containerd-shim-runsc-v1", "runc", "dadoo", "nstar"}
    if decoded and Path(decoded[0]).name in runtime_names and any(owned(argument) for argument in decoded):
        result["processes"].append({"pid": int(directory.name), "args": decoded})

with open("/var/vcap/jobs/garden/config/containerd.toml", "rb") as config_file:
    state_root = tomllib.load(config_file)["state"]
roots = ["/var/vcap/data/garden/depot", "/run/containerd/runsc",
         str(Path(state_root) / "io.containerd.runtime.v2.task/garden"), "/sys/fs/cgroup/garden"]
for root in roots:
    if not Path(root).is_dir():
        raise RuntimeError("required runtime inventory root absent: " + root)
    for directory, children, files in os.walk(root, onerror=lambda error: result["read_errors"].append(str(error))):
        for name in children + files:
            if owned(name):
                result["paths"].append(str(Path(directory) / name))

print(json.dumps(result, sort_keys=True))
if any(result.values()):
    raise SystemExit("FAIL: owned residue or incomplete inventory")
print("PASS: no owned Garden task, metadata, argv, bundle, runsc state or cgroup residue; read-only post-destroy audit")