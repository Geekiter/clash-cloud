# Stage 2: tx deployment

## Deployment record

| Date | Event |
| --- | --- |
| 2026-10-28 | Deployed commit `a5984a6` to `/opt/clash-cloud/releases/a5984a6` on tx; `/opt/clash-cloud/current` selects that release. |
| 2026-10-28 | Built the Go backend on tx with Go 1.25 against its Ubuntu 20.04 / glibc 2.31 runtime after the ac-built cgo binary proved ABI-incompatible. Temporary toolchain and build inputs were removed. |
| 2026-10-28 | Added dedicated Basic Auth for the admin UI and management API, obtained a Let's Encrypt certificate, and registered `clash-cloud.chenkeyan.com` with tx FRPS. |
| 2026-10-28 | Enabled `clash-cloud.service`, `clash-cloud-edge-shadow.service`, and `clash-cloud-frpc.service`. Existing `clash-edge.service` and other Clash services were left unchanged. |

## Runtime topology

- Public URL: `https://clash-cloud.chenkeyan.com`.
- UI and `/api/v1/` require Basic Auth (`clash-cloud` username). The random password is stored root-only at `/etc/clash-cloud/admin-password`; the Nginx password hash is at `/etc/nginx/clash-cloud-admin.htpasswd`.
- Nginx terminates TLS and proxies the domain through FRPS on loopback port 8007. A dedicated FRPC client registers the custom domain and forwards to loopback Nginx port 18080. The internal listener requires a random ingress header set by the HTTPS vhost; direct requests to FRPS port 8007 and loopback port 18080 without that header return 403.
- `/s/{token}` is public by design and is not protected by Basic Auth. Nginx access logging is disabled for this route, and the backend redacts it from application access logs.
- Go API: `127.0.0.1:8080`. Shadow Mihomo proxy: `127.0.0.1:17890`. Shadow controller: `127.0.0.1:19090`. The current Clash proxy ports were not reused.
- The service account is `clash-cloud`; edge output is shared with the existing `clash-edge` identity through `clash-cloud-edge`. Database and encryption key are private to the backend account/root.

## Deployment checks

- Nginx configuration passed `nginx -t`; the domain's TLS certificate includes `clash-cloud.chenkeyan.com` and expires 2027-01-26.
- All three Clash Cloud systemd services and existing `frps`, `nginx`, and `clash-edge` are active.
- Unauthenticated UI request returns 401; authenticated UI and `/api/v1/health` return 200.
- Direct FRPS and internal-Nginx bypass requests return 403. `/s/` requests did not leave the token in the Nginx access log.
- There are currently no subscription feeds configured. `/api/v1/output` reports `no_nodes`; the generated subscription URL will return 503 until a subscription has a successful refresh. Add the first feed through the authenticated UI.
- No automated test suite was run during deployment.

## Rollback

Stop and disable only `clash-cloud-frpc.service`, `clash-cloud.service`, and `clash-cloud-edge-shadow.service`; remove the Clash Cloud Nginx site symlinks/configs and service files; then remove the Clash Cloud user/group and state only after preserving the database and encryption key if they are needed. Do not stop or replace tx's existing `clash-edge.service`, `frps.service`, or other Clash services.
