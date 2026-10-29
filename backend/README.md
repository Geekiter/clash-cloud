# Clash Cloud backend (MVP)

A single-process Go service for subscription CRUD, guarded fetching, last-good snapshots, and merged Clash/Mihomo YAML subscription output. It uses SQLite and AES-GCM encryption for source URLs and output-link tokens. Build artifacts and runtime state are not stored in this directory.

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

Generate a key with `openssl rand -base64 32` and store it in a root-readable systemd environment file. `CLASH_CLOUD_ENCRYPTION_KEY` accepts base64 or hex encoding of exactly 32 bytes. Back up the database and key together. Losing the key makes stored subscription URLs and output-link tokens unreadable. Production deployments set `CLASH_CLOUD_OAUTH_ENABLED=1` through the required `mattermost-oauth.env` systemd drop-in; a missing credentials file prevents the service from starting.

The management UI and API use Mattermost OAuth when `CLASH_CLOUD_OAUTH_ENABLED=1`. OAuth uses the authorization-code flow and limits access to Mattermost `system_admin` accounts. The backend stores a signed 12-hour HttpOnly session cookie and does not persist Mattermost access tokens. Keep the client secret and session signing key in a root-only systemd environment file. The public `/s/{token}` endpoint is independent and accepts only its bearer token. Do not log request headers, OAuth callback query strings, or subscription bodies. Output-link management uses `GET /api/v1/output` (including `links`), `POST /api/v1/output/links` with `{"name":"..."}`, and `PATCH /api/v1/output/links/{id}` with `name` and/or `enabled`. The existing default subscription token is migrated into the `Default` link; the legacy rotate endpoint continues to rotate that link.

## Configuration

- `CLASH_CLOUD_DB`: SQLite database path (default `./clash-cloud.db`).
- `CLASH_CLOUD_LISTEN`: listen address (default `127.0.0.1:8080`).
- `CLASH_CLOUD_ENCRYPTION_KEY`: required 32-byte AES-GCM key as base64 or hex.
- `CLASH_CLOUD_PUBLIC_URL`: optional public base URL used in output metadata. If unset, API returns a relative `/s/{token}` URL.
- `CLASH_CLOUD_OAUTH_ENABLED`: set to `1` to require a signed-in Mattermost system administrator and enable `/auth/login`, `/auth/callback`, `/auth/me`, and `/auth/logout`.
- `CLASH_CLOUD_MATTERMOST_CLIENT_ID`, `CLASH_CLOUD_MATTERMOST_CLIENT_SECRET`: confidential OAuth application credentials registered in Mattermost. Keep the secret out of Git.
- `CLASH_CLOUD_OAUTH_SESSION_KEY`: independent base64 or hex key with at least 32 bytes of entropy used to sign browser sessions.
- `CLASH_CLOUD_VERSION`: optional health response version.
## MVP limitations

- Accepts Clash/Mihomo YAML, standard/raw Base64-encoded YAML containing a `proxies` list, and plaintext or Base64-encoded URI lists using `ss://`, `vmess://`, `vless://`, `trojan://`, `hysteria2://`, or `hy2://`. This is a defined scheme subset, not full Mihomo-link compatibility. It maps common TLS, WebSocket, HTTP/H2, gRPC, ALPN, SNI, Reality, and selected Shadowsocks plugin fields. VLESS accepts `encryption=none`; VMess, VLESS, and Trojan transport support is protocol-specific. `ssr://`, provider-specific encrypted formats, and unrecognized transport/plugin/encryption options are not supported.
- URI input is parsed as one strict format at a time, with an explicit scheme allowlist and byte, line, and node limits. A clearly unsupported URI scheme reports `unsupported_format`; malformed links or YAML report `invalid_subscription`. Errors never include source text or credentials.
- Subscription fetches send `User-Agent: clash.meta` by default because some providers negotiate the response format by client identity. Fetch protection rejects non-public DNS answers and rechecks them at connection time, limits redirects, response size, and request time. Provider-specific authentication headers and custom CA bundles are not supported.
- The public `/s/{token}` output is a standalone config with a `PROXY` selector and `MATCH,PROXY`. It contains the merged subscription nodes; node selection and runtime behavior belong to the user's Clash client. Clash Cloud does not read, write, validate, or reload any Mihomo runtime config.
- API management authentication is expected from the local trusted ingress. There are no user accounts, refresh history table, rate limiting, or encrypted node snapshots.
- With no enabled subscription nodes, the public output becomes a valid `DIRECT`-only `PROXY` config and status is `no_nodes`.
