# County id package

**Out-of-scope decision.** Did not edit `plans/scraper-enrichment.md`. PR 11 owns that file. Appendix C says scrape-1 replaces `county.CountyID` with `county.ID` after this package exists. That edit belongs to the scrape plan.

**Out-of-scope decision.** Left `models.ParcelIdentity.CountyID` as `string` and did not edit the Union translator. Appendix B defers branding that field. Changing it now would touch the translator while the soil and scraper plans are still open.

**Skipped.** Did not add `pstack/`, a `/loop` audit, a swarm, Bugbot triage, or `/deslop`. Appendix C records that this repo has none of that machinery. Inventing it would be a different change.

**Out-of-scope decision.** Did not edit `000001_init_schema.sql` or `000002_parcels_storage_srid.sql`. The parcels check is a new goose file so those migrations stay immutable. The next prefix on main was `000003`.

**Out-of-scope decision.** Left the Postgres lanes for the operator's machine. This cloud VM cannot reach that database, and the operator will apply the migration locally after the other checks pass. The new goose file is still the parcels check the plan specifies.
