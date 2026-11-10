# Stream vacant parcel IDs

`./landman` streams parcel IDs from Union County GIS, keeps the ones that look like vacant land, and prints them in batches. It does not save anything to Postgres.

## Sub-features

- `stream-build` produces the `./landman` binary.
- `stream-start` prints that streaming has begun.
- `stream-count` reports how many parcels the county layer will return.
- `stream-batch` prints each non-empty vacant-ID batch and a small sample of IDs.
- `stream-complete` prints the total vacant ID count and exits 0.

## How to get to it (user POV)

- From the repo root, run `make run`.
- From the repo root, run `make build` and then `./landman`.

## Driving it with make and the landman binary

Preconditions:

- `make build` has exited 0 and `./landman` is executable.
- CLI doctor succeeded: the Union County MapServer layer 215 metadata URL returned HTTP 200.
- No other `./landman` stream drive is running against the live county endpoint.

- **Build.** Run `make build`. Exit code `0` and `./landman` exists.
- **Start stream.** Run `./landman`, capturing stdout, stderr, and the exit code under `artifacts/verify/$RUN_ID/`. Stdout contains `Streaming parcel IDs from Union County GIS...`.
- **See the layer size.** Stderr contains `[union_county_gis] Total parcels to fetch: <n>` with `<n>` greater than 0, then `[union_county_gis] Batch size: 500, Total batches: <m>`.
- **See vacant ID batches.** Stdout contains one or more lines matching `Batch <n>: <count> parcel IDs`. A line may include `(sample: [<id> ...])` with up to three IDs. `<n>` is the upstream GIS batch number (1-based). Batches with no vacant IDs produce no stdout line, so printed batch numbers can skip.
- **Finish.** The process exits 0. Stdout contains `Total likely-vacant parcel IDs streamed: <n>`. Stderr contains `[union_county_gis] Fetch complete:`.
- **Proof.** Keep `stream.stdout.txt`, `stream.stderr.txt`, and `stream.exit.txt`. The stdout file must show the start line, at least one batch line (or a documented empty total of `0` if the layer returned no vacant IDs), and the total line. The exit file must contain `0`. The stderr file must show the total-parcels line. The stdout file must not show database insert messages; this path does not write rows.

## Gotchas

- This drive hits the live county ArcGIS service. There is no dry-run flag. Do not run two stream drives at once.
- The process deadline is 10 minutes, and the client sends about one request per second. Stderr prints `Batch size: 500`. If the deadline fires first, stderr contains `Failed to stream parcel IDs:` followed by `context deadline exceeded` or `rate: Wait(n=1) would exceed context deadline`. That is a failed drive. Do not stop after the first batch and call it verified.
- Each county request uses a 30-second client timeout and waits 15 seconds for response headers. A stalled request fails the drive while the 10-minute deadline is still running.
- If a page returns the wrong row count, the drive fails. The last page may be shorter than 500 when that shorter count is the remainder, and that page still succeeds. Any other short page fails. Stderr then contains `Failed to stream parcel IDs:` and `count mismatch: page <n> got <x> want <y>`. The page number in that error is 0-based. `Batch <i>/<total> complete` is 1-based.
- `make run` rebuilds before it executes. If you already built, `./landman` is the more precise drive.
- `Batch <n>` on stdout is the GIS batch index, not a count of printed lines. Empty vacant filters skip the stdout line but still increment that index.
- Fatal errors go to stderr as `Failed to create GIS source:` or `Failed to stream parcel IDs:` and the process exits non-zero. Do not treat a timeout or HTTP error as success.
- Success does not create or update rows in `parcels` or `parcel_sales`. Checking those tables cannot prove this feature.
