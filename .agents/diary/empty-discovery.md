# Empty discovery default

- **Skipped refactor:** Keep the existing finder suggestion controls for this change. A switch to Skeleton's Combobox needs a separate keyboard and accessibility check. Limit this change to direct suggestion selection and filter-driven matches.

- **Unrelated bug:** The prototype's in-view count tests parcel centers, not full boundary intersections. Verify full geometry against viewport bounds when the stored-data API replaces the sample map; do not extend this UI-state change into spatial-query work.
