# Clash Cloud backend (MVP)

A single-process Go control plane for subscription CRUD, guarded fetching, last-good snapshots, merged Mihomo YAML output, and the documented `/api/v1` endpoints. It uses SQLite and AES-GCM encryption for source URLs. Build artifacts and runtime state are not stored in this directory.

## Build and run

Use Go 1.23 or newer. On the development host the Go binary may be at `/usr/local/go/bin/go`.

```sh
cd backend
/usr/local/go/bin/go mod download
/usr/local/go/bin/go build -o clash-cloud .
export CLASH_CLOUD_ENCRYPTION_KEY="<base64-32-byte-key>" # use the same value on every restart
CLASH_CLOUD_DB=/var/lib/clash-cloud/state.db \
CLASH_CLOUD_LISTEN=127.0.0.1:8080 \
CLASH_CLOUD_PUBLIC_URL=https://example.invalid \
./clash-cloud
```

Generate a key with `openssl rand -base64 32` and store it in a root-readable systemd environment file. `CLASH_CLOUD_ENCRYPTION_KEY` accepts base64 or hex encoding of exactly 32 bytes. Back up the database and key together. Losing the key makes stored subscription URLs unreadable. Production deployments set `CLASH_CLOUD_OAUTH_ENABLED=1` through the required `mattermost-oauth.env` systemd drop-in; a missing credentials file prevents the service from starting.

The management UI and API use Mattermost OAuth when `CLASH_CLOUD_OAUTH_ENABLED=1`. OAuth uses the authorization-code flow and limits access to Mattermost `system_admin` accounts. The backend stores a signed 12-hour HttpOnly session cookie and does not persist Mattermost access tokens. Keep the client secret and session signing key in a root-only systemd environment file. The public `/s/{token}` endpoint is independent and accepts only its bearer token. Do not log request headers, OAuth callback query strings, or subscription bodies.

## Configuration

- `CLASH_CLOUD_DB`: SQLite database path (default `./clash-cloud.db`).
- `CLASH_CLOUD_LISTEN`: listen address (default `127.0.0.1:8080`).
- `CLASH_CLOUD_ENCRYPTION_KEY`: required 32-byte AES-GCM key as base64 or hex.
- `CLASH_CLOUD_PUBLIC_URL`: optional public base URL used in output metadata. If unset, API returns a relative `/s/{token}` URL.
- `CLASH_CLOUD_OAUTH_ENABLED`: set to `1` to require a signed-in Mattermost system administrator and enable `/auth/login`, `/auth/callback`, `/auth/me`, and `/auth/logout`.
- `CLASH_CLOUD_MATTERMOST_CLIENT_ID`, `CLASH_CLOUD_MATTERMOST_CLIENT_SECRET`: confidential OAuth application credentials registered in Mattermost. Keep the secret out of Git.
- `CLASH_CLOUD_OAUTH_SESSION_KEY`: independent base64 or hex key with at least 32 bytes of entropy used to sign browser sessions.
- `CLASH_CLOUD_VERSION`: optional health response version.
- `MIHOMO_CONTROLLER_URL`, `MIHOMO_CONTROLLER_SECRET`: controller integration for delay testing and proxy selection. When edge publishing is enabled, both are required and the URL must be an explicit loopback HTTP address and port.
- `CLASH_CLOUD_BASE_CONFIG`: absolute path to an operator-maintained Mihomo YAML file. Its ports, DNS, rules, listeners, and other settings are preserved by the edge renderer.
- `CLASH_CLOUD_EDGE_CONFIG`: absolute path Mihomo reads as its active config. The backend writes mode `0640` using a same-directory temporary file and atomic rename, then reloads through the controller. Its parent directory must exist and be writable by the service; use a setgid directory to supply the Mihomo reader group.
- `CLASH_CLOUD_MIHOMO_BIN`: absolute path to the Mihomo executable used for candidate validation (`mihomo -t -f <candidate>`).

Set all three edge variables together to enable production publishing; leave all three unset for development mode, which serves only the standalone unified subscription YAML. Edge mode also requires the loopback controller URL and secret. Seed the generated edge path with a valid base config before starting Mihomo, since its service needs a config file at startup. Mihomo's `SAFE_PATHS` must allow the edge directory so temporary validation candidates can be checked.

## MVP limitations

- Accepts Clash/Mihomo YAML, standard/raw Base64-encoded YAML containing a `proxies` list, and plaintext or Base64-encoded URI lists using `ss://`, `vmess://`, `vless://`, `trojan://`, `hysteria2://`, or `hy2://`. This is a defined scheme subset, not full Mihomo-link compatibility. It maps common TLS, WebSocket, HTTP/H2, gRPC, ALPN, SNI, Reality, and selected Shadowsocks plugin fields. VLESS accepts `encryption=none`; VMess, VLESS, and Trojan transport support is protocol-specific. `ssr://`, provider-specific encrypted formats, and unrecognized transport/plugin/encryption options are not supported.
- URI input is parsed as one strict format at a time, with an explicit scheme allowlist and byte, line, and node limits. A clearly unsupported URI scheme reports `unsupported_format`; malformed links or YAML report `invalid_subscription`. Errors never include source text or credentials.
- Subscription fetches send `User-Agent: clash.meta` by default because some providers negotiate the response format by client identity. Fetch protection rejects non-public DNS answers and rechecks them at connection time, limits redirects, response size, and request time. Provider-specific authentication headers and custom CA bundles are not supported.
- The public `/s/{token}` output remains a standalone conservative config with a `PROXY` selector and `MATCH,PROXY`; it does not expose the operator base config, controller secret, or local API settings.
- Optional edge publishing reads the operator base config, appends subscription proxies, and replaces the reserved `PROXY` group's members while preserving other base config keys, proxies, and groups. It validates the candidate with Mihomo before atomic replacement and requires a successful controller reload. On failure it restores the previous file and keeps the previous public output.
- The edge renderer treats `PROXY` as a reserved Clash Cloud group. The base config must not use it for unrelated operator policy. It does not perform a full compatibility audit of every operator group/provider reference.
- API management authentication is expected from the local trusted ingress. There are no user accounts, refresh history table, rate limiting, or encrypted node snapshots.
- The optional controller integration is only for delay test/selection. When no controller is configured, delay tests return `mihomo_unavailable`; selection is recorded for UI indication but does not change the generated selector's runtime state.
- With no enabled subscription nodes, the public output becomes a valid `DIRECT`-only `PROXY` config and status is `no_nodes`; the edge renderer removes subscription nodes while retaining operator base proxies and other groups.
