# Parcel ingestion evidence

## Objective and scope

Define one ingestion contract for source evidence, fingerprints, conditional publication, field history, and normalization replay. Implement and verify the contract on Union parcels first. Stop before extending it to any other ingestion method. The operator reviews the parcel result before authorizing that extension.

Preserve stable parcel and sale UUIDs, complete multipart geometry, last-part scalar precedence, omitted-sale deletion, PID-wide quarantine, fallback OBJECTID identity, independent schedules, the county request budget, and the existing ten-minute execution limit. Keep source-specific behavior outside executable configuration. Route full imports, live quarantine repair, and replay through `jobs.Runner.Run` with stable job identity and explicit attempt kinds. Replay is not a clean full source observation.

This work does not authorize live county requests, operator database writes, deployment, automatic artifact deletion, or implementation of other source adapters. Existing V1 release requirements remain unchanged.

## Contract

### Retained evidence

Retain exact count and feature-page response bytes in private immutable files. Save ordered request receipts, source identity, retrieval time, byte count, SHA-256, declared CRS, and validation state in an import manifest. Retain all source attributes and geometry in those files. Preserve original source identifiers and source timestamps in JSONB evidence. Keep typed keys and operational timestamps for indexed comparisons.

Use bounded memory and disk staging. Verify saved bytes before parsing and before publication. An incomplete or corrupt capture cannot become a published source observation. A complete local capture is evidence of the requests received. It is not proof that the remote service supplied a transactionally consistent snapshot during offset pagination.

Use local private disk for this first implementation. Keep all snapshots. Add no automatic deletion. Cloud storage and retention expiry need a later operator decision. Store portable artifact references, not absolute paths, in database records.

### Fingerprints

Keep artifact-byte, canonical source, normalized-content, and geometry SHA-256 hashes distinct. Version each canonicalization contract. Keep the normalizer version separate from the source hash.

Canonical source JSON must preserve missing, null, zero, source strings, source timestamps, and geometry. Canonicalize JSON key order and equivalent numeric representations without first converting source numbers to floating point. Exclude local retrieval metadata from content hashes.

Calculate normalized and geometry fingerprints for the completed parcel after its parts are combined. Include all persisted parcel content, vacancy, and the complete final sale list in normalized content. Exclude generated IDs, observation metadata, and database timestamps. Match numeric canonicalization to explicit storage precision. Retain exact original values in source evidence. Do not add an arbitrary acreage rounding rule or coordinate grid.

Canonicalize polygon ring start, direction, and component order without coordinate snapping. Define geometric equality conservatively. Redundant vertices may cause extra work, but small real geometry changes must not disappear. Retain full-resolution stored geometry and preserve existing spatial measurement rules.

### Publication and field history

A new source observation can advance current source evidence only when it is newer than the current observation. Record successful observations even when content is unchanged. Unchanged content must leave parcel and sale rows, IDs, and content timestamps unchanged.

Publish only complete accepted parcel candidates. An invalid part excludes its complete PID. Write current content, source links, and field history in one transaction. A failed or cancelled publication preserves previous current rows and source links. Evidence files can remain available after publication failure.

Record previous and new ownership, acreage, land use, and assessed value fields. Link changes to the previous and new source evidence and to the normalizer and fingerprint versions. An owner-field change is an observed source assertion, not proof of a legal transfer or its date. Distinguish a newly observed source change from a normalization correction.

Treat existing rows without retained source evidence as a baseline. Do not fabricate old source observations or original CRS metadata. The first verified observation establishes source evidence for those rows.

### Replay

Replay verifies retained files and uses the same parser and normalizer without HTTP requests. It does not claim a new county observation or advance source freshness. An empty-database rebuild restores the original observation time and records offline publication provenance. Historical replay must not overwrite newer source state or generate source-change history. It can produce an isolated validation result. A replay of current evidence can publish a normalization correction with explicit provenance after eligibility checks under the publication lock.

Live quarantine repair remains a new bounded source request, not historical replay. It must retain its source responses and complete PID membership under the same contract.

### Derived evidence

Owner-only and unchanged-content imports must avoid soil intersections and other unnecessary spatial calculations. Geometry changes must request the existing derived reconciliation path. Preserve independent checks for changed soil inputs, qualification generations, method versions, missing assessments, freshness expiry, and failed or pending refreshes. Do not make a transient in-memory changed flag the only recovery signal.

### Later source inputs

The evidence contract describes verified bytes, source identity, source times, declared CRS, hash versions, and publication order. ArcGIS adapters supply count and page receipts. A future FEMA adapter supplies an archive receipt and verified member references. Tax files and USDA tables can use the same contract after parcel review. The contract does not require a file source to emulate ArcGIS pages. This change implements none of those later migrations.

## Design decision

Two independent sketches compared an evidence-led parcel module with a minimal extension of the current writer. Use the source-owned workflow from the first sketch and retain the current query tables from the second. Keep artifact storage concrete for the first implementation. Avoid a plugin registry and an event-sourcing replacement. Group complete parcel candidates before comparing hashes or writing current rows.

The design comparison used the configured GPT-6 Sol and GPT-6 Luna models. The configured cross-judge pool contains only the same vendor. No cross-vendor design-judge result is claimed. A separate code review must inspect the actual diff and verification receipts.

## Work and verification

- [x] Retain complete private parcel captures and declared CRS. Reject incomplete, corrupt, or unsupported-CRS input before publication.
- [x] Verify stable canonical JSON numbers, object ordering, missing values, explicit null, numeric zero, ring rotation, ring direction, and component order.
- [x] Verify source hashes exclude retrieval metadata and remain independent of normalizer version.
- [x] Verify full and repair ingestion through the job runner. Preserve PID-wide quarantine and complete multipart publication.
- [x] Verify identical observations leave parcel and sale content rows and timestamps unchanged while source observation history advances.
- [x] Verify ownership, acreage, land use, and assessed-value changes retain previous and new values with source and normalization provenance.
- [x] Verify an ownership-only update avoids soil intersection work. Verify changed geometry refreshes affected derived evidence.
- [x] Verify changed dependencies and failed or pending derived work remain eligible even when parcel geometry is unchanged.
- [x] Verify capture followed by replay with the source server unavailable. Verify historical replay cannot replace newer ownership or advance source freshness.
- [x] Verify current-evidence normalization corrections and repeated replay do not become new source-change events.
- [x] Verify publication failure, cancellation, stable UUIDs, fallback IDs, last-part precedence, omitted-sale deletion, and foreign-key integrity.
- [x] Verify migration on an empty database and a populated pre-evidence fixture. Do not invent historical evidence during migration.
- [x] Measure a representative full-import fixture on an empty migrated database and after a failed first import. Include foreign-key checks and omitted-sale deletion in query measurements. Preserve the ten-minute limit. Synthetic fixture results do not prove live county performance.
- [ ] Run local Go CI equivalents, relevant PostGIS tests, and independent code review. Record selected checks and specific incomplete gates before handoff.
- [ ] Open a matching PR and stop at parcels. Do not merge or extend the contract to other ingestion methods.

## Throughput checkpoint

Use one implementation writer in one worktree. Run independent design reads in parallel. Keep capture, parcel publication, and replay changes under one writer because they share invariants. Verify focused tests before broad checks. Run a separate reviewer against the committed candidate and saved test results. Add focused fix verification when review changes the candidate.
