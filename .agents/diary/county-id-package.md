# County id package

**Out-of-scope decision.** Did not edit `plans/scraper-enrichment.md`. PR 11 owns that file on `cursor/scraper-enrichment-plan-1e02`. Appendix C of `plans/county-id-package.md` states the scrape-1 edit.

**Out-of-scope decision.** county-1 does not change `models.ParcelIdentity.CountyID` from `string`. Branding that field edits the Union translator while the soil and scraper plans are in flight. The leaf package removes the backwards import without that edit.

**Out-of-scope decision.** Did not change `.github/scripts/check-pr-naming.sh`. The task requires a `cursor/` branch. The script expects `plan/county-id-package`. The naming check fails on the branch. A workflow edit is config, and this PR is docs only.

**Skipped.** Did not add `pstack/` or a `control-cli` skill. Both are absent. Appendix C records the fallback. Putting them in tree is a different PR.
