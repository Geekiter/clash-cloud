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

## Managed proxy groups

- `GET /groups` → `{items:[group],total}`; returns managed groups in renderer order.
- `POST /groups` accepts editable group fields and returns `201 {group}`. The server assigns `id` and `builtIn:false`.
- `PATCH /groups/{id}` accepts any subset of editable fields and returns `{group}`. `id` and `builtIn` are immutable.
- `DELETE /groups/{id}` → `204`; built-in groups cannot be deleted and return `409 builtin_group`.
- A group has `{id,name,enabled,rules,nodeMode,nodeIds,strategy,testUrl,intervalSeconds,timeoutMs,expectedStatus,lazy,builtIn}`. Rules are `{type,value}`, where `type` is `geosite` or `domain-suffix`. The only supported strategy is `url-test`. `expectedStatus` is a string containing an HTTP status code, inclusive range, or slash-separated combination (for example `204`, `200-399`, or `200-399/404`).
- The service seeds eight built-ins in this order: `chatgpt`/ChatGPT, `claude`/Claude, `google-us`/Google-US, `github`/GitHub, `reddit`/Reddit, `general`/通用, `europe-us`/美欧, and `asia`/亚洲. ChatGPT, Claude, GitHub, and Reddit use `all_enabled`; Google-US, 美欧, and 亚洲 use `selected` and start with no node IDs; 通用 is the enabled final MATCH group and uses `all_enabled`. Region groups start without domain rules. Built-ins may be edited, but their IDs and `builtIn` state cannot change; 通用 cannot be disabled.
- Initial preset values (all use `url-test`, 180-second interval, 5000 ms timeout, and `lazy:false`):

  | ID | Name | Rules | Membership | Test URL | Expected status |
  | --- | --- | --- | --- | --- | --- |
  | `chatgpt` | ChatGPT | `geosite:openai` | `all_enabled` | `https://chatgpt.com/` | `200-399` |
  | `claude` | Claude | `geosite:anthropic` | `all_enabled` | `https://claude.ai/` | `200-399` |
  | `google-us` | Google-US | `geosite:google`, `geosite:youtube` | `selected`, empty | `https://www.google.com/generate_204` | `204` |
  | `github` | GitHub | `geosite:github` | `all_enabled` | `https://github.com/` | `200-399` |
  | `reddit` | Reddit | `geosite:reddit` | `all_enabled` | `https://www.reddit.com/` | `200-399` |
  | `general` | 通用 | none | `all_enabled` | `https://www.gstatic.com/generate_204` | `204` |
  | `europe-us` | 美欧 | none | `selected`, empty | `https://www.gstatic.com/generate_204` | `204` |
  | `asia` | 亚洲 | none | `selected`, empty | `https://www.gstatic.com/generate_204` | `204` |
- `all_enabled` membership includes enabled subscription snapshots and node-enabled nodes. `selected` membership uses `nodeIds` from `GET /nodes`; disabled subscriptions and individually disabled nodes never enter output. Selected groups with no currently usable members and their rules are omitted until members are available. With no usable proxies, output is direct-only with `MATCH,DIRECT`.
- Group changes persist and regenerate the public unified YAML atomically. They do not control or reload the Mihomo core.

## Direct routing

- `GET /direct-routing` → `{settings:{chinaDomainDirect,chinaIPDirect,domains}}`.
- `PATCH /direct-routing` accepts any subset of those fields and returns `{settings}`. `domains` is a list of up to 100 domain suffixes; values are lowercased, deduplicated, and validated before saving.
- New installs enable mainland domain and IP direct rules by default. The settings page allows each preset to be toggled and accepts one custom domain suffix per line.
- The renderer places service-group rules first, custom direct domain rules next, then optional `GEOSITE,CN,DIRECT` and `GEOIP,CN,DIRECT`, and the final `MATCH` route last. Direct settings and the generated YAML are persisted atomically. This only changes the client-facing unified subscription; Clash Cloud does not control tx Mihomo.

## Merged nodes and output

- `GET /nodes?q=&subscriptionId=&limit=&offset=` → `{items:[{id,name,type,sourceId,sourceName,enabled}],total}`.
- `PATCH /nodes/{id}` body `{enabled:boolean}` → `{id,enabled}`; node-level state persists across subscription refreshes. Disabled nodes are omitted from every unified output link.
- `GET /health` → `{status,version}` without secrets.

- `GET /output` → `{status,nodeCount,generatedAt,revision,url,links}`. Each `links` item is `{id,name,url,enabled,createdAt}`; `url` remains the default link URL for compatibility. Link URLs are bearer credentials and are returned only through the authenticated management ingress.
- `POST /output/links` body `{name}` → `201 {link}`; creates an enabled link with its own token to the shared output.
- `PATCH /output/links/{id}` body `{name?,enabled?}` → `{link}`; names are trimmed, 1–80 Unicode characters, and case-insensitively unique. Disabled links return 404 from their public URL.
- `POST /output/rotate-token` → `{url,rotatedAt}`; retained for compatibility and rotates the default link token.
- `GET /s/{token}` → `application/yaml` unified config, or 404 for an unknown/disabled link and a clear non-200 error if no valid output exists.

## Errors

`{ "error": { "code": "stable_machine_code", "message": "short operator-readable text" } }`. Messages must not contain source URLs, credentials, response bodies, or proxy secrets.
