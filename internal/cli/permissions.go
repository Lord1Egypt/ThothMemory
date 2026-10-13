package cli

import (
	"encoding/json"
	"errors"
	"fmt"
	"github.com/Lord1Egypt/ThothMemory/internal/memory"
	"os"
	"strings"
)

// Permissions is a local owner action. It changes no database and never reloads a
// running service automatically. Existing tokens and unrelated policy fields survive.
func Permissions(args []string) error {
	if len(args) < 2 {
		return errors.New("permissions PRINCIPAL --projects UUID[,UUID]|* [--read-only|--write] [--yes]; or --revoke")
	}
	principal := args[0]
	projects := ""
	revoke, yes, read, write := false, false, false, false
	for i := 1; i < len(args); i++ {
		switch args[i] {
		case "--projects":
			i++
			if i >= len(args) {
				return errors.New("project allowlist required")
			}
			projects = args[i]
		case "--revoke":
			revoke = true
		case "--read-only":
			read = true
		case "--write":
			write = true
		case "--yes":
			yes = true
		default:
			return errors.New("unknown permissions option")
		}
	}
	if read && write || revoke && (projects != "" || read || write) || !revoke && projects == "" {
		return errors.New("choose revoke, or a project allowlist with optional access mode")
	}
	list := strings.Split(projects, ",")
	if !revoke {
		for _, project := range list {
			if !memory.UUID(project) && project != "*" {
				return errors.New("projects must be UUIDs or an explicit wildcard")
			}
		}
		if len(list) > 1 && strings.Contains(projects, "*") {
			return errors.New("wildcard must stand alone")
		}
	}
	path := ".secrets/tokens.json"
	b, e := os.ReadFile(path)
	if e != nil {
		return e
	}
	var policy []map[string]any
	if e = json.Unmarshal(b, &policy); e != nil || policy == nil {
		return errors.New("invalid credential policy")
	}
	out := []map[string]any{}
	found := false
	for _, p := range policy {
		if p["id"] == principal {
			found = true
			if revoke {
				continue
			}
			p["projects"] = list
			if read {
				p["write"] = false
			}
			if write {
				p["write"] = true
			}
		}
		out = append(out, p)
	}
	if !found {
		return errors.New("principal not found; no policy changed")
	}
	fmt.Printf("Principal: %s; revoke=%t; projects=%s; read-only=%t\n", principal, revoke, projects, read)
	if e = confirm([]string{path}, yes); e != nil {
		return e
	}
	if e = save(path, jsonBytes(out)); e != nil {
		return e
	}
	// File is bind-mounted for the non-root runtime; the containing directory is private.
	if e = privatePath(".secrets", true); e != nil {
		return e
	}
	if e = os.Chmod(path, 0644); e != nil {
		return e
	}
	fmt.Println("Policy saved; tokens unchanged. Reload requires owner action: docker compose up -d --no-deps --force-recreate app. Existing running requests retain their startup policy until then.")
	return nil
}
