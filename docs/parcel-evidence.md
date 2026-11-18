# Parcel evidence

## Capture and storage

Parcel imports retain each exact count response and feature page in private local files.
The default directory is `~/.local/share/landman/parcel-evidence`.
Set `PARCEL_EVIDENCE_DIR` to select another private directory.
Directories require mode `0700`. Response and manifest files require mode `0600`.
Back up this directory. The application does not delete snapshots automatically.

The manifest retains request order, source identity, retrieval times, declared CRS, byte counts, and checksums.
Database JSONB evidence retains source identifiers, timestamps, attributes, and geometry.
The database stores snapshot IDs rather than local paths.

`source_hash` identifies canonical source content. `normalized_hash` identifies the stored parcel content and final sales.
`geometry_hash` identifies the completed parcel geometry. Each hash has an explicit rule version.
The normalizer version is separate from the source hash.
ArcGIS ring direction identifies shells and holes. The source hash preserves that direction.
The completed-geometry fingerprint ignores ring start, direction, and component order without a coordinate grid.

## Commands

Run `landman run union.gis.parcels` for a full source observation.
Run `landman reprocess union.gis.parcels --limit N` for a bounded live repair.
Both commands use the county host lock and the ten-minute job limit.
The repair fetch has an additional two-minute limit.

Run `landman replay union.gis.parcels --snapshot ID` to process saved evidence without a county request.
Find published IDs in `parcel_evidence_observations.artifact_id`.
An operator can also select a complete local capture when the database is empty or its first publication failed.
The rebuild retains the original capture time. History identifies `offline_rebuild` or `offline_retry`.
A known snapshot must match the database manifest and each current parcel's source hash and version.
Older snapshots can validate, but cannot replace newer evidence.
Current replay can publish a normalization correction. It does not report a new county observation.
Geometry corrections reconcile stored soil and qualification dependencies.
Owner-only corrections do not require soil intersections when those dependencies remain current.

## Limits

Checksums verify local byte integrity. They are not signed proof of county authenticity.
An empty database must trust the operator-selected local evidence.
Offset pages do not establish a transactionally consistent county snapshot.
Original coordinates and CRS declarations remain in the saved responses.
Source JSON numbers remain exact in those files.
Working geometry uses EPSG:3857. Esri 102100 and EPSG:3857 are accepted as equivalent Web Mercator codes.
Other or missing CRS declarations stop the import.

See [parcel verification](parcel-evidence-verification.md) for the completed checks and remaining release limits.
