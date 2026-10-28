# Stage 1: single-instance MVP

## Background and value

The repository starts empty and the existing subscription manager is an installed binary without source. This stage builds a reviewable replacement on `ac` before touching `tx`.

- Value: let an operator add and refresh multiple sources, inspect their nodes, and publish one stable combined output.
- Work estimate: L.

## Tasks

- [x] Go service, SQLite schema/migrations, source CRUD, refresh queue/scheduler, YAML/base64-YAML parsing, safe merge and generated output (owner: backend agent).
- [x] React/MUI responsive admin pages for subscriptions, nodes, and unified output, using `docs/api-contract.md` (owner: frontend agent).
- [x] Build/development instructions and isolated systemd deployment examples (owner: deployment agent).
- [x] Integrate backend, frontend, and deployment artifacts; review API contract and security boundaries (owner: leader).

## Actual record

| Time | Event | Owner |
| --- | --- | --- |
| 2026-10-28 | Created empty GitHub repository checkout and shared API contract. | Leader |
| 2026-10-28 | Started parallel backend, frontend, and deployment work on `ac` in separate directories. | Leader + agents |
| 2026-10-28 | Integrated subscription API, React/MUI UI, generated output, Mihomo edge publishing, and isolated systemd/development docs. | Agents + leader |
| 2026-10-28 | Go backend build and frontend TypeScript/Vite production build passed; no tests were run. `git diff --check` passed. | Leader |

## Acceptance criteria

- [x] Add, edit, enable/disable, refresh, and remove source subscriptions.
- [x] A failed refresh preserves the last valid snapshot and existing generated output.
- [x] Enabled sources contribute individually selectable, collision-safe namespaced nodes.
- [x] A single generated endpoint returns a validated merged Mihomo YAML configuration.
- [x] The UI shows source state, node counts, refresh time, node inventory, and output URL without exposing source credentials.
- [x] Source credentials and output tokens do not appear in logs or ordinary list responses.
- [x] The code can be built locally on `ac`, and deployment examples default to loopback-only control endpoints.
- [x] No service on `tx` is changed in this stage.

## Summary

Stage 1 is implemented and integrated on `ac`. The deployment artifacts are review templates only; this stage did not install or modify services on `tx`. Edge publishing is optional and requires a validated seed config, Mihomo, and loopback controller settings. Production cutover remains a separate reviewed stage.
