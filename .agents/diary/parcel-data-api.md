# Parcel data API

- [Out-of-scope decision] Do not add a parcel publication counter or change ingestion in this read-only API task. Use response snapshots and explicit source stamps. Document that the stamp does not retain a snapshot across requests.
- [Scope tangent] County-wide map counts still require a county-wide spatial query. Revisit source-versioned aggregates or vector tiles only if agreed UI limits require them. Do not add another data service in this change.
