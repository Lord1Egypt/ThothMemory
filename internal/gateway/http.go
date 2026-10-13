package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"github.com/Lord1Egypt/ThothMemory/internal/store"
	"github.com/Lord1Egypt/ThothMemory/internal/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

func Handler(s *store.Store, principals []store.Principal) http.Handler {
	handlers := make([]http.Handler, len(principals))
	for i, p := range principals {
		srv := Server(s, p)
		handlers[i] = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if s.Pool.Ping(ctx) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"status":"ready","version":"0.2.0-rc.1"}`))
	})
	mux.HandleFunc("/mcp", func(w http.ResponseWriter, r *http.Request) {
		i := authenticate(r, principals)
		if i < 0 {
			http.Error(w, "unauthorized", 401)
			return
		}
		handlers[i].ServeHTTP(w, r)
	})
	mux.HandleFunc("GET /api/read", func(w http.ResponseWriter, r *http.Request) {
		i := authenticate(r, principals)
		if i < 0 {
			http.Error(w, "unauthorized", 401)
			return
		}
		tool := r.URL.Query().Get("tool")
		if store.IsWrite(tool) {
			http.Error(w, "read-only dashboard", 403)
			return
		}
		a := store.Input{ProjectID: r.URL.Query().Get("project_id"), Query: r.URL.Query().Get("query"), Limit: 40}
		v, e := s.Call(r.Context(), principals[i], tool, a)
		if e != nil {
			http.Error(w, store.SafeError(e), 400)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": v})
	})
	mux.Handle("/", web.Handler())
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
		host := r.Host
		if h, _, e := net.SplitHostPort(host); e == nil {
			host = h
		}
		if host != "localhost" && host != "127.0.0.1" && host != "::1" {
			http.Error(w, "invalid host", 403)
			return
		}
		if o := r.Header.Get("Origin"); o != "" {
			u, e := url.Parse(o)
			if e != nil || u.Scheme != "http" || u.Host != r.Host {
				http.Error(w, "invalid origin", 403)
				return
			}
		}
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		mux.ServeHTTP(w, r)
	})
}
func authenticate(r *http.Request, ps []store.Principal) int {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return -1
	}
	for i, p := range ps {
		if subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) == 1 {
			return i
		}
	}
	return -1
}
