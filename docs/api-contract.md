# Shared API contract — v1

Same-origin JSON under `/api/v1`. Management routes are reachable only through the authenticated ingress; during development bind the Go server to loopback. JSON uses `camelCase`, timestamps are RFC 3339 UTC, list results use `{ "items": [], "total": 0 }`.

## Subscriptions

- `GET /subscriptions` → `{items:[{id,name,enabled,refreshIntervalSeconds,nodeCount,lastAttemptAt,lastSuccessAt,status,errorCode}],total}`. Never return URL, auth headers, or provider response body.
- `POST /subscriptions` body `{name,url,refreshIntervalSeconds?}` → created source without URL; queues initial refresh.
- `GET /subscriptions/{id}` → safe source fields (no URL).
- `PATCH /subscriptions/{id}` body accepts `{name?,url?,enabled?,refreshIntervalSeconds?}`.
- `DELETE /subscriptions/{id}` → `204`; deletes this source and its snapshots only.
- `POST /subscriptions/{id}/refresh` → `202 {runId,status:"queued"}`; one refresh per source at a time.
- `GET /subscriptions/{id}/nodes` → `{items:[{name,type,sourceId,sourceName,enabled}],total}` from the last valid snapshot.

Statuses: `pending`, `healthy`, `degraded`, `failed`, `disabled`. The current implementation reports refresh failures as `failed` and disables as `disabled`. Keep the last valid node snapshot when a refresh fails.

## Merged nodes and output

- `GET /nodes?q=&subscriptionId=&limit=&offset=` → `{items:[{id,name,type,sourceId,sourceName,enabled}],total}`.
- `PATCH /nodes/{id}` body `{enabled:boolean}` → `{id,enabled}`; node-level state persists across subscription refreshes and renamed sources. Disabled nodes are omitted from every unified output link.
- `GET /health` → `{status,version}` without secrets.

- `GET /output` → `{status,nodeCount,generatedAt,revision,url,links}`. Each `links` item is `{id,name,url,enabled,createdAt}`; `url` remains the default link URL for compatibility. Link URLs are bearer credentials and are returned only through the authenticated management ingress.
- `POST /output/links` body `{name}` → `201 {link}`; creates an enabled link with its own token to the shared output.
- `PATCH /output/links/{id}` body `{name?,enabled?}` → `{link}`; names are trimmed, 1–80 Unicode characters, and case-insensitively unique. Disabled links return 404 from their public URL.
- `POST /output/rotate-token` → `{url,rotatedAt}`; retained for compatibility and rotates the default link token.
- `GET /s/{token}` → `application/yaml` unified config, or 404 for an unknown/disabled link and a clear non-200 error if no valid output exists.

## Errors

`{ "error": { "code": "stable_machine_code", "message": "short operator-readable text" } }`. Messages must not contain source URLs, credentials, response bodies, or proxy secrets.
