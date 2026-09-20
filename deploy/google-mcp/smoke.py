#!/usr/bin/env python3
"""Verify a built container with disposable credentials and no Google accounts."""
import hashlib
import http.client
import json
from pathlib import Path
import secrets
import subprocess
import sys
import tempfile
import time
import urllib.error
import urllib.request


def docker(*args):
    return subprocess.check_output(["docker", *args], text=True, stderr=subprocess.STDOUT).strip()


def main():
    container = None
    with tempfile.TemporaryDirectory(prefix="gog-mcp-container-") as directory:
        root = Path(directory)
        root.chmod(0o755)
        token = secrets.token_urlsafe(32)
        config = {
            "host": "google-mcp.test",
            "allowed_origins": ["https://google-mcp.test"],
            "callers": [{
                "id": "fixture", "principal_id": "fixture",
                "token_sha256": hashlib.sha256(token.encode()).hexdigest(),
                "allow_operations": ["accounts_list"], "grants": [],
            }],
        }
        (root / "http-config.json").write_text(json.dumps(config))
        (root / "keyring-password").write_text(secrets.token_urlsafe(32))
        # Disposable fixtures only; the nonroot container needs read access.
        for path in root.iterdir():
            path.chmod(0o444)
        try:
            container = docker(
                "run", "--detach", "--read-only", "--cap-drop=ALL",
                "--security-opt=no-new-privileges", "--pids-limit=128",
                "--memory=768m", "--cpus=1", "--user=10001:10001",
                "--tmpfs=/state:rw,uid=10001,gid=10001,mode=0700",
                "--tmpfs=/tmp:rw,noexec,nosuid,size=32m",
                "--mount", f"type=bind,src={root},dst=/run/secrets,readonly",
                "--publish=127.0.0.1::8080", sys.argv[1],
                "gog-mcp", "--http-addr=0.0.0.0:8080",
                "--http-config=/run/secrets/http-config.json",
                "--discovery=compact", "--api-catalog", "--max-upstream-calls=8",
                "--registry-file=/state/gogcli/mcp-accounts.json",
                "--client-name=native-mcp", "--max-concurrency=8",
            )
            endpoint = "http://" + docker("port", container, "8080/tcp") + "/mcp"

            def request(method="tools/list", *, auth=True, origin=None, host="google-mcp.test", bearer=token):
                headers = {
                    "Host": host, "Content-Type": "application/json",
                    "Accept": "application/json, text/event-stream",
                    "MCP-Protocol-Version": "2025-03-26",
                }
                if auth:
                    headers["Authorization"] = "Bearer " + bearer
                if origin:
                    headers["Origin"] = origin
                params = {}
                if method == "initialize":
                    params = {
                        "protocolVersion": "2025-03-26", "capabilities": {},
                        "clientInfo": {"name": "container-smoke", "version": "1"},
                    }
                payload = {"jsonrpc": "2.0", "id": 1, "method": method, "params": params}
                req = urllib.request.Request(endpoint, data=json.dumps(payload).encode(), headers=headers)
                try:
                    with urllib.request.urlopen(req, timeout=10) as response:
                        return response.status, response.read()
                except urllib.error.HTTPError as response:
                    return response.code, response.read()

            def wait_ready():
                deadline = time.monotonic() + 30
                while True:
                    try:
                        status, _ = request(auth=False)
                        assert status == 401, status
                        break
                    except (urllib.error.URLError, ConnectionError, http.client.RemoteDisconnected):
                        if time.monotonic() >= deadline:
                            raise
                        time.sleep(0.25)

            wait_ready()
            assert request(origin="https://attacker.test")[0] == 403
            assert request(host="attacker.test")[0] in (400, 403)
            status, body = request("initialize")
            assert status == 200 and "result" in json.loads(body), (status, body)
            status, body = request()
            assert status == 200, (status, body)
            tools = json.loads(body)["result"]["tools"]
            assert {tool["name"] for tool in tools} == {
                "capabilities_search", "capabilities_describe", "capabilities_execute"}
            status, body = request("resources/list")
            assert status == 200 and len(json.loads(body)["result"]["resources"]) == 5
            assert docker("exec", container, "id", "-u") == "10001"
            second_owner = subprocess.run(
                ["docker", "exec", container, "gog-mcp-entrypoint", "true"],
                capture_output=True, check=False,
            )
            assert second_owner.returncode == 1, second_owner.returncode
            rotated = secrets.token_urlsafe(32)
            config["callers"][0]["token_sha256"] = hashlib.sha256(rotated.encode()).hexdigest()
            config_path = root / "http-config.json"
            config_path.chmod(0o644)
            config_path.write_text(json.dumps(config))
            config_path.chmod(0o444)
            docker("kill", "--signal=HUP", container)
            signal_deadline = time.monotonic() + 5
            while "SIGHUP ignored in HTTP mode" not in docker("logs", container):
                if time.monotonic() >= signal_deadline:
                    raise AssertionError("HTTP process did not handle SIGHUP")
                time.sleep(0.1)
            assert request()[0] == 200
            assert request(bearer=rotated)[0] == 401
            docker("restart", "--time=10", container)
            endpoint = "http://" + docker("port", container, "8080/tcp") + "/mcp"
            wait_ready()
            assert request()[0] == 401
            assert request(bearer=rotated)[0] == 200
            docker("stop", "--time=10", container)
            assert docker("inspect", "--format={{.State.ExitCode}}", container) == "0"
            print("PASS: HTTP container auth, Host/Origin, discovery, resources, nonroot, SIGHUP/restart rotation and shutdown; no Google accounts")
            print("Tested image:", docker("image", "inspect", "--format={{.Id}}", sys.argv[1]))
        except BaseException:
            if container:
                print(docker("logs", container), file=sys.stderr)
                print(docker("inspect", "--format={{json .State}}", container), file=sys.stderr)
            raise
        finally:
            if container:
                docker("rm", "--force", container)


if __name__ == "__main__":
    main()
