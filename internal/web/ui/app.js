// SessionKeep dashboard. Plain modules, no dependencies, nothing loaded from elsewhere.
// Every piece of data is put on the page with textContent, never as markup.

const view = document.getElementById('view');
const title = document.getElementById('title');
const notice = document.getElementById('notice');
let csrf = '';
let projects = [];

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (k === 'class') node.className = v;
    else if (k === 'text') node.textContent = v;
    else if (k.startsWith('on')) node.addEventListener(k.slice(2), v);
    else if (v === true) node.setAttribute(k, '');
    else if (v !== false && v != null) node.setAttribute(k, v);
  }
  for (const c of children.flat()) {
    if (c == null) continue;
    node.append(c.nodeType ? c : document.createTextNode(String(c)));
  }
  return node;
}

function say(text, kind = '') {
  notice.textContent = text;
  notice.className = kind;
}

async function api(method, path, body) {
  const opt = { method, headers: {}, credentials: 'same-origin' };
  if (method !== 'GET') {
    opt.headers['X-SessionKeep-CSRF'] = csrf;
    opt.headers['Content-Type'] = 'application/json';
    opt.body = JSON.stringify(body || {});
  }
  let res;
  try {
    res = await fetch(path, opt);
  } catch (e) {
    return { ok: false, status: 0, error: { code: 'offline', message: 'The SessionKeep server is not reachable. It stops when the terminal command that started it ends.' } };
  }
  let json = {};
  try { json = await res.json(); } catch (e) { /* empty body */ }
  return { ok: res.ok, status: res.status, data: json.data, error: json.error };
}

function problem(r) {
  if (r.status === 401) return 'This browser is not signed in. Run "sessionkeep dashboard" again and open the link it prints.';
  return (r.error && r.error.message) || 'The request failed.';
}

function table(headers, rows) {
  const t = el('table', {}, el('thead', {}, el('tr', {}, headers.map(h => el('th', { scope: 'col', text: h })))));
  t.append(el('tbody', {}, rows.map(cells => el('tr', {}, cells.map(c => el('td', {}, c))))));
  return el('div', { class: 'scroll' }, t);
}

function field(label, control) { return el('label', {}, label, control); }

function select(id, options, value) {
  const s = el('select', { id });
  for (const [v, text] of options) s.append(el('option', { value: v, text, selected: v === value }));
  return s;
}

const LEVELS = [['off', 'off - save nothing'], ['metadata', 'metadata - no text'], ['conversation', 'conversation - prompts and replies'], ['extended', 'extended - also tool output snippets']];

async function loadProjects() {
  const r = await api('GET', '/api/projects');
  projects = r.ok ? r.data : [];
  return r;
}

function projectSelect(id, withAll) {
  const opts = projects.map(p => [p.id, `${p.name} (${p.id.slice(0, 8)})`]);
  if (withAll) opts.push(['*', 'all projects']);
  return select(id, opts, opts.length ? opts[0][0] : '');
}

// ---- Overview -------------------------------------------------------------------------------

async function overview() {
  const [st, set] = await Promise.all([api('GET', '/api/status'), api('GET', '/api/settings')]);
  if (!st.ok) return say(problem(st), 'bad');
  const s = st.data;
  view.append(
    el('section', { class: 'panel' }, el('h2', { text: 'Stored' }),
      el('p', { text: `${s.stored.projects} project(s), ${s.stored.sessions} session(s), ${s.stored.events} event(s), ${s.stored.evidence} text record(s), ${s.stored.capture_gaps} capture gap(s).` })),
    el('section', { class: 'panel' }, el('h2', { text: 'Waiting to be stored' }),
      el('p', { class: s.spool_pending ? 'state-warn' : 'state-ok', text: s.spool_pending ? `Waiting: ${s.spool_pending} captured event(s), oldest ${s.spool_oldest}. They are not searchable yet.` : 'Nothing waiting: everything captured is stored.' }),
      el('p', { text: `Quarantined files: ${s.quarantined_files}. Gaps not yet imported: ${s.gaps_not_yet_imported}.` }),
      el('button', { type: 'button', text: 'Store waiting events now', onclick: async () => {
        const r = await api('POST', '/api/ingest');
        say(r.ok ? `Stored ${r.data.ingested} event(s).` : problem(r), r.ok ? 'ok' : 'bad');
        if (r.ok) show('overview');
      } })),
  );
  if (set.ok) {
    const a = set.data.auto_add;
    view.append(el('section', { class: 'panel' }, el('h2', { text: 'Folders SessionKeep has not seen' }),
      el('p', { text: a.mode === 'auto' ? `Mode: auto. New folders are registered automatically at level "${a.level}", except excluded folders.`
        : a.mode === 'manual' ? 'Mode: manual. Nothing is saved until you add a project.' : 'Mode: ask. Nothing is saved; you get a reminder to add the project.' }),
      el('p', {}, el('a', { href: '#/projects', text: 'Change this, add a project or exclude a folder' }))));
  }
}

// ---- Projects (onboarding) ------------------------------------------------------------------

async function projectsView() {
  const [pr, set] = await Promise.all([loadProjects(), api('GET', '/api/settings')]);
  if (!pr.ok) return say(problem(pr), 'bad');

  const rows = projects.map(p => {
    const level = select(`level-${p.id}`, LEVELS, p.capture_level);
    level.setAttribute('aria-label', `Capture level for ${p.name}`);
    return [
      p.name,
      el('span', { class: p.paused ? 'state-warn' : (p.capture_level === 'off' ? 'muted' : 'state-ok'), text: p.paused ? `paused (level ${p.capture_level})` : `capturing: ${p.capture_level}` }),
      p.added_by,
      el('span', { class: 'text', text: p.roots.join('\n') }),
      el('div', { class: 'row' }, level,
        el('button', { type: 'button', text: 'Set level', onclick: () => act(`/api/projects/${p.id}/capture`, { level: level.value }, 'Level changed.') }),
        el('button', { type: 'button', text: p.paused ? 'Resume' : 'Pause', onclick: () => act(`/api/projects/${p.id}/${p.paused ? 'resume' : 'pause'}`, {}, p.paused ? 'Resumed.' : 'Paused.') })),
    ];
  });
  view.append(el('h2', { text: 'Registered projects' }),
    projects.length ? table(['Name', 'State', 'Added', 'Folder', 'Change'], rows) : el('p', { text: 'No projects registered yet. Nothing is being saved.' }));

  const path = el('input', { type: 'text', id: 'add-path', required: true, autocomplete: 'off' });
  const level = select('add-level', [['', 'choose what to save...'], ...LEVELS], '');
  const name = el('input', { type: 'text', id: 'add-name', autocomplete: 'off' });
  view.append(el('h2', { text: 'Add a project' }),
    el('form', { onsubmit: e => { e.preventDefault(); act('/api/projects', { path: path.value, level: level.value, name: name.value }, 'Project registered.'); } },
      field('Folder (full path)', path), field('What to save (required; there is no default)', level), field('Name (optional)', name),
      el('div', {}, el('button', { type: 'submit', class: 'primary', text: 'Register project' }))));

  if (!set.ok) return;
  const a = set.data.auto_add;
  const mode = select('mode', [['ask', 'ask - remind me, save nothing'], ['manual', 'manual - save nothing, say nothing'], ['auto', 'auto - register new folders automatically']], a.mode);
  const autoLevel = select('auto-level', [['', 'level for auto mode...'], ...LEVELS.slice(1)], a.level || '');
  view.append(el('h2', { text: 'Folders SessionKeep has not seen' }),
    el('form', { onsubmit: e => { e.preventDefault(); act('/api/settings/auto-add', { mode: mode.value, level: mode.value === 'auto' ? autoLevel.value : '' }, 'Setting saved.'); } },
      field('What should happen', mode), field('Level (only for auto)', autoLevel),
      el('p', { class: 'muted', text: 'In auto mode, opening Claude Code in any folder that is not excluded starts saving it at this level. Set up exclusions first.' }),
      el('div', {}, el('button', { type: 'submit', text: 'Save setting' }))));

  const ex = el('input', { type: 'text', id: 'ex-path', required: true, autocomplete: 'off' });
  const list = set.data.exclusions || [];
  view.append(el('h2', { text: 'Excluded folders' }),
    el('p', { class: 'muted', text: 'Nothing at or below an excluded folder is ever saved, registered or mentioned, even if it is a registered project.' }),
    list.length ? table(['Folder', ''], list.map(p => [el('span', { class: 'text', text: p }),
      el('button', { type: 'button', text: 'Remove exclusion', 'aria-label': `Remove exclusion ${p}`, onclick: () => act('/api/exclusions', { path: p, remove: true }, 'Exclusion removed.') })]))
      : el('p', { text: 'No folders are excluded.' }),
    el('form', { onsubmit: e => { e.preventDefault(); act('/api/exclusions', { path: ex.value }, 'Folder excluded.'); } },
      field('Folder to exclude (full path)', ex), el('div', {}, el('button', { type: 'submit', text: 'Exclude folder' }))));
}

async function act(path, body, done) {
  const r = await api('POST', path, body);
  if (r.ok) { await show(current); say(done, 'ok'); } else say(problem(r), 'bad');
  return r;
}

// ---- Search ---------------------------------------------------------------------------------

async function searchView() {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  if (!projects.length) return view.append(el('p', { text: 'Register a project first; there is nothing to search yet.' }));
  const q = el('input', { type: 'search', id: 'q', required: true });
  const proj = projectSelect('scope', true);
  const who = select('who', [['', 'anyone'], ['user', 'me'], ['assistant', 'the assistant'], ['tool', 'a tool']], '');
  const exact = el('input', { type: 'checkbox', id: 'exact' });
  const results = el('div', { id: 'results', 'aria-live': 'polite' });
  const run = async cursor => {
    const p = new URLSearchParams({ q: q.value });
    if (proj.value === '*') p.set('all', '1'); else p.set('project', proj.value);
    if (who.value) p.set('who', who.value);
    if (exact.checked) p.set('exact', '1');
    if (cursor) p.set('cursor', cursor);
    const r = await api('GET', '/api/search?' + p);
    if (!cursor) results.replaceChildren();
    if (!r.ok) return say(problem(r), 'bad');
    const d = r.data;
    say(d.hits.length ? `${d.hits.length} result(s) in ${d.scope}.` : `Not found: nothing stored in ${d.scope} matches.`, d.hits.length ? 'ok' : '');
    if (d.method === 'any') results.append(el('p', { class: 'state-warn', text: 'Partial matches only: no stored record contains all of these words.' }));
    if (d.not_searched_pending_events) results.append(el('p', { class: 'state-warn', text: `${d.not_searched_pending_events} captured event(s) are not stored yet and were not searched. Use "Store waiting events now" on the Overview.` }));
    for (const h of d.hits) {
      const detail = el('div', {});
      results.append(el('article', { class: 'hit' },
        el('p', { class: 'muted', text: `${h.who} - ${h.observed_at.replace('T', ' ').slice(0, 16)} - ${h.id.slice(0, 8)}` }),
        el('p', { class: 'text', text: h.snippet }),
        el('button', { type: 'button', text: 'Show full record', onclick: async () => {
          const one = await api('GET', `/api/records/${h.id}?project=${encodeURIComponent(h.project_id)}`);
          if (!one.ok) return say(problem(one), 'bad');
          const x = one.data;
          detail.replaceChildren(el('div', { class: 'panel' },
            el('p', { class: 'muted', text: `Project ${x.project_name}. Said by ${x.who} (${x.origin}) at ${x.observed_at}. Session ${x.session_id || 'unknown'}. ${x.completeness}; ${x.redactions} redaction(s).` }),
            el('pre', { text: x.text })));
        } }), detail));
    }
    if (d.next_cursor) results.append(el('button', { type: 'button', text: 'More results', onclick: e => { e.target.remove(); run(d.next_cursor); } }));
  };
  view.append(el('form', { role: 'search', onsubmit: e => { e.preventDefault(); run(''); } },
    field('Search stored conversation text', q),
    el('div', { class: 'row' }, field('In', proj), field('Said by', who), el('label', { class: 'inline' }, exact, 'match as one exact string')),
    el('div', {}, el('button', { type: 'submit', class: 'primary', text: 'Search' }))), results);
}

// ---- Instructions ---------------------------------------------------------------------------

async function instructionsView() {
  await loadProjects();
  const r = await api('GET', '/api/instructions?all=1');
  if (!r.ok) return say(problem(r), 'bad');
  const rows = r.data.map(i => {
    const p = projects.find(x => x.id === i.project_id);
    const tools = el('div', { class: 'row' }, el('button', { type: 'button', text: 'History', onclick: () => history(i.id) }));
    if (i.state === 'active') tools.append(
      el('button', { type: 'button', text: 'Replace text', onclick: () => change(i, 'supersede') }),
      el('button', { type: 'button', text: 'Archive', onclick: () => change(i, 'archive') }));
    return [el('span', { class: i.state === 'active' ? 'state-ok' : 'muted', text: `${i.state}, revision ${i.revision}` }),
      i.scope === 'global' ? 'every project' : (p ? p.name : i.project_id.slice(0, 8)), el('pre', { text: i.text }), tools];
  });
  const work = el('div', { id: 'work', 'aria-live': 'polite' });
  view.append(el('p', { class: 'muted', text: 'Active instructions are given word for word to every new session of the projects they apply to.' }),
    r.data.length ? table(['State', 'Applies to', 'Text', ''], rows) : el('p', { text: 'No instructions recorded.' }), work);

  const text = el('textarea', { id: 'new-text', required: true });
  const scope = select('new-scope', [['', 'choose where it applies...'], ['project', 'one project'], ['global', 'every registered project']], '');
  const proj = projectSelect('new-project', false);
  const preview = el('div', {});
  const submit = async confirm => {
    const body = { text: text.value, scope: scope.value, confirm };
    if (scope.value === 'project') body.project = proj.value;
    const res = await api('POST', '/api/instructions', body);
    if (res.ok) { await show('instructions'); return say('Instruction recorded.', 'ok'); }
    if (res.status === 409 && res.data && res.data.saved === false) {
      say('Not saved yet. Check the exact text below, then confirm.');
      preview.replaceChildren(el('div', { class: 'preview' },
        el('p', { text: `This would be recorded word for word and apply to: ${res.data.scope === 'global' ? 'every registered project' : 'the selected project'}.` }),
        el('pre', { text: res.data.text }),
        el('button', { type: 'button', class: 'primary', text: 'Confirm and record', onclick: () => submit(true) })));
      return;
    }
    say(problem(res), 'bad');
  };
  view.append(el('h2', { text: 'Record an instruction' }),
    el('form', { onsubmit: e => { e.preventDefault(); submit(false); } },
      field('Instruction (your exact words)', text), el('div', { class: 'row' }, field('Applies to', scope), field('Project', proj)),
      el('div', {}, el('button', { type: 'submit', text: 'Preview' }))), preview);

  async function history(id) {
    const h = await api('GET', `/api/instructions/${id}`);
    if (!h.ok) return say(problem(h), 'bad');
    work.replaceChildren(el('h2', { text: 'History' }), ...h.data.versions.map(v => el('div', { class: 'panel' },
      el('p', { class: 'muted', text: `Revision ${v.revision}, ${v.status}, ${v.created_at}${v.reason ? ', reason: ' + v.reason : ''}` }), el('pre', { text: v.text }))));
  }
  function change(i, action) {
    const t = el('textarea', { id: 'chg-text' }); t.value = i.text;
    const reason = el('input', { type: 'text', id: 'chg-reason' });
    const go = async () => {
      const res = await api('POST', `/api/instructions/${i.id}/${action}`, { text: action === 'supersede' ? t.value : '', reason: reason.value, confirm: true });
      if (res.ok) { await show('instructions'); say(action === 'archive' ? 'Archived. It is no longer given to sessions.' : 'Text replaced. The old text is kept in the history.', 'ok'); } else say(problem(res), 'bad');
    };
    work.replaceChildren(el('h2', { text: action === 'archive' ? 'Archive this instruction?' : 'Replace the text' }),
      el('div', { class: 'preview' }, el('p', { text: 'Current text:' }), el('pre', { text: i.text })),
      el('form', { onsubmit: e => { e.preventDefault(); go(); } },
        action === 'supersede' ? field('New text (your exact words)', t) : null, field('Reason (optional)', reason),
        el('div', { class: 'row' }, el('button', { type: 'submit', class: 'primary', text: action === 'archive' ? 'Confirm: archive' : 'Confirm: replace' }),
          el('button', { type: 'button', text: 'Cancel', onclick: () => work.replaceChildren() }))));
    work.querySelector('textarea, input').focus();
  }
}

// ---- Resume ---------------------------------------------------------------------------------

async function resumeView() {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  if (!projects.length) return view.append(el('p', { text: 'Register a project first.' }));
  const proj = projectSelect('resume-project', false);
  const out = el('div', { 'aria-live': 'polite' });
  const load = async () => {
    const r = await api('GET', '/api/resume?project=' + encodeURIComponent(proj.value));
    if (!r.ok) return say(problem(r), 'bad');
    const d = r.data, c = d.checkpoints[0], g = d.git_now;
    say('');
    out.replaceChildren(
      el('h2', { text: 'Latest checkpoint' }),
      c ? el('div', { class: 'panel' }, el('p', { class: 'muted', text: `Recorded ${c.created_at}` + (c.git_repo ? `, on branch ${c.branch} at ${String(c.head).slice(0, 10)}, ${c.changed_files} uncommitted file(s).` : ', no Git repository.') }), el('pre', { text: c.note }))
        : el('p', { text: 'No checkpoint recorded; nothing says where work stopped.' }),
      el('h2', { text: 'Code now' }),
      el('p', { text: g.repo ? `Branch ${g.branch} at ${String(g.head || '').slice(0, 10)}, ${g.changed_files} uncommitted file(s).` : `No Git state: ${g.reason}.` }),
      ...(d.warnings || []).map(w => el('p', { class: 'state-bad', text: 'Warning: ' + w })),
      c && !(d.warnings || []).length ? el('p', { class: 'state-ok', text: 'Matches the checkpoint: same branch, same commit, same uncommitted state.' }) : null,
      el('h2', { text: `Recorded instructions (${d.instructions.length})` }),
      ...d.instructions.map(i => el('pre', { class: 'panel', text: i.text })),
      el('h2', { text: 'Most recent exchanges' }),
      ...(d.recent.length ? d.recent.map(e => el('div', { class: 'hit' }, el('p', { class: 'muted', text: `${e.who} - ${e.observed_at.replace('T', ' ').slice(0, 16)}` }), el('p', { class: 'text', text: e.text })))
        : [el('p', { text: 'Nothing stored yet.' })]),
      d.not_included_pending_events ? el('p', { class: 'state-warn', text: `${d.not_included_pending_events} captured event(s) are not stored yet and are missing above.` }) : null);
  };
  proj.addEventListener('change', load);
  const note = el('textarea', { id: 'cp-note', required: true });
  view.append(el('div', { class: 'row' }, field('Project', proj)), out,
    el('h2', { text: 'Record a checkpoint' }),
    el('form', { onsubmit: async e => { e.preventDefault(); const r = await api('POST', '/api/checkpoints', { note: note.value, project: proj.value }); if (r.ok) { note.value = ''; await load(); say('Checkpoint recorded.', 'ok'); } else say(problem(r), 'bad'); } },
      field('What is done, what is next, anything blocked', note), el('div', {}, el('button', { type: 'submit', text: 'Record checkpoint' }))));
  await load();
}

// ---- Diagnostics ----------------------------------------------------------------------------

async function diagnostics() {
  const r = await api('GET', '/api/doctor');
  if (!r.data) return say(problem(r), 'bad');
  view.append(el('p', { class: r.data.healthy ? 'state-ok' : 'state-bad', text: r.data.healthy ? 'All checks passed.' : 'One or more checks failed.' }),
    table(['Check', 'Result', 'Detail'], r.data.checks.map(c => [c.name, el('span', { class: c.status === 'ok' ? 'state-ok' : 'state-bad', text: c.status === 'ok' ? 'ok' : 'FAILED' }), c.detail])),
    el('p', { class: 'muted', text: 'Not checked: ' + r.data.not_checked }));
}

// ---- Routing --------------------------------------------------------------------------------

const views = { overview: ['Overview', overview], projects: ['Projects and capture', projectsView], search: ['Search', searchView],
  instructions: ['Instructions', instructionsView], resume: ['Resume work', resumeView], diagnostics: ['Diagnostics', diagnostics] };
let current = 'overview';

async function show(name) {
  current = views[name] ? name : 'overview';
  const [label, render] = views[current];
  title.textContent = label;
  document.title = label + ' - SessionKeep';
  for (const a of document.querySelectorAll('#nav a')) {
    if (a.dataset.view === current) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  }
  view.replaceChildren();
  say('Loading...');
  await render();
  if (notice.textContent === 'Loading...') say('');
}

window.addEventListener('hashchange', () => { show(location.hash.replace('#/', '')); document.getElementById('main').focus(); });

(async () => {
  const s = await api('GET', '/api/session');
  if (!s.ok) { title.textContent = 'Not signed in'; return say(problem(s), 'bad'); }
  csrf = s.data.csrf;
  document.getElementById('foot').textContent += ' SessionKeep ' + s.data.version + '.';
  show(location.hash.replace('#/', '') || 'overview');
})();
