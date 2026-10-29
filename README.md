# Clash Cloud

Manage multiple Mihomo-compatible subscriptions and publish their usable nodes through one unified subscription output.

## Scope

- Subscription management and refresh status.
- Unified node inventory and selection/testing.
- One generated Mihomo YAML subscription endpoint.
- React + TypeScript + Material UI frontend; Go control plane; SQLite for a single initial deployment.
- The active application runtime is on `ac`; `tx` provides the public TLS/FRP ingress for `clash-cloud.chenkeyan.com`.

The current GitHub repository was empty at project start. Design and technical proposal are in Outline:
- UI: https://outline.chenkeyan.com/doc/clash-cloud-ui-oZTQb0lmHe
- Technical proposal: https://outline.chenkeyan.com/doc/clash-cloud-ODjZQRPDHN

See `docs/` for the implementation stages and shared API contract.
