# Deployment examples

Clash Cloud is deployed as a subscription-management service. It does not control tx's Mihomo. Existing Mihomo services are independent and must remain untouched by Clash Cloud deploys. The edge-shadow examples below are historical reference material only; the application no longer consumes their variables, writes their configs, or calls their controllers.

## Active deployment topology

The Clash Cloud application runs on `ac`. `tx` provides the public TLS endpoint and FRP server for `clash-cloud.chenkeyan.com`; AC runs the Go backend, SQLite database, static web gateway, and an FRP client that registers the domain through TX. The AC app listens only on loopback (`127.0.0.1:18082` for the backend and `127.0.0.1:18081` for the web gateway). The gateway validates `X-Clash-Cloud-Ingress` before serving the UI or proxying `/auth/`, `/api/v1/`, and `/s/` to the backend.

TX does not run the active Clash Cloud backend or database. Its previous `/opt/clash-cloud` release, `clash-cloud.service`, and `clash-cloud-frpc.service` are retained as a rollback copy and are disabled. The TX FRPS and TLS Nginx ingress remain active. AC user-level services are enabled with lingering so they start at boot. Keep the AC encryption key, OAuth secrets, SQLite database, and FRP token out of Git.

On AC, the active release pointer is `~/.local/opt/clash-cloud/current`; mutable state and protected environment files live under `~/.local/share/clash-cloud` with mode `0700`, and secret files use mode `0600`. The AC backend listens on loopback port `18082`. The Node web gateway serves the React build on `127.0.0.1:18081`, validates the ingress header, and forwards application endpoints to the backend. The AC FRPC client registers the custom domain with TX FRPS. User services are defined in `systemd/clash-cloud-*.user.service`; enable user lingering so they survive logout and start at boot. The gateway source is `deploy/ac-web-server.mjs`.

At cutover, stop and disable the old TX `clash-cloud-frpc.service` and `clash-cloud.service`, then start the AC FRPC service after confirming the AC backend and gateway are healthy. Keep the old TX release and database until the AC deployment has been verified. For rollback, stop the AC FRPC service and restart the TX domain FRPC and backend services.

The following TX-local install examples are historical references; they are not the active deployment topology.

## Filesystem layout

- /opt/clash-cloud/releases/<version>/: immutable application release, owned by root:root, mode 0755.
- /opt/clash-cloud/current: symlink to the selected release; switch it only as part of a reviewed rollout or rollback.
- /etc/clash-cloud/clash-cloud.env: root:clash-cloud, mode 0640; contains the encryption key and is readable only by root and the service account's group. Keep secrets out of command-line arguments and Git.
- /var/lib/clash-cloud/: SQLite database and mutable application state, owned by clash-cloud:clash-cloud-edge, directory mode 0710. The shared edge group can traverse this parent but cannot list it; the database file remains clash-cloud-owned mode 0600. The edge subdirectory is separately owned by clash-cloud:clash-cloud-edge, mode 2750.
- journald: service logs; do not write subscription URLs or credentials to application logs.

Use a dedicated unprivileged clash-cloud user and group. The systemd unit pins CLASH_CLOUD_LISTEN to 127.0.0.1:8080 even if the environment file contains a different value. The Go service has no built-in admin authentication. Expose it only through an authenticated ingress, and protect every /api/v1/ path with the existing admin-authentication mechanism. Keep the generated subscription endpoint token separate from admin authentication.

## Runtime environment

The example unit reads /etc/clash-cloud/clash-cloud.env. Create it as root:clash-cloud with mode 0640. Set these variables:

~~~~dotenv
CLASH_CLOUD_DB=/var/lib/clash-cloud/state.db
CLASH_CLOUD_ENCRYPTION_KEY=<base64-or-hex-encoded-32-byte-key>
CLASH_CLOUD_BASE_CONFIG=/etc/clash-cloud/base.yaml
CLASH_CLOUD_EDGE_CONFIG=/var/lib/clash-cloud/edge/clash-edge.yaml
CLASH_CLOUD_MIHOMO_BIN=/usr/local/bin/clash
SAFE_PATHS=/var/lib/clash-cloud/edge
MIHOMO_CONTROLLER_URL=http://127.0.0.1:19090
MIHOMO_CONTROLLER_SECRET=<same-shadow-controller-secret>
CLASH_CLOUD_PUBLIC_URL=https://your-subscription-host.example
~~~~

Generate the key on the target host with openssl rand -base64 32, insert it into the protected environment file, and do not commit or log it. Back up the database and key together: without the key, stored subscription URLs cannot be read. Set CLASH_CLOUD_PUBLIC_URL only to the intended externally reachable subscription base URL; omit it to use a relative output URL. Do not put source subscription URLs or output tokens in this file. The base config path, Mihomo binary, edge-config path, and controller settings are optional; leave all edge-publishing variables unset for DB-output-only development. For shadow mode, use the values above and a separate protected base config.

## systemd example

systemd/clash-cloud.service runs the foreground binary at /opt/clash-cloud/current/clash-cloud. Build it from backend/main.go as described in backend/README.md and install it in an immutable release directory. The unit forces a loopback listener so the API remains unreachable from the network if Nginx or its admin-auth layer is absent.

The Nginx fragments route `/auth/` and `/api/v1/` through the OAuth-enabled backend. Disable access logging for `/auth/`, since Mattermost returns a one-time code in the callback query. The `/s/` output route has no login requirement because its bearer token is the authorization. Its access-log format replaces the token path with a redacted marker. Replace existing raw access-log formats for the host and audit location-level access_log directives so no second log records `$request`, `$request_uri`, or an unmasked `$uri`. Keep Nginx debug logging disabled and review error-log behavior and the full ingress configuration before any deployment.

A typical install sequence after review is: create the service account and directories, install one immutable release, create the protected environment file and state directory, review the unit, then enable and start it. For rollback, stop the service, point current to the previous immutable release, and start it again. Preserve the database and take a consistent backup before any schema migration. This document intentionally provides no commands that act on a production host.

## Mihomo edge service

systemd/clash-edge.service is an isolated Mihomo process for the generated edge-core config. It uses the existing binary path /usr/local/bin/clash, runs as clash-edge in the dedicated clash-cloud-edge read group, writes only to its private /var/lib/clash-edge runtime directory, and reads /var/lib/clash-cloud/edge/clash-edge.yaml. The backend validates the candidate with the same binary before publishing and asks the loopback controller API to reload it. The edge unit never restarts the backend or any existing Mihomo service.

Keep these paths and permissions separate:

- Backend database and its encryption-key environment stay private to root and the clash-cloud service account. The backend environment file is root:clash-cloud mode 0640; only the clash-cloud service account belongs to the clash-cloud group. SQLite data is clash-cloud-owned mode 0600 under a private state directory.
- The optional base config is root:clash-cloud mode 0640. It may contain server-only routing and controller settings; only the backend reads it.
- The generated edge directory is clash-cloud:clash-cloud-edge mode 2750. The backend writes a candidate there as clash-cloud:clash-cloud-edge mode 0640 using an atomic rename. Only root and members of the dedicated clash-cloud-edge group can traverse/read it.
- Add only the clash-cloud and clash-edge service accounts to clash-cloud-edge. The generated YAML contains proxy credentials and the edge controller secret required by Mihomo, so do not add human users or unrelated services to that group.
- The edge runtime directory is clash-edge:clash-cloud-edge mode 0700. Mihomo cannot write into the backend state, base config, or generated-config directory.

The backend service needs the shared group as a supplementary group to create the generated file with the setgid directory group. Set SAFE_PATHS to the edge directory so Mihomo validation can inspect candidate files there. Its umask stays 0077 for the database and environment-derived files; the publisher must explicitly set the generated config to 0640 before publication. Keep the base config and controller secret out of the public /s/{token} subscription render. That endpoint remains a standalone client config.

### First install and startup order

The edge unit reads the generated YAML at process start, while the backend reloads the controller only after it validates and publishes a new candidate. Bootstrap the edge with a valid seed before starting the integrated backend:

1. Create the clash-cloud-edge group and dedicated clash-cloud and clash-edge service accounts. Add only those accounts to the shared group.
2. Create /var/lib/clash-cloud as clash-cloud:clash-cloud-edge mode 0710, then create /var/lib/clash-cloud/edge as clash-cloud:clash-cloud-edge mode 2750. The shared group needs execute-only traversal on the parent; keep the database file 0600 and the key in its protected environment file.
3. Prepare a shadow base config from the reviewed operator base config. Set the shadow ports and controller secret below; ensure every proxy group and rule reference resolves. For an empty source set, a minimal valid seed uses `proxies: []`, a PROXY selector containing the built-in DIRECT entry, and a MATCH,PROXY rule. Do not define a proxy named DIRECT: the target Mihomo treats DIRECT as built-in and rejects a duplicate. Keep the base config root:clash-cloud mode 0640 and never use the live production ports in shadow mode.
4. Install that seed as /var/lib/clash-cloud/edge/clash-edge.yaml, owned by clash-cloud:clash-cloud-edge mode 0640. The setgid edge directory preserves the group for future atomic replacements.
5. Validate the seed using the installed binary and the same safe path:

   ~~~~sh
   SAFE_PATHS=/var/lib/clash-cloud/edge /usr/local/bin/clash -t -f /var/lib/clash-cloud/edge/clash-edge.yaml
   ~~~~
6. Start the edge shadow unit and confirm it is active and its controller listens only on 127.0.0.1:19090. Then start the backend with CLASH_CLOUD_EDGE_CONFIG, CLASH_CLOUD_BASE_CONFIG, CLASH_CLOUD_MIHOMO_BIN, and matching MIHOMO_CONTROLLER_URL/MIHOMO_CONTROLLER_SECRET. The first successful subscription refresh produces a replacement edge config and asks the already-running core to reload it.

For reboot ordering, install the matching systemd drop-in from systemd/clash-cloud.service.d/: edge-ordering.conf orders the final clash-edge.service before the app; edge-shadow-ordering.conf.example shows the equivalent dependency when the shadow unit is named clash-cloud-edge-shadow.service. Do not install both. This ordering starts the core before the backend at boot; the seed is still required on first install. Keep the edge unit independent of the backend so the core can start first.

### Shadow mode and cutover

Run a shadow core beside the current edge only with distinct loopback ports. For example, set these values in a shadow base config before publishing it:

~~~~yaml
allow-lan: false
mixed-port: 17890
external-controller: 127.0.0.1:19090
secret: replace-with-a-random-shadow-controller-secret
~~~~

The current tx edge uses ports 7890 and 9090; do not bind those while the current service is active. Keep the shadow controller on loopback and do not proxy it through Nginx. The backend controller URL should be http://127.0.0.1:19090 and its secret must match the shadow base config. With allow-lan disabled, the proxy port is not exposed to other devices in shadow mode. Do not add external-controller-unix or another controller listener; Mihomo's unix controller socket does not enforce its bearer secret.

The repository unit is named clash-edge.service to show the intended final service. Because tx already has a clash-edge.service, any shadow installation must use a separate unit name such as clash-cloud-edge-shadow.service and a separate runtime directory. Do not replace, restart, or otherwise alter the existing unit during shadow work. At cutover, first review generated output, controller reload behavior, routing, listeners, and rollback; schedule an explicit stop/rename/port handoff so two cores never bind the same proxy or controller port. Change the proxy bind address or publish route only as part of that reviewed cutover.

The default unit grants no Linux capabilities and uses PrivateDevices. Keep TUN disabled for the shadow core. If a reviewed production config needs TUN or other privileged device access, define and justify a separate least-privilege capability/device policy before rollout; do not widen this example silently.


### FRP domain ingress

The tx deployment registers `clash-cloud.chenkeyan.com` as an HTTP custom domain on the existing local FRPS vhost (`127.0.0.1:8004` for clients, `127.0.0.1:8007` for Nginx). Nginx terminates TLS and proxies through FRPS to a loopback-only Nginx listener on `127.0.0.1:18080`. The public Nginx vhost and loopback Nginx listener must share a random `X-Clash-Cloud-Ingress` value; the internal server returns 403 if it is absent or wrong. This prevents direct access to the FRPS HTTP port from bypassing the TLS ingress. Keep the FRPC config root-readable only by its dedicated service user and never commit the FRPS token or ingress secret.

Use `nginx/clash-cloud.conf.example`, `nginx/clash-cloud-internal.conf.example`, `frp/clash-cloud-frpc.ini.example`, `systemd/clash-cloud-frpc.service`, and the `systemd/clash-cloud.service.d/mattermost-oauth.conf` drop-in together. The backend requires Mattermost OAuth in production, and the systemd drop-in loads the root-only `/etc/clash-cloud/mattermost-oauth.env`. `/auth/` access logging must be disabled because callback query strings contain one-time codes. The public subscription route is intentionally unauthenticated beyond its random path token; the management UI and API require a signed-in Mattermost system administrator.
