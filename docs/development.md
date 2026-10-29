# Development guide

## Prerequisites

- Go 1.23 or newer.
- Node.js 22.14.0 for the frontend CI baseline, plus npm.
- OpenSSL for generating a local encryption key.

## Start the API

The API listens on loopback by default. Keep the encryption key stable while using a local database; changing or losing it makes stored subscription URLs unreadable.

~~~~sh
mkdir -p "$HOME/.config/clash-cloud" "$HOME/.local/share/clash-cloud"
if [ ! -f "$HOME/.config/clash-cloud/dev-key" ]; then
  umask 077
  openssl rand -base64 32 > "$HOME/.config/clash-cloud/dev-key"
fi
export CLASH_CLOUD_ENCRYPTION_KEY="$(cat "$HOME/.config/clash-cloud/dev-key")"
export CLASH_CLOUD_DB="$HOME/.local/share/clash-cloud/dev.db"
export CLASH_CLOUD_LISTEN=127.0.0.1:8080
cd backend
go run .
~~~~

Keep the key file private and stable while using this database. Never commit it or print its contents. The intended API base is http://127.0.0.1:8080/api/v1.

## Start the frontend

~~~~sh
cd web
npm ci
npm run dev -- --host 127.0.0.1
~~~~

The package lock is committed, so use npm ci for reproducible installs. Vite proxies `/api` and `/s` to the loopback Go service on port 8080. Do not put subscription URLs, the encryption key, or output tokens in frontend build-time variables. The browser should receive only the safe fields in docs/api-contract.md.

## Build and lint

~~~~sh
cd backend
go build -trimpath -o clash-cloud .

cd ../web
npm ci
npm run build
~~~~

The frontend currently defines no lint script. CI installs and builds it from package-lock.json; lint runs automatically if a lint script is added. CI also compiles the Go backend using the version in go.mod. CI does not run tests.

## Local data and secrets

Keep local state outside the checkout. Use a database file with owner-only access (0600). Never paste a real subscription URL into an issue, log, screenshot, or committed config. Use a disposable source URL for manual development.

The generated /s/{token} URL is a bearer credential. Treat it like a password, keep it out of shell history and logs, and rotate it if exposed. GET /output returns metadata for the admin UI; it is not an admin API credential.

Back up the SQLite database and CLASH_CLOUD_ENCRYPTION_KEY together. The key is required and must decode from base64 or hex to exactly 32 bytes. Clash Cloud only manages subscriptions and renders client configuration; it has no Mihomo controller or runtime configuration settings.

## Deployment boundary

deploy/ contains deployment and ingress examples. See deploy/README.md. The app deployment is independent of tx's Mihomo services; subscription refreshes and output generation do not control the proxy runtime.
