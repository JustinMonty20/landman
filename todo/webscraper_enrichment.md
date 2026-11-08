# Web scraper enrichment

Replaced by [plans/scraper-enrichment.md](../plans/scraper-enrichment.md).

The scraper is a Union package with a small interface defined in package `union`, the package that calls it. It does not implement `connector.SingleParcelFetcher`, and it does not mirror `SpatialistEnricher`. Those types are removed in [plans/spatialist-removal-and-critical-fixes.md](../plans/spatialist-removal-and-critical-fixes.md).

Source: [Go architecture audit, revised](https://cursor.com/canvas/store-11be7bad-acc0-4616-92d5-8d428c775323/99b2a528-396b-4acf-9126-69a9bf84bab3).
