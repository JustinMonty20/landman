# Google login

- **Unrelated bug:** The existing Make migration recipes can print database connection settings when Make echoes a command. Use `make -s` in the login instructions and execution. A broader change to the existing migration recipes is outside login.

- **Unrelated bug:** `npm audit` now reports a high-severity `picomatch` advisory through the UI build tools. The report lists all versions as affected and suggests a breaking Vite downgrade. Keep that dependency change separate from login; do not force an incompatible toolchain change. The authentication, database, and browser behavior checks pass independently of this audit result. During the later security investigation, the original upstream source passed 1,983 tests, but the first local guard failed 45 compatibility tests. Restore the installed original source; do not ship that rejected candidate. A verified dependency fix remains a release blocker.

- **Out-of-scope decision:** The application database has migrations 15–18 pending before the new authentication migration. Stop before applying them because they change job history and add tax tables outside login. The operator then approved migrations 15–19. They were applied in order, and the authentication tables were verified.
