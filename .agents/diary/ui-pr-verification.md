# UI PR verification

- **Out-of-scope decision:** Keep the existing tax-test database guards unchanged. Use a temporary PostGIS container on a nondefault loopback port for local checks instead of weakening the guards or sharing application data.

- **Skipped refactor:** The installed reviewer profile lacks the shell tool required by repository review checks. Leave the external agent configuration unchanged. Use a checked profile with read and shell tools, with explicit read-only review instructions.

- **Out-of-scope decision:** Defer the review's low-priority pending-login replacement correction to a separate change. The login cookie path excludes the start endpoint, so the prior attempt remains until expiry and later cleanup. The old callback still fails the cookie check. Draft publication does not require this correction. Use a shared login-cookie path and a browser cookie-jar regression test in the follow-up.
