# Clash Cloud project overview

## Goal

Clash Cloud manages subscription sources and combines their valid nodes into one or more user-facing Clash subscription links. Users choose and switch nodes in their own Clash client. The service does not control, configure, or reload tx's Mihomo process.

The management UI uses React + Material UI and avoids redundant helper copy and small-print clutter.

## Scope

- Manage multiple subscription sources, refresh them safely, and retain each source's last valid snapshot after fetch or parse failures.
- Accept Clash/Mihomo YAML, Base64-encoded YAML, and the supported subset of plain/Base64 URI feeds.
- Merge enabled source nodes that are individually enabled into one standalone Clash configuration, exposed through named links with independent tokens. Output node names use the node-name-subscription-name format.
- Keep the subscription manager isolated from proxy runtime control. No Mihomo controller, node delay test, runtime selection, base config, or edge config is part of the application.
- Develop on `ac`; deploy the subscription-management service to `tx`. Existing tx Mihomo services remain independently managed and untouched by Clash Cloud.

## Repository layout

- `backend/`: Go API, persistence, refresh scheduler, parser, and config renderer.
- `web/`: React + TypeScript + MUI admin UI.
- `deploy/`: deployment and ingress examples; old edge-shadow examples are historical and are not used by the subscription manager.
- `docs/`: scope, API contract, stage plans, and decisions.

## Stages

1. MVP implementation on `ac`: subscription CRUD/refresh, valid-snapshot fallback, merged output, admin UI, developer build/run docs.
2. Initial `tx` rollout and ingress setup.
3. URI subscriptions: convert supported proxy URI feeds into Clash nodes and display sanitized format errors.
4. Multiple unified output links: create named links with independent tokens and enable or disable each link.
5. Subscription-only architecture: remove Mihomo controller, edge publishing, server-side delay testing, and server-side node selection.
6. Managed traffic and region groups: configure domain-specific groups and client-side health checks without connecting Clash Cloud to tx Mihomo.

## Stage records

- Stage 1: `stage-1.md`.
- Stage 2: `stage-2.md` (historical rollout record).
- Stage 3: `stage-3.md`.
- Stage 5: `stage-5.md`.
- Stage 6: `stage-groups.md`.
- Shared API contract: `api-contract.md`.
