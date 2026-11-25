//go:build postgres

package auth

import (
	"context"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/go-jose/go-jose/v4"
	"github.com/jackc/pgx/v5/pgxpool"
)

const testOrigin = "http://127.0.0.1:5173"

type grant struct {
	claims    map[string]any
	challenge string
}
type fixture struct {
	handler   *server
	pool      *pgxpool.Pool
	issuer    *httptest.Server
	signer    jose.Signer
	mu        sync.Mutex
	grants    map[string]grant
	exchanges int
}

func setup(t *testing.T) *fixture {
	t.Helper()
	dsn := os.Getenv("AUTH_TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("AUTH_TEST_DATABASE_URL must name a disposable landman_auth_test_ database")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	var database string
	if err := pool.QueryRow(context.Background(), "SELECT current_database()").Scan(&database); err != nil || !strings.HasPrefix(database, "landman_auth_test_") {
		t.Fatal("refusing a database without the landman_auth_test_ prefix")
	}
	if err := CheckSchema(context.Background(), pool); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(context.Background(), "TRUNCATE auth_sessions,auth_login_attempts,app_users CASCADE"); err != nil {
		t.Fatal(err)
	}
	key, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := jose.NewSigner(jose.SigningKey{Algorithm: jose.RS256, Key: key}, (&jose.SignerOptions{}).WithType("JWT").WithHeader("kid", "fixture"))
	if err != nil {
		t.Fatal(err)
	}
	f := &fixture{pool: pool, signer: signer, grants: make(map[string]grant)}
	mux := http.NewServeMux()
	f.issuer = httptest.NewServer(mux)
	t.Cleanup(f.issuer.Close)
	mux.HandleFunc("GET /.well-known/openid-configuration", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(map[string]any{"issuer": f.issuer.URL, "authorization_endpoint": f.issuer.URL + "/authorize", "token_endpoint": f.issuer.URL + "/token", "jwks_uri": f.issuer.URL + "/keys", "id_token_signing_alg_values_supported": []string{"RS256"}})
	})
	mux.HandleFunc("GET /keys", func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode(jose.JSONWebKeySet{Keys: []jose.JSONWebKey{{Key: &key.PublicKey, KeyID: "fixture", Algorithm: "RS256", Use: "sig"}}})
	})
	mux.HandleFunc("POST /token", func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		f.mu.Lock()
		g, ok := f.grants[r.Form.Get("code")]
		delete(f.grants, r.Form.Get("code"))
		f.exchanges++
		f.mu.Unlock()
		sum := sha256.Sum256([]byte(r.Form.Get("code_verifier")))
		id, secret, basic := r.BasicAuth()
		if !ok || !basic || id != "fixture-client" || secret != "fixture-secret" || r.Form.Get("redirect_uri") != testOrigin+"/auth/google/callback" || base64.RawURLEncoding.EncodeToString(sum[:]) != g.challenge {
			http.Error(w, "invalid grant", 400)
			return
		}
		body, _ := json.Marshal(g.claims)
		signed, err := f.signer.Sign(body)
		if err != nil {
			http.Error(w, "signing failed", 500)
			return
		}
		raw, err := signed.CompactSerialize()
		if err != nil {
			http.Error(w, "serialization failed", 500)
			return
		}
		if bad, _ := g.claims["fixture_bad_signature"].(bool); bad {
			parts := strings.Split(raw, ".")
			first := "A"
			if strings.HasPrefix(parts[2], first) {
				first = "B"
			}
			parts[2] = first + parts[2][1:]
			raw = strings.Join(parts, ".")
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"access_token": "fixture-access-token", "token_type": "Bearer", "expires_in": 3600, "id_token": raw})
	})
	config := Config{ClientID: "fixture-client", ClientSecret: "fixture-secret", RedirectURI: testOrigin + "/auth/google/callback", AllowedEmails: []string{"buyer@example.test", "second@example.test"}}
	f.handler, err = newServer(context.Background(), pool, config, f.issuer.URL)
	if err != nil {
		t.Fatal(err)
	}
	return f
}

func (f *fixture) request(method, path string, cookies ...*http.Cookie) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, testOrigin+path, nil)
	for _, cookie := range cookies {
		r.AddCookie(cookie)
	}
	if method == "POST" {
		r.Header.Set("Origin", testOrigin)
	}
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	return w
}

func responseCookie(t *testing.T, w *httptest.ResponseRecorder, name string) *http.Cookie {
	t.Helper()
	for _, cookie := range w.Result().Cookies() {
		if cookie.Name == name {
			return cookie
		}
	}
	t.Fatalf("missing cookie %s; status %d", name, w.Code)
	return nil
}

func (f *fixture) attempt(t *testing.T, overrides map[string]any) (string, *http.Cookie, string) {
	t.Helper()
	start := f.request("GET", "/auth/google/start")
	if start.Code != 302 {
		t.Fatalf("start status %d: %s", start.Code, start.Body.String())
	}
	target, err := url.Parse(start.Header().Get("Location"))
	if err != nil {
		t.Fatal(err)
	}
	q := target.Query()
	if q.Get("scope") != "openid email profile" || q.Get("code_challenge_method") != "S256" || q.Get("access_type") == "offline" {
		t.Fatal("incorrect scopes or PKCE")
	}
	state := q.Get("state")
	cookie := responseCookie(t, start, "landman_login")
	if cookie.Value != state || !cookie.HttpOnly || cookie.SameSite != http.SameSiteLaxMode || cookie.MaxAge != 600 {
		t.Fatal("unsafe login cookie")
	}
	claims := map[string]any{"iss": f.issuer.URL, "aud": "fixture-client", "sub": "buyer-subject", "exp": time.Now().Add(time.Hour).Unix(), "iat": time.Now().Unix(), "nonce": q.Get("nonce"), "email": "buyer@example.test", "email_verified": true, "name": "Buyer"}
	for key, value := range overrides {
		claims[key] = value
	}
	code := randomToken()
	f.mu.Lock()
	f.grants[code] = grant{claims: claims, challenge: q.Get("code_challenge")}
	f.mu.Unlock()
	return state, cookie, code
}

func (f *fixture) login(t *testing.T, overrides map[string]any) *http.Cookie {
	t.Helper()
	state, cookie, code := f.attempt(t, overrides)
	callback := f.request("GET", "/auth/google/callback?state="+state+"&code="+code, cookie)
	if callback.Code != 303 || callback.Header().Get("Location") != "/" {
		t.Fatalf("login failed: %d %s", callback.Code, callback.Header().Get("Location"))
	}
	session := responseCookie(t, callback, "landman_session")
	if !session.HttpOnly || session.Secure || session.SameSite != http.SameSiteLaxMode || session.Path != "/" || session.MaxAge != int(sessionAge.Seconds()) {
		t.Fatal("unsafe local session cookie")
	}
	return session
}

func TestProfilePhotoClaimAndSession(t *testing.T) {
	f := setup(t)
	first := f.login(t, map[string]any{"picture": "https://lh3.googleusercontent.com/a/fixture"})
	var user User
	if err := json.Unmarshal(f.request("GET", "/api/me", first).Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	if user.ProfileImageURL != "https://lh3.googleusercontent.com/a/fixture" {
		t.Fatalf("verified picture missing: %+v", user)
	}
	other := f.login(t, map[string]any{"sub": "other-subject", "email": "second@example.test"})
	var isolated User
	json.Unmarshal(f.request("GET", "/api/me", other).Body.Bytes(), &isolated)
	if isolated.ProfileImageURL != "" {
		t.Fatal("another user's photo leaked")
	}
	updated := f.login(t, map[string]any{"picture": "javascript:alert(1)"})
	user = User{}
	json.Unmarshal(f.request("GET", "/api/me", updated).Body.Bytes(), &user)
	if user.ProfileImageURL != "" {
		t.Fatal("invalid updated photo persisted")
	}
	user = User{}
	json.Unmarshal(f.request("GET", "/api/me", first).Body.Bytes(), &user)
	if user.ProfileImageURL != "" {
		t.Fatal("current session did not see profile update")
	}
}
func TestCheckSchemaResolvedUsersColumn(t *testing.T) {
	f := setup(t)
	ctx := context.Background()
	schema := fmt.Sprintf("auth_photo_check_%d", time.Now().UnixNano())
	if _, err := f.pool.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = f.pool.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE") })
	if _, err := f.pool.Exec(ctx, "CREATE TABLE "+schema+".app_users AS SELECT id, google_subject, email, display_name, parcel_detail_fields FROM public.app_users WITH NO DATA"); err != nil {
		t.Fatal(err)
	}
	config := f.pool.Config()
	config.ConnConfig.RuntimeParams["search_path"] = schema + ",public"
	shadow, err := pgxpool.NewWithConfig(ctx, config)
	if err != nil {
		t.Fatal(err)
	}
	defer shadow.Close()
	if err := CheckSchema(ctx, shadow); err == nil || !strings.Contains(err.Error(), "migration 49") {
		t.Fatalf("migration 48 shadow should fail before serving auth: %v", err)
	}
	if _, err := f.pool.Exec(ctx, "ALTER TABLE "+schema+".app_users ADD COLUMN profile_image_url text"); err != nil {
		t.Fatal(err)
	}
	if err := CheckSchema(ctx, shadow); err != nil {
		t.Fatalf("restored migration 49 column rejected: %v", err)
	}
}

func TestOptionalPictureMetadata(t *testing.T) {
	f := setup(t)
	for _, picture := range []any{nil, 42, true, map[string]any{"url": "https://lh3.googleusercontent.com/a/fixture"}, []string{"https://lh3.googleusercontent.com/a/fixture"}} {
		session := f.login(t, map[string]any{"picture": picture})
		var user User
		if err := json.Unmarshal(f.request("GET", "/api/me", session).Body.Bytes(), &user); err != nil || user.ProfileImageURL != "" {
			t.Fatalf("optional picture %v blocked login or leaked: %+v %v", picture, user, err)
		}
	}
	f.login(t, map[string]any{"picture": "https://lh3.googleusercontent.com/a/fixture"})
	state, cookie, code := f.attempt(t, map[string]any{"email": 42, "picture": nil})
	callback := f.request("GET", "/auth/google/callback?state="+state+"&code="+code, cookie)
	if callback.Header().Get("Location") != "/?auth_error=invalid_login" {
		t.Fatalf("non-string identity accepted: %s", callback.Header().Get("Location"))
	}
}

func TestProfileImageURL(t *testing.T) {
	for _, raw := range []string{"", "http://lh3.googleusercontent.com/a", "https://lh3.googleusercontent.com.evil.test/a", "https://user@lh3.googleusercontent.com/a", "https://lh3.googleusercontent.com:443/a", "https://lh3.googleusercontent.com/a#secret", "https://lh3.googleusercontent.com/a?token=secret", "data:image/png;base64,foo", strings.Repeat("a", 2049)} {
		if got := profileImageURL(raw); got != "" {
			t.Errorf("accepted unsafe photo URL %q", raw)
		}
	}
	if got := profileImageURL("https://lh3.googleusercontent.com/a/fixture"); got == "" {
		t.Fatal("Google photo rejected")
	}
}
func TestLoginSessions(t *testing.T) {
	f := setup(t)
	first := f.login(t, nil)
	me := f.request("GET", "/api/me", first)
	if me.Code != 200 || !strings.Contains(me.Body.String(), "buyer@example.test") || me.Header().Get("Cache-Control") != "no-store" {
		t.Fatal("session was not authenticated")
	}
	var user User
	if err := json.Unmarshal(me.Body.Bytes(), &user); err != nil {
		t.Fatal(err)
	}
	var stored []byte
	if err := f.pool.QueryRow(context.Background(), "SELECT token_hash FROM auth_sessions WHERE user_id=$1", user.ID).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if string(stored) == first.Value || len(stored) != 32 {
		t.Fatal("raw token stored")
	}
	// A service restart must not discard the database session.
	config := Config{ClientID: "fixture-client", ClientSecret: "fixture-secret", RedirectURI: testOrigin + "/auth/google/callback", AllowedEmails: []string{"buyer@example.test", "second@example.test"}}
	restarted, err := newServer(context.Background(), f.pool, config, f.issuer.URL)
	if err != nil {
		t.Fatal(err)
	}
	f.handler = restarted
	if f.request("GET", "/api/me", first).Code != 200 {
		t.Fatal("session lost on restart")
	}
	second := f.login(t, map[string]any{"sub": "second-subject", "email": "second@example.test", "name": "Second"})
	other := f.request("GET", "/api/me", second)
	var otherUser User
	json.Unmarshal(other.Body.Bytes(), &otherUser)
	if other.Code != 200 || otherUser.ID == user.ID || otherUser.Email != "second@example.test" {
		t.Fatal("users were not isolated")
	}
	repeated := f.login(t, nil)
	repeatedMe := f.request("GET", "/api/me", repeated)
	var repeatedUser User
	json.Unmarshal(repeatedMe.Body.Bytes(), &repeatedUser)
	if repeatedUser.ID != user.ID {
		t.Fatal("Google subject did not preserve user identity")
	}
	logout := f.request("POST", "/auth/logout", first)
	if logout.Code != 204 || f.request("GET", "/api/me", first).Code != 401 || f.request("GET", "/api/me", second).Code != 200 {
		t.Fatal("logout did not revoke only the current session")
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE auth_sessions SET created_at=now()-interval '8 days',expires_at=now()-interval '1 day' WHERE token_hash=$1", digest(second.Value)); err != nil {
		t.Fatal(err)
	}
	if f.request("GET", "/api/me", second).Code != 401 {
		t.Fatal("expired session accepted")
	}
	state, loginCookie, code := f.attempt(t, nil)
	rotation := f.request("GET", "/auth/google/callback?state="+state+"&code="+code, loginCookie, repeated)
	rotated := responseCookie(t, rotation, "landman_session")
	if rotated.Value == repeated.Value || f.request("GET", "/api/me", repeated).Code != 401 || f.request("GET", "/api/me", rotated).Code != 200 {
		t.Fatal("successful login did not rotate the prior session")
	}
	repeated = rotated
	config.AllowedEmails = []string{"second@example.test"}
	f.handler, err = newServer(context.Background(), f.pool, config, f.issuer.URL)
	if err != nil {
		t.Fatal(err)
	}
	if f.request("GET", "/api/me", repeated).Code != 401 {
		t.Fatal("removed allowlist entry retained access")
	}
}

func TestLoginRejectsInvalidIdentity(t *testing.T) {
	f := setup(t)
	for _, item := range []struct {
		name   string
		claims map[string]any
	}{
		{"unapproved", map[string]any{"email": "outsider@example.test"}},
		{"unverified", map[string]any{"email_verified": false}},
		{"signature", map[string]any{"fixture_bad_signature": true}},
		{"nonce", map[string]any{"nonce": "wrong"}},
		{"audience", map[string]any{"aud": "wrong"}},
		{"issuer", map[string]any{"iss": "https://wrong.example.test"}},
		{"expired", map[string]any{"exp": time.Now().Add(-time.Hour).Unix()}},
		{"authorized party", map[string]any{"azp": "wrong"}},
		{"empty subject", map[string]any{"sub": ""}},
	} {
		t.Run(item.name, func(t *testing.T) {
			state, cookie, code := f.attempt(t, item.claims)
			result := f.request("GET", "/auth/google/callback?state="+state+"&code="+code, cookie)
			if result.Code != 303 || !strings.HasPrefix(result.Header().Get("Location"), "/?auth_error=") {
				t.Fatal("invalid identity accepted")
			}
			for _, c := range result.Result().Cookies() {
				if c.Name == "landman_session" && c.Value != "" {
					t.Fatal("session cookie issued on failure")
				}
			}
		})
	}
	var users, sessions int
	f.pool.QueryRow(context.Background(), "SELECT count(*) FROM app_users").Scan(&users)
	f.pool.QueryRow(context.Background(), "SELECT count(*) FROM auth_sessions").Scan(&sessions)
	if users != 0 || sessions != 0 {
		t.Fatal("failed identity created private records")
	}
}

func TestLoginStateAndLogoutBoundaries(t *testing.T) {
	f := setup(t)
	state, cookie, code := f.attempt(t, nil)
	path := "/auth/google/callback?state=" + state + "&code=" + code
	if result := f.request("GET", path); !strings.Contains(result.Header().Get("Location"), "invalid_login") {
		t.Fatal("missing state cookie accepted")
	}
	mismatch := *cookie
	mismatch.Value = randomToken()
	if result := f.request("GET", path, &mismatch); !strings.Contains(result.Header().Get("Location"), "invalid_login") {
		t.Fatal("mismatched state accepted")
	}
	success := f.request("GET", path, cookie)
	if success.Header().Get("Location") != "/" {
		t.Fatal("valid state rejected")
	}
	if replay := f.request("GET", path, cookie); !strings.Contains(replay.Header().Get("Location"), "invalid_login") {
		t.Fatal("state replay accepted")
	}
	session := responseCookie(t, success, "landman_session")
	r := httptest.NewRequest("POST", testOrigin+"/auth/logout", nil)
	r.Header.Set("Origin", "https://evil.example.test")
	r.AddCookie(session)
	w := httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 403 || f.request("GET", "/api/me", session).Code != 200 {
		t.Fatal("cross-origin logout changed session")
	}
	r.Header.Del("Origin")
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 403 {
		t.Fatal("logout accepted a missing origin")
	}
	if f.request("GET", "/auth/logout", session).Code != 405 {
		t.Fatal("GET logout accepted")
	}
	state, cookie, code = f.attempt(t, nil)
	if _, err := f.pool.Exec(context.Background(), "UPDATE auth_login_attempts SET expires_at=now()-interval '1 minute' WHERE state_hash=$1", digest(state)); err != nil {
		t.Fatal(err)
	}
	if result := f.request("GET", "/auth/google/callback?state="+state+"&code="+code, cookie); !strings.Contains(result.Header().Get("Location"), "invalid_login") {
		t.Fatal("expired state accepted")
	}
	state, cookie, _ = f.attempt(t, nil)
	if result := f.request("GET", "/auth/google/callback?state="+state+"&error=access_denied", cookie); !strings.Contains(result.Header().Get("Location"), "cancelled") {
		t.Fatal("provider refusal not shown")
	}
	malformed := &http.Cookie{Name: "landman_session", Value: "invalid"}
	if f.request("GET", "/api/me", malformed).Code != 401 {
		t.Fatal("malformed session accepted")
	}
	r = httptest.NewRequest("GET", "http://evil.example.test/auth/google/start", nil)
	w = httptest.NewRecorder()
	f.handler.ServeHTTP(w, r)
	if w.Code != 400 {
		t.Fatal("unconfigured host accepted")
	}
}

func TestAuthMigration(t *testing.T) {
	f := setup(t)
	migration, err := os.ReadFile("../../db/migrations/000019_auth.sql")
	if err != nil {
		t.Fatal(err)
	}
	parts := strings.Split(string(migration), "-- +goose Down")
	if len(parts) != 2 {
		t.Fatal("auth migration must have one Up and one Down section")
	}
	ctx := context.Background()
	var referencesBefore int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND confrelid='public.app_users'::regclass`).Scan(&referencesBefore); err != nil {
		t.Fatal(err)
	}
	tx, err := f.pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	schema := fmt.Sprintf("auth_migration_%d", time.Now().UnixNano())
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+schema+", pg_catalog"); err != nil {
		t.Fatal(err)
	}
	up := strings.TrimPrefix(parts[0], "-- +goose Up")
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, parts[1]); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, up); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO app_users(google_subject,email,display_name) VALUES('subject','buyer@example.test','Buyer')"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "INSERT INTO auth_sessions(token_hash,user_id,expires_at) SELECT decode(repeat('ab',32),'hex'),id,now()+interval '1 day' FROM app_users"); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "DELETE FROM app_users"); err != nil {
		t.Fatal(err)
	}
	var count int
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM auth_sessions").Scan(&count); err != nil {
		t.Fatal(err)
	}
	if count != 0 {
		t.Fatal("user delete did not revoke sessions")
	}
	if err := tx.Rollback(ctx); err != nil {
		t.Fatal(err)
	}
	var referencesAfter int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM pg_constraint WHERE contype='f' AND confrelid='public.app_users'::regclass`).Scan(&referencesAfter); err != nil {
		t.Fatal(err)
	}
	if referencesAfter != referencesBefore {
		t.Fatalf("public account references changed: before %d, after %d", referencesBefore, referencesAfter)
	}
}

func TestProtectedParcelRoutes(t *testing.T) {
	f := setup(t)
	called := 0
	f.handler.mountData(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { called++; w.WriteHeader(http.StatusNoContent) }))
	for _, path := range []string{"/api/v1/parcels", "/api/v1/parcels/map", "/api/v1/parcels/lookup", "/api/v1/sources", "/api/v1/parcels/00000000-0000-0000-0000-000000000000/tax", "/api/v1/parcels/00000000-0000-0000-0000-000000000000/soil", "/api/v1/parcels/00000000-0000-0000-0000-000000000000/zoning", "/api/v1/parcels/00000000-0000-0000-0000-000000000000/roads"} {
		if w := f.request("GET", path); w.Code != http.StatusUnauthorized {
			t.Errorf("unprotected %s: %d", path, w.Code)
		}
	}
	session := f.login(t, nil)
	if w := f.request("GET", "/api/v1/parcels", session); w.Code != http.StatusNoContent || called != 1 {
		t.Fatalf("signed in status %d called %d", w.Code, called)
	}
	if w := f.request("GET", "/api/v1/parcels", &http.Cookie{Name: session.Name, Value: "bad"}); w.Code != http.StatusUnauthorized {
		t.Fatal("malformed session accepted")
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE auth_sessions SET created_at=now()-interval '8 days',expires_at=now()-interval '1 second' WHERE token_hash=$1", digest(session.Value)); err != nil {
		t.Fatal(err)
	}
	if f.request("GET", "/api/v1/parcels", session).Code != 401 {
		t.Fatal("expired session accepted")
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE auth_sessions SET expires_at=now()+interval '1 hour' WHERE token_hash=$1", digest(session.Value)); err != nil {
		t.Fatal(err)
	}
	f.handler.allowed = map[string]bool{}
	if f.request("GET", "/api/v1/parcels", session).Code != 401 {
		t.Fatal("disallowed session accepted")
	}
	f.handler.allowed["buyer@example.test"] = true
	if _, err := f.pool.Exec(context.Background(), "DELETE FROM auth_sessions WHERE token_hash=$1", digest(session.Value)); err != nil {
		t.Fatal(err)
	}
	if f.request("GET", "/api/v1/parcels", session).Code != 401 {
		t.Fatal("revoked session accepted")
	}
	session = f.login(t, nil)
	f.pool.Close()
	if f.request("GET", "/api/v1/parcels", session).Code != http.StatusServiceUnavailable {
		t.Fatal("database failure did not deny parcel access")
	}
	if called != 1 {
		t.Fatalf("data handler called %d times", called)
	}
}
