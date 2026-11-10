# GIS page size

## Out-of-scope decision

`main` passes 50 to `NewParcelIDImportService`. On this path the importer calls `FetchBatches` and never uses that value. The GIS `BatchSize` is what becomes `resultRecordCount`. Left the importer argument alone.

## Out-of-scope decision

`plans/soil-layer.md` says `DefaultGISSourceConfig.BatchSize` stays 50 so the default test still locks it, and that a future import should page at 2000. This change sets the default to 500 because that is the measured page Justin asked for. The plan file stays untouched.

## Out-of-scope decision

The page-size sketch that adds `featurePage`, `layerParams`, and a hardcoded `orderByFields` inside `ToURLValues` lost. The field on `ArcGISParams` is the smaller edit, and a later query can name a different column. The encoder does not reject a page that forgets the field. `FetchBatches` is the only production writer of `OBJECTID`. A negative count check and `exceededTransferLimit` parsing stay out. The per-page length check stays because a short page is a few lines in `FetchBatches`.

## Out-of-scope decision

`FetchBatches` now fails when a page length disagrees with the count. It still does not read `exceededTransferLimit`. That flag would be a second signal for the same short page.

## Missed skill

`control-cli` and `deslop` from cursor-team-kit are not installed. The CLI was driven with `go run` and `go build`. The diff was read against the unslop rules by hand.
