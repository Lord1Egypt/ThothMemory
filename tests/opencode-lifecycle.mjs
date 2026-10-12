// Run against an installed generated plugin in an isolated Git fixture.
import assert from 'node:assert/strict';
import { pathToFileURL } from 'node:url';
const [plugin, directory] = process.argv.slice(2);
const { ThothMemory } = await import(pathToFileURL(plugin).href);
const hooks = await ThothMemory({ directory });
const system = { system: [] };
await hooks['experimental.chat.system.transform']({}, system);
assert.match(system.system[0], /UNTRUSTED DATA/);
assert.match(system.system[0], /current_revision/);
assert.doesNotMatch(system.system[0], /project unregistered/);
await hooks.event({ event: { type: 'session.idle' } });
await hooks.event({ event: { type: 'session.error' } });
const compact = { context: [] };
await hooks['experimental.session.compacting']({}, compact);
assert.match(compact.context[0], /persist meaningful progress/);
console.log('OpenCode system context, idle/error metadata and compaction callback: PASS');
