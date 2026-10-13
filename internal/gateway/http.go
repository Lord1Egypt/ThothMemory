package gateway

import (
	"context"
	"crypto/subtle"
	"encoding/json"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"github.com/Lord1Egypt/ThothMemory/internal/web"
	"github.com/modelcontextprotocol/go-sdk/mcp"
)

// dashboardReads are the only operations /api/read executes: MCP read tools
// plus dashboard views. Writes are never reachable from the browser.
func dashboardRead(tool string) bool {
	if memory.DashboardViews[tool] {
		return true
	}
	for _, c := range Contracts {
		if c.Name == tool {
			return !memory.IsWrite(tool)
		}
	}
	return false
}

func Handler(b memory.Backend, principals []memory.Principal) http.Handler {
	handlers := make([]http.Handler, len(principals))
	for i, p := range principals {
		srv := Server(b, p)
		handlers[i] = mcp.NewStreamableHTTPHandler(func(*http.Request) *mcp.Server { return srv }, &mcp.StreamableHTTPOptions{Stateless: true, JSONResponse: true})
	}
	info := b.Info()
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if b.Ping(ctx) != nil {
			http.Error(w, "unavailable", 503)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ready", "version": info.Version, "edition": info.Edition})
	})
	mux.HandleFunc("GET /api/info", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(info)
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
		q := r.URL.Query()
		tool := q.Get("tool")
		if !dashboardRead(tool) {
			http.Error(w, "read-only dashboard: unknown or write operation", 403)
			return
		}
		a := memory.Input{ProjectID: q.Get("project_id"), Query: q.Get("query"), TaskID: q.Get("task_id"), State: q.Get("state"), Kind: q.Get("kind"), ExactID: q.Get("exact_id"), GitRevision: q.Get("git_revision"), Limit: 40}
		for _, f := range []struct {
			name string
			v    *int
		}{{"limit", &a.Limit}, {"offset", &a.Offset}} {
			if s := q.Get(f.name); s != "" {
				n, e := strconv.Atoi(s)
				if e != nil {
					http.Error(w, "invalid "+f.name, 400)
					return
				}
				*f.v = n
			}
		}
		ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
		defer cancel()
		v, e := memory.Call(ctx, b, principals[i], tool, a)
		if e != nil {
			http.Error(w, memory.SafeError(e), 400)
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
		w.Header().Set("Content-Security-Policy", "default-src 'self'; script-src 'self'; style-src 'self'; connect-src 'self'; img-src 'self'; frame-ancestors 'none'; base-uri 'none'; form-action 'none'")
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

func authenticate(r *http.Request, ps []memory.Principal) int {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	if token == "" {
		return -1
	}
	found := -1
	for i, p := range ps {
		if subtle.ConstantTimeCompare([]byte(token), []byte(p.Token)) == 1 {
			found = i
		}
	}
	return found
}
