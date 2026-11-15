# Ingestion gaps

- **Out-of-scope decision:** Defer cloud deployment, process supervision, and monitoring setup. The operator has not selected a cloud runtime. Local execution remains the development path; it does not prove unattended cloud operation.
- **Skipped refactor:** Keep ArcGIS offset pagination and absent-parcel retention unchanged. A controlled unchanged fixture proves repeatability, not a complete live snapshot; keyset paging and deletion detection require a separate source contract.
- **Out-of-scope decision:** Preserve the topology plan's treatment of self-touching source rings that PostGIS currently accepts. This change quarantines missing or unreadable rings and invalid constrained values; it does not add geometry repair, boundary changes, or a broader topology rejection policy.
