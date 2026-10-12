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

func TestCodexQuotedAndInlineRegistration(t *testing.T) {
	for _, input := range []string{"# keep this comment\n[mcp_servers.\"thothmemory\"]\ncommand='thothmemory'\nargs=['bridge','--config','client.json']\nstartup_timeout_sec=30\n", "mcp_servers = {thothmemory = {command='thothmemory', args=['bridge','--config','client.json']}}\n"} {
		out, e := MergeCodex([]byte(input), "thothmemory", "client.json")
		if e != nil || string(out) != input {
			t.Fatal("existing TOML registration not preserved", e, string(out))
		}
	}
	original := "# preserve comment\nmodel='custom'\n[mcp_servers.other]\ncommand='other'\n"
	out, e := MergeCodex([]byte(original), "thothmemory", "client.json")
	if e != nil || !strings.HasPrefix(string(out), original) {
		t.Fatal(e)
	}
	again, e := MergeCodex(out, "thothmemory", "client.json")
	if e != nil || string(again) != string(out) {
		t.Fatal("not idempotent", e)
	}
	if _, e = MergeCodex(out, "different", "client.json"); e == nil {
		t.Fatal("conflicting registration overwritten")
	}
}

func TestCodexExtendsInlineOtherServer(t *testing.T) {
	out, e := MergeCodex([]byte("model='custom'\nmcp_servers={other={command='safe'}}\n"), "thothmemory", "client.json")
	if e != nil || !strings.Contains(string(out), "safe") || !strings.Contains(string(out), "thothmemory") || !strings.Contains(string(out), "custom") {
		t.Fatal(e, string(out))
	}
}
