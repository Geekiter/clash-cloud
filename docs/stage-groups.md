# Stage 6: Managed traffic and region groups

## Background and value

- Background: the public subscription currently emits a single `PROXY` selector and `MATCH,PROXY`; service-specific health and routing are not configurable.
- Value: keep ChatGPT, Claude, Docker registry, Google and YouTube, GitHub, Reddit, and general traffic on independent Mihomo groups. Add region-scoped groups for US/Europe and Asia without routing domains to them yet.
- Workload: M.

## Decisions

- Clash Cloud stores group policy and emits client-side proxy groups and ordered domain rules. It does not issue runtime health checks or manage tx Mihomo.
- Service groups default to independent `url-test` groups with a 180-second interval and all enabled nodes as candidates. Docker uses `geosite:docker` and probes the Docker Hub Registry API, where an unauthenticated `401` confirms reachability.
- Google-US combines Google and YouTube domain rules and uses manually selected node membership so region eligibility is explicit. Region groups 美欧 and 亚洲 also use explicit membership and have no domain rules by default.
- Mainland domain and IP traffic defaults to direct. Custom domain suffixes can also be routed direct.
- The generic group is the final `MATCH` route after service and direct rules.
- Group rules use `GEOSITE` references by default and allow additional domain suffix rules.
- Candidate probing does not guarantee all app endpoints/accounts/features work. Each group has a configurable probe URL and expected status.

## Tasks

- [x] Persist built-in and custom group policies; validate inputs and membership (owner: backend agent).
- [x] Render proxy groups and ordered rules into standalone output; retain enabled-node filtering (owner: backend and renderer agents).
- [x] Build a compact MUI group-management page for settings, rules, member selection, and health-check parameters (owner: frontend agent).
- [x] Keep the public subscription credential and source URL fields out of group APIs/output (owner: backend agent).
- [x] Reconcile implementation with this contract and build Go and web production artifacts (owner: agents + leader).

## Acceptance criteria

- [x] All nine initial groups are visible and editable as allowed; built-in groups cannot be deleted.
- [x] Service groups route their domain rules to independent `url-test` groups; each can use different candidates and probe settings.
- [x] Google-US, 美欧, and 亚洲 include only manually selected, currently enabled nodes.
- [x] 美欧 and 亚洲 have no domain routes by default; domain rules can be added later without changing the `MATCH` route.
- [x] Disabled subscription nodes are absent from every rendered group.
- [x] Group health-check interval defaults to 180 seconds and is emitted for client-side Mihomo execution.
- [x] Service proxy routes precede custom and mainland direct routes; all routes precede `MATCH`.
- [x] Direct rule settings persist and regenerate the public unified YAML without Mihomo control.
- [x] The output contains no management credentials or subscription URLs.
- [x] UI follows existing React/MUI patterns and avoids helper copy or small-print descriptions.

## Actual record

| Time | Event | Owner |
|---|---|---|
| 2026-10-29 | Implemented persistent group API and public YAML renderer without Mihomo controller calls. | Backend + renderer |
| 2026-10-29 | Added MUI group management, domain rules, node membership, and probe settings. | Frontend |
| 2026-10-29 | Go and web builds and `git diff --check` passed; no tests or deployment were run. | Agents + leader |

## Summary

Nine built-in groups and custom group management are available in the ac working tree. Region membership is explicitly selected, and empty selected groups are omitted until they have enabled nodes. The feature updates only the public unified subscription output.
