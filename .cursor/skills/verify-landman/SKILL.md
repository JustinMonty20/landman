---
name: verify-landman
description: Select Landman verification skills for CLI ingestion and replay, manual tax files, API sessions and private parcel records, or browser UI changes. Use when a change can affect more than one Landman executable path.
---
# Verify Landman

Select every skill that covers the changed user path. Default to small seeded datasets and local edge-case fixtures. Live-source requests, full-county imports, operator-file drives, and performance checks require explicit selection. Report those optional checks as not run, not as blockers for a default fixture pass. A default pass is not a full release verification.

## Select

- Use [CLI verification](../verify-landman-cli/SKILL.md) for `cmd/landman`, parcel or soil ingestion and replay, FEMA, vacancy and assessed-value ingestion, GIS, scheduler, qualification jobs, CLI migrations, and CLI storage.
- Use [tax verification](../verify-landman-tax/SKILL.md) for `union.tax`, manual tax-file inputs, tax artifacts, tax parsers, tax tables, and tax publication.
- Use [API verification](../verify-landman-api/SKILL.md) for `cmd/api`, `internal/auth`, `internal/parcelapi`, `internal/parcelcrm`, API migrations, Google sign-in, sessions, private favorites, lead states, detail preferences, and API routes.
- Use [UI verification](../verify-landman-ui/SKILL.md) for `ui/`, SvelteKit, MapLibre, parcel discovery and evidence, layers, favorites and Leads, browser routing, authentication behavior, and Playwright tests.

## Combine

- A migration change requires each skill that reads or writes the changed table.
- A change to shared parcel, tax, soil, or qualification evidence requires the CLI skill and the API skill when API output can change.
- A browser change that changes API requests requires the UI skill and the API skill. Private favorites and lead-state changes require the API skill's private database lane; browser fixtures cannot prove account isolation or storage.
- Stored tax display uses the API and UI skills. Tax file ingestion uses the tax skill. External tax research links do not prove tax ingestion.
- A release that changes CLI, API, and UI paths requires all relevant skills. Keep isolated databases, test fixtures, and live-source checks separate.

## Completion

Report each selected skill as `in sync`, `updated`, `bug`, or `blocked`. Name each skipped skill and the reason.
