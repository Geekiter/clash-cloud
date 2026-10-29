# Stage 4: multiple unified output links

## Goal

Let administrators create multiple named subscription links for the same generated Clash Cloud output. Each link has its own token and can be enabled or disabled independently.

## Work items

- [x] Add an idempotent database migration that seeds the existing singleton token as the enabled Default link.
- [x] Add authenticated management APIs to list, create, rename, enable, and disable output links.
- [x] Keep the old /output default URL field and /output/rotate-token endpoint compatible.
- [x] Return 404 for disabled and unknown output tokens.
- [x] Replace the single-link UI with a compact link list, create/rename controls, enable switches, and copy actions.
- [x] Review migration, authorization boundary, disabled behavior, and token rotation.
- [x] Build the backend and production web app; do not run automated tests.

## Actual record

| Date | Event | Owner |
| --- | --- | --- |
| 2026-10-29 | Added output_links; startup copies the existing singleton token into Default without invalidating the existing URL. | Backend |
| 2026-10-29 | Added per-link random tokens, encrypted storage, name validation/uniqueness, and create/patch APIs. | Backend |
| 2026-10-29 | Public subscription requests now authorize against enabled links; disabled links return the same 404 as unknown links. | Backend |
| 2026-10-29 | Kept legacy default URL metadata and token rotation coherent; transaction checks both rows before commit. | Backend |
| 2026-10-29 | Added the React/MUI link management list with URL masking, copying, naming, and enable switches. | Frontend |
| 2026-10-29 | Static review and Go/web production builds passed; no tests were run. | Backend + frontend + reviewer |
| 2026-10-29 | Deployed release 4151f23 to tx. The existing token migrated to the enabled Default link; service health and the public UI returned successfully, and the configured feed remained healthy with 52 nodes. | Deployment |

## Acceptance criteria

- [x] Existing default /s/{token} remains valid after migration.
- [x] Administrators can create multiple links, set unique names, rename links, and enable/disable links.
- [x] Every link resolves to the shared generated output; each token can be disabled independently.
- [x] Disabled links return 404 without revealing whether a token exists.
- [x] UI masks tokens and provides a direct copy action.
- [x] Existing default URL and token-rotation API fields remain available.

## Summary

Unified output supports multiple independently managed links. Existing deployments retain their current link as the enabled Default entry, and all links serve the same generated configuration. Release 4151f23 is active on tx.
