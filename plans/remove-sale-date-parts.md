# Remove redundant sale date-part columns

## Context
- User noted `sale_year`, `sale_month`, and `sale_day` are unnecessary when normalized `sale_date DATE` is present.
- The initial migration is still being audited before use, so edit `000001_init_schema.up.sql` directly rather than adding a corrective migration.

## Approach
- Keep `parcel_sales.sale_date DATE` as the canonical normalized sale date.
- Keep `parcel_sales.og_sale_date BIGINT` as the original ArcGIS epoch-milliseconds value for traceability.
- Remove redundant `sale_year`, `sale_month`, and `sale_day` columns and their check constraints from the SQL migration.

## Files to modify
- `db/migrations/000001_init_schema.up.sql`

## Steps
- [x] Remove `sale_year`, `sale_month`, and `sale_day` columns from `parcel_sales`.
- [x] Remove their check constraints.
- [x] Run `go test ./...`.

## Verification
- `go test ./...`
