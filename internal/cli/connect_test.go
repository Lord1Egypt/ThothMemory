package cli

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestOpenCodeSchemaPreservesSettings(t *testing.T) {
	for _, tc := range []struct{ name, input, schema string }{{"legacy", `{"model":"custom/model","mcp":{"other":{"type":"local","command":["safe"]}}}`, "legacy"}, {"v2", `{"providers":{"local":{}},"mcp":{"servers":{"other":{"type":"remote","url":"http://localhost:123"}}}}`, "v2"}, {"jsonc", "{ // comment\n \"model\":\"https://example.com/a//b\",\n \"mcp\":{},\n}", "legacy"}} {
		t.Run(tc.name, func(t *testing.T) {
			m, e := JSONC([]byte(tc.input))
			if e != nil {
				t.Fatal(e)
			}
			schema, e := MergeOpenCode(m, []string{"thothmemory", "bridge"})
			if e != nil || schema != tc.schema {
				t.Fatal(schema, e)
			}
			b, _ := json.Marshal(m)
			if !strings.Contains(string(b), "thothmemory") {
				t.Fatal(string(b))
			}
			if _, e = MergeOpenCode(m, []string{"thothmemory", "bridge"}); e != nil {
				t.Fatal("not idempotent", e)
			}
			if _, e = MergeOpenCode(m, []string{"different"}); e == nil {
				t.Fatal("overwrote conflicting registration")
			}
		})
	}
}
func TestJSONCDoesNotCorruptStrings(t *testing.T) {
	m, e := JSONC([]byte(`{"url":"https://host/path/*a*/","quoted":"a\"//b",/* hi */"tail":[1,2,],}`))
	if e != nil {
		t.Fatal(e)
	}
	if m["url"] != "https://host/path/*a*/" {
		t.Fatal(m)
	}
}
