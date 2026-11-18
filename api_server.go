package main

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/tls"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"
)

type inventoryServer struct {
	db *sql.DB
	mu sync.Mutex
}
type serverPrincipal struct{ User, Token string }
type serverToken struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Expires int64  `json:"expires"`
	Secret  string `json:"secret,omitempty"`
}

func openInventoryServer(dir string) (*inventoryServer, error) {
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, err
	}
	path := filepath.Join(dir, "server.db")
	f, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0600)
	if err != nil {
		return nil, err
	}
	f.Close()
	u := sqliteFileURL(path)
	u.RawQuery = "_pragma=busy_timeout(5000)"
	db, err := sql.Open("sqlite", u.String())
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, stmt := range []string{
		`CREATE TABLE IF NOT EXISTS users(id TEXT PRIMARY KEY,name TEXT UNIQUE NOT NULL,disabled INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS tokens(id TEXT PRIMARY KEY,user_id TEXT NOT NULL,name TEXT NOT NULL,hash TEXT UNIQUE NOT NULL,expires INTEGER NOT NULL,revoked INTEGER NOT NULL DEFAULT 0)`,
		`CREATE TABLE IF NOT EXISTS inventories(id TEXT PRIMARY KEY,name TEXT UNIQUE NOT NULL,path TEXT UNIQUE NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS grants(user_id TEXT NOT NULL,inventory_id TEXT NOT NULL,access TEXT NOT NULL,PRIMARY KEY(user_id,inventory_id))`,
	} {
		if _, err = db.Exec(stmt); err != nil {
			db.Close()
			return nil, err
		}
	}
	return &inventoryServer{db: db}, nil
}

func tokenDigest(token string) string {
	sum := sha256.Sum256([]byte(token))
	return hex.EncodeToString(sum[:])
}
func (s *inventoryServer) issueToken(user, name string, days int) (serverToken, error) {
	if !inventoryName(name) || days < 1 || days > 365 {
		return serverToken{}, fmt.Errorf("token name and expiry (1..365 days) required")
	}
	var disabled int
	if err := s.db.QueryRow("SELECT disabled FROM users WHERE id=?", user).Scan(&disabled); err != nil || disabled != 0 {
		return serverToken{}, fmt.Errorf("user unavailable")
	}
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return serverToken{}, err
	}
	v := serverToken{ID: inventoryID(), Name: name, Expires: time.Now().Add(time.Duration(days) * 24 * time.Hour).Unix(), Secret: hex.EncodeToString(b)}
	_, err := s.db.Exec("INSERT INTO tokens(id,user_id,name,hash,expires) VALUES(?,?,?,?,?)", v.ID, user, name, tokenDigest(v.Secret), v.Expires)
	return v, err
}

func (s *inventoryServer) authenticate(r *http.Request) (serverPrincipal, error) {
	value := r.Header.Get("Authorization")
	if !strings.HasPrefix(value, "Bearer ") || len(value) != 7+64 {
		return serverPrincipal{}, fmt.Errorf("unauthorized")
	}
	var p serverPrincipal
	err := s.db.QueryRow(`SELECT t.user_id,t.id FROM tokens t JOIN users u ON u.id=t.user_id WHERE t.hash=? AND t.revoked=0 AND t.expires>? AND u.disabled=0`, tokenDigest(value[7:]), time.Now().Unix()).Scan(&p.User, &p.Token)
	return p, err
}

func apiRespond(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
func apiError(w http.ResponseWriter, status int, message string) {
	apiRespond(w, status, map[string]string{"error": message})
}
func decodeAPI(w http.ResponseWriter, r *http.Request, out any) error {
	r.Body = http.MaxBytesReader(w, r.Body, apiBodyLimit)
	d := json.NewDecoder(r.Body)
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		return fmt.Errorf("one JSON object required")
	}
	return nil
}

func (s *inventoryServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("X-Content-Type-Options", "nosniff")
	// ponytail: one lock serializes API writes and token revocation across this
	// small server; per-inventory locks if concurrency becomes measurable.
	s.mu.Lock()
	defer s.mu.Unlock()
	p, err := s.authenticate(r)
	if err != nil {
		apiError(w, 401, "unauthorized")
		return
	}
	path := strings.TrimPrefix(r.URL.Path, "/v1/")
	if r.URL.Path == "/v1/tokens" || strings.HasPrefix(r.URL.Path, "/v1/tokens/") {
		s.serveTokens(w, r, p, path)
		return
	}
	if path == "inventories" && r.Method == "GET" {
		rows, e := s.db.Query(`SELECT i.id,i.name,g.access FROM inventories i JOIN grants g ON g.inventory_id=i.id WHERE g.user_id=? ORDER BY i.name`, p.User)
		if e != nil {
			apiError(w, 500, "inventory discovery failed")
			return
		}
		defer rows.Close()
		values := []InventorySource{}
		for rows.Next() {
			var v InventorySource
			var access string
			if e = rows.Scan(&v.ID, &v.Name, &access); e != nil {
				break
			}
			v.Type = "api"
			v.ReadOnly = access != "read-write"
			values = append(values, v)
		}
		if e != nil || rows.Err() != nil {
			apiError(w, 500, "inventory discovery failed")
			return
		}
		apiRespond(w, 200, values)
		return
	}
	if !strings.HasPrefix(path, "inventories/") {
		apiError(w, 404, "not found")
		return
	}
	id := strings.TrimPrefix(path, "inventories/")
	if !inventoryName(id) {
		apiError(w, 404, "not found")
		return
	}
	var source InventorySource
	var access string
	err = s.db.QueryRow(`SELECT i.id,i.name,i.path,g.access FROM inventories i JOIN grants g ON g.inventory_id=i.id WHERE i.id=? AND g.user_id=?`, id, p.User).Scan(&source.ID, &source.Name, &source.Path, &access)
	if err != nil {
		apiError(w, 403, "inventory access denied")
		return
	}
	source.Type = "sqlite"
	if r.Method != "GET" && r.Method != "POST" {
		apiError(w, 405, "method not allowed")
		return
	}
	if r.Method == "POST" && access != "read-write" {
		apiError(w, 403, "read-only inventory")
		return
	}
	v, err := localAPISnapshot(source)
	if err != nil {
		apiError(w, 500, "inventory cannot be read")
		return
	}
	if r.Method == "GET" {
		v.ReadOnly = access != "read-write"
		w.Header().Set("ETag", v.Revision)
		apiRespond(w, 200, v)
		return
	}
	if r.Header.Get("If-Match") == "" || r.Header.Get("If-Match") != v.Revision {
		apiError(w, 412, "inventory revision changed")
		return
	}
	var m apiMutation
	if decodeAPI(w, r, &m) != nil {
		apiError(w, 400, "invalid inventory request")
		return
	}
	if err = applyAPIMutation(source, m); err != nil {
		apiError(w, 409, "inventory mutation rejected: "+safeAPIError(err))
		return
	}
	apiRespond(w, 200, map[string]bool{"saved": true})
}

func safeAPIError(err error) string {
	// Keep storage paths and SQLite implementation details out of remote errors.
	message := err.Error()
	if strings.Contains(message, "SQL") || strings.ContainsAny(message, "/\\") {
		return "validation or reference check failed"
	}
	return clean(message)
}

func (s *inventoryServer) serveTokens(w http.ResponseWriter, r *http.Request, p serverPrincipal, path string) {
	if path == "tokens" {
		switch r.Method {
		case "GET":
			rows, err := s.db.Query("SELECT id,name,expires FROM tokens WHERE user_id=? AND revoked=0 ORDER BY name", p.User)
			if err != nil {
				apiError(w, 500, "token list failed")
				return
			}
			defer rows.Close()
			values := []serverToken{}
			for rows.Next() {
				var v serverToken
				if err = rows.Scan(&v.ID, &v.Name, &v.Expires); err != nil {
					apiError(w, 500, "token list failed")
					return
				}
				values = append(values, v)
			}
			if rows.Err() != nil {
				apiError(w, 500, "token list failed")
				return
			}
			apiRespond(w, 200, values)
		case "POST":
			var request struct {
				Name string `json:"name"`
				Days int    `json:"days"`
			}
			if decodeAPI(w, r, &request) != nil {
				apiError(w, 400, "invalid token request")
				return
			}
			var count int
			_ = s.db.QueryRow("SELECT count(*) FROM tokens WHERE user_id=? AND revoked=0 AND expires>?", p.User, time.Now().Unix()).Scan(&count)
			if count >= 20 {
				apiError(w, 409, "revoke an old token before creating more")
				return
			}
			v, err := s.issueToken(p.User, request.Name, request.Days)
			if err != nil {
				apiError(w, 400, "invalid token name or expiry")
				return
			}
			apiRespond(w, 201, v)
		default:
			apiError(w, 405, "method not allowed")
		}
		return
	}
	if r.Method != "DELETE" {
		apiError(w, 405, "method not allowed")
		return
	}
	id := strings.TrimPrefix(path, "tokens/")
	if !inventoryName(id) {
		apiError(w, 404, "not found")
		return
	}
	if id == "current" {
		id = p.Token
	}
	_, err := s.db.Exec("UPDATE tokens SET revoked=1 WHERE id=? AND user_id=?", id, p.User)
	if err != nil {
		apiError(w, 500, "revoke failed")
		return
	}
	apiRespond(w, 200, map[string]bool{"revoked": true})
}

func (s *inventoryServer) serve(address, cert, key string) error {
	if (cert == "") != (key == "") {
		return fmt.Errorf("TLS certificate and key must be supplied together")
	}
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return err
	}
	if cert == "" || key == "" {
		ip := net.ParseIP(host)
		if host != "localhost" && (ip == nil || !ip.IsLoopback()) {
			return fmt.Errorf("plaintext server must bind loopback behind an HTTPS reverse proxy; otherwise supply --tls-cert and --tls-key")
		}
	}
	server := &http.Server{Addr: address, Handler: s, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10, TLSConfig: &tls.Config{MinVersion: tls.VersionTLS12}}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	done := make(chan error, 1)
	go func() {
		if cert != "" {
			done <- server.ListenAndServeTLS(cert, key)
		} else {
			done <- server.ListenAndServe()
		}
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return err
		}
		return nil
	}
}
