package cli

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"

	"github.com/Lord1Egypt/ThothMemory/internal/portable"
)

// Portable runs the owner-only export and import commands. Export writes the
// document to stdout; import reads it from stdin and prints the result.
func Portable(ctx context.Context, b portable.Backend, cmd string, args []string) error {
	if cmd == "export" {
		if len(args) != 1 {
			return errors.New("export PROJECT_ID > project.thothmemory.json")
		}
		st, e := os.Stdout.Stat()
		if e == nil && st.Mode()&os.ModeCharDevice != 0 {
			return errors.New("redirect the export to a file")
		}
		doc, e := portable.Export(ctx, b, args[0], os.Stdout)
		if e == nil {
			fmt.Fprintf(os.Stderr, "Exported project %s: %v, payload sha256=%s\n", doc.ProjectID, doc.Counts, doc.PayloadSHA256)
		}
		return e
	}
	if len(args) != 0 {
		return errors.New("import < project.thothmemory.json")
	}
	doc, e := portable.Read(os.Stdin)
	if e != nil {
		return e
	}
	res, e := portable.Import(ctx, b, doc, "local-owner")
	if e != nil {
		return e
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(res)
}
