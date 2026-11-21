# Unmatched tax PIN review

A PIN is the county parcel identifier. An unmatched PIN has no exact match in the stored Union GIS parcels when the tax file is published. It is not a duplicate or a personal-property skip.

## Stored evidence

All four tax feeds save unmatched PINs in the same transaction as the file publication. A failed transaction saves no findings. Each import keeps one finding per normalized PIN and tax year. Foreclosure feeds have no tax year; the API returns `null`.

Findings include the import, capture, source, PIN, year, reason, and observation time. A capture identifies the retained input. The capture also identifies the job attempt. Direct storage calls without a capture return null capture and attempt IDs. Findings contain no owner names, addresses, balances, or private file paths.

Historical findings remain attached to their import. Repeated storage of the same import does not create duplicate findings. `matched_now` reports whether a stored parcel now has the PIN. It does not prove that tax data has been linked or that the county PIN is correct.

## Read API

Use the existing authenticated API:

```text
GET /api/v1/tax/unmatched?source=union_billing
```

Supported sources are `union_delinquent`, `union_billing`, `union_foreclosure`, and `union_foreclosure_pipeline`. Without `import_id`, the API selects the active import. The response contains its ID, source URL, file import time, records, and `next_after`.

Use the returned import ID for all later pages:

```text
GET /api/v1/tax/unmatched?source=union_billing&import_id=<uuid>&after=<next_after>&limit=100
```

The default page size is 100. The maximum is 500. `next_after` is null on the last page. A missing import returns 404. An import from another source also returns 404.

Use the PIN to check a county data source of your choice. The stored source URL is provenance only. The API does not contact that source.

## Limits

This change does not add a browser review page or change PIN normalization. It does not reconstruct findings for imports completed before this migration. An unchanged observation can avoid publication, so merely repeating an old file does not guarantee historical findings. Fresh imports record findings. A volume reset removes findings with the rest of the database.
