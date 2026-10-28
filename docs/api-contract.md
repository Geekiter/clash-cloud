# Shared API contract — v1

Same-origin JSON under `/api/v1`. Management routes are reachable only through the authenticated ingress; during development bind the Go server to loopback. JSON uses `camelCase`, timestamps are RFC 3339 UTC, list results use `{ "items": [], "total": 0 }`.

## Subscriptions

- `GET /subscriptions` → `{items:[{id,name,enabled,refreshIntervalSeconds,nodeCount,lastAttemptAt,lastSuccessAt,status,errorCode}],total}`. Never return URL, auth headers, or provider response body.
- `POST /subscriptions` body `{name,url,refreshIntervalSeconds?}` → created source without URL; queues initial refresh.
- `GET /subscriptions/{id}` → safe source fields (no URL).
- `PATCH /subscriptions/{id}` body accepts `{name?,url?,enabled?,refreshIntervalSeconds?}`.
- `DELETE /subscriptions/{id}` → `204`; deletes this source and its snapshots only.
- `POST /subscriptions/{id}/refresh` → `202 {runId,status:"queued"}`; one refresh per source at a time.
- `GET /subscriptions/{id}/nodes` → `{items:[{name,type,sourceId,sourceName,latencyMs?,available?}],total}` from the last valid snapshot.

Statuses: `pending`, `healthy`, `degraded`, `failed`, `disabled`. Keep the last valid node snapshot when a refresh fails.

## Merged nodes and output

- `GET /nodes?q=&subscriptionId=&limit=&offset=` → `{items:[{id,name,type,sourceId,sourceName,latencyMs?,available?,selected?}],total}`.
- `POST /nodes/{id}/delay-test` body `{url,timeoutMs?}` → `{delayMs}` or a safe error code.
- `POST /proxy-groups/{groupName}/selection` body `{name}` → `204`; escape names as URL path segments.
- `GET /output` → `{status,nodeCount,generatedAt,revision,url}`. The admin UI may mask the full URL visually; the output credential is not an admin API token.
- `POST /output/rotate-token` → `{url,rotatedAt}`; the previous token stops working.
- `GET /s/{token}` → `application/yaml` unified config, or a clear non-200 error if no valid output exists.
- `GET /health` → `{status,version}` without secrets.

## Errors

`{ "error": { "code": "stable_machine_code", "message": "short operator-readable text" } }`. Messages must not contain source URLs, credentials, response bodies, or proxy secrets.
