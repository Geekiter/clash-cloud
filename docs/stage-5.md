# Stage 5 — Subscription-only service boundary

## Decision

Clash Cloud is a subscription manager and configuration publisher. It does not control tx's Mihomo process. It does not run delay tests through Mihomo, change runtime proxy-group selections, read operator base configs, write edge configs, validate configs with a Mihomo binary, or call a Mihomo controller.

The merged public profile contains the enabled subscription proxies, a `PROXY` select group containing those nodes plus `DIRECT`, and a `MATCH,PROXY` rule. Clash clients perform node selection locally.

## Implementation

- Removed backend controller settings and Mihomo edge publishing.
- Removed server-side node delay-test and proxy selection endpoints and UI actions.
- Removed edge configuration fields from the app runtime model.
- Subscription regeneration now updates only the saved unified output in SQLite.
- Removed obsolete API fields and documented the independent runtime boundary.

## Deployment boundary

The tx Clash Cloud release is deployed separately from existing Mihomo units. The deployment must not restart, stop, reconfigure, or otherwise alter tx's Mihomo services. Legacy edge-shadow unit and configuration examples in `deploy/` are not part of the Clash Cloud application path.
