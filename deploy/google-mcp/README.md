# Google MCP on Linode

Owner: RM Infra / Charlie for deployment; this repository owns the server and image. This package prepares one Google MCP container behind Linode's existing Traefik instance. Other MCP servers should have separate containers, storage, credentials and release versions. No deployment or Google sign-in is performed by these files.

The authoritative acceptance and migration gates remain in [the native MCP plan](../../docs/plans/native-google-mcp.html#verification). Remote HTTP does not enable writes or retire the CLI.

## Deployment inputs

Resolve these before rollout:

- Approved hostname. Proposed: `mcp-google.builtbyrobben.com`, matching the existing `mcp-n8n.builtbyrobben.com` naming. Confirm DNS and TLS issuance.
- Immutable tested image tag or digest, and a protected absolute deployment directory on Linode. Keep this separate from the shared `~/docker/docker-compose.yml` stack.
- App-owned Google OAuth project/client, consent-screen audience and publishing status, enabled APIs, and registered callback `http://127.0.0.1:8787/oauth/callback`. Google permits loopback redirects only for appropriate client configurations; verify the chosen application's registration and actual consent flow. No service-account key is used.
- Account owner principal, named personal/Workspace pilot accounts, and the first harness profile. Each harness needs Streamable HTTP and a configurable Authorization header. This service uses provisioned bearer tokens, not an OAuth authorization server for MCP clients.
- Keyring encryption password and per-harness random bearer tokens, stored outside Git and model context. Back up the encrypted registry/keyring and its password through the infrastructure team's protected backup system. Confirm restoration before enabling accounts.

Linode inspection found Ubuntu, Docker Compose, the `traefik_network` network, `websecure` entrypoint and `mytlschallenge` certificate resolver. Recheck these at deployment time. Existing applications and Traefik need no restart.

## Image and service configuration

Build from the reviewed commit at the repository root:

```sh
docker build -f deploy/google-mcp/Dockerfile -t google-mcp:REVIEWED_COMMIT .
```

CI builds the image and runs `python3 deploy/google-mcp/smoke.py google-mcp:ci` with disposable credentials and no Google accounts. The same smoke script accepts an image tag for Charlie to verify before rollout. Charlie must smoke-test the exact built image that will be published or deployed, record its image ID and registry digest if published, and deploy that digest. A separately rebuilt image needs its own smoke test.

The image contains native `gog-mcp` and the existing `gog` administrative CLI. Tool requests never invoke the CLI. The entrypoint reads the keyring password from a mounted file, acquires a nonblocking exclusive lock on the mounted state directory, and execs the selected binary without an extra resident wrapper process. A second service/onboarding/admin process against that directory fails rather than becoming a concurrent writer. Do not delete the lock file while a process is running. The service runs as UID/GID 10001, drops capabilities, uses a read-only image filesystem, limits memory/CPU/processes, and publishes no host ports. Traefik reaches `/mcp` through its existing Docker network. Do not attach retry middleware: ambiguous writes must not be replayed by the ingress.

Outside Git, prepare:

```text
DEPLOY_DIR/
  compose.yaml
  deployment.env
  secrets/
    keyring-password
    http-config.json
  state/
    gogcli/
```

Use mode 0700 for the deployment/secrets/state directories and 0600 for secret files. The container must be able to read its mounted secrets and write state as UID/GID 10001; local Compose file-backed secrets retain host ownership, so merely setting a Compose secret UID does not solve permissions. The infrastructure operator should set ownership explicitly. Never put real credentials in the example JSON or repository.

`deployment.env` contains nonsecret settings:

```dotenv
GOG_MCP_IMAGE=google-mcp:REVIEWED_COMMIT
GOG_MCP_DEPLOY_DIR=/ABSOLUTE/PROTECTED/PATH
GOG_MCP_HOST=mcp-google.builtbyrobben.com
GOG_MCP_NETWORK=traefik_network
GOG_MCP_OWNER=jeremy
```

Copy [http-config.example.json](http-config.example.json) into the protected secrets directory and replace its placeholders. Generate each bearer with at least 32 cryptographically random bytes; hash the exact token text with SHA-256, without a trailing newline. The server stores only that hexadecimal digest. Store the bearer itself in the intended harness's protected configuration. Distinct harness credentials can share `principal_id` to access the same owner's accounts while having different grants. Distinct account owners must have distinct principals. Caller IDs and token digests must be unique.

Keep both the caller's `allow_operations` and account/client/action grants narrow. An empty grants list grants no Google access. `--api-catalog` makes capabilities available for discovery; it grants no access by itself. The example is deliberately invalid until its token digest is replaced.

The configured Host must match the external hostname passed by Traefik. Nonbrowser clients may omit Origin; supplied Origins must exactly match the configured HTTPS origins. Forwarded identity headers are not trusted. Do not expose port 8080 directly or rely on a public HTTP URL to protect bearer tokens. Authentication is enforced by Google MCP as well as TLS at ingress; an ingress header alone never selects the caller.

## Browser onboarding without a public account page

The normal service has no account-page listener. The registry supports one process owner. Stop the service before running the onboarding profile or any command that changes its credentials. Never run a second writer against its state directory.

From the protected deployment directory, with the Compose file copied from this package:

```sh
docker compose --env-file deployment.env stop google-mcp
```

Install the app credential once, using a protected file already on the server. The mounted source file must be readable by UID 10001 (for example, owned by 10001 with mode 0600). This command operates on local configuration, without sending Google API requests:

```sh
docker compose --env-file deployment.env run --rm -T \
  -v /SECURE/oauth-client.json:/run/secrets/oauth-client.json:ro \
  onboarding gog --client native-mcp auth credentials /run/secrets/oauth-client.json
```

On your workstation, open an SSH tunnel:

```sh
ssh -N -L 8787:127.0.0.1:8787 linode
```

In a separate server terminal, keep the onboarding process attached:

```sh
docker compose --env-file deployment.env run --rm --interactive onboarding
```

The onboarding profile uses host networking only to bind the existing account page to server loopback. It has no Traefik route. Open `http://127.0.0.1:8787/` through the tunnel; complete browser consent for the selected owner. Default consent is Gmail read access. Add other service scopes only when the pilot needs them. Obtain opaque account IDs from `/accounts`, then update the protected HTTP caller grants. Stop onboarding with Ctrl-C before restarting the main service. Repeat serially with the appropriate `GOG_MCP_OWNER` for another owner; personal and Workspace accounts for one person can use the same owner.

## Rollout and rollback

RM Infra / Charlie should execute this only after the named inputs and acceptance evidence are complete:

1. Verify the exact reviewed commit/image, current origin, hostname, network/resolver and secret permissions. For updates, stop all writers and save the prior image/configuration plus a consistent protected snapshot of the complete state directory, preserving ownership and modes.
2. Validate the resolved Compose configuration without printing secrets:
   `docker compose --env-file deployment.env config --quiet`.
3. Start only this service:
   `docker compose --env-file deployment.env up -d google-mcp`.
4. Check the unauthenticated `/mcp` request returns 401 without account data. Through a fresh authorized client, verify discovery, resources, personal/Workspace selection and a denied account. Verify another harness's narrower grants cannot be bypassed with headers or arguments. Verify cancellation and bounded calls with read-only pilot tasks.
5. Observe container memory/CPU, request latency and API attempts under representative concurrent harness use. The initial 768 MiB limit is a deployment bound, not a measured capacity claim. Adjust only from measurements.
6. Change one named harness after its remote smoke test passes; preserve its previous CLI/stdio configuration. Writes, recipient rendering, native Skills activation and full workflow performance each retain their separate gates.

For a new deployment, rollback stops this Compose service and restores the harness's previous configuration. For an update, stop the service, restore its prior image/configuration, then restart; restore state only after confirming compatibility and recovery needs. Do not revoke Google grants merely to roll back routing. Keep protected state and secrets; do not use `down --volumes` as rollback.

HTTP caller tokens, grants and command policies are startup snapshots. Replace the protected configuration and restart only the Google MCP service to rotate or revoke access. HTTP mode logs and ignores SIGHUP; stdio keeps its existing grant reload behavior. A process restart terminates in-flight requests; reconcile any unknown write outcome before retrying.

## Evidence required for Charlie's handoff

Attach the reviewed commit and PR, local CI result, container build/smoke result, HTTP caller-isolation tests, actual harness/version compatibility result, and any remaining live OAuth or DNS/TLS gaps. Fixture tests establish transport and authorization behavior, not live Google consent or a completed deployment. No browser UI change is part of this transport package.
