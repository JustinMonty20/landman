# Union GIS Parcel Identity Fallback

## Goal
Make Union GIS imports use stable source identifiers so all storable parcels are persisted without inventing fake parcel IDs.

## Diagram
```mermaid
flowchart TD
    A[Raw Union GIS feature] --> B[Extract attributes]
    B --> C{PID present?}
    C -->|yes| D[source_parcel_id = PID]
    C -->|no| E{ACCTNO present?}
    E -->|yes| F[source_parcel_id = ACCTNO]
    E -->|no| G{stable ArcGIS ID present?]
    G -->|yes| H[source_parcel_id = stable ArcGIS ID]
    G -->|no| I[dead-letter / skip storage]
    D --> J[Translate parcel]
    F --> J
    H --> J
    J --> K[Persist parcel]
```

## Steps
1. Define Union GIS identity priority.
   - What to do: Document and implement the identity chain as `PID`, then existing `parcel_id` / `PARCEL_ID` if present, then `ACCTNO`, then a stable ArcGIS identifier such as `OBJECTID` only if accepted as stable enough.
   - Why it's needed: Every persisted parcel needs a deterministic source identity for upsert, dedupe, retries, and joins.
   - Risks or gotchas: Do not generate counters or random IDs for `source_parcel_id`; those are not stable across imports.

2. Keep parcel number and account number semantically separate.
   - What to do: Populate `parcel_number` from `PID` when present and `account_number` from `ACCTNO` when present. If `PID` is missing and `ACCTNO` becomes `source_parcel_id`, do not pretend it is definitely a parcel number unless source semantics confirm that.
   - Why it's needed: `PID` and `ACCTNO` may often match, but they represent different source concepts and may diverge.
   - Risks or gotchas: Collapsing both fields into one concept will make future tax/GIS/deed joins harder to reason about.

3. Fix raw GIS record construction.
   - What to do: Ensure `RawRecord.ParcelID` is populated from the identity fallback chain during GIS fetch, not only from a single expected field.
   - Why it's needed: Current errors indicate records are reaching translation with no parcel ID even though usable identifiers may exist in attributes.
   - Risks or gotchas: Field lookup should be case-insensitive where existing GIS parsing already treats attributes that way.

4. Fix translation identity validation.
   - What to do: Make the Union translator use the same identity fallback chain when constructing `models.GISParcel.Identity.SourceParcelID`.
   - Why it's needed: The translator should not reject a record only because the primary parcel field is absent when a stable fallback exists.
   - Risks or gotchas: Avoid duplicating fallback logic in multiple places unless tests pin both paths; prefer one helper in the Union connector package if possible.

5. Add tests for identity fallback behavior.
   - What to do: Cover records with `PID`, records without `PID` but with `ACCTNO`, records with lowercase/alternate parcel keys, and records with no stable identifier.
   - Why it's needed: Identity bugs silently corrupt imports or drop records.
   - Risks or gotchas: Tests should assert both stored identity fields and failure behavior for unidentifiable records.

6. Add dead-letter handling for unidentifiable records.
   - What to do: For records with no stable identifier, write a structured failure containing batch index, source name, stage, error message, and raw payload.
   - Why it's needed: Unstored parcels need to be inspectable and replayable instead of disappearing into logs.
   - Risks or gotchas: This can start as structured logging or a file, but the durable target should be a database dead-letter table.

7. Re-run import on a clean parcel table.
   - What to do: Truncate existing parcel data, run migrations if needed, then re-import all GIS parcels.
   - Why it's needed: Existing partial imports may be missing parcels dropped by the old identity behavior.
   - Risks or gotchas: Use `TRUNCATE TABLE parcels RESTART IDENTITY CASCADE;` only in local/dev or after confirming destructive reset is intended.

## Open Questions
- Is `OBJECTID` stable enough across Union County GIS exports to use as a last-resort source identity?
- Should `ACCTNO` fallback also populate `parcel_number`, or should `parcel_number` remain null when `PID` is absent?
- Should the first dead-letter implementation be a database table immediately, or a structured local file until the import workflow stabilizes?
