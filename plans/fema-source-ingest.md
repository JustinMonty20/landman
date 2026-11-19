# FEMA source ingestion for Union County

Use the official FEMA Map Service Center catalog for Union County study `37179C`. Use the National Flood Hazard Layer (NFHL) county archive. Do not use the county layer from 2014 as a fallback.

## Source setup

- Select one matching product from the effective county collection. Preserve the product ID, file name, issue date, effective date, and catalog response.
- Limit the response sizes. Reject unexpected URLs, redirects outside the approved source, and partial downloads. Retain the exact response bytes through the shared ingestion recorder.
- Use local HTTP fixtures for tests. Check a public catalog response without publishing data.

The latest catalog probe selected `NFHL_37179C`, product ID `284041519`, and file `37179C_20260914.zip`. The product issue date is September 14, 2026. The study effective date is November 16, 2018. The catalog describes the product as `01/28/2025`. Keep these dates separate. The archive server reset a HEAD request but returned HTTP 206 for a 512-byte range request. These probes do not verify the complete archive or its coverage.

## Manual import

Apply migrations through `000036_fema_releases.sql`. Set `DATABASE_URL` and these paths:

- `UNION_FEMA_CATALOG_PATH`: saved official catalog JSON for the effective county product.
- `UNION_FEMA_ARCHIVE_PATH`: saved official ZIP. Keep the file name from the catalog.
- `UNION_FEMA_ARTIFACT_DIR`: private local directory for the retained files.

Run `landman run union.fema`. The command does not fetch data. The catalog confirms which archive FEMA listed, but does not authenticate the supplied ZIP bytes. Use files obtained from the official source. Local disk is the only backend selected by this command. No FEMA schedule is registered.

Run `landman replay union.fema --snapshot UUID` to verify and process a registered capture. Use the UUID from `ingest_captures` for source `fema_union`. Replay needs the database and artifact directory, not the original input paths. It cannot replace a newer release or advance its observation time.

## Publication and replay

- Run the job through `jobs.Runner` with a stable source ID. Keep FEMA separate from the county request budget.
- Use `ingest.Engine` to register captures and verify bytes before preparation or offline replay. Decode typed zones and panels from the verified archive. Check the ZIP checksums, required files, study identity, and feature counts. Preserve original spatial references.
- Publish zones, panels, and coverage in one transaction. An identical current archive records an observation without replacing the release. A failure keeps the previous release. A panel gap is unknown, not proof of low flood risk.
- Source shapes declare NAD83 (EPSG:4269). The publisher rejects empty or invalid repair results and results whose area differs by more than one square meter. Keep the exact ZIP and record the repair and area change separately.
- Development checks used the saved official ZIP. The manual import published 1,551 zones, 152 panels, and valid coverage geometries in disposable PostGIS. Offline replay succeeded without the original file paths.
- Before release, verify catalog behavior across product revisions, archive identity, geometry repair limits, coverage gaps, and redistribution terms. Treat gaps as unknown.

Do not add encumbrance ingestion or change zoning and roads in this work. See [FEMA flood evidence and parcel encumbrances](fema-parcel-encumbrances.md) for the V1 release checks.
