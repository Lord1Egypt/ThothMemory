package cli

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

func TestOpenCodeLifecycleFailureReported(t *testing.T) {
	node, err := exec.LookPath("node")
	if err != nil {
		t.Skip("Node unavailable; generated OpenCode plugin failure callback NOT executed")
	}
	root := t.TempDir()
	plugin := filepath.Join(root, "plugin.mjs")
	if err = os.WriteFile(plugin, []byte(OpenCodePlugin(filepath.Join(root, "missing-binary"), "synthetic-config.json")), 0600); err != nil {
		t.Fatal(err)
	}
	harness := `import { pathToFileURL } from 'node:url';
import assert from 'node:assert/strict';
const {ThothMemory}=await import(pathToFileURL(process.argv[1]).href);
const logs=[];
const hooks=await ThothMemory({directory:process.argv[2],client:{app:{log:async item=>logs.push(item)}}});
await hooks.event({event:{type:'session.idle'}});
assert.equal(logs.length,1,'failed lifecycle metadata write was silently ignored');
assert.equal(logs[0].body.level,'warn');
assert.match(logs[0].body.message,/unsynced/i);
assert.doesNotMatch(JSON.stringify(logs),/missing-binary|synthetic-config/);
console.log('OpenCode failed lifecycle callback reports sanitized unsynced warning: PASS');`
	out, err := exec.Command(node, "--input-type=module", "-e", harness, plugin, root).CombinedOutput()
	if err != nil {
		t.Fatalf("plugin failure reporting: %v\n%s", err, out)
	}
	t.Log(string(out))
}
