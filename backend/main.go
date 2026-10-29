package main

import (
	"bytes"
	"context"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"path"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	_ "github.com/mattn/go-sqlite3"
	"gopkg.in/yaml.v3"
)

const (
	maxFetchBytes = 5 << 20
	maxNodes      = 2000
	maxNameLength = 160
	defaultEvery  = 3600
)

type Node struct {
	ID         string                 `json:"id"`
	Name       string                 `json:"name"`
	Type       string                 `json:"type"`
	SourceID   string                 `json:"sourceId"`
	SourceName string                 `json:"sourceName"`
	Raw        map[string]interface{} `json:"-"`
}

type snapshotNode struct {
	Node
	Proxy map[string]interface{} `json:"proxy"`
}

type Source struct {
	ID                     string     `json:"id"`
	Name                   string     `json:"name"`
	Enabled                bool       `json:"enabled"`
	RefreshIntervalSeconds int        `json:"refreshIntervalSeconds"`
	NodeCount              int        `json:"nodeCount"`
	LastAttemptAt          *time.Time `json:"lastAttemptAt"`
	LastSuccessAt          *time.Time `json:"lastSuccessAt"`
	Status                 string     `json:"status"`
	ErrorCode              string     `json:"errorCode,omitempty"`
}

type App struct {
	db                *sql.DB
	cipher            cipher.AEAD
	client            *http.Client
	refreshMu         sync.Mutex
	refreshing        map[string]bool
	oauthEnabled      bool
	oauthClientID     string
	oauthClientSecret string
	oauthSessionKey   []byte
	publicURL         string
}

func main() {
	key, err := encryptionKey(os.Getenv("CLASH_CLOUD_ENCRYPTION_KEY"))
	if err != nil {
		log.Fatal("CLASH_CLOUD_ENCRYPTION_KEY must be a base64 or hex encoded 32-byte key")
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		log.Fatal("cannot initialize encryption")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		log.Fatal("cannot initialize encryption")
	}
	dbPath := env("CLASH_CLOUD_DB", "./clash-cloud.db")
	db, err := sql.Open("sqlite3", dbPath+"?_busy_timeout=5000&_journal_mode=WAL&_foreign_keys=on")
	if err != nil {
		log.Fatal("cannot open database")
	}
	defer db.Close()
	if err = migrate(db); err != nil {
		log.Fatal("database migration failed")
	}
	outURL := strings.TrimRight(os.Getenv("CLASH_CLOUD_PUBLIC_URL"), "/")
	if outURL == "" {
		outURL = ""
	}
	a := &App{
		db: db, cipher: aead, client: safeClient(), refreshing: map[string]bool{},
		oauthEnabled:      os.Getenv("CLASH_CLOUD_OAUTH_ENABLED") == "1",
		oauthClientID:     os.Getenv("CLASH_CLOUD_MATTERMOST_CLIENT_ID"),
		oauthClientSecret: os.Getenv("CLASH_CLOUD_MATTERMOST_CLIENT_SECRET"),
		publicURL:         strings.TrimRight(os.Getenv("CLASH_CLOUD_PUBLIC_URL"), "/"),
	}
	if a.oauthEnabled {
		a.oauthSessionKey, err = decodeSecretKey(os.Getenv("CLASH_CLOUD_OAUTH_SESSION_KEY"))
		if err != nil || a.oauthClientID == "" || a.oauthClientSecret == "" || a.publicURL == "" {
			log.Fatal("Mattermost OAuth configuration is incomplete")
		}
		u, parseErr := url.Parse(a.publicURL)
		if parseErr != nil || u.Scheme != "https" || u.Host == "" || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			log.Fatal("CLASH_CLOUD_PUBLIC_URL must be an HTTPS origin")
		}
	}
	if err := a.ensureOutputToken(outURL); err != nil {
		log.Fatal("cannot initialize output token")
	}
	go a.scheduler()
	addr := env("CLASH_CLOUD_LISTEN", "127.0.0.1:8080")
	mux := http.NewServeMux()
	mux.HandleFunc("/auth/", a.oauth)
	mux.HandleFunc("/api/v1/", a.api)
	mux.HandleFunc("/s/", a.subscription)
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/" {
			jsonResponse(w, 200, map[string]string{"service": "clash-cloud"})
			return
		}
		http.NotFound(w, r)
	})
	server := &http.Server{Addr: addr, Handler: accessLog(mux), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second}
	log.Printf("clash-cloud listening on %s", addr)
	log.Fatal(server.ListenAndServe())
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func encryptionKey(s string) ([]byte, error) {
	if b, e := base64.StdEncoding.DecodeString(s); e == nil && len(b) == 32 {
		return b, nil
	}
	b, e := hex.DecodeString(s)
	if e == nil && len(b) == 32 {
		return b, nil
	}
	return nil, errors.New("invalid key")
}
func decodeSecretKey(s string) ([]byte, error) {
	if b, err := base64.StdEncoding.DecodeString(s); err == nil && len(b) >= 32 {
		return b, nil
	}
	b, err := hex.DecodeString(s)
	if err == nil && len(b) >= 32 {
		return b, nil
	}
	return nil, errors.New("invalid secret key")
}
func migrate(db *sql.DB) error {
	_, err := db.Exec(`CREATE TABLE IF NOT EXISTS subscriptions(
	id TEXT PRIMARY KEY,name TEXT NOT NULL,encrypted_url BLOB NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,
	refresh_interval INTEGER NOT NULL,created_at TEXT NOT NULL,last_attempt_at TEXT,last_success_at TEXT,
	status TEXT NOT NULL DEFAULT 'pending',error_code TEXT NOT NULL DEFAULT '',node_count INTEGER NOT NULL DEFAULT 0,
	next_refresh_at TEXT NOT NULL);
	CREATE TABLE IF NOT EXISTS snapshots(subscription_id TEXT PRIMARY KEY,nodes_json BLOB NOT NULL,updated_at TEXT NOT NULL,
	FOREIGN KEY(subscription_id) REFERENCES subscriptions(id) ON DELETE CASCADE);
	CREATE TABLE IF NOT EXISTS output_settings(id INTEGER PRIMARY KEY CHECK(id=1),token_hash TEXT NOT NULL,encrypted_token BLOB NOT NULL,
	public_url TEXT NOT NULL DEFAULT '',config BLOB NOT NULL DEFAULT '',generated_at TEXT,revision INTEGER NOT NULL DEFAULT 0);
	CREATE TABLE IF NOT EXISTS output_links(id TEXT PRIMARY KEY,name TEXT NOT NULL,name_key TEXT NOT NULL UNIQUE,token_hash TEXT NOT NULL UNIQUE,
	encrypted_token BLOB NOT NULL,enabled INTEGER NOT NULL DEFAULT 1,created_at TEXT NOT NULL);
`)
	return err
}
func safeClient() *http.Client {
	tr := &http.Transport{Proxy: nil, TLSHandshakeTimeout: 5 * time.Second, ResponseHeaderTimeout: 8 * time.Second, IdleConnTimeout: 30 * time.Second, MaxIdleConns: 10}
	tr.DialContext = func(ctx context.Context, network, address string) (net.Conn, error) {
		host, port, err := net.SplitHostPort(address)
		if err != nil {
			return nil, err
		}
		ips, err := net.DefaultResolver.LookupIPAddr(ctx, host)
		if err != nil {
			return nil, errors.New("dns_lookup_failed")
		}
		if len(ips) == 0 {
			return nil, errors.New("dns_lookup_failed")
		}
		var last error
		for _, ip := range ips {
			if !publicIP(ip.IP) {
				return nil, errors.New("blocked_address")
			}
			c, e := (&net.Dialer{Timeout: 5 * time.Second}).DialContext(ctx, network, net.JoinHostPort(ip.IP.String(), port))
			if e == nil {
				return c, nil
			}
			last = e
		}
		return nil, last
	}
	return &http.Client{Transport: tr, Timeout: 15 * time.Second, CheckRedirect: func(req *http.Request, via []*http.Request) error {
		if len(via) >= 4 {
			return errors.New("too_many_redirects")
		}
		if err := validateURL(req.URL); err != nil {
			return err
		}
		return nil
	}}
}
func publicIP(ip net.IP) bool {
	if ip == nil || ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsLinkLocalMulticast() || ip.IsUnspecified() || ip.IsMulticast() {
		return false
	}
	v4 := ip.To4()
	if v4 == nil {
		return true
	}
	blocked := []*net.IPNet{}
	for _, cidr := range []string{"0.0.0.0/8", "100.64.0.0/10", "192.0.0.0/24", "192.0.2.0/24", "198.18.0.0/15", "198.51.100.0/24", "203.0.113.0/24", "224.0.0.0/4", "240.0.0.0/4"} {
		_, n, _ := net.ParseCIDR(cidr)
		blocked = append(blocked, n)
	}
	for _, n := range blocked {
		if n.Contains(v4) {
			return false
		}
	}
	return true
}
func validateURL(u *url.URL) error {
	if u == nil || (u.Scheme != "http" && u.Scheme != "https") || u.Hostname() == "" {
		return errors.New("invalid_url")
	}
	if ip := net.ParseIP(u.Hostname()); ip != nil {
		if !publicIP(ip) {
			return errors.New("blocked_address")
		}
		return nil
	}
	ips, err := net.LookupIP(u.Hostname())
	if err != nil || len(ips) == 0 {
		return errors.New("dns_lookup_failed")
	}
	for _, ip := range ips {
		if !publicIP(ip) {
			return errors.New("blocked_address")
		}
	}
	return nil
}
func (a *App) encrypt(b []byte) ([]byte, error) {
	nonce := make([]byte, a.cipher.NonceSize())
	if _, e := rand.Read(nonce); e != nil {
		return nil, e
	}
	return append(nonce, a.cipher.Seal(nil, nonce, b, nil)...), nil
}
func (a *App) decrypt(b []byte) ([]byte, error) {
	n := a.cipher.NonceSize()
	if len(b) < n {
		return nil, errors.New("bad ciphertext")
	}
	return a.cipher.Open(nil, b[:n], b[n:], nil)
}
func randomToken() (string, error) {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}
func hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (a *App) ensureOutputToken(publicURL string) error {
	var count int
	err := a.db.QueryRow(`SELECT count(*) FROM output_settings WHERE id=1`).Scan(&count)
	if err != nil {
		return err
	}
	if count > 0 {
		if publicURL != "" {
			_, err = a.db.Exec(`UPDATE output_settings SET public_url=? WHERE id=1`, publicURL)
			if err != nil {
				return err
			}
		}
	} else {
		t, e := randomToken()
		if e != nil {
			return e
		}
		encrypted, e := a.encrypt([]byte(t))
		if e != nil {
			return e
		}
		if _, e = a.db.Exec(`INSERT INTO output_settings(id,token_hash,encrypted_token,public_url,config,revision) VALUES(1,?,?,?,?,0)`, hash(t), encrypted, publicURL, []byte{}); e != nil {
			return e
		}
	}
	// Seed after the singleton exists so upgrades keep the existing /s/{token} valid.
	var tokenHash string
	var encrypted []byte
	if err := a.db.QueryRow(`SELECT token_hash,encrypted_token FROM output_settings WHERE id=1`).Scan(&tokenHash, &encrypted); err != nil {
		return err
	}
	_, err = a.db.Exec(`INSERT OR IGNORE INTO output_links(id,name,name_key,token_hash,encrypted_token,enabled,created_at) VALUES('default','Default','default',?,?,1,?)`, tokenHash, encrypted, stamp(time.Now().UTC()))
	return err
}
func (a *App) api(w http.ResponseWriter, r *http.Request) { // Routes are protected by the trusted ingress; bind to loopback by default.
	p := strings.TrimPrefix(r.URL.Path, "/api/v1")
	if p == "/health" && r.Method == http.MethodGet {
		jsonResponse(w, 200, map[string]string{"status": "ok", "version": env("CLASH_CLOUD_VERSION", "dev")})
		return
	}
	if a.oauthEnabled {
		if _, ok := a.sessionUser(r); !ok {
			apiError(w, http.StatusUnauthorized, "unauthorized", "Sign in required")
			return
		}
	}
	if p == "/subscriptions" {
		switch r.Method {
		case http.MethodGet:
			a.listSources(w)
		case http.MethodPost:
			a.createSource(w, r)
		default:
			method(w)
		}
		return
	}
	if p == "/nodes" && r.Method == http.MethodGet {
		a.listNodes(w, r)
		return
	}
	if p == "/output" && r.Method == http.MethodGet {
		a.outputInfo(w)
		return
	}
	if p == "/output/links" {
		if r.Method == http.MethodPost {
			a.createOutputLink(w, r)
		} else {
			method(w)
		}
		return
	}
	if strings.HasPrefix(p, "/output/links/") {
		if r.Method == http.MethodPatch {
			a.patchOutputLink(w, r, strings.TrimPrefix(p, "/output/links/"))
		} else {
			method(w)
		}
		return
	}
	if p == "/output/rotate-token" && r.Method == http.MethodPost {
		a.rotateToken(w)
		return
	}
	if strings.HasPrefix(p, "/subscriptions/") {
		a.sourceRoute(w, r, strings.TrimPrefix(p, "/subscriptions/"))
		return
	}
	apiError(w, 404, "not_found", "Not found")
}

const mattermostBaseURL = "https://mm.chenkeyan.com"

type oauthSession struct {
	UserID   string `json:"sub"`
	Username string `json:"name"`
	Expires  int64  `json:"exp"`
}

func (a *App) oauth(w http.ResponseWriter, r *http.Request) {
	if !a.oauthEnabled {
		if r.URL.Path == "/auth/me" && r.Method == http.MethodGet {
			jsonResponse(w, http.StatusOK, map[string]string{"id": "local", "username": "local"})
			return
		}
		http.Error(w, "Authentication is not configured", http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case "/auth/login":
		if r.Method != http.MethodGet {
			method(w)
			return
		}
		a.startOAuth(w, r)
	case "/auth/callback":
		if r.Method != http.MethodGet {
			method(w)
			return
		}
		a.finishOAuth(w, r)
	case "/auth/me":
		if r.Method != http.MethodGet {
			method(w)
			return
		}
		user, ok := a.sessionUser(r)
		if !ok {
			apiError(w, http.StatusUnauthorized, "unauthorized", "Sign in required")
			return
		}
		jsonResponse(w, http.StatusOK, map[string]string{"id": user.UserID, "username": user.Username})
	case "/auth/logout":
		if r.Method != http.MethodPost {
			method(w)
			return
		}
		http.SetCookie(w, &http.Cookie{Name: "clash_cloud_session", Value: "", Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
		http.Redirect(w, r, "/", http.StatusSeeOther)
	default:
		http.NotFound(w, r)
	}
}

func (a *App) startOAuth(w http.ResponseWriter, r *http.Request) {
	state := make([]byte, 32)
	if _, err := rand.Read(state); err != nil {
		http.Error(w, "Unable to start sign in", http.StatusInternalServerError)
		return
	}
	stateValue := base64.RawURLEncoding.EncodeToString(state)
	http.SetCookie(w, &http.Cookie{Name: "clash_cloud_oauth_state", Value: stateValue, Path: "/auth/callback", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: 600})
	callback := a.publicURL + "/auth/callback"
	query := url.Values{"client_id": {a.oauthClientID}, "response_type": {"code"}, "redirect_uri": {callback}, "state": {stateValue}}
	http.Redirect(w, r, mattermostBaseURL+"/oauth/authorize?"+query.Encode(), http.StatusFound)
}

func (a *App) finishOAuth(w http.ResponseWriter, r *http.Request) {
	stateCookie, err := r.Cookie("clash_cloud_oauth_state")
	clearCookie(w, "clash_cloud_oauth_state", "/auth/callback")
	if err != nil || stateCookie.Value == "" || r.URL.Query().Get("state") == "" || !hmac.Equal([]byte(stateCookie.Value), []byte(r.URL.Query().Get("state"))) {
		http.Error(w, "Invalid sign in request", http.StatusBadRequest)
		return
	}
	if r.URL.Query().Get("error") != "" {
		http.Error(w, "Mattermost sign in was not completed", http.StatusUnauthorized)
		return
	}
	code := r.URL.Query().Get("code")
	if code == "" {
		http.Error(w, "Missing authorization code", http.StatusBadRequest)
		return
	}
	callback := a.publicURL + "/auth/callback"
	form := url.Values{"grant_type": {"authorization_code"}, "client_id": {a.oauthClientID}, "client_secret": {a.oauthClientSecret}, "code": {code}, "redirect_uri": {callback}}
	req, err := http.NewRequestWithContext(r.Context(), http.MethodPost, mattermostBaseURL+"/oauth/access_token", strings.NewReader(form.Encode()))
	if err != nil {
		http.Error(w, "Unable to complete sign in", http.StatusBadGateway)
		return
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := a.client.Do(req)
	if err != nil {
		http.Error(w, "Unable to complete sign in", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	var token struct {
		AccessToken string `json:"access_token"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 64<<10)).Decode(&token) != nil || token.AccessToken == "" {
		http.Error(w, "Unable to complete sign in", http.StatusUnauthorized)
		return
	}
	profileReq, err := http.NewRequestWithContext(r.Context(), http.MethodGet, mattermostBaseURL+"/api/v4/users/me", nil)
	if err != nil {
		http.Error(w, "Unable to verify Mattermost account", http.StatusBadGateway)
		return
	}
	profileReq.Header.Set("Authorization", "Bearer "+token.AccessToken)
	profileResp, err := a.client.Do(profileReq)
	if err != nil {
		http.Error(w, "Unable to verify Mattermost account", http.StatusBadGateway)
		return
	}
	defer profileResp.Body.Close()
	var profile struct {
		ID       string `json:"id"`
		Username string `json:"username"`
		Roles    string `json:"roles"`
	}
	if profileResp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(profileResp.Body, 64<<10)).Decode(&profile) != nil {
		http.Error(w, "Unable to verify Mattermost account", http.StatusUnauthorized)
		return
	}
	if profile.ID == "" || profile.Username == "" || !hasRole(profile.Roles, "system_admin") {
		http.Error(w, "A Mattermost system administrator account is required", http.StatusForbidden)
		return
	}
	if err := a.setSession(w, oauthSession{UserID: profile.ID, Username: profile.Username, Expires: time.Now().Add(12 * time.Hour).Unix()}); err != nil {
		http.Error(w, "Unable to create session", http.StatusInternalServerError)
		return
	}
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func hasRole(roles, role string) bool {
	for _, item := range strings.Fields(roles) {
		if item == role {
			return true
		}
	}
	return false
}

func (a *App) setSession(w http.ResponseWriter, session oauthSession) error {
	data, err := json.Marshal(session)
	if err != nil {
		return err
	}
	payload := base64.RawURLEncoding.EncodeToString(data)
	mac := hmac.New(sha256.New, a.oauthSessionKey)
	_, _ = mac.Write([]byte(payload))
	http.SetCookie(w, &http.Cookie{Name: "clash_cloud_session", Value: payload + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil)), Path: "/", Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, Expires: time.Unix(session.Expires, 0), MaxAge: 12 * 60 * 60})
	return nil
}

func (a *App) sessionUser(r *http.Request) (oauthSession, bool) {
	cookie, err := r.Cookie("clash_cloud_session")
	if err != nil {
		return oauthSession{}, false
	}
	parts := strings.Split(cookie.Value, ".")
	if len(parts) != 2 {
		return oauthSession{}, false
	}
	provided, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return oauthSession{}, false
	}
	mac := hmac.New(sha256.New, a.oauthSessionKey)
	_, _ = mac.Write([]byte(parts[0]))
	if !hmac.Equal(provided, mac.Sum(nil)) {
		return oauthSession{}, false
	}
	payload, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return oauthSession{}, false
	}
	var session oauthSession
	if json.Unmarshal(payload, &session) != nil || session.UserID == "" || session.Expires <= time.Now().Unix() {
		return oauthSession{}, false
	}
	return session, true
}

func clearCookie(w http.ResponseWriter, name, cookiePath string) {
	http.SetCookie(w, &http.Cookie{Name: name, Value: "", Path: cookiePath, Secure: true, HttpOnly: true, SameSite: http.SameSiteLaxMode, MaxAge: -1})
}
func (a *App) listSources(w http.ResponseWriter) {
	rows, e := a.db.Query(`SELECT id,name,enabled,refresh_interval,node_count,last_attempt_at,last_success_at,status,error_code FROM subscriptions ORDER BY created_at,id`)
	if e != nil {
		internal(w)
		return
	}
	defer rows.Close()
	items := []Source{}
	for rows.Next() {
		s := Source{}
		var en int
		var at, ok sql.NullString
		if rows.Scan(&s.ID, &s.Name, &en, &s.RefreshIntervalSeconds, &s.NodeCount, &at, &ok, &s.Status, &s.ErrorCode) != nil {
			internal(w)
			return
		}
		s.Enabled = en != 0
		s.LastAttemptAt = parseTime(at)
		s.LastSuccessAt = parseTime(ok)
		items = append(items, s)
	}
	jsonResponse(w, 200, map[string]interface{}{"items": items, "total": len(items)})
}
func parseTime(v sql.NullString) *time.Time {
	if !v.Valid {
		return nil
	}
	t, e := time.Parse(time.RFC3339Nano, v.String)
	if e != nil {
		return nil
	}
	return &t
}
func (a *App) createSource(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name  string `json:"name"`
		URL   string `json:"url"`
		Every int    `json:"refreshIntervalSeconds"`
	}
	if !decode(w, r, &in) {
		return
	}
	in.Name = strings.TrimSpace(in.Name)
	if len(in.Name) == 0 || len(in.Name) > maxNameLength || len(in.URL) > 4096 {
		apiError(w, 400, "invalid_input", "Name and URL are required")
		return
	}
	u, e := url.Parse(in.URL)
	if e != nil || validateURL(u) != nil {
		apiError(w, 400, "invalid_url", "A public HTTP or HTTPS URL is required")
		return
	}
	if in.Every == 0 {
		in.Every = defaultEvery
	}
	if in.Every < 60 || in.Every > 7*86400 {
		apiError(w, 400, "invalid_interval", "Refresh interval must be between 60 and 604800 seconds")
		return
	}
	id, e := newID()
	if e != nil {
		internal(w)
		return
	}
	enc, e := a.encrypt([]byte(in.URL))
	if e != nil {
		internal(w)
		return
	}
	now := time.Now().UTC()
	_, e = a.db.Exec(`INSERT INTO subscriptions(id,name,encrypted_url,enabled,refresh_interval,created_at,status,next_refresh_at) VALUES(?,?,?,1,?,?,'pending',?)`, id, in.Name, enc, in.Every, stamp(now), stamp(now))
	if e != nil {
		internal(w)
		return
	}
	s := Source{ID: id, Name: in.Name, Enabled: true, RefreshIntervalSeconds: in.Every, Status: "pending"}
	jsonResponse(w, 201, s)
	go a.refresh(id)
}
func (a *App) sourceRoute(w http.ResponseWriter, r *http.Request, rest string) {
	parts := strings.Split(rest, "/")
	id := parts[0]
	if id == "" {
		apiError(w, 404, "not_found", "Not found")
		return
	}
	if len(parts) == 1 {
		switch r.Method {
		case http.MethodGet:
			a.getSource(w, id)
		case http.MethodPatch:
			a.patchSource(w, r, id)
		case http.MethodDelete:
			a.deleteSource(w, id)
		default:
			method(w)
		}
		return
	}
	if len(parts) == 2 && parts[1] == "refresh" && r.Method == http.MethodPost {
		if !a.beginRefresh(id) {
			apiError(w, 404, "not_found", "Subscription not found")
			return
		}
		run, _ := newID()
		jsonResponse(w, 202, map[string]string{"runId": run, "status": "queued"})
		go a.refreshStarted(id)
		return
	}
	if len(parts) == 2 && parts[1] == "nodes" && r.Method == http.MethodGet {
		a.sourceNodes(w, id)
		return
	}
	apiError(w, 404, "not_found", "Not found")
}
func (a *App) getSource(w http.ResponseWriter, id string) {
	var s Source
	var en int
	var at, ok sql.NullString
	e := a.db.QueryRow(`SELECT id,name,enabled,refresh_interval,node_count,last_attempt_at,last_success_at,status,error_code FROM subscriptions WHERE id=?`, id).Scan(&s.ID, &s.Name, &en, &s.RefreshIntervalSeconds, &s.NodeCount, &at, &ok, &s.Status, &s.ErrorCode)
	if e == sql.ErrNoRows {
		apiError(w, 404, "not_found", "Subscription not found")
		return
	}
	if e != nil {
		internal(w)
		return
	}
	s.Enabled = en != 0
	s.LastAttemptAt = parseTime(at)
	s.LastSuccessAt = parseTime(ok)
	jsonResponse(w, 200, s)
}
func (a *App) patchSource(w http.ResponseWriter, r *http.Request, id string) {
	var in map[string]json.RawMessage
	if !decode(w, r, &in) {
		return
	}
	var name, urlText string
	var enabled bool
	var every int
	var exists int
	e := a.db.QueryRow(`SELECT count(*) FROM subscriptions WHERE id=?`, id).Scan(&exists)
	if e != nil || exists == 0 {
		apiError(w, 404, "not_found", "Subscription not found")
		return
	}
	if b, ok := in["name"]; ok {
		if json.Unmarshal(b, &name) != nil || strings.TrimSpace(name) == "" || len(name) > maxNameLength {
			apiError(w, 400, "invalid_name", "Invalid subscription name")
			return
		}
		name = strings.TrimSpace(name)
		_, e = a.db.Exec(`UPDATE subscriptions SET name=? WHERE id=?`, name, id)
		if e != nil {
			internal(w)
			return
		}
	}
	if b, ok := in["url"]; ok {
		if json.Unmarshal(b, &urlText) != nil || len(urlText) > 4096 {
			apiError(w, 400, "invalid_url", "Invalid subscription URL")
			return
		}
		u, pe := url.Parse(urlText)
		if pe != nil || validateURL(u) != nil {
			apiError(w, 400, "invalid_url", "A public HTTP or HTTPS URL is required")
			return
		}
		enc, ee := a.encrypt([]byte(urlText))
		if ee != nil {
			internal(w)
			return
		}
		_, e = a.db.Exec(`UPDATE subscriptions SET encrypted_url=?,status='pending',error_code='',next_refresh_at=? WHERE id=?`, enc, stamp(time.Now().UTC()), id)
		if e != nil {
			internal(w)
			return
		}
		go a.refresh(id)
	}
	if b, ok := in["enabled"]; ok {
		if json.Unmarshal(b, &enabled) != nil {
			apiError(w, 400, "invalid_enabled", "Enabled must be boolean")
			return
		}
		val := 0
		if enabled {
			val = 1
		}
		_, e = a.db.Exec(`UPDATE subscriptions SET enabled=?,status=CASE WHEN ?=0 THEN 'disabled' ELSE 'pending' END,next_refresh_at=? WHERE id=?`, val, val, stamp(time.Now().UTC()), id)
		if e != nil {
			internal(w)
			return
		}
		if enabled {
			go a.refresh(id)
		} else if e = a.regenerate(); e != nil {
			apiError(w, 409, "output_unavailable", "Could not safely regenerate output")
			return
		}
	}
	if b, ok := in["refreshIntervalSeconds"]; ok {
		if json.Unmarshal(b, &every) != nil || every < 60 || every > 7*86400 {
			apiError(w, 400, "invalid_interval", "Refresh interval must be between 60 and 604800 seconds")
			return
		}
		_, e = a.db.Exec(`UPDATE subscriptions SET refresh_interval=?,next_refresh_at=? WHERE id=?`, every, stamp(time.Now().UTC().Add(time.Duration(every)*time.Second)), id)
		if e != nil {
			internal(w)
			return
		}
	}
	a.getSource(w, id)
}
func (a *App) deleteSource(w http.ResponseWriter, id string) {
	_, e := a.db.Exec(`DELETE FROM subscriptions WHERE id=?`, id)
	if e != nil {
		internal(w)
		return
	}
	if a.regenerate() != nil {
		apiError(w, 409, "output_unavailable", "Could not safely regenerate output")
		return
	}
	w.WriteHeader(204)
}
func (a *App) sourceNodes(w http.ResponseWriter, id string) {
	if !a.exists(id) {
		apiError(w, 404, "not_found", "Subscription not found")
		return
	}
	nodes, err := a.allNodes(false)
	if err != nil {
		internal(w)
		return
	}
	items := []Node{}
	for _, node := range nodes {
		if node.SourceID == id {
			items = append(items, node)
		}
	}
	jsonResponse(w, 200, map[string]interface{}{"items": items, "total": len(items)})
}
func (a *App) listNodes(w http.ResponseWriter, r *http.Request) {
	q := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("q")))
	sub := r.URL.Query().Get("subscriptionId")
	limit := parseInt(r.URL.Query().Get("limit"), 100)
	offset := parseInt(r.URL.Query().Get("offset"), 0)
	if limit < 1 {
		limit = 100
	}
	if limit > 500 {
		limit = 500
	}
	if offset < 0 {
		offset = 0
	}
	nodes, e := a.allNodes(false)
	if e != nil {
		internal(w)
		return
	}
	filtered := []Node{}
	for _, n := range nodes {
		if sub != "" && n.SourceID != sub {
			continue
		}
		if q != "" && !strings.Contains(strings.ToLower(n.Name), q) && !strings.Contains(strings.ToLower(n.Type), q) && !strings.Contains(strings.ToLower(n.SourceName), q) {
			continue
		}
		filtered = append(filtered, n)
	}
	total := len(filtered)
	if offset > total {
		offset = total
	}
	end := offset + limit
	if end > total {
		end = total
	}
	jsonResponse(w, 200, map[string]interface{}{"items": filtered[offset:end], "total": total})
}

type outputLink struct {
	ID        string `json:"id"`
	Name      string `json:"name"`
	URL       string `json:"url"`
	Enabled   bool   `json:"enabled"`
	CreatedAt string `json:"createdAt"`
}

func (a *App) linkURL(public, token string) string {
	if public == "" {
		return "/s/" + token
	}
	return public + "/s/" + token
}

func (a *App) getOutputLinks(public string) ([]outputLink, error) {
	rows, err := a.db.Query(`SELECT id,name,encrypted_token,enabled,created_at FROM output_links ORDER BY created_at,id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	links := []outputLink{}
	for rows.Next() {
		var link outputLink
		var encrypted []byte
		var enabled int
		if err := rows.Scan(&link.ID, &link.Name, &encrypted, &enabled, &link.CreatedAt); err != nil {
			return nil, err
		}
		token, err := a.decrypt(encrypted)
		if err != nil {
			return nil, err
		}
		link.URL = a.linkURL(public, string(token))
		link.Enabled = enabled != 0
		links = append(links, link)
	}
	return links, rows.Err()
}

func (a *App) outputInfo(w http.ResponseWriter) {
	var public string
	var rev int
	var generated sql.NullString
	var config []byte
	e := a.db.QueryRow(`SELECT public_url,revision,generated_at,config FROM output_settings WHERE id=1`).Scan(&public, &rev, &generated, &config)
	if e != nil {
		internal(w)
		return
	}
	links, e := a.getOutputLinks(public)
	if e != nil {
		internal(w)
		return
	}
	nodes, nodeErr := a.allNodes(true)
	if nodeErr != nil {
		internal(w)
		return
	}
	status := "healthy"
	if len(nodes) == 0 {
		status = "no_nodes"
	} else if len(config) == 0 {
		status = "pending"
	}
	var gt interface{} = nil
	if generated.Valid {
		gt = parseTime(generated)
	}
	legacyURL := ""
	for _, link := range links {
		if link.ID == "default" {
			legacyURL = link.URL
			break
		}
	}
	jsonResponse(w, 200, map[string]interface{}{"status": status, "nodeCount": len(nodes), "generatedAt": gt, "revision": rev, "url": legacyURL, "links": links})
}

func validOutputLinkName(name string) (string, string, bool) {
	name = strings.TrimSpace(name)
	if name == "" || utf8.RuneCountInString(name) > 80 || strings.IndexFunc(name, unicode.IsControl) >= 0 {
		return "", "", false
	}
	return name, strings.ToLower(name), true
}

func (a *App) createOutputLink(w http.ResponseWriter, r *http.Request) {
	var in struct {
		Name string `json:"name"`
	}
	if !decode(w, r, &in) {
		return
	}
	name, key, ok := validOutputLinkName(in.Name)
	if !ok {
		apiError(w, 400, "invalid_name", "Name must be 1 to 80 characters")
		return
	}
	var exists int
	if err := a.db.QueryRow(`SELECT count(*) FROM output_links WHERE name_key=?`, key).Scan(&exists); err != nil {
		internal(w)
		return
	}
	if exists > 0 {
		apiError(w, 409, "duplicate_name", "A link with this name already exists")
		return
	}
	id, err := newID()
	if err != nil {
		internal(w)
		return
	}
	token, err := randomToken()
	if err != nil {
		internal(w)
		return
	}
	encrypted, err := a.encrypt([]byte(token))
	if err != nil {
		internal(w)
		return
	}
	createdAt := stamp(time.Now().UTC())
	_, err = a.db.Exec(`INSERT INTO output_links(id,name,name_key,token_hash,encrypted_token,enabled,created_at) VALUES(?,?,?,?,?,1,?)`, id, name, key, hash(token), encrypted, createdAt)
	if err != nil {
		var duplicate int
		if checkErr := a.db.QueryRow(`SELECT count(*) FROM output_links WHERE name_key=?`, key).Scan(&duplicate); checkErr != nil {
			internal(w)
		} else if duplicate > 0 {
			apiError(w, 409, "duplicate_name", "A link with this name already exists")
		} else {
			internal(w)
		}
		return
	}
	var public string
	if err := a.db.QueryRow(`SELECT public_url FROM output_settings WHERE id=1`).Scan(&public); err != nil {
		internal(w)
		return
	}
	jsonResponse(w, 201, map[string]interface{}{"link": outputLink{ID: id, Name: name, URL: a.linkURL(public, token), Enabled: true, CreatedAt: createdAt}})
}

func (a *App) patchOutputLink(w http.ResponseWriter, r *http.Request, id string) {
	if id == "" || strings.Contains(id, "/") {
		apiError(w, 404, "not_found", "Output link not found")
		return
	}
	var in struct {
		Name    *string `json:"name"`
		Enabled *bool   `json:"enabled"`
	}
	if !decode(w, r, &in) {
		return
	}
	if in.Name == nil && in.Enabled == nil {
		apiError(w, 400, "invalid_request", "No changes provided")
		return
	}
	var currentName string
	var enabled int
	var createdAt string
	var encrypted []byte
	if err := a.db.QueryRow(`SELECT name,enabled,created_at,encrypted_token FROM output_links WHERE id=?`, id).Scan(&currentName, &enabled, &createdAt, &encrypted); err != nil {
		apiError(w, 404, "not_found", "Output link not found")
		return
	}
	name, key := currentName, strings.ToLower(currentName)
	if in.Name != nil {
		var ok bool
		name, key, ok = validOutputLinkName(*in.Name)
		if !ok {
			apiError(w, 400, "invalid_name", "Name must be 1 to 80 characters")
			return
		}
	}
	if in.Enabled != nil {
		if *in.Enabled {
			enabled = 1
		} else {
			enabled = 0
		}
	}
	if _, err := a.db.Exec(`UPDATE output_links SET name=?,name_key=?,enabled=? WHERE id=?`, name, key, enabled, id); err != nil {
		var duplicate int
		if checkErr := a.db.QueryRow(`SELECT count(*) FROM output_links WHERE name_key=? AND id<>?`, key, id).Scan(&duplicate); checkErr != nil {
			internal(w)
		} else if duplicate > 0 {
			apiError(w, 409, "duplicate_name", "A link with this name already exists")
		} else {
			internal(w)
		}
		return
	}
	token, err := a.decrypt(encrypted)
	if err != nil {
		internal(w)
		return
	}
	var public string
	if err := a.db.QueryRow(`SELECT public_url FROM output_settings WHERE id=1`).Scan(&public); err != nil {
		internal(w)
		return
	}
	jsonResponse(w, 200, map[string]interface{}{"link": outputLink{ID: id, Name: name, URL: a.linkURL(public, string(token)), Enabled: enabled != 0, CreatedAt: createdAt}})
}

func (a *App) rotateToken(w http.ResponseWriter) {
	t, e := randomToken()
	if e != nil {
		internal(w)
		return
	}
	encrypted, encErr := a.encrypt([]byte(t))
	if encErr != nil {
		internal(w)
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		internal(w)
		return
	}
	var result sql.Result
	result, e = tx.Exec(`UPDATE output_settings SET token_hash=?,encrypted_token=? WHERE id=1`, hash(t), encrypted)
	if e == nil {
		var affected int64
		affected, e = result.RowsAffected()
		if e == nil && affected != 1 {
			e = errors.New("output settings row missing")
		}
	}
	if e == nil {
		result, e = tx.Exec(`UPDATE output_links SET token_hash=?,encrypted_token=? WHERE id='default'`, hash(t), encrypted)
	}
	if e == nil {
		var affected int64
		affected, e = result.RowsAffected()
		if e == nil && affected != 1 {
			e = errors.New("default output link missing")
		}
	}
	if e != nil {
		_ = tx.Rollback()
		internal(w)
		return
	}
	if e = tx.Commit(); e != nil {
		internal(w)
		return
	}
	var public string
	_ = a.db.QueryRow(`SELECT public_url FROM output_settings WHERE id=1`).Scan(&public)
	jsonResponse(w, 200, map[string]interface{}{"url": a.linkURL(public, t), "rotatedAt": stamp(time.Now().UTC())})
}
func (a *App) subscription(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		method(w)
		return
	}
	token := strings.TrimPrefix(r.URL.Path, "/s/")
	if token == "" || strings.Contains(token, "/") {
		apiError(w, 404, "not_found", "Subscription not found")
		return
	}
	var want string
	var config []byte
	var enabled int
	err := a.db.QueryRow(`SELECT token_hash,enabled FROM output_links WHERE token_hash=?`, hash(token)).Scan(&want, &enabled)
	if err != nil || enabled == 0 || !constantEqual(want, hash(token)) {
		apiError(w, 404, "not_found", "Subscription not found")
		return
	}
	if err := a.db.QueryRow(`SELECT config FROM output_settings WHERE id=1`).Scan(&config); err != nil {
		apiError(w, 404, "not_found", "Subscription not found")
		return
	}
	if len(config) == 0 {
		apiError(w, 503, "output_unavailable", "No valid output is available")
		return
	}
	w.Header().Set("Content-Type", "application/yaml; charset=utf-8")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(200)
	_, _ = w.Write(config)
}
func (a *App) exists(id string) bool {
	var n int
	return a.db.QueryRow(`SELECT count(*) FROM subscriptions WHERE id=?`, id).Scan(&n) == nil && n > 0
}
func (a *App) allNodes(enabledOnly bool) ([]Node, error) {
	query := `SELECT s.id,s.name,x.nodes_json FROM subscriptions s JOIN snapshots x ON x.subscription_id=s.id`
	if enabledOnly {
		query += ` WHERE s.enabled=1`
	}
	query += ` ORDER BY s.created_at,s.id`
	rows, err := a.db.Query(query)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Node{}
	for rows.Next() {
		var sid, sourceName string
		var data []byte
		if err := rows.Scan(&sid, &sourceName, &data); err != nil {
			return nil, err
		}
		var stored []snapshotNode
		if json.Unmarshal(data, &stored) != nil {
			continue
		}
		for _, item := range stored {
			node := item.Node
			node.Raw = item.Proxy
			out = append(out, node)
		}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	used := map[string]bool{}
	for i := range out {
		name := out[i].Name
		if used[name] {
			suffix := out[i].SourceID
			if len(suffix) > 6 {
				suffix = suffix[:6]
			}
			base := fmt.Sprintf("%s [%s]", name, suffix)
			name = base
			for n := 2; used[name]; n++ {
				name = fmt.Sprintf("%s-%d", base, n)
			}
		}
		used[name] = true
		out[i].Name = name
		if out[i].Raw != nil {
			out[i].Raw["name"] = name
		}
	}
	return out, nil
}
func (a *App) refresh(id string) {
	if a.beginRefresh(id) {
		a.refreshStarted(id)
	}
}
func (a *App) beginRefresh(id string) bool {
	a.refreshMu.Lock()
	defer a.refreshMu.Unlock()
	if a.refreshing[id] || !a.exists(id) {
		return false
	}
	a.refreshing[id] = true
	return true
}
func (a *App) refreshStarted(id string) {
	defer func() { a.refreshMu.Lock(); delete(a.refreshing, id); a.refreshMu.Unlock() }()
	var name string
	var enc []byte
	err := a.db.QueryRow(`SELECT name,encrypted_url FROM subscriptions WHERE id=?`, id).Scan(&name, &enc)
	if err != nil {
		return
	}
	now := time.Now().UTC()
	_, _ = a.db.Exec(`UPDATE subscriptions SET last_attempt_at=?,status=CASE WHEN status='healthy' THEN 'degraded' ELSE status END WHERE id=?`, stamp(now), id)
	plain, err := a.decrypt(enc)
	if err != nil {
		a.refreshFailed(id, "stored_secret_invalid")
		return
	}
	u, e := url.Parse(string(plain))
	if e != nil || validateURL(u) != nil {
		a.refreshFailed(id, "blocked_url")
		return
	}
	req, e := http.NewRequestWithContext(context.Background(), "GET", u.String(), nil)
	if e != nil {
		a.refreshFailed(id, "invalid_url")
		return
	}
	req.Header.Set("User-Agent", "clash.meta")
	resp, e := a.client.Do(req)
	if e != nil {
		a.refreshFailed(id, safeFetchCode(e))
		return
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		a.refreshFailed(id, "upstream_http_error")
		return
	}
	body, e := io.ReadAll(io.LimitReader(resp.Body, maxFetchBytes+1))
	if e != nil || len(body) > maxFetchBytes {
		a.refreshFailed(id, "response_too_large")
		return
	}
	nodes, e := parseNodes(body, id, name)
	if e != nil {
		code := "invalid_subscription"
		var parseErr *parseCodeError
		if errors.As(e, &parseErr) && (parseErr.code == "unsupported_format" || parseErr.code == "invalid_subscription") {
			code = parseErr.code
		}
		a.refreshFailed(id, code)
		return
	}
	if len(nodes) == 0 {
		a.refreshFailed(id, "no_nodes")
		return
	}
	stored := make([]snapshotNode, 0, len(nodes))
	for _, node := range nodes {
		stored = append(stored, snapshotNode{Node: node, Proxy: node.Raw})
	}
	encoded, e := json.Marshal(stored)
	if e != nil {
		a.refreshFailed(id, "invalid_subscription")
		return
	}
	tx, e := a.db.Begin()
	if e != nil {
		a.refreshFailed(id, "storage_error")
		return
	}
	_, e = tx.Exec(`INSERT INTO snapshots(subscription_id,nodes_json,updated_at) VALUES(?,?,?) ON CONFLICT(subscription_id) DO UPDATE SET nodes_json=excluded.nodes_json,updated_at=excluded.updated_at`, id, encoded, stamp(now))
	if e == nil {
		_, e = tx.Exec(`UPDATE subscriptions SET node_count=?,last_success_at=?,status='healthy',error_code='',next_refresh_at=? WHERE id=?`, len(nodes), stamp(now), stamp(now.Add(time.Duration(a.interval(id))*time.Second)), id)
	}
	if e != nil {
		tx.Rollback()
		a.refreshFailed(id, "storage_error")
		return
	}
	if tx.Commit() != nil {
		a.refreshFailed(id, "storage_error")
		return
	}
	if e = a.regenerate(); e != nil {
		_, _ = a.db.Exec(`UPDATE subscriptions SET status='degraded',error_code='output_generation_failed' WHERE id=?`, id)
	}
}
func safeFetchCode(e error) string {
	s := strings.ToLower(e.Error())
	switch {
	case strings.Contains(s, "blocked_address"):
		return "blocked_address"
	case strings.Contains(s, "dns"):
		return "dns_error"
	case strings.Contains(s, "timeout"):
		return "upstream_timeout"
	case strings.Contains(s, "redirect"):
		return "redirect_rejected"
	default:
		return "upstream_error"
	}
}
func (a *App) refreshFailed(id, code string) {
	_, _ = a.db.Exec(`UPDATE subscriptions SET status='failed',error_code=?,next_refresh_at=? WHERE id=?`, code, stamp(time.Now().UTC().Add(time.Minute)), id)
}
func (a *App) interval(id string) int {
	var n int
	_ = a.db.QueryRow(`SELECT refresh_interval FROM subscriptions WHERE id=?`, id).Scan(&n)
	if n < 60 {
		n = defaultEvery
	}
	return n
}
func (a *App) scheduler() {
	t := time.NewTicker(30 * time.Second)
	defer t.Stop()
	for range t.C {
		rows, e := a.db.Query(`SELECT id FROM subscriptions WHERE enabled=1 AND next_refresh_at<=?`, stamp(time.Now().UTC()))
		if e != nil {
			continue
		}
		ids := []string{}
		for rows.Next() {
			var id string
			if rows.Scan(&id) == nil {
				ids = append(ids, id)
			}
		}
		rows.Close()
		for _, id := range ids {
			go a.refresh(id)
		}
	}
}
func parseNodes(body []byte, sourceID, sourceName string) ([]Node, error) {
	if proxies, handled, err := parseURIList(body); handled {
		if err != nil {
			return nil, err
		}
		return nodesFromProxyMaps(proxies, sourceID, sourceName)
	}

	candidates := [][]byte{body}
	if decoded := decodeSubscriptionBase64(body); len(decoded) > 0 && !bytes.Equal(decoded, body) {
		candidates = append(candidates, decoded)
	}
	for _, data := range candidates {
		var doc map[string]interface{}
		if err := yaml.Unmarshal(data, &doc); err != nil || doc == nil {
			continue
		}
		value, exists := doc["proxies"]
		if !exists {
			continue
		}
		list, ok := value.([]interface{})
		if !ok || len(list) == 0 || len(list) > maxNodes {
			return nil, parseFailure("invalid_subscription")
		}
		proxies := make([]map[string]interface{}, 0, len(list))
		for _, item := range list {
			raw, ok := item.(map[string]interface{})
			if !ok {
				return nil, parseFailure("invalid_subscription")
			}
			name, nameOK := raw["name"].(string)
			typ, typeOK := raw["type"].(string)
			if !nameOK || strings.TrimSpace(name) == "" || len(name) > maxNameLength || !typeOK || strings.TrimSpace(typ) == "" || len(typ) > 40 {
				return nil, parseFailure("invalid_subscription")
			}
			proxies = append(proxies, raw)
		}
		return nodesFromProxyMaps(proxies, sourceID, sourceName)
	}

	if len(candidates) > 1 {
		if proxies, handled, err := parseURIList(candidates[1]); handled {
			if err != nil {
				return nil, err
			}
			return nodesFromProxyMaps(proxies, sourceID, sourceName)
		}
	}
	return nil, parseFailure("invalid_subscription")
}

func nodesFromProxyMaps(proxies []map[string]interface{}, sourceID, sourceName string) ([]Node, error) {
	if len(proxies) == 0 || len(proxies) > maxNodes {
		return nil, parseFailure("invalid_subscription")
	}
	out := make([]Node, 0, len(proxies))
	seen := map[string]int{}
	for _, raw := range proxies {
		name, ok := raw["name"].(string)
		if !ok || strings.TrimSpace(name) == "" || len(name) > maxNameLength {
			return nil, parseFailure("invalid_subscription")
		}
		typ, ok := raw["type"].(string)
		if !ok || strings.TrimSpace(typ) == "" || len(typ) > 40 {
			return nil, parseFailure("invalid_subscription")
		}
		base := strings.TrimSpace(sourceName) + " / " + strings.TrimSpace(name)
		seen[base]++
		mergedName := base
		if seen[base] > 1 {
			mergedName = fmt.Sprintf("%s (%d)", base, seen[base])
		}
		raw["name"] = mergedName
		buf, err := json.Marshal(raw)
		if err != nil {
			return nil, parseFailure("invalid_subscription")
		}
		sum := sha256.Sum256(append([]byte(sourceID+"\x00"), buf...))
		out = append(out, Node{ID: hex.EncodeToString(sum[:12]), Name: mergedName, Type: typ, SourceID: sourceID, SourceName: sourceName, Raw: raw})
	}
	return out, nil
}

func (a *App) regenerate() error {
	nodes, err := a.allNodes(true)
	if err != nil {
		return err
	}
	proxies := make([]map[string]interface{}, 0, len(nodes))
	names := make([]string, 0, len(nodes))
	for _, node := range nodes {
		if node.Raw != nil {
			proxies = append(proxies, node.Raw)
			names = append(names, node.Name)
		}
	}
	group := map[string]interface{}{"name": "PROXY", "type": "select", "proxies": append(names, "DIRECT")}
	config := map[string]interface{}{"mixed-port": 7890, "allow-lan": false, "mode": "rule", "log-level": "info", "proxies": proxies, "proxy-groups": []interface{}{group}, "rules": []string{"MATCH,PROXY"}}
	publicConfig, err := yaml.Marshal(config)
	if err != nil {
		return err
	}
	var old []byte
	_ = a.db.QueryRow(`SELECT config FROM output_settings WHERE id=1`).Scan(&old)
	if bytes.Equal(old, publicConfig) {
		return nil
	}
	_, err = a.db.Exec(`UPDATE output_settings SET config=?,generated_at=?,revision=revision+1 WHERE id=1`, publicConfig, stamp(time.Now().UTC()))
	return err
}

func stamp(t time.Time) string { return t.UTC().Format(time.RFC3339Nano) }
func parseInt(s string, d int) int {
	if s == "" {
		return d
	}
	n, e := strconv.Atoi(s)
	if e != nil {
		return d
	}
	return n
}
func newID() (string, error) {
	b := make([]byte, 16)
	if _, e := rand.Read(b); e != nil {
		return "", e
	}
	b[6] = (b[6] & 0x0f) | 0x40
	b[8] = (b[8] & 0x3f) | 0x80
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16]), nil
}
func decode(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	r.Body = http.MaxBytesReader(w, r.Body, 1<<20)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if d.Decode(v) != nil {
		apiError(w, 400, "invalid_json", "Invalid JSON request")
		return false
	}
	return true
}
func jsonResponse(w http.ResponseWriter, status int, v interface{}) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
func apiError(w http.ResponseWriter, status int, code, message string) {
	jsonResponse(w, status, map[string]interface{}{"error": map[string]string{"code": code, "message": message}})
}
func internal(w http.ResponseWriter) { apiError(w, 500, "internal_error", "Internal server error") }
func method(w http.ResponseWriter) {
	w.Header().Set("Allow", "GET, POST, PATCH, DELETE")
	apiError(w, 405, "method_not_allowed", "Method not allowed")
}
func accessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		next.ServeHTTP(w, r)
		if !strings.HasPrefix(r.URL.Path, "/s/") {
			log.Printf("method=%s path=%s elapsed_ms=%d", r.Method, path.Clean(r.URL.Path), time.Since(start).Milliseconds())
		}
	})
}
func constantEqual(a, b string) bool {
	if len(a) != len(b) {
		return false
	}
	var x byte
	for i := range a {
		x |= a[i] ^ b[i]
	}
	return x == 0
}
