# Parcel owner and mailing labels

Union County's GIS layer 215 supplies current owner names (`CURR_NAME1`, `CURR_NAME2`) and current mailing address (`CURR_STATE`, among other fields). Label parcels for **mailing outside North Carolina** and for **legal-entity designators in the current owner's name**. The former is not proof of the owner's residence; the latter is a name-based indicator, not a verified Secretary of State registration or beneficial-ownership finding. A corporation is a subset of entities: an LLC must not be called a corporation.

This plan is classification and queryability only. It does not send alerts, infer death/probate, scrape court or deeds sites, buy RPA extracts, or build contact enrichment. The action backlog lives in `.agents/diary/parcel-actions.md`, not here.

## Inputs and boundaries

- `internal/connector/union/translator.go` already maps `CURR_NAME1/2` to `models.GISParcel.Owner.CurrentName1/2` and `CURR_STATE` to `models.GISParcel.Mailing.State`. `JAN1_NAME1/2` are a tax-year snapshot, not the current owner; do not use them for these labels. The situs address is not the mailing address and currently has only a street line.
- `db/migrations/000001_init_schema.sql` stores these fields in `parcels`. `internal/parcelstore/parcels.go` upserts by `(county_id, source_name, source_parcel_id)` and preserves the parcel UUID. Keep normalized county data in `parcels`; derived labels belong in a separate table keyed by `parcels.id`, not in `source_extras`.
- The current `main.go` is not a durable full-county ingest. `plans/soil-layer.md` persist-1 is the planned write path for all GIS parcels. Do not claim labels will refresh automatically until that ingest is landed and the post-upsert classification is wired into its final transaction. Do not introduce a second GIS download.
- These rules apply first to Union GIS. A second county can reuse the classifier only after verifying that its normalized owner and mailing fields mean the same thing. No new county adapter or swappable classifier interface is needed.

## Label contract

Classify one normalized parcel into two independent axes, with explicit unknown/not-detected states:

| Field | Values | Meaning |
| --- | --- | --- |
| `out_of_state_mailing` | `yes`, `no`, `unknown` | `yes` means a valid current mailing state other than `NC`; `no` means `NC`; `unknown` means missing/invalid/ambiguous. It never asserts where the owner lives. |
| `entity_name_indicator` | `matched`, `not_detected`, `unknown` | `matched` means a recognized legal-entity suffix appears on a current owner name. `not_detected` means names are present but no suffix matched, **not** that the owner is a person. |
| `corporate_name_indicator` | `matched`, `not_detected`, `unknown` | A corporation-specific subset of entity matches. An LLC/LP/LLP alone is `not_detected` here, **not** proof that the owner is not a corporation. |

Return evidence without copying the full owner name into the label table: normalized mailing state (nullable), exact matched suffix (nullable), and which current owner line matched (nullable). For `not_detected`/`unknown`, the reason is the status itself plus absence of evidence; expose the source parcel ID through the parcel join for manual inspection. `corporate_name_indicator = matched` implies `entity_name_indicator = matched`; encode this invariant in the result type/classifier and, if persisted separately, a database check.

Rules:

1. Trim whitespace, uppercase the state, and accept only known two-letter USPS states plus DC and territories; `NC` is `no`, another recognized state/territory is `yes`. Missing, malformed, military routing (`AA`, `AE`, `AP`), and foreign addresses are `unknown` until the format is investigated. Do not infer a state from ZIP code or street text.
2. Trim and case-fold current owner names; normalize periods and repeated whitespace for suffix matching. Match **whole-word legal suffixes at the end of a complete name line** (allow trailing punctuation), not arbitrary substrings or sale grantors. Corporate: `INC`, `INCORPORATED`, `CORP`, `CORPORATION`. Other entity: `LLC`, `LP`, `LLP`, `LTD`, `LIMITED`, `PLC`, `PC` (including punctuated spellings such as `L.L.C.`). If either owner line contains a complete match, report the suffix and that line. A complete corporation match wins when both classes appear. Never match `CO`/`COMPANY` as high confidence.
3. If both current owner lines are empty, both name indicators are `unknown`; if names exist without a recognized suffix, both are `not_detected`. Do not concatenate two possibly distinct owners to manufacture a suffix: `CURR_NAME1=ACME`, `CURR_NAME2=LLC` needs review, not a high-confidence entity match. Likewise `TRUST`, `ESTATE`, `HEIRS`, `ET AL`, and apparent business words without a suffix do not imply a corporation; consider separate review signals later.
4. When a source record changes, **recompute** labels from the new snapshot, including clearing a prior `matched`/`yes` on missing or changed fields. Do not retain stale classifications, and do not interpret `updated_at` as a recording date or as evidence of an owner change.

## Work and evidence

### 1. Pure classifier

- [ ] Add a small classifier alongside normalized parcel logic (for example `internal/parcelowner/labels.go`) that accepts `models.GISParcel.Owner` and `.Mailing` and returns typed states/evidence. No HTTP, database, clock, notifications, and no county-specific ArcGIS field lookup inside the classifier.
- [ ] Table-driven tests cover `SC`, ` nc `, blank, garbage, DC/territory, and military/foreign state codes; `ACME LLC`, `ACME L.L.C.`, `ACME INC.`, `ACME CORPORATION`, person with no suffix, `TRUST`, empty owner, `CO`/`COMPANY`, substring (`LINCOLN`), split owner lines, and two complete owner lines of different classes. Assert corporation implies entity and nonmatches do not assert natural-person ownership.
- [ ] Run `go test -race ./internal/parcelowner/...` and `go test ./...` without a database; both pass.

### 2. Persist queryable, explainable results

- [ ] Add **one new Goose migration** using the next free version under `db/migrations/`; do not edit applied migrations. Create `parcel_owner_labels` with `parcel_id UUID PRIMARY KEY REFERENCES parcels(id) ON DELETE CASCADE`, three constrained status columns named above, nullable evidence columns (`mailing_state`, `matched_suffix`, `matched_owner_line`), and `classified_at TIMESTAMPTZ NOT NULL`. Add checks for valid states, corporate-implies-entity, non-null normalized mailing-state evidence exactly when the mailing status is known, and non-null suffix/owner-line evidence exactly when an entity match exists. Index the positive states for parcel filtering; avoid unneeded indexes on raw owner names. Up and down migrations included.
- [ ] Add an upsert in the owning storage package using the parcel UUID returned by `SaveParcel`. Update the labels and timestamp on reclassification, including transitions to `unknown`/`not_detected`; rerunning the same snapshot must not create duplicate rows. No separate copy of the current names in this table.
- [ ] Add a one-time backfill path over existing Union GIS `parcels` rows (using stored current names and mailing state); no county HTTP request. Make it restart-safe, bounded in batches, and check counts of source parcels versus labeled parcels. Do not assume this table is populated just because its migration exists.
- [ ] Verify migration up/down on local PostGIS, including a parcel delete cascading to labels and rejection of invalid statuses/inconsistent corporation results. Verify a query for `out_of_state_mailing = 'yes'` and `entity_name_indicator = 'matched'` returns joined parcel IDs and reasons, without labeling every NC-mailing owner as a resident.

### 3. Refresh on the canonical GIS ingest

- [ ] After persist-1 in `plans/soil-layer.md` lands, call classification once for each final parcel upsert **in the same final transaction**, using that run's translated current owner and mailing fields. The existing import stages multipart parcel features before one final commit; do not classify intermediate page fragments and do not add another download or commit per page.
- [ ] Test first ingest, identical rerun, owner/suffix change, mailing-state change, field becoming blank, and transaction rollback. A failed import must leave both parcel and label state at the previous committed snapshot.
- [ ] On a local sample of live Union records, manually inspect at least 20 positive out-of-state results, 20 positive entity results (including corporations and LLCs), and 20 `unknown`/`not_detected` results against the GIS source. Record counts and false-positive cases; revise suffix rules before using labels to drive outreach.
- [ ] Run `go test -race ./...` and `make build`. Check the queryable label counts after the full ingest/backfill and confirm no Union parcel lacks a label; if persist-1 has not landed, report this step blocked rather than pretending a classifier-only package completes the feature.

## Further insights (not part of this plan)

The same stored GIS snapshots may support *candidate* signals such as mailing/situs mismatch (situs address is incomplete), tax-year-versus-current-owner discrepancy (expected timing differences), valuation or building changes, owner-plus-mailing-address portfolio candidates (shared mailboxes are not proof of common ownership), and trust/estate terms in owner names. The Union tax-file plan separately covers delinquency and foreclosure. None is a verified death, probate case, corporate registration, or timely deed recording; investigate their source semantics and false-positive rates before implementing alerts. Next steps for actions are tracked in `.agents/diary/parcel-actions.md`.
