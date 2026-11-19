# Parcel owner labels

The Union GIS parcel publisher writes name-based indicators and mailing-state labels in the same transaction as the parcel. The labels do not establish the owner's residence, identity, or corporate registration. `not_detected` does not mean that the owner is a person.

**Migration order.** Do not merge the owner-label branch until FEMA migration `000036` is reachable from `origin/main`. The PR and merge gate must verify that branch ancestry. Apply `000036` before owner-label migration `000037` on every shared database. Goose does not automatically apply a lower migration after a database reaches version `000037`; do not use out-of-order migration mode to bypass this gate. This branch does not contain FEMA migration files. Local tests may apply the migrations present in this checkout, through `000035` and then `000037`, only on a fresh disposable PostGIS database.

After migration, classify existing stored Union GIS parcels without contacting the county:

```sh
DATABASE_URL='postgres://…' go run ./cmd/owner-label-backfill
```

The command commits up to 500 stored parcels per batch. It fills missing labels, can be rerun after interruption, and fails if its final count of labeled parcels differs from the stored Union GIS parcel count. The publisher refreshes labels on changed snapshots. An unchanged parcel replay fills a missing label. The command does not overwrite existing classifications; republish a changed parcel to recompute its label.

The API source stamp reads one snapshot-visible revision row. A deferred trigger advances the revision once per transaction that changes owner labels, after parcel row locks have been acquired. This briefly serializes owner-label transaction commits on the revision row, but avoids scanning all labels for every API stamp. A 500-parcel backfill batch advances the revision once, not 500 times. The trigger still executes per changed label, so large batches incur per-row trigger checks. Backfill and publication can wait for each other's parcel rows, but neither acquires the revision lock until commit. The commit trigger takes only the revision lock and never requests a parcel lock, so a revision-to-parcel wait edge cannot close a cycle. Do not lock the revision row explicitly before parcel rows; that reverses the lock order and risks deadlock.

The authenticated `GET /api/v1/parcels` and `GET /api/v1/parcels/map` endpoints accept `out_of_state_mailing=yes|no|unknown` and `entity_name_indicator` or `corporate_name_indicator` with `matched|not_detected|unknown`. The detail endpoint includes `owner_labels` with status, evidence, and classification time. A null `owner_labels` means that the stored parcel has not yet been classified; run the backfill. `source_parcel_id` in the detail response identifies the source record for manual inspection. The browser does not expose these filters or render labels in this change.

No source-record false-positive review was performed without approved access. Do not use these labels for outreach before inspecting the required positive and negative samples against source evidence.
