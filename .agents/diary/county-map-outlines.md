# County map outlines

- **Out-of-scope decision:** The grouped filter panel is a design preview, as requested. Keep parcel queries and county soil-option loading outside this change; show an unloaded soil dropdown instead of invented choices.

- **Skipped refactor:** Retain `ui/src/lib/discovery.ts` and its unit tests. The county page does not import its fictional parcel fixtures; removal of the existing filter model is outside this display change.

- **Out-of-scope decision:** Keep the Union parcel API connection outside this change. The operator selected county outlines only, without fictional parcels or new ingestion jobs.

- **Unrelated code smell:** `npm ci` reports eight high-severity dependency findings. Dependency versions are unchanged; dependency updates are outside this county display change.
