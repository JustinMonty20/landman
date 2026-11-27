# Verification skill maintenance

- **Out-of-scope decision:** Keep product code and application documents unchanged. This task changes verification skills only; `ui/README.md` has older parcel-availability claims that need a separate source audit.
- **Unrelated code smell:** `unionExecutor.qualify` is configured but is not called by its source-success path. Commit `bb3da92` explicitly separated qualification from source imports. Keep that behavior; consider removal of the unused field in a separate refactor.
- **Out-of-scope decision:** The first full private-record test package also ran its 1,000-favorite measurement fixture. Keep that measurement as an explicit optional check. The maintained default selects the five behavior tests instead.
- **Skipped refactor:** Each database setup compiles the Dockerized goose tool again. Keep the existing setup model in this pass. A shared compiler cache or a prebuilt tool can reduce later verification time.
