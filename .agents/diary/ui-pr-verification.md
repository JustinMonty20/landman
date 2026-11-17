# UI PR verification

- **Out-of-scope decision:** Keep the existing tax-test database guards unchanged. Use a temporary PostGIS container on a nondefault loopback port for local checks instead of weakening the guards or sharing application data.

- **Skipped refactor:** The installed reviewer profile lacks the shell tool required by repository review checks. Leave the external agent configuration unchanged. Use a checked profile with read and shell tools, with explicit read-only review instructions.
