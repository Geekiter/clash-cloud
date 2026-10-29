# Clash Cloud project overview

## Background and goal

Replace the opaque subscription-management binary in the current Mihomo edge setup with a maintainable service. Manage multiple subscription sources and merge their valid nodes into a single Mihomo configuration and subscription URL. Admin UI uses React + Material UI and avoids redundant helper copy and small-print clutter.

## Scope assumptions

- “One node” means one unified subscription/config output containing multiple individually selectable proxy nodes.
- Input formats: Clash/Mihomo YAML with a `proxies` list, Base64-encoded YAML, and a defined subset of plain/Base64 URI feeds.
- Initial deployment is one instance on `tx`; implementation is developed on `ac`.
- Mihomo remains a separate process; the control plane manages sources, snapshots, rendering, and status.
- The existing Nginx access boundary, node test/switch behavior, and MetaCubeXD access will be preserved after their exact compatibility requirements are inventoried.

## Repository layout

- `backend/`: Go API, persistence, refresh scheduler, parser, config renderer.
- `web/`: React + TypeScript + MUI admin UI.
- `deploy/`: service units and deployment/rollback examples; no production changes from this directory alone.
- `docs/`: scope, API contract, stage plans, decisions.

## Stages

1. MVP implementation on `ac`: subscription CRUD/refresh, valid-snapshot fallback, merged output, admin UI, developer build/run docs.
2. Mihomo integration and `tx` rollout: test against the edge core, preserve routing/UI compatibility, shadow deploy, verify output, prepare rollback, then schedule cutover.
3. URI subscriptions: convert supported proxy URI feeds into Mihomo nodes and display sanitized format errors.

## Stage records

- Stage 1: `stage-1.md`.
- Stage 2: `stage-2.md`.
- Stage 3: `stage-3.md`.
- Shared API contract: `api-contract.md`.

## Decisions to confirm before production cutover

- Additional URI schemes required beyond the Stage 3 subset.
- Proxy group strategy and routing rules that must match current `tx` behavior.
- How the existing Nginx authentication and FRP/MetaCubeXD paths map to the new app.
- Whether the initial deployment remains a single admin space or needs multiple users.
