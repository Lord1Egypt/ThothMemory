'use strict';
// ThothMemory dashboard. Read-only: it calls GET /api/read only. Every value
// from memory is untrusted and is rendered as text, never as HTML.

const UUID = /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i;
const SHA = /^([0-9a-f]{40}|[0-9a-f]{64})$/i;
const state = { token: '', info: null, overview: null, filter: 'all', text: '', head: {} };
const $ = id => document.getElementById(id);

function h(tag, attrs, ...children) {
  const el = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs || {})) {
    if (v === false || v == null) continue;
    if (k === 'class') el.className = v;
    else if (k === 'on') for (const [ev, fn] of Object.entries(v)) el.addEventListener(ev, fn);
    else el.setAttribute(k, v === true ? '' : v);
  }
  for (const c of children.flat()) if (c != null && c !== false) el.append(c instanceof Node ? c : String(c));
  return el;
}

// ---------- service access ----------
async function api(tool, params = {}) {
  const q = new URLSearchParams({ tool });
  for (const [k, v] of Object.entries(params)) if (v !== '' && v != null) q.set(k, v);
  const r = await fetch('/api/read?' + q, { headers: { Authorization: 'Bearer ' + state.token } });
  if (r.status === 401) { lock('That token is not accepted. Paste a current access token.'); throw new Error('Not authorized'); }
  if (!r.ok) throw new Error((await r.text()).trim() || 'Request failed (' + r.status + ')');
  return (await r.json()).data;
}
async function overview(force) {
  if (!state.overview || force) state.overview = await api('overview');
  return state.overview;
}

// ---------- formatting ----------
const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: 'auto' });
function ago(ts) {
  if (!ts) return 'never';
  const s = (Date.parse(ts) - Date.now()) / 1000;
  for (const [unit, size] of [['year', 31536000], ['month', 2592000], ['week', 604800], ['day', 86400], ['hour', 3600], ['minute', 60]]) {
    if (Math.abs(s) >= size) return rtf.format(Math.round(s / size), unit);
  }
  return 'just now';
}
function time(ts) { return ts ? h('time', { datetime: ts, title: new Date(ts).toLocaleString() }, ago(ts)) : 'never'; }
function sha(rev) { return rev ? h('code', { class: 'sha', title: rev }, rev.slice(0, 10)) : null; }
function seal(kind, text, title) { return h('span', { class: 'seal ' + kind, title: title || null }, text); }
const KIND = { decision: 'Decision', issue: 'Issue', observation: 'Observation', handoff: 'Handoff', event: 'Activity', evidence: 'Evidence', task: 'Task' };
const ACTION = { create_task: 'Task created', claim_task: 'Task claimed', release_task: 'Claim released', update_task: 'Task updated', start_session: 'Session started', checkpoint_session: 'Session checkpoint', finish_session: 'Session finished', append_event: 'Progress note', record_decision: 'Decision recorded', record_issue: 'Issue recorded', record_observation: 'Observation recorded', attach_evidence: 'Evidence reported', record_test_result: 'Test result reported', attest_evidence: 'Evidence attested by owner' };
const STATE = { planned: 'Planned', active: 'Active', blocked: 'Blocked', implemented: 'Implemented', committed: 'Committed', tested: 'Tested', verified: 'Verified', released: 'Released' };

function toast(msg) {
  const t = $('toast'); t.textContent = msg; t.classList.add('show');
  clearTimeout(toast.t); toast.t = setTimeout(() => t.classList.remove('show'), 2200);
}
async function copy(text, done) {
  try { await navigator.clipboard.writeText(text); toast(done); }
  catch { toast('Copy is unavailable here; select the text instead.'); }
}

// Trust seals: what a record is allowed to claim.
function trust(row) {
  const out = [];
  if (row.source_kind || row.kind === 'evidence') {
    if (row.owner_attested === true) out.push(seal('attested', 'Owner attested', 'An owner reviewed this evidence. It still applies only to its recorded revision.'));
    else out.push(seal('reported', 'Reported', 'Reported by an agent; not independently verified.'));
    if (row.outcome === 'fail') out.push(seal('fail', 'Failed'));
  } else if (row.kind && row.kind !== 'task') {
    out.push(seal('reported', 'Reported', 'Written by an agent; not independently verified.'));
  }
  if (row.status === 'incomplete') out.push(seal('open', 'Unfinished', 'No final handoff was written for this session.'));
  if (row.status === 'finished') out.push(seal('closed', 'Finished'));
  if (row.stale === true) out.push(seal('stale', 'Different revision', 'Recorded at a different commit than the HEAD you entered.'));
  if (row.superseded === true) out.push(seal('superseded', 'Superseded'));
  return out;
}

function record(row, project) {
  const isTask = row.kind === 'task' || (row.state && !row.kind && !row.source_kind);
  const isEvent = row.kind === 'event' || (ACTION[row.kind] && !row.title);
  const sessionRecord = row.session_id && row.title === 'Session checkpoint';
  let title = row.title || row.locator || KIND[row.kind] || 'Record';
  let kind = row.source_kind ? 'Evidence: ' + row.source_kind.replaceAll('_', ' ') : (isTask || isEvent ? null : (row.kind === 'observation' && row.session_id ? 'Session checkpoint' : KIND[row.kind] || row.kind));
  if (sessionRecord) { title = (row.agent_id || row.principal_id) + (row.kind === 'handoff' ? ' handoff' : ' checkpoint'); }
  if (isEvent) title = ACTION[row.title] || ACTION[row.kind] || row.title || 'Activity';
  const head = h('div', { class: 'record-head' },
    row.state && !row.source_kind ? h('span', { class: 'state ' + row.state }, STATE[row.state] || row.state) : null,
    kind ? h('span', { class: 'state' }, kind) : null,
    h('strong', {}, h('bdi', {}, title)), ...trust(row));
  let body = row.body || row.summary;
  if (isEvent && body) { const tool = row.title || row.kind; if (body.startsWith(tool + ':')) body = body.slice(tool.length + 1).trim(); }
  const meta = h('p', { class: 'meta' },
    row.git_revision ? h('span', {}, 'Revision ', sha(row.git_revision)) : null,
    row.agent_id ? h('span', {}, 'Agent ' + row.agent_id) : null,
    (row.principal_id || row.reported_by) ? h('span', {}, 'Credential ' + (row.principal_id || row.reported_by)) : null,
    row.created_at || row.updated_at ? h('span', {}, time(row.created_at || row.updated_at)) : null,
    project && row.id && isTask ? h('a', { href: `#/p/${project}/task/${row.id}` }, 'Open task') : null,
    project && row.id && row.kind && !isTask && !row.section ? h('a', { href: `#/p/${project}/record/${row.id}` }, 'Open record') : null);
  return h('li', { class: 'record' }, head, body && body !== row.state ? h('p', { class: 'body', dir: 'auto' }, body) : null, meta);
}
function list(rows, project, empty) {
  if (!rows || !rows.length) return h('p', { class: 'empty' }, empty);
  return h('ul', { class: 'records' }, rows.map(r => record(r, project)));
}

// ---------- overview ----------
const FILTERS = [
  ['all', 'All projects', () => true],
  ['attention', 'Needs attention', p => p.incomplete_sessions > 0 || count(p, 'blocked') > 0 || p.evidence_awaiting_attestation > 0],
  ['blocked', 'Blocked work', p => count(p, 'blocked') > 0],
  ['review', 'Awaiting owner review', p => p.evidence_awaiting_attestation > 0],
  ['incomplete', 'Unfinished sessions', p => p.incomplete_sessions > 0],
  ['recent', 'Active this week', p => p.last_activity_at && Date.now() - Date.parse(p.last_activity_at) < 7 * 86400000],
];
function count(p, ...states) { return states.reduce((n, s) => n + ((p.task_counts || {})[s] || 0), 0); }

function renderFilters(rows) {
  const root = $('filters'); root.replaceChildren();
  for (const [id, label, fn] of FILTERS) {
    root.append(h('button', { type: 'button', 'aria-pressed': String(state.filter === id), on: { click: () => { state.filter = id; route(); } } },
      h('span', {}, label), h('span', { class: 'n' }, rows.filter(fn).length)));
  }
}

async function showOverview(main) {
  const rows = await overview(true);
  renderFilters(rows);
  const fn = FILTERS.find(f => f[0] === state.filter)[2];
  const text = state.text.toLowerCase();
  const shown = rows.filter(fn).filter(p => !text || p.name.toLowerCase().includes(text) || p.repository.toLowerCase().includes(text))
    .sort((a, b) => (b.last_activity_at || '').localeCompare(a.last_activity_at || '') || a.name.localeCompare(b.name));
  const attention = rows.filter(FILTERS[1][2]).length;
  main.replaceChildren(
    h('div', { class: 'page-head' },
      h('div', {}, h('h1', {}, 'Projects'),
        h('p', {}, rows.length === 1 ? 'One project in memory.' : rows.length + ' projects in memory.', attention ? ` ${attention} ${attention === 1 ? 'needs' : 'need'} attention.` : ' Nothing needs attention.'))),
    shown.length ? h('div', { class: 'register' }, shown.map(entry))
      : h('p', { class: 'empty page' }, rows.length ? 'No project matches this filter.' : 'No projects yet. An agent registers a project with memory_create_project when it first works in a repository.'),
    h('p', { class: 'trust-note' }, 'Revisions shown are the last ones agents recorded, not your current checkout. Open a project and enter your HEAD to see which records are stale.'));
}

function entry(p) {
  const chips = [];
  const add = (n, label, cls) => { if (n) chips.push(h('li', { class: cls || null }, `${n} ${label}`)); };
  add(count(p, 'active'), 'active');
  add(count(p, 'blocked'), 'blocked', 'bad');
  add(count(p, 'planned'), 'planned');
  add(count(p, 'implemented', 'committed', 'tested'), 'awaiting verification', 'warn');
  add(count(p, 'verified', 'released'), 'done', 'good');
  add(p.open_issues, p.open_issues === 1 ? 'open issue' : 'open issues');
  add(p.evidence_awaiting_attestation, 'awaiting owner review', 'warn');
  add(p.incomplete_sessions, p.incomplete_sessions === 1 ? 'unfinished session' : 'unfinished sessions', 'warn');
  const s = p.last_session, o = p.last_observed, ev = p.latest_evidence, ho = p.latest_handoff;
  const facts = [];
  const fact = (label, ...v) => facts.push(h('div', {}, h('dt', {}, label), h('dd', {}, ...v)));
  if (o) fact('Last observed', sha(o.git_revision), s && s.branch ? ' on ' + s.branch : '', ' ', time(o.observed_at));
  if (s) fact('Last agent', s.agent_id, ' ', s.status === 'incomplete' ? seal('open', 'Unfinished') : seal('closed', 'Finished'));
  if (ho) fact('Latest handoff', time(ho.created_at), ho.agent_id ? ' by ' + ho.agent_id : '');
  if (ev) fact('Latest evidence', ev.owner_attested ? seal('attested', 'Owner attested') : seal('reported', 'Reported'), ' ',
    o && ev.git_revision === o.git_revision ? 'at the last observed revision' : 'at an older revision');
  return h('article', { class: 'entry' },
    h('div', {}, h('h2', {}, h('a', { href: '#/p/' + p.id }, h('bdi', {}, p.name))), h('div', { class: 'repo' }, p.repository)),
    h('div', { class: 'when' }, p.last_activity_at ? ['Active ', time(p.last_activity_at)] : 'No activity yet'),
    chips.length ? h('ul', { class: 'counts wide' }, chips) : null,
    facts.length ? h('dl', { class: 'facts wide' }, facts) : null,
    h('p', { class: 'next wide' }, p.next_recommended_action));
}

// ---------- project ----------
const TABS = [['resume', 'Resume'], ['tasks', 'Tasks'], ['sessions', 'Sessions'], ['records', 'Decisions & issues'], ['evidence', 'Evidence'], ['timeline', 'Timeline'], ['imports', 'Imports']];

async function projectHeader(main, id, tab) {
  const rows = await overview();
  const p = rows.find(r => r.id === id);
  if (!p) throw new Error('This project does not exist or your token does not cover it.');
  renderFilters(rows);
  main.replaceChildren(
    h('div', { class: 'page-head' },
      h('div', {}, h('h1', {}, h('bdi', {}, p.name)), h('div', { class: 'repo' }, p.repository)),
      h('button', { class: 'quiet', type: 'button', on: { click: () => copy(p.id, 'Project ID copied') } }, 'Copy project ID')),
    h('nav', { class: 'tabs', 'aria-label': 'Project views' },
      TABS.map(([t, label]) => h('a', { href: `#/p/${id}/${t}`, 'aria-current': t === tab ? 'page' : null }, label))));
  return p;
}

async function showResume(main, id) {
  const p = await projectHeader(main, id, 'resume');
  const head = state.head[id] || '';
  const input = h('input', { id: 'head', value: head, placeholder: 'Full commit SHA you have checked out', spellcheck: 'false', autocomplete: 'off' });
  const apply = h('button', { class: 'quiet', type: 'submit' }, 'Compare');
  const form = h('form', { class: 'field', on: { submit: ev => { ev.preventDefault(); const v = input.value.trim(); if (v && !SHA.test(v)) { toast('Enter a full 40 or 64 character commit SHA.'); return; } state.head[id] = v.toLowerCase(); route(); } } },
    h('label', { for: 'head' }, 'Compare with your HEAD'), input, apply);
  const c = await api('get_project_context', { project_id: id, git_revision: head });
  const handoff = c.handoff[0];
  const unfinished = c.sessions.filter(s => s.status === 'incomplete');
  const summary = [`${p.name} (${p.repository})`, `Next: ${p.next_recommended_action}`];
  if (handoff) summary.push('', `Latest handoff by ${handoff.agent_id || handoff.principal_id} at ${handoff.created_at}, revision ${handoff.git_revision}:`, handoff.body);
  if (unfinished.length) summary.push('', `Unfinished sessions: ${unfinished.map(s => s.agent_id + ' since ' + s.started_at).join('; ')}`);
  summary.push('', 'Memory content is reported by agents and is not independently verified.');
  main.append(form, h('div', { class: 'brief' },
    h('div', {},
      h('h2', {}, 'Where to pick up'),
      h('p', { class: 'next' }, p.next_recommended_action),
      h('h2', {}, 'Latest handoff'),
      handoff ? h('div', { class: 'handoff' },
        h('div', { class: 'record-head' }, h('strong', {}, handoff.agent_id || handoff.principal_id), time(handoff.created_at), sha(handoff.git_revision), ...trust(handoff)),
        h('p', { class: 'body', dir: 'auto' }, handoff.body),
        h('p', {}, h('button', { class: 'quiet', type: 'button', on: { click: () => copy(summary.join('\n'), 'Handoff summary copied') } }, 'Copy handoff summary'), ' ',
          h('a', { href: `#/p/${id}/record/${handoff.id}` }, 'Open full record')))
        : h('p', { class: 'empty' }, 'No handoff yet. Agents write one when they finish a session.'),
      h('h2', {}, 'Recent checkpoints'),
      list(c.checkpoints, id, 'No checkpoints. Agents checkpoint long sessions so work survives a crash.'),
      h('h2', {}, 'Work in progress'),
      list(c.tasks, id, 'No tasks recorded.')),
    h('div', {},
      h('h2', {}, 'Sessions'),
      list(c.sessions.map(s => ({ ...s, title: s.agent_id + (s.branch ? ' on ' + s.branch : ''), kind: null, created_at: s.started_at, body: s.status === 'incomplete' ? 'Unfinished: no final handoff was written. Resume from its latest checkpoint.' : 'Finished ' + new Date(s.finished_at).toLocaleString() })), null, 'No sessions yet.'),
      h('h2', {}, 'Current decisions and open issues'),
      list(c.decisions_and_issues, id, 'None recorded.'),
      h('h2', {}, 'Evidence'),
      list(c.evidence, id, 'No evidence attached.'))),
    h('p', { class: 'trust-note' }, c.content_trust));
}

// A paginated view with optional filter buttons.
async function showList(main, id, tab, tool, filters, empty, params = {}) {
  await projectHeader(main, id, tab);
  const key = tab + id;
  const filter = showList.filter[key] || '';
  const bar = filters ? h('div', { class: 'toolbar', role: 'group', 'aria-label': 'Filter' },
    filters.map(([v, label]) => h('button', { class: 'quiet', type: 'button', 'aria-pressed': String(filter === v), on: { click: () => { showList.filter[key] = v; route(); } } }, label))) : null;
  const holder = h('div', {});
  main.append(bar || '', holder);
  let offset = 0;
  const more = h('button', { class: 'quiet more', type: 'button' }, 'Load more');
  const ul = h('ul', { class: 'records' });
  async function page() {
    more.disabled = true;
    const rows = await api(tool, { project_id: id, limit: 40, offset, ...params, [tool === 'list_tasks' ? 'state' : 'kind']: filter });
    for (const r of rows) ul.append(record(tool === 'explore_sessions' ? { ...r, title: r.agent_id + (r.branch ? ' on ' + r.branch : ''), kind: null, created_at: r.started_at, body: `${r.checkpoints} checkpoint(s). ` + (r.status === 'incomplete' ? 'Unfinished: no final handoff.' : 'Finished ' + new Date(r.finished_at).toLocaleString() + '.') } : r, id));
    offset += rows.length;
    if (!offset) holder.replaceChildren(h('p', { class: 'empty' }, empty));
    more.hidden = rows.length < 40; more.disabled = false;
  }
  more.addEventListener('click', () => page().catch(fail));
  holder.append(ul, more);
  await page();
}
showList.filter = {};

async function showTask(main, id, task) {
  await projectHeader(main, id, 'tasks');
  const rows = await api('explore_task', { project_id: id, task_id: task });
  const t = rows.find(r => r.section === 'task');
  if (!t) throw new Error('Task not found in this project.');
  const sec = name => rows.filter(r => r.section === name);
  main.append(
    h('h2', { class: 'section-title', dir: 'auto' }, t.title),
    h('dl', { class: 'facts' },
      h('div', {}, h('dt', {}, 'State'), h('dd', {}, h('span', { class: 'state ' + t.state }, STATE[t.state]))),
      h('div', {}, h('dt', {}, 'Revision'), h('dd', {}, 'r' + t.revision, ' ', sha(t.git_revision))),
      h('div', {}, h('dt', {}, 'Claimed by'), h('dd', {}, t.claimed_by ? [t.claimed_by, ' until ', time(t.claim_expires_at)] : 'nobody')),
      h('div', {}, h('dt', {}, 'Updated'), h('dd', {}, time(t.updated_at)))),
    h('h3', { class: 'section-title' }, 'Depends on'), list(sec('dependency').map(d => ({ ...d, section: null })), id, 'No dependencies.'),
    h('h3', { class: 'section-title' }, 'Needed by'), list(sec('dependent').map(d => ({ ...d, section: null })), id, 'No other task depends on this one.'),
    h('h3', { class: 'section-title' }, 'Evidence for this task'), list(sec('evidence'), id, 'No evidence is linked to this task.'),
    h('h3', { class: 'section-title' }, 'History'),
    list(sec('transition').map(e => ({ ...e, kind: 'event', title: e.kind, section: null, state: null, body: e.state ? `${STATE[e.state] || e.state}, task revision ${e.revision}` : e.body })), id, 'No history.'));
}

async function showRecord(main, id, rec) {
  await projectHeader(main, id, 'records');
  const rows = await api('search', { project_id: id, exact_id: rec, git_revision: state.head[id] || '' });
  main.append(rows.length ? list(rows, null, '') : h('p', { class: 'empty page' }, 'No record with this ID in this project.'));
}

// ---------- search across projects ----------
async function showSearch(main, query) {
  const rows = await overview();
  renderFilters(rows);
  main.replaceChildren(h('div', { class: 'page-head' }, h('div', {}, h('h1', {}, 'Search'), h('p', {}, 'Results for ', h('strong', { dir: 'auto' }, query), ' across your projects.'))));
  const q = query.toLowerCase();
  const named = rows.filter(p => p.name.toLowerCase().includes(q) || p.repository.toLowerCase().includes(q));
  if (named.length) main.append(h('h2', { class: 'section-title' }, 'Projects'), h('div', { class: 'register' }, named.map(entry)));
  const status = h('p', { class: 'loading' }, `Searching ${Math.min(rows.length, 50)} projects`);
  main.append(status);
  const queue = rows.slice(0, 50);
  let found = 0;
  async function worker() {
    while (queue.length) {
      const p = queue.shift();
      const hits = await api('search', { project_id: p.id, query, limit: 8 });
      if (hits.length) { found += hits.length; main.insertBefore(h('section', {}, h('h2', { class: 'section-title' }, h('a', { href: '#/p/' + p.id }, p.name)), list(hits, p.id, '')), status); }
    }
  }
  await Promise.all([worker(), worker(), worker(), worker()]);
  status.textContent = found ? `${found} matching records. Each project shows its best 8.` : (named.length ? 'No records match.' : 'Nothing matches. Search looks at decisions, issues, observations, handoffs, tasks, evidence and activity.');
}

// ---------- router ----------
function fail(e) { const m = $('main'); m.prepend(h('p', { class: 'error', role: 'alert' }, e.message || String(e))); }

async function route() {
  if (!state.token) return;
  const main = $('main');
  main.replaceChildren(h('p', { class: 'loading' }, 'Loading'));
  const [path, qs] = location.hash.slice(1).split('?');
  const parts = path.split('/').filter(Boolean);
  try {
    if (parts[0] === 'search') {
      const q = new URLSearchParams(qs).get('q') || '';
      $('global-query').value = q;
      return await showSearch(main, q);
    }
    if (parts[0] === 'p' && UUID.test(parts[1] || '')) {
      const id = parts[1].toLowerCase(), tab = parts[2] || 'resume';
      if (tab === 'task' && UUID.test(parts[3] || '')) return await showTask(main, id, parts[3]);
      if (tab === 'record' && UUID.test(parts[3] || '')) return await showRecord(main, id, parts[3]);
      switch (tab) {
        case 'tasks': return await showList(main, id, tab, 'list_tasks', [['', 'All'], ['active', 'Active'], ['blocked', 'Blocked'], ['planned', 'Planned'], ['tested', 'Tested'], ['verified', 'Verified']], 'No tasks match.');
        case 'sessions': return await showList(main, id, tab, 'explore_sessions', [['', 'All'], ['incomplete', 'Unfinished'], ['finished', 'Finished']], 'No sessions match.');
        case 'records': return await showList(main, id, tab, 'explore_records', [['', 'All'], ['decision', 'Decisions'], ['issue', 'Issues'], ['observation', 'Observations'], ['handoff', 'Handoffs']], 'Nothing recorded yet.');
        case 'evidence': return await showList(main, id, tab, 'explore_evidence', [['', 'All'], ['awaiting_attestation', 'Awaiting owner review'], ['attested', 'Owner attested']], 'No evidence matches.');
        case 'timeline': return await showList(main, id, tab, 'explore_events', null, 'No activity recorded.');
        case 'imports': return await showList(main, id, tab, 'explore_imports', null, 'Nothing has been imported into this project.');
        default: return await showResume(main, id);
      }
    }
    await showOverview(main);
  } catch (e) {
    if (state.token) { main.replaceChildren(); fail(e); }
  } finally {
    main.focus({ preventScroll: true });
  }
}

// ---------- session ----------
function lock(message) {
  state.token = ''; state.overview = null;
  try { sessionStorage.removeItem('thoth-token'); } catch {}
  $('app').hidden = true; $('search-form').hidden = true; $('lock').hidden = true; $('unlock').hidden = false;
  if (message) toast(message);
  $('token').focus();
}
function unlocked() {
  $('unlock').hidden = true; $('app').hidden = false; $('search-form').hidden = false; $('lock').hidden = false;
  route();
}
$('unlock-form').addEventListener('submit', ev => {
  ev.preventDefault();
  state.token = $('token').value.trim(); $('token').value = '';
  if ($('remember').checked) try { sessionStorage.setItem('thoth-token', state.token); } catch {}
  unlocked();
});
$('lock').addEventListener('click', () => lock('Locked. The token was removed from this tab.'));
$('search-form').addEventListener('submit', ev => {
  ev.preventDefault();
  const q = $('global-query').value.trim();
  if (q) location.hash = '#/search?q=' + encodeURIComponent(q);
});
$('project-filter').addEventListener('input', ev => { state.text = ev.target.value; if (!location.hash || location.hash === '#/') route(); else location.hash = '#/'; });
window.addEventListener('hashchange', route);

fetch('/healthz').then(r => r.ok ? r.json() : Promise.reject()).then(v => {
  $('health').textContent = 'Ready'; $('health').className = 'health ok';
  const ed = $('edition'); ed.textContent = v.edition === 'mini' ? 'Mini' : 'Full'; ed.title = 'Version ' + v.version; ed.hidden = false;
}).catch(() => { $('health').textContent = 'Database unavailable'; $('health').className = 'health down'; });

try { state.token = sessionStorage.getItem('thoth-token') || ''; } catch {}
if (state.token) unlocked(); else lock();
