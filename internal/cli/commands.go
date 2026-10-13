package cli

import (
	"context"
	"errors"
	"os"
	"strings"
)

// Secret reads NAME_FILE when set, otherwise NAME.
func Secret(name string) (string, error) {
	if p := os.Getenv(name + "_FILE"); p != "" {
		b, e := os.ReadFile(p)
		return strings.TrimSpace(string(b)), e
	}
	v := os.Getenv(name)
	if v == "" {
		return "", errors.New(name + " or " + name + "_FILE required")
	}
	return v, nil
}

// ClientCommand runs an edition-independent client-side command. It reports
// false when cmd is not one of them.
func ClientCommand(ctx context.Context, cmd, config string, rest []string) (bool, error) {
	switch cmd {
	case "permissions":
		return true, Permissions(rest)
	case "bridge":
		return true, Bridge(ctx, config)
	case "preflight":
		return true, Preflight(ctx, config)
	case "doctor":
		return true, Doctor(ctx, config)
	case "connect":
		return true, Connect(ctx, config, rest)
	case "hooks":
		return true, Hooks(ctx, config, rest)
	case "instructions":
		return true, Instructions(rest)
	case "lifecycle":
		return true, Lifecycle(ctx, config, rest)
	case "demo":
		return true, Demo(ctx, config)
	}
	return false, nil
}

// SplitConfig extracts --config PATH, defaulting to client.json.
func SplitConfig(args []string) (string, []string) {
	path := "client.json"
	rest := []string{}
	for i := 0; i < len(args); i++ {
		if args[i] == "--config" && i+1 < len(args) {
			i++
			path = args[i]
		} else {
			rest = append(rest, args[i])
		}
	}
	return path, rest
}
