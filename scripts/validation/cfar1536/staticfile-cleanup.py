import hashlib
import json
import subprocess
import urllib.request


def checked(*arguments):
    result = subprocess.run(["cf", *arguments], capture_output=True, text=True,
                            stdin=subprocess.DEVNULL, timeout=60)
    assert result.returncode == 0, (arguments, result.returncode, result.stderr)
    return result.stdout


def api(path):
    return json.loads(checked("curl", path))


def main():
    apps = {"stock": ("0f521273-8c1d-427a-a3c3-3701a314e259", "745725f8-548e-47ad-b5ac-f2770e599718"),
            "gvisor": ("f1ca1ea0-4e03-4495-beb8-7fa6d3df58a0", "205b3a2c-db22-4765-9ef5-f89267803266")}
    for backend, (app, preserved) in apps.items():
        checked("target", "-o", "cfar-1499-gvisor", "-s", "cfar1536-" + backend)
        assert "api.cf.lod-cfk8s-t09a.cfrt-sof.sapcloud.io" in checked("target")
        assert checked("app", "cfar1536-static", "--guid").strip() == app
        assert checked("app", "cfar1536-web", "--guid").strip() == preserved
        resource = api("/v3/apps/" + app)
        assert resource["name"] == "cfar1536-static"
        routes = api("/v3/apps/" + app + "/routes")["resources"]
        assert len(routes) == 1
        route = routes[0]
        host = "cfar1536-static-" + backend
        assert route["url"] == host + ".cfapps.lod-cfk8s-t09a.cfrt-sof.sapcloud.io"
        destinations = api("/v3/routes/" + route["guid"] + "/destinations")["destinations"]
        assert destinations and all(row["app"]["guid"] == app for row in destinations)
        checked("delete", "cfar1536-static", "-f", "-r")
        space = resource["relationships"]["space"]["data"]["guid"]
        assert api("/v3/apps?names=cfar1536-static&space_guids=" + space)["resources"] == []
        assert api("/v3/routes?hosts=" + host)["resources"] == []
        assert checked("app", "cfar1536-web", "--guid").strip() == preserved
        route_url = "https://cfar1536-" + backend + ".cfapps.lod-cfk8s-t09a.cfrt-sof.sapcloud.io/"
        with urllib.request.urlopen(route_url, timeout=5) as response:
            assert response.status == 200
            assert hashlib.sha256(response.read()).hexdigest() == "80097eecb2a798755b10bc1d4b0056e27654d9aa980205e0e80a5ada497006a2"
        print(json.dumps({"backend": backend, "deleted_app": app, "deleted_route": route["guid"],
                          "preserved_app": preserved, "preserved_route_exact": True}), flush=True)
    print("PASS: both exact disposable apps/routes absent; original app GUIDs and exact route payloads preserved")


if __name__ == "__main__":
    main()