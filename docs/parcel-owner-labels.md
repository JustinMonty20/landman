# Parcel owner labels

The Union GIS parcel publisher writes name-based indicators and mailing-state labels in the same transaction as the parcel. The labels do not establish the owner's residence, identity, or corporate registration. `not_detected` does not mean that the owner is a person.

Apply migrations in order. Migration `000036` belongs to the FEMA change and must be applied before `000037` on shared databases. Goose does not automatically apply a lower migration after the database reaches version `000037`.

After migration, classify existing stored Union GIS parcels without contacting the county:

```sh
DATABASE_URL='postgres://…' go run ./cmd/owner-label-backfill
```

The command commits up to 500 stored parcels per batch. It fills missing labels, can be rerun after interruption, and fails if its final count of labeled parcels differs from the stored Union GIS parcel count. The publisher refreshes labels on changed snapshots. An unchanged parcel replay fills a missing label. The command does not overwrite existing classifications; republish a changed parcel to recompute its label.

The authenticated `GET /api/v1/parcels` and `GET /api/v1/parcels/map` endpoints accept `out_of_state_mailing=yes|no|unknown` and `entity_name_indicator` or `corporate_name_indicator` with `matched|not_detected|unknown`. The detail endpoint includes `owner_labels` with status, evidence, and classification time. A null `owner_labels` means that the stored parcel has not yet been classified; run the backfill. `source_parcel_id` in the detail response identifies the source record for manual inspection. The browser does not expose these filters or render labels in this change.

No source-record false-positive review was performed without approved access. Do not use these labels for outreach before inspecting the required positive and negative samples against source evidence.
