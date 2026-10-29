# Stage 3: URI subscription formats

## Background and value

Clash Cloud currently accepts Clash/Mihomo YAML only. Subscription providers that return URI-per-line feeds otherwise appear invalid, which blocks the goal of combining multiple providers. This stage adds a defined URI subset and clear status text without exposing subscription data.

Estimated size: M.

## Work items

- [x] Parse plain and Base64-wrapped URI feeds in the backend (backend agent).
- [x] Map supported protocol and transport fields to Mihomo proxy objects (backend agent).
- [x] Return sanitized `unsupported_format` and `invalid_subscription` states (backend agent).
- [x] Show concise Chinese messages in the React/MUI subscription list (frontend agent).
- [x] Review Base64 handling, transport mappings, and unsupported inputs (deployment agent).
- [ ] Deploy the release to tx and refresh the affected subscription.

## Actual record

| Date | Event | Owner |
| --- | --- | --- |
| 2026-10-29 | Added plaintext/Base64 URI-list parsing for SS, VMess, VLESS, Trojan, Hysteria2, and HY2; existing YAML and Base64-YAML parsing remains. | Backend |
| 2026-10-29 | Added strict scheme/input limits, safer parse error codes, and concise Chinese UI messages. | Backend + frontend |
| 2026-10-29 | Static review identified and corrected Base64 plus handling, H2 option mapping, and unsupported VLESS encryption handling. | Backend + reviewer |
| 2026-10-29 | Go build and production web build passed; no tests were run. | Backend + frontend |

## Acceptance criteria

- [x] Existing Clash/Mihomo YAML and Base64-YAML feeds remain supported.
- [x] Plain and Base64 URI feeds support the documented protocol subset.
- [x] Malformed links fail as `invalid_subscription`; clearly unknown URI schemes fail as `unsupported_format`.
- [x] Error messages do not contain source URLs, response bodies, or credentials.
- [x] Go and web production builds succeed.
- [ ] The tx UI accepts the existing affected subscription after refresh.

## Summary

The backend now handles common URI-based feeds while retaining strict size and scheme limits. `ssr://`, provider-specific encrypted feeds, and unsupported protocol options remain outside this defined subset. tx rollout and a live refresh remain to be recorded.
