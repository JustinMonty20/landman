# Union County elevation and slope ingestion

## Outcome and scope

Give the buyer a slope layer for comparing proposed lots in the [subdivision sketch](parcel-subdivision-sketching.md). Use terrain as screening evidence. Do not use it to certify buildability, infer grading costs, or declare a proposed lot compliant. Keep this work separate from [required V1 qualification evidence](v1.md). Do not change FEMA, easement, soil, or zoning ingestion in this work. This plan does not authorize implementation or a full county import.

Use branch `plan/union-terrain-ingestion` and PR title `[plan:union-terrain-ingestion] <summary>`.

## T1. Source and verified read path

Use the [USGS National Map products API](https://tnmaccess.nationalmap.gov/api/v1/products). Query `datasets=Digital Elevation Model (DEM) 1 meter` with a longitude/latitude bounding box in west, south, east, north order. Read the tile ID, project, revision dates, format, size, footprint, and download URL from the response. A digital elevation model (DEM) is a grid of ground heights. A Cloud Optimized GeoTIFF (COG) is a tiled raster file that supports reads of selected byte ranges.

The probe requested `bbox=-80.555,34.979,-80.547,34.987`. The API returned a [337 MB North Carolina tile](https://prd-tnm.s3.amazonaws.com/StagedProducts/Elevation/1m/Projects/NC_Phase_4_CentralWestNC_GEIGER_A16/TIFF/USGS_1M_17_x54y388_NC_Phase_4_CentralWestNC_GEIGER_A16.tif). A request for `bytes=0-16383` returned HTTP `206` and `Content-Range: bytes 0-16383/336825767`. Rasterio, a Python library that uses GDAL to read raster data, opened the URL directly. The probe converted longitude and latitude to the tile coordinate system, read a masked `64 × 64` pixel window, and calculated percent grade from adjacent samples. It did not download the full file. The tile has 1-meter pixels in NAD83 / UTM zone 17N (EPSG:26917). The source metadata specifies bare-earth heights in meters relative to NAVD88. The tested tile marks missing cells with `-999999`.

The follow-up probe used `bbox=-80.85,34.81,-80.27,35.21` and the stored county boundary from the active zoning generation. The API returned 40 of 40 records. Actual raster footprints from 37 records intersected the county:

| Check | Observed result |
| --- | --- |
| Primary project | The 27 `NC_Phase_4_CentralWestNC_GEIGER_A16` footprints covered the entire stored county boundary. |
| Coarse missing-data check | The primary project had data in all 1,617,002 county sample cells on the 32-meter grid. This is not a native-resolution completeness check. |
| Native tile edges | All 215 sampled windows across 43 adjacent primary-project tile edges had valid data. Shared heights differed by up to 0.034 m. |
| Cross-project overlap | One native-resolution overlap sample differed by up to 0.84 m. A newer publication date does not by itself make another project a better source. |

The native elevation scan checked all 27 primary tiles. It counted 1,655,820,110 cell centers inside the stored boundary. Two adjacent cells in `x53y388` contained NoData. A separate read confirmed both. The overview did not detect them.

The countywide slope prototype used the same 27 tiles. It produced valid slope for 1,655,820,098 cells. The two missing heights caused 12 slope cells to be unknown because their 3 × 3 windows contained a missing height. No unknown cells occurred at nominal tile edges inside the county. The flat, 50% grade, and missing-data fixtures passed. The reread of the output COG matched all calculated cells. The 7.82 GB file took about 7 minutes to stage, generate, and check in this local run. This test does not verify field accuracy or production import performance.

## T2. Capture and publish one terrain release

- Add an explicitly registered `union.terrain` job. `cmd/landman` configures it. The terrain job owns USGS discovery, raster validation, slope generation, and publication. Use `jobs.Runner.Run` for both `landman run union.terrain` and the existing `jobs.Scheduler` dispatch. Use an independent, optional `UNION_TERRAIN_CRON` with `CRON_TZ` and five calendar fields. Do not change other schedules or add terrain to the required default import. Check the catalog without downloading unchanged tiles. Select job time, request, disk, and memory limits from measured runs before enabling a schedule.
- Use a separate USGS request budget. Do not hold the county GIS host lock. Page the catalog and reconcile record counts before selection. Start with the verified North Carolina project. Pin tile revisions in a release manifest. Use each tile's nominal grid cell for ownership and the shared edge pixels for validation. Do not mix acquisition projects automatically. Review new projects and changed coverage explicitly. Keep gaps visible as unknown.
- Reuse the verified capture and publication contract in `internal/ingest` for terrain when implementation is authorized. Retain exact catalog responses, selected GeoTIFF bytes, metadata, and checksums through the existing artifact store. A range read verifies access, not a replayable capture. Ingest complete selected files with bounded staging. Reject invalid ranges, size mismatches, and changed validators during a transfer. Keep source identity, acquisition and publication dates when available, retrieval time, raster grid, and the NoData mask (the cells without valid heights).
- Validate raster headers, units, grid alignment, and native coverage. Assign each elevation cell to its nominal tile on a common EPSG:26917 grid. Read one extra cell across tile edges and outside the county boundary before clipping the result to the county. Do not average overlapping heights or fill missing heights. Compute Horn percent grade from a 3 × 3 window: use weighted east–west and north–south height differences, then multiply the gradient magnitude by 100. If any of the nine elevations is missing, mark the slope cell unknown. Store slope as a versioned Cloud Optimized GeoTIFF (COG), or as aligned COG tiles with a shared manifest. Reuse the result when the source hashes and slope method match.
- Publish the release manifest and stored slope references in one transaction only after validation. A failed or interrupted run must leave the prior release active. Store source-check status separately from the active release. Support offline `landman replay union.terrain --snapshot UUID` through the same runner. Verify retained bytes before processing. An older replay must not replace a newer release or advance its observation time.

## T3. Serve terrain to the sketch tool

Serve stored slope data through the existing API and map. Do not contact USGS when the buyer moves a cut line. Show slope bands, source date, and missing coverage. Calculate acreage by band within each proposed lot from the analysis grid, not from display tiles. Keep any later potential-building-area calculation separate. Use explicit screening rules, show the largest connected area that meets them, and do not count unknown ground as suitable. Keep flood, easement, and access evidence separate from slope.

## T4. Completion checks

- [ ] Local HTTP and raster fixtures verify catalog paging, project selection, partial or changed downloads, native missing cells, and mismatched grids or units. Routine tests do not contact USGS.
- [ ] Known flat and inclined grids verify percent grade. A tile-edge fixture verifies neighbor reads, deterministic tile selection, and explicit missing-data results. Measure the effect of height differences at seams before choosing a seam policy.
- [ ] A disposable PostGIS database verifies manual and scheduled dispatch, overlap exclusion, unchanged releases, failed or canceled publication, and offline replay. Use the [review verification procedure](../docs/review-verification.md).
- [ ] An approved county-scale run verifies native 1-meter coverage and records download bytes, peak disk and memory, elapsed time, and stored-query latency. Do not treat the overview probe as release evidence. Confirm attribution and redistribution terms before public delivery.
- [ ] The sketch shows slope and unknown coverage separately. Per-lot results update from stored data after a line moves and do not imply subdivision approval.
