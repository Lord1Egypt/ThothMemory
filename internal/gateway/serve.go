package gateway

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"time"

	"github.com/Lord1Egypt/ThothMemory/internal/memory"
)

// LoadCredentials reads and validates the token policy file.
func LoadCredentials(path string) ([]memory.Principal, error) {
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, errors.New("THOTH_TOKENS_FILE required")
	}
	var ps []memory.Principal
	if e = json.Unmarshal(b, &ps); e != nil {
		return nil, errors.New("invalid token configuration")
	}
	return ps, memory.ValidateCredentials(ps)
}

// Serve registers principals and serves MCP and the dashboard until ctx ends.
func Serve(ctx context.Context, b memory.Backend, ps []memory.Principal, addr string) error {
	for _, p := range ps {
		if e := b.Register(ctx, p); e != nil {
			return e
		}
	}
	srv := &http.Server{Addr: addr, Handler: Handler(b, ps), ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 20 * time.Second, WriteTimeout: 30 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(c)
	}()
	info := b.Info()
	slog.Info(info.Product+" ready", "edition", info.Edition, "address", addr, "version", info.Version)
	e := srv.ListenAndServe()
	if errors.Is(e, http.ErrServerClosed) {
		return nil
	}
	return e
}
