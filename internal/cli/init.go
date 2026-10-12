package cli

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"strings"
)

func InitConfig() error {
	if e := os.MkdirAll(".secrets", 0700); e != nil {
		return e
	}
	for _, name := range []string{"db_owner_password", "db_app_password", "agent_token", "read_token", "codex_token", "claude_token", "opencode_token"} {
		p := filepath.Join(".secrets", name)
		if b, e := os.ReadFile(p); e == nil {
			if len(b) < 32 {
				return errors.New("existing secret is invalid; refusing automatic replacement")
			}
			continue
		} else if !os.IsNotExist(e) {
			return e
		}
		var b [32]byte
		if _, e := rand.Read(b[:]); e != nil {
			return e
		}
		if e := exclusive(p, []byte(hex.EncodeToString(b[:]))); e != nil {
			return e
		}
	}
	var ps []map[string]any
	path := ".secrets/tokens.json"
	if b, e := os.ReadFile(path); e == nil {
		if e = json.Unmarshal(b, &ps); e != nil {
			return errors.New("invalid existing tokens.json")
		}
	} else if !os.IsNotExist(e) {
		return e
	}
	changed := false
	for _, item := range []struct {
		id, file string
		write    bool
	}{{"local-agent", "agent_token", true}, {"local-reader", "read_token", false}, {"codex", "codex_token", true}, {"claude", "claude_token", true}, {"opencode", "opencode_token", true}} {
		found := false
		for _, p := range ps {
			if p["id"] == item.id {
				found = true
			}
		}
		if found {
			continue
		}
		token, e := os.ReadFile(filepath.Join(".secrets", item.file))
		if e != nil {
			return e
		}
		ps = append(ps, map[string]any{"id": item.id, "token": string(token), "write": item.write, "projects": []string{"*"}})
		changed = true
	}
	if changed {
		if e := save(path, jsonBytes(ps)); e != nil {
			return e
		}
	}
	image := os.Getenv("THOTH_IMAGE")
	if image == "" {
		image = "lord1egypt/thoth-memory:0.1.1"
	}
	port := os.Getenv("THOTH_PORT")
	if port == "" {
		port = "7319"
	}
	if b, e := os.ReadFile(".env"); e == nil {
		for _, line := range strings.Split(string(b), "\n") {
			if strings.HasPrefix(line, "THOTH_PORT=") && os.Getenv("THOTH_PORT") == "" {
				port = strings.TrimPrefix(line, "THOTH_PORT=")
			}
		}
	}
	if _, e := net.LookupPort("tcp", port); e != nil {
		return errors.New("invalid THOTH_PORT")
	}
	if _, e := os.Stat(".env"); os.IsNotExist(e) {
		if e = exclusive(".env", []byte("THOTH_IMAGE="+image+"\nTHOTH_PORT="+port+"\nTHOTH_DATABASE=thoth\n")); e != nil {
			return e
		}
	} else if e != nil {
		return e
	}
	for _, item := range []struct{ path, token string }{{"client.json", "agent_token"}, {"client-codex.json", "codex_token"}, {"client-claude.json", "claude_token"}, {"client-opencode.json", "opencode_token"}} {
		if _, e := os.Stat(item.path); os.IsNotExist(e) {
			if e = exclusive(item.path, jsonBytes(ClientConfig{Endpoint: "http://127.0.0.1:" + port + "/mcp", TokenFile: ".secrets/" + item.token})); e != nil {
				return e
			}
		} else if e != nil {
			return e
		}
	}
	return nil
}
func exclusive(path string, b []byte) error {
	f, e := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0600)
	if e != nil {
		return e
	}
	_, e = f.Write(b)
	closeErr := f.Close()
	if e != nil {
		return e
	}
	return closeErr
}
