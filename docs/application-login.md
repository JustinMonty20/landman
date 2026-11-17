# Local application login

## Scope

The private alpha uses Google sign-in. Only accounts in `AUTH_ALLOWED_EMAILS` can create or use an application session. Google test-user settings do not replace this application restriction.

The Go application API is in `cmd/api/`. Authentication is in `internal/auth/`. The SvelteKit UI is in `ui/`. The existing ingestion command remains separate in the repository root.

Calendar permissions, saved leads, private notes, and reminder delivery are not implemented. The map still uses fictional sample parcels. Authentication establishes user identities; it does not make the sample data real.

## Configure

Keep these settings in the repository-root `.env`:

```dotenv
GOOGLE_CLIENT_ID=your-client-id
GOOGLE_CLIENT_SECRET=your-client-secret
GOOGLE_REDIRECT_URI=http://127.0.0.1:5173/auth/google/callback
AUTH_ALLOWED_EMAILS=first@example.test,second@example.test
```

Keep the secret out of Git, browser code, and `VITE_*` settings. Use plain email addresses separated by commas. The server normalizes their case. Restart the API after changing the allowed accounts.

In Google Cloud, select a Web application client. Use `http://127.0.0.1:5173` as the JavaScript origin and the exact callback URI above as the authorized redirect URI. Add the approved accounts as test users while the Google application is in Testing. Use `127.0.0.1` consistently rather than alternating with `localhost`.

## Database

Migration `db/migrations/000019_auth.sql` adds:

- `app_users`: a stable UUID and unique Google subject, with the last verified email and display name.
- `auth_login_attempts`: hashed state and nonce, a temporary PKCE verifier, and a ten-minute expiry.
- `auth_sessions`: hashed session tokens, the owning user UUID, and a seven-day expiry. Deleting a user deletes that user's sessions.

PKCE means Proof Key for Code Exchange. It binds the Google authorization code to the application that started login.

Apply pending migrations only after reviewing them:

```sh
make -s migrate-status
make -s migrate-up
```

Use `-s` to suppress Make recipe output that can contain database connection settings. The API checks for authentication tables at startup. It does not apply migrations automatically. The login migration does not change parcel or tax records.

Expired attempts and sessions are rejected immediately. Their database rows are removed on a later login start. User records remain until an administrator deletes them. There is no account-deletion screen yet.

## Run

Start the API from the repository root:

```sh
make -s api
```

Start the UI in another terminal:

```sh
cd ui
npm run dev
```

Open `http://127.0.0.1:5173/`. The UI proxies `/auth` and `/api` to `127.0.0.1:8080`, preserving the browser origin. Use the UI URL, not port 8080, for sign-in. The API accepts only the configured application host. Its listener is loopback-only.

Select **Sign in with Google**. A successful login opens parcel discovery. An unapproved account remains signed out. Select **Sign out** to revoke the current session. A service failure does not permit access to discovery.

## Security contract

- Verify Google's token signature, issuer, audience, expiry, nonce, nonempty subject, verified email, and authorized party when supplied.
- Require a matching HttpOnly state cookie and a single-use, unexpired database login attempt. Use PKCE for code exchange.
- Request only `openid`, `email`, and `profile`. Do not request offline or calendar access. Do not store Google access, refresh, or identity tokens.
- Store only a SHA-256 hash of each random application session token. Send the token in an HttpOnly, SameSite=Lax cookie. Use Secure cookies and the `__Host-` session-cookie prefix with HTTPS. Permit HTTP only for loopback development.
- Recheck the allowed-account list on every authenticated profile request. Removing an account and restarting the API blocks its existing sessions.
- Keep session identity on the server. Never use an email or user ID supplied by the browser as proof of identity. Future private tables must reference `app_users.id` and enforce ownership in their API operations.
- Accept logout only by POST from the exact configured origin. Rotate the browser's prior session after a successful login. Return no-store headers on authentication responses. Keep tokens and provider error details out of logs and URLs returned to the UI.

A cloud runtime and deployment configuration are not selected. The local proxy is not a production deployment design.

## Verification

```sh
go test -race ./...
go vet ./...
go build ./...
cd ui
npm run check
npm test
npm run test:e2e
```

Database tests require `AUTH_TEST_DATABASE_URL` in the shell. It must name a separate database whose name starts with `landman_auth_test_`. Migrate that database before running:

```sh
make -s migrate-up DATABASE_URL="$AUTH_TEST_DATABASE_URL"
go test -race -tags postgres ./internal/auth -count=1
```

The database tests clear only authentication tables in that explicitly disposable database. They use a local signed-token provider, not Google. They verify separate user identities, stable identity, restart persistence, token hashing, session rotation, expiry, revocation, allowlist removal, rejected claims and signatures, callback replay, origin checks, and migration rollback.

Browser tests use explicit API fixtures. They verify the signed-out screen, provider errors, unavailable service, logout, and the approved discovery layout. They do not bypass authentication in the application build. A real Google consent and callback check still requires an operator-approved account in a browser.
