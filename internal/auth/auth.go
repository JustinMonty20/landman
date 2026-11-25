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
	"io"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"

	"github.com/JustinMonty20/landman/internal/parcelcrm"
	"github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/oauth2"
)

const googleIssuer = "https://accounts.google.com"
const sessionAge = 7 * 24 * time.Hour
const loginAge = 10 * time.Minute

type User struct {
	ID              string `json:"id"`
	Email           string `json:"email"`
	Name            string `json:"name"`
	ProfileImageURL string `json:"profile_image_url,omitempty"`
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
func New(ctx context.Context, pool *pgxpool.Pool, c Config, data ...http.Handler) (http.Handler, error) {
	s, err := newServer(ctx, pool, c, googleIssuer)
	if err != nil {
		return nil, err
	}
	if len(data) > 1 {
		return nil, errors.New("at most one data handler")
	}
	if len(data) == 1 && data[0] != nil {
		s.mountData(data[0])
	}
	return s, nil
}

func (s *server) mountData(data http.Handler) {
	s.mux.Handle("/api/v1/", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}
		_, err := s.currentUser(r)
		if errors.Is(err, pgx.ErrNoRows) {
			http.Error(w, "Sign-in required", http.StatusUnauthorized)
			return
		}
		if err != nil {
			unavailable(w)
			return
		}
		data.ServeHTTP(w, r)
	}))
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
	s.mux.HandleFunc("/api/me/crm/", s.crm)
	s.mux.HandleFunc("/api/me/favorites/", s.crm)
	s.mux.HandleFunc("GET /api/me/parcel-detail-fields", s.getParcelDetailFields)
	s.mux.HandleFunc("PUT /api/me/parcel-detail-fields", s.putParcelDetailFields)
	s.mux.HandleFunc("GET /api/me/parcel-owner/{id}", s.getParcelOwner)
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
		Email           string          `json:"email"`
		Verified        bool            `json:"email_verified"`
		Name            string          `json:"name"`
		Picture         json.RawMessage `json:"picture"`
		Nonce           string          `json:"nonce"`
		AuthorizedParty string          `json:"azp"`
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
	err = tx.QueryRow(r.Context(), `INSERT INTO app_users(google_subject,email,display_name,profile_image_url) VALUES($1,$2,$3,$4)
 ON CONFLICT(google_subject) DO UPDATE SET email=excluded.email,display_name=excluded.display_name,profile_image_url=excluded.profile_image_url,last_login_at=now() RETURNING id`, identity.Subject, email, claims.Name, profileImageURL(pictureString(claims.Picture))).Scan(&id)
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
	err = s.pool.QueryRow(r.Context(), `SELECT u.id,u.email,u.display_name,u.profile_image_url FROM auth_sessions s
 JOIN app_users u ON u.id=s.user_id WHERE s.token_hash=$1 AND s.expires_at>now()`, digest(cookie.Value)).Scan(&user.ID, &user.Email, &user.Name, &user.ProfileImageURL)
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

// The account's detail selection is independent of map layers and parcel reads.
var detailFieldOrder = []string{"mapped_acres", "valuation_land", "valuation_improvement", "valuation_total", "valuation_assessed", "owner_name", "owner_mailing", "entity_indicator", "sale_count", "delinquent_total", "tax_bill_count", "foreclosure_count", "vacancy_tier", "vacancy_reason", "soil_acres", "soil_septic", "soil_dwelling_without", "soil_dwelling_with", "zoning_district", "zoning_jurisdiction", "zoning_unmapped", "road_distance", "road_name", "road_status", "flood_unknown", "floodway_acres", "flood_zones"}

type detailFieldsResponse struct {
	Fields []string `json:"fields"`
}

func (s *server) detailUser(w http.ResponseWriter, r *http.Request) (User, bool) {
	user, err := s.currentUser(r)
	if errors.Is(err, pgx.ErrNoRows) {
		http.Error(w, "Sign-in required", http.StatusUnauthorized)
		return User{}, false
	}
	if err != nil {
		unavailable(w)
		return User{}, false
	}
	return user, true
}

func (s *server) getParcelDetailFields(w http.ResponseWriter, r *http.Request) {
	user, ok := s.detailUser(w, r)
	if !ok {
		return
	}
	var groups []string
	if err := s.pool.QueryRow(r.Context(), "SELECT parcel_detail_fields FROM app_users WHERE id=$1", user.ID).Scan(&groups); err != nil {
		unavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detailFieldsResponse{Fields: groups})
}

func (s *server) putParcelDetailFields(w http.ResponseWriter, r *http.Request) {
	if r.Header.Get("Origin") != s.origin {
		http.Error(w, "Invalid request origin", http.StatusForbidden)
		return
	}
	user, ok := s.detailUser(w, r)
	if !ok {
		return
	}
	if r.Header.Get("Content-Type") != "application/json" {
		http.Error(w, "Expected JSON", http.StatusUnsupportedMediaType)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, 512)
	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()
	var request struct {
		Fields *[]string `json:"fields"`
	}
	if err := decoder.Decode(&request); err != nil || request.Fields == nil {
		http.Error(w, "Invalid fields", http.StatusBadRequest)
		return
	}
	var trailing any
	if err := decoder.Decode(&trailing); !errors.Is(err, io.EOF) {
		http.Error(w, "Invalid JSON body", http.StatusBadRequest)
		return
	}
	selected := make(map[string]bool, len(*request.Fields))
	for _, group := range *request.Fields {
		valid := false
		for _, allowed := range detailFieldOrder {
			if group == allowed {
				valid = true
				break
			}
		}
		if !valid || selected[group] {
			http.Error(w, "Invalid fields", http.StatusBadRequest)
			return
		}
		selected[group] = true
	}
	groups := make([]string, 0, len(selected))
	for _, group := range detailFieldOrder {
		if selected[group] {
			groups = append(groups, group)
		}
	}
	if _, err := s.pool.Exec(r.Context(), "UPDATE app_users SET parcel_detail_fields=$1 WHERE id=$2", groups, user.ID); err != nil {
		unavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(detailFieldsResponse{Fields: groups})
}

// Owner and mailing data are private county parcel fields available only to signed-in buyers.
func (s *server) getParcelOwner(w http.ResponseWriter, r *http.Request) {
	if _, ok := s.detailUser(w, r); !ok {
		return
	}
	id := r.PathValue("id")
	if !regexp.MustCompile(`^[0-9a-fA-F]{8}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{4}-[0-9a-fA-F]{12}$`).MatchString(id) {
		http.Error(w, "Invalid parcel ID", http.StatusBadRequest)
		return
	}
	var first, second, line1, line2, city, state, postal *string
	err := s.pool.QueryRow(r.Context(), `SELECT NULLIF(current_name_1,''),NULLIF(current_name_2,''),NULLIF(mailing_line_1,''),NULLIF(mailing_line_2,''),NULLIF(mailing_city,''),NULLIF(mailing_state,''),NULLIF(mailing_postal_code,'') FROM parcels WHERE id=$1::uuid AND county_id='union' AND source_name='union_county_gis'`, id).Scan(&first, &second, &line1, &line2, &city, &state, &postal)
	if errors.Is(err, pgx.ErrNoRows) {
		http.NotFound(w, r)
		return
	}
	if err != nil {
		unavailable(w)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]any{"current_name_1": first, "current_name_2": second, "mailing_line_1": line1, "mailing_line_2": line2, "mailing_city": city, "mailing_state": state, "mailing_postal_code": postal})
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
	var column bool
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('app_users') AND attname='parcel_detail_fields' AND NOT attisdropped)").Scan(&column); err != nil || !column {
		return errors.New("missing parcel detail preferences migration")
	}
	if err := pool.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM pg_attribute WHERE attrelid=to_regclass('app_users') AND attname='profile_image_url' AND NOT attisdropped)").Scan(&column); err != nil || !column {
		return errors.New("missing profile_image_url on app_users; apply migration 49 before starting the API")
	}
	return nil
}

// crm passes the authenticated identity to the private workflow boundary.
func (s *server) crm(w http.ResponseWriter, r *http.Request) {
	user, ok := s.detailUser(w, r)
	if !ok {
		return
	}
	(parcelcrm.Store{DB: s.pool}).Serve(w, r, parcelcrm.AccountID(user.ID), s.origin)
}

func pictureString(raw json.RawMessage) string {
	var picture string
	_ = json.Unmarshal(raw, &picture)
	return picture
}

// Only Google-hosted HTTPS profile images from verified ID-token claims may reach the browser.
// Missing or malformed pictures are optional and never block authentication.
func profileImageURL(raw string) string {
	if len(raw) == 0 || len(raw) > 2048 {
		return ""
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.User != nil || u.Fragment != "" || u.RawQuery != "" || u.Port() != "" || u.Opaque != "" || u.Hostname() != "lh3.googleusercontent.com" {
		return ""
	}
	return u.String()
}
