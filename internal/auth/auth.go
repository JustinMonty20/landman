package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

const googleIssuer = "https://accounts.google.com"
const sessionAge = 7 * 24 * time.Hour
const loginAge = 10 * time.Minute

type User struct {
	ID    string `json:"id"`
	Email string `json:"email"`
	Name  string `json:"name"`
}

type server struct {
	pool          *pgxpool.Pool
	oauth         oauth2.Config
	verifier      *oidc.IDTokenVerifier
	allowed       map[string]bool
	origin        string
	secure        bool
	sessionCookie string
	mux           *http.ServeMux
}

// New establishes Google's identity contract. It does not request calendar access.
func New(ctx context.Context, pool *pgxpool.Pool, c Config) (http.Handler, error) {
	return newServer(ctx, pool, c, googleIssuer)
}

func newServer(ctx context.Context, pool *pgxpool.Pool, c Config, issuer string) (*server, error) {
	if err := c.validate(); err != nil {
		return nil, err
	}
	if pool == nil {
		return nil, errors.New("authentication requires a database")
	}
	ctx = oidc.ClientContext(ctx, &http.Client{Timeout: 10 * time.Second})
	provider, err := oidc.NewProvider(ctx, issuer)
	if err != nil {
		return nil, errors.New("cannot obtain Google identity configuration")
	}
	redirect, _ := url.Parse(c.RedirectURI)
	s := &server{pool: pool, allowed: make(map[string]bool), origin: redirect.Scheme + "://" + redirect.Host, secure: redirect.Scheme == "https", sessionCookie: "landman_session", mux: http.NewServeMux()}
	if s.secure {
		s.sessionCookie = "__Host-landman_session"
	}
	for _, email := range c.AllowedEmails {
		s.allowed[email] = true
	}
	s.oauth = oauth2.Config{ClientID: c.ClientID, ClientSecret: c.ClientSecret, RedirectURL: c.RedirectURI, Endpoint: provider.Endpoint(), Scopes: []string{oidc.ScopeOpenID, "email", "profile"}}
	s.verifier = provider.Verifier(&oidc.Config{ClientID: c.ClientID})
	s.mux.HandleFunc("GET /auth/google/start", s.start)
	s.mux.HandleFunc("GET /auth/google/callback", s.callback)
	s.mux.HandleFunc("POST /auth/logout", s.logout)
	s.mux.HandleFunc("GET /api/me", s.me)
	return s, nil
}

func (s *server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Cache-Control", "no-store")
	w.Header().Set("X-Content-Type-Options", "nosniff")
	w.Header().Set("Referrer-Policy", "no-referrer")
	// Do not trust forwarded headers to set the public origin.
	if r.Host != strings.TrimPrefix(strings.TrimPrefix(s.origin, "https://"), "http://") {
		http.Error(w, "Invalid application host", http.StatusBadRequest)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	s.mux.ServeHTTP(w, r.WithContext(ctx))
}

func randomToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic("secure random source unavailable")
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func digest(token string) []byte { sum := sha256.Sum256([]byte(token)); return sum[:] }

func validToken(token string) bool {
	decoded, err := base64.RawURLEncoding.DecodeString(token)
	return err == nil && len(decoded) == 32 && len(token) == 43
}

func (s *server) cookie(w http.ResponseWriter, name, value, path string, age time.Duration) {
	expires := time.Now().Add(age)
	maxAge := int(age.Seconds())
	if age < 0 {
		maxAge = -1
		expires = time.Unix(1, 0)
	}
	http.SetCookie(w, &http.Cookie{Name: name, Value: value, Path: path, Expires: expires, MaxAge: maxAge, HttpOnly: true, Secure: s.secure, SameSite: http.SameSiteLaxMode})
}

func (s *server) start(w http.ResponseWriter, r *http.Request) {
	// A new attempt replaces only this browser's previous pending attempt.
	if previous, err := r.Cookie("landman_login"); err == nil && validToken(previous.Value) {
		if _, err := s.pool.Exec(r.Context(), "DELETE FROM auth_login_attempts WHERE state_hash=$1", digest(previous.Value)); err != nil {
			unavailable(w)
			return
		}
	}
	state, nonce, verifier := randomToken(), randomToken(), oauth2.GenerateVerifier()
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(r.Context())
	_, err = tx.Exec(r.Context(), "DELETE FROM auth_login_attempts WHERE expires_at <= now()")
	if err == nil {
		_, err = tx.Exec(r.Context(), "DELETE FROM auth_sessions WHERE expires_at <= now()")
	}
	if err == nil {
		_, err = tx.Exec(r.Context(), "INSERT INTO auth_login_attempts(state_hash,nonce_hash,pkce_verifier,expires_at) VALUES($1,$2,$3,now()+interval '10 minutes')", digest(state), digest(nonce), verifier)
	}
	if err != nil {
		unavailable(w)
		return
	}
	if err = tx.Commit(r.Context()); err != nil {
		unavailable(w)
		return
	}
	s.cookie(w, "landman_login", state, "/auth/google/callback", loginAge)
	http.Redirect(w, r, s.oauth.AuthCodeURL(state, oauth2.S256ChallengeOption(verifier), oidc.Nonce(nonce), oauth2.SetAuthURLParam("prompt", "select_account")), http.StatusFound)
}

func (s *server) fail(w http.ResponseWriter, r *http.Request, reason string) {
	http.Redirect(w, r, "/?auth_error="+url.QueryEscape(reason), http.StatusSeeOther)
}

func (s *server) callback(w http.ResponseWriter, r *http.Request) {
	s.cookie(w, "landman_login", "", "/auth/google/callback", -time.Second)
	state := r.URL.Query().Get("state")
	cookie, err := r.Cookie("landman_login")
	if err != nil || !validToken(state) || subtle.ConstantTimeCompare([]byte(state), []byte(cookie.Value)) != 1 {
		s.fail(w, r, "invalid_login")
		return
	}
	var nonceHash []byte
	var verifier string
	err = s.pool.QueryRow(r.Context(), "DELETE FROM auth_login_attempts WHERE state_hash=$1 AND expires_at>now() RETURNING nonce_hash,pkce_verifier", digest(state)).Scan(&nonceHash, &verifier)
	if errors.Is(err, pgx.ErrNoRows) {
		s.fail(w, r, "invalid_login")
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	if r.URL.Query().Get("error") != "" || r.URL.Query().Get("code") == "" {
		s.fail(w, r, "cancelled")
		return
	}
	token, err := s.oauth.Exchange(r.Context(), r.URL.Query().Get("code"), oauth2.VerifierOption(verifier))
	if err != nil {
		s.fail(w, r, "provider_failed")
		return
	}
	raw, ok := token.Extra("id_token").(string)
	if !ok {
		s.fail(w, r, "invalid_login")
		return
	}
	identity, err := s.verifier.Verify(r.Context(), raw)
	if err != nil {
		s.fail(w, r, "invalid_login")
		return
	}
	var claims struct {
		Email           string `json:"email"`
		Verified        bool   `json:"email_verified"`
		Name            string `json:"name"`
		Nonce           string `json:"nonce"`
		AuthorizedParty string `json:"azp"`
	}
	if err := identity.Claims(&claims); err != nil || identity.Subject == "" || !claims.Verified || subtle.ConstantTimeCompare(digest(claims.Nonce), nonceHash) != 1 || (claims.AuthorizedParty != "" && claims.AuthorizedParty != s.oauth.ClientID) {
		s.fail(w, r, "invalid_login")
		return
	}
	email := strings.ToLower(strings.TrimSpace(claims.Email))
	if !s.allowed[email] {
		s.fail(w, r, "not_allowed")
		return
	}
	session := randomToken()
	tx, err := s.pool.Begin(r.Context())
	if err != nil {
		unavailable(w)
		return
	}
	defer tx.Rollback(r.Context())
	var id string
	err = tx.QueryRow(r.Context(), `INSERT INTO app_users(google_subject,email,display_name) VALUES($1,$2,$3)
 ON CONFLICT(google_subject) DO UPDATE SET email=excluded.email,display_name=excluded.display_name,last_login_at=now() RETURNING id`, identity.Subject, email, claims.Name).Scan(&id)
	if err == nil {
		_, err = tx.Exec(r.Context(), "INSERT INTO auth_sessions(token_hash,user_id,expires_at) VALUES($1,$2,now()+interval '7 days')", digest(session), id)
	}
	if err != nil {
		unavailable(w)
		return
	}
	// Replace, rather than retain, this browser's prior session after successful login.
	if previous, err := r.Cookie(s.sessionCookie); err == nil && validToken(previous.Value) {
		if _, err := tx.Exec(r.Context(), "DELETE FROM auth_sessions WHERE token_hash=$1", digest(previous.Value)); err != nil {
			unavailable(w)
			return
		}
	}
	if err := tx.Commit(r.Context()); err != nil {
		unavailable(w)
		return
	}
	s.cookie(w, s.sessionCookie, session, "/", sessionAge)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (s *server) currentUser(r *http.Request) (User, error) {
	var user User
	cookie, err := r.Cookie(s.sessionCookie)
	if err != nil || !validToken(cookie.Value) {
		return user, pgx.ErrNoRows
	}
	err = s.pool.QueryRow(r.Context(), `SELECT u.id,u.email,u.display_name FROM auth_sessions s
 JOIN app_users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, digest(cookie.Value)).Scan(&user.ID, &user.Email, &user.Name)
	if err != nil {
		return User{}, err
	}
	if !s.allowed[user.Email] {
		return User{}, pgx.ErrNoRows
	}
	return user, nil
}

func (s *server) me(w http.ResponseWriter, r *http.Request) {
	user, err := s.currentUser(r)
	if errors.Is(err, pgx.ErrNoRows) {
		s.cookie(w, s.sessionCookie, "", "/", -time.Second)
		http.Error(w, "Sign-in required", http.StatusUnauthorized)
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(user)
}

func (s *server) logout(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != s.origin {
		http.Error(w, "Invalid request origin", http.StatusForbidden)
		return
	}
	if cookie, err := r.Cookie(s.sessionCookie); err == nil && validToken(cookie.Value) {
		if _, err := s.pool.Exec(r.Context(), "DELETE FROM auth_sessions WHERE token_hash=$1", digest(cookie.Value)); err != nil {
			unavailable(w)
			return
		}
	}
	s.cookie(w, s.sessionCookie, "", "/", -time.Second)
	w.WriteHeader(http.StatusNoContent)
}

func unavailable(w http.ResponseWriter) {
	http.Error(w, "Sign-in service unavailable. Try again.", http.StatusServiceUnavailable)
}

// CheckSchema fails before accepting traffic if the authentication migration is absent.
func CheckSchema(ctx context.Context, pool *pgxpool.Pool) error {
	for _, table := range []string{"app_users", "auth_sessions", "auth_login_attempts"} {
		var exists bool
		if err := pool.QueryRow(ctx, "SELECT to_regclass($1) IS NOT NULL", table).Scan(&exists); err != nil {
			return errors.New("cannot check authentication schema")
		}
		if !exists {
			return fmt.Errorf("missing %s; apply the authentication migration first", table)
		}
	}
	return nil
}
