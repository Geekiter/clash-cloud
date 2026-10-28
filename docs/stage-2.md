# Stage 2: tx deployment

## Deployment record

| Date | Event |
| --- | --- |
| 2026-10-28 | Deployed commit `a5984a6` to `/opt/clash-cloud/releases/a5984a6` on tx; `/opt/clash-cloud/current` selects that release. |
| 2026-10-28 | Built the Go backend on tx with Go 1.25 against its Ubuntu 20.04 / glibc 2.31 runtime after the ac-built cgo binary proved ABI-incompatible. Temporary toolchain and build inputs were removed. |
| 2026-10-28 | Added dedicated Basic Auth for the admin UI and management API, obtained a Let's Encrypt certificate, and registered `clash-cloud.chenkeyan.com` with tx FRPS. |
| 2026-10-28 | Enabled `clash-cloud.service`, `clash-cloud-edge-shadow.service`, and `clash-cloud-frpc.service`. Existing `clash-edge.service` and other Clash services were left unchanged. |
| 2026-10-28 | Deployed commit `5f00d4e` to `/opt/clash-cloud/releases/5f00d4e`; management access now uses Mattermost OAuth. |
| 2026-10-28 | Registered the confidential OAuth client with the exact callback URI, then restored Mattermost DCR to disabled and restored its previous redirect allowlist. |

## Runtime topology

- Public URL: `https://clash-cloud.chenkeyan.com`.
- The UI signs in through Mattermost OAuth. Only accounts with the Mattermost `system_admin` role receive a 12-hour signed HttpOnly session. OAuth credentials are in the root-only `/etc/clash-cloud/mattermost-oauth.env`; systemd loads it through the Clash Cloud service drop-in.
- Nginx terminates TLS and proxies the domain through FRPS on loopback port 8007. A dedicated FRPC client registers the custom domain and forwards to loopback Nginx port 18080. The internal listener requires a random ingress header set by the HTTPS vhost; direct requests to FRPS port 8007 and loopback port 18080 without that header return 403.
- `/s/{token}` is public by design and authorized by its random bearer token. Nginx access logging is disabled for this route, and the backend redacts it from application access logs.
- Go API: `127.0.0.1:8080`. Shadow Mihomo proxy: `127.0.0.1:17890`. Shadow controller: `127.0.0.1:19090`. The current Clash proxy ports were not reused.
- The service account is `clash-cloud`; edge output is shared with the existing `clash-edge` identity through `clash-cloud-edge`. Database and encryption key are private to the backend account/root.

## Deployment checks

- Nginx configuration passed `nginx -t`; the domain's TLS certificate includes `clash-cloud.chenkeyan.com` and expires 2027-01-26.
- All three Clash Cloud systemd services and existing `frps`, `nginx`, and `clash-edge` are active.
- The unauthenticated UI shell returns 200 and starts Mattermost login through `/auth/login` (302 to Mattermost). `/auth/me` and management API requests return 401 without a session; `/api/v1/health` remains public.
- The registered callback `/auth/callback` is exact; the authorization endpoint accepts it and redirects to Mattermost login. OAuth callback logs are disabled in both Nginx layers. Full browser consent and post-login profile verification still require an interactive Mattermost administrator session.
- Mattermost DCR is disabled after registration. The OAuth environment file is root-owned mode `0600`; the service has all four OAuth values loaded.
- Direct FRPS and internal-Nginx bypass requests return 403. `/s/` requests did not leave the token in the Nginx access log.
- There are currently no subscription feeds configured. `/api/v1/output` reports `no_nodes`; the generated subscription URL will return 503 until a subscription has a successful refresh. Add the first feed through the authenticated UI.
- No automated test suite was run during deployment.

## Rollback

Stop and disable only `clash-cloud-frpc.service`, `clash-cloud.service`, and `clash-cloud-edge-shadow.service`; remove the Clash Cloud Nginx site symlinks/configs and service files; then remove the Clash Cloud user/group and state only after preserving the database and encryption key if they are needed. Do not stop or replace tx's existing `clash-edge.service`, `frps.service`, or other Clash services.
