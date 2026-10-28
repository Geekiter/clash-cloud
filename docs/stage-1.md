# Stage 1: single-instance MVP

## Background and value

The repository starts empty and the existing subscription manager is an installed binary without source. This stage builds a reviewable replacement on `ac` before touching `tx`.

- Value: let an operator add and refresh multiple sources, inspect their nodes, and publish one stable combined output.
- Work estimate: L.

## Tasks

- [ ] Go service, SQLite schema/migrations, source CRUD, refresh queue/scheduler, YAML/base64-YAML parsing, safe merge and generated output (owner: backend agent).
- [ ] React/MUI responsive admin pages for subscriptions, nodes, and unified output, using `docs/api-contract.md` (owner: frontend agent).
- [ ] Build/development instructions and isolated systemd deployment examples (owner: deployment agent).
- [ ] Integrate backend, frontend, and deployment artifacts; review API contract and security boundaries (owner: leader).

## Actual record

| Time | Event | Owner |
| --- | --- | --- |
| 2026-10-28 | Created empty GitHub repository checkout and shared API contract. | Leader |

## Acceptance criteria

- [ ] Add, edit, enable/disable, refresh, and remove source subscriptions.
- [ ] A failed refresh preserves the last valid snapshot and existing generated output.
- [ ] Enabled sources contribute individually selectable, collision-safe namespaced nodes.
- [ ] A single generated endpoint returns a validated merged Mihomo YAML configuration.
- [ ] The UI shows source state, node counts, refresh time, node inventory, and output URL without exposing source credentials.
- [ ] Source credentials and output tokens do not appear in logs or ordinary list responses.
- [ ] The code can be built locally on `ac`, and deployment examples default to loopback-only control endpoints.
- [ ] No service on `tx` is changed in this stage.

## Summary

Pending implementation.
