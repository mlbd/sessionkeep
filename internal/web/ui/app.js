// SessionKeep dashboard. Plain modules, no dependencies, nothing loaded from elsewhere.
// Every piece of data is put on the page with textContent, never as markup.

const view = document.getElementById('view');
const title = document.getElementById('title');
const lede = document.getElementById('lede');
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
    if (c == null || c === false) continue;
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
    return { ok: false, status: 0, error: { code: 'offline', message: 'SessionKeep is not answering. It stops when the terminal command that started this page ends; run "sessionkeep dashboard" again.' } };
  }
  let json = {};
  try { json = await res.json(); } catch (e) { /* empty body */ }
  return { ok: res.ok, status: res.status, data: json.data, error: json.error };
}

function problem(r) {
  if (r.status === 401) return 'This browser is not signed in. Run "sessionkeep dashboard" again and open the link it prints.';
  return (r.error && r.error.message) || 'That did not work.';
}

const when = iso => (iso || '').replace('T', ' ').slice(0, 16);
const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

function field(label, control, hint) {
  return el('label', {}, label, control, hint ? el('span', { class: 'hint', text: hint }) : null);
}

function select(id, options, value) {
  const s = el('select', { id });
  for (const [v, text] of options) s.append(el('option', { value: v, text, selected: v === value }));
  return s;
}

// A group of radio tiles: every option is visible together with what it means.
function choice(name, legend, options, value) {
  const tiles = el('div', { class: 'tiles' });
  for (const [v, label, meaning] of options) {
    const input = el('input', { type: 'radio', name, value: v, checked: v === value });
    tiles.append(el('label', { class: 'tile' }, input, el('b', { text: label }), el('span', { text: meaning })));
  }
  const set = el('fieldset', {}, el('legend', { text: legend }), tiles);
  set.value = () => { const c = set.querySelector('input:checked'); return c ? c.value : ''; };
  return set;
}

const LEVELS = [
  ['conversation', 'Conversation', 'What you ask and what Claude answers. The usual choice.'],
  ['extended', 'Conversation and tool output', 'Also short excerpts of what tools printed.'],
  ['metadata', 'Activity only', 'Times and tool names. No text at all.'],
  ['off', 'Nothing', 'Registered, but nothing is saved.'],
];
const levelName = v => (LEVELS.find(l => l[0] === v) || [v, v])[1].toLowerCase();

async function loadProjects() {
  const r = await api('GET', '/api/projects');
  projects = r.ok ? r.data : [];
  return r;
}

function projectSelect(id, withAll) {
  const opts = projects.map(p => [p.id, p.name]);
  if (withAll) opts.push(['*', 'Every project']);
  return select(id, opts, opts.length ? opts[0][0] : '');
}

function strandFor(p) {
  if (p.paused) return ['paused', 'state-warn', `Paused. It was saving: ${levelName(p.capture_level)}.`];
  if (p.capture_level === 'off') return ['off', 'state-off', 'Saving nothing.'];
  return ['on', 'state-ok', `Saving: ${levelName(p.capture_level)}.`];
}

// Deletion always shows what would be removed first; nothing is deleted until confirmed.
async function forget(body, into, after) {
  const r = await api('POST', '/api/forget', body);
  if (r.status !== 409 || !r.data || r.data.deleted !== false) return say(problem(r), 'bad');
  const w = r.data.would_remove;
  say('Nothing is deleted yet. Check what would go, then confirm.');
  into.replaceChildren(el('div', { class: 'preview danger' },
    el('h3', { text: 'Delete permanently?' }),
    el('p', { class: 'said', text: r.data.target }),
    el('p', { text: `This removes ${plural(w.events, 'event', 'events')}, ${plural(w.text_records, 'piece of text', 'pieces of text')}, ${plural(w.sessions, 'session', 'sessions')}, ${plural(w.instructions, 'instruction', 'instructions')}, ${plural(w.checkpoints, 'checkpoint', 'checkpoints')} and ${plural(w.projects, 'project', 'projects')}. It cannot be undone.` }),
    el('p', { class: 'quiet small', text: 'Exports you made earlier still contain it, and so do Claude Code\u2019s own session files.' }),
    el('div', { class: 'row' },
      el('button', { type: 'button', class: 'danger', text: 'Delete permanently', onclick: async () => {
        const done = await api('POST', '/api/forget', { ...body, confirm: true });
        if (!done.ok) return say(problem(done), 'bad');
        await after();
        say('Deleted.', 'ok');
      } }),
      el('button', { type: 'button', text: 'Keep it', onclick: () => { into.replaceChildren(); say(''); } }))));
  into.querySelector('button').focus();
}

async function act(path, body, done) {
  const r = await api('POST', path, body);
  if (r.ok) { await show(current); say(done, 'ok'); } else say(problem(r), 'bad');
  return r;
}

// ---- Overview -------------------------------------------------------------------------------

async function overview() {
  const [st, set, pr, ses] = await Promise.all([api('GET', '/api/status'), api('GET', '/api/settings'), loadProjects(), api('GET', '/api/sessions?all=1&limit=6')]);
  if (!st.ok) return say(problem(st), 'bad');
  const s = st.data;
  const saving = projects.filter(p => !p.paused && p.capture_level !== 'off');

  if (!projects.length) {
    lede.textContent = 'Nothing is being saved yet. Add the first project to start.';
    view.append(el('div', { class: 'table' },
      el('h3', { text: 'Start here' }),
      el('p', { text: 'SessionKeep saves what you and Claude say in the projects you choose, so a later session can pick up where you stopped. It saves nothing until you add a project.' }),
      el('p', {}, el('a', { href: '#/projects', text: 'Add your first project' }))));
  } else {
    lede.textContent = `Saving ${plural(saving.length, 'project', 'projects')} of ${projects.length}.` + (s.spool_pending ? ` ${plural(s.spool_pending, 'event is', 'events are')} waiting to be stored.` : ' Everything captured is stored.');
  }

  if (s.spool_pending) {
    view.append(el('div', { class: 'preview' },
      el('p', { class: 'state-warn', text: `Waiting: ${plural(s.spool_pending, 'captured event', 'captured events')}, the oldest from ${when(s.spool_oldest)}. They cannot be searched until they are stored.` }),
      el('button', { type: 'button', class: 'primary', text: 'Store them now', onclick: async () => {
        const r = await api('POST', '/api/ingest');
        if (r.ok) { await show('overview'); say(`Stored ${plural(r.data.ingested, 'event', 'events')}.`, 'ok'); } else say(problem(r), 'bad');
      } })));
  }

  if (projects.length) {
    view.append(el('h2', { text: 'Projects' }), el('ul', { class: 'threads' }, projects.map(p => {
      const [strand, cls, words] = strandFor(p);
      return el('li', { class: 'thread' }, el('span', { class: 'strand ' + strand }),
        el('div', {}, el('h3', { text: p.name }), el('p', { class: cls, text: words }), el('p', { class: 'path', text: p.roots.join(', ') })));
    })));
  }

  if (ses.ok && ses.data.length) {
    view.append(el('h2', { text: 'Latest sessions' }), el('ul', { class: 'threads' }, ses.data.map(x =>
      el('li', { class: 'thread' }, el('span', { class: 'strand on' }),
        el('div', {}, el('p', { class: 'quiet small', text: `${x.project_name}, ${when(x.started_at)}, ${plural(x.text_records, 'piece of text', 'pieces of text')}` }),
          el('p', { class: 'said', text: x.first_prompt || '(no text was saved in this session)' }),
          el('a', { href: `#/history/${x.project_id}/${x.id}`, text: 'Read this session' }))))));
  }

  view.append(el('h2', { text: 'What is stored' }), el('dl', { class: 'ledger' },
    [['Projects', s.stored.projects], ['Sessions', s.stored.sessions], ['Pieces of text', s.stored.evidence], ['Events', s.stored.events],
      ['Gaps on record', s.stored.capture_gaps], ['Files set aside as unreadable', s.quarantined_files]]
      .map(([k, v]) => el('div', {}, el('dt', { text: k }), el('dd', { text: String(v) })))));

  if (set.ok) {
    const a = set.data.auto_add;
    const words = a.mode === 'auto' ? `New folders are added automatically and saved as "${levelName(a.level)}", unless you excluded them.`
      : a.mode === 'manual' ? 'New folders are ignored until you add them.' : 'In a new folder SessionKeep saves nothing and reminds you once that you can add it.';
    view.append(el('h2', { text: 'Folders SessionKeep has not seen' }), el('p', { text: words }), el('p', {}, el('a', { href: '#/projects', text: 'Change this' })));
  }
}

// ---- Projects -------------------------------------------------------------------------------

async function projectsView() {
  const [pr, set] = await Promise.all([loadProjects(), api('GET', '/api/settings')]);
  if (!pr.ok) return say(problem(pr), 'bad');
  lede.textContent = 'Choose which folders SessionKeep remembers, and how much.';
  const removal = el('div', { 'aria-live': 'polite' });

  if (projects.length) {
    view.append(el('ul', { class: 'threads' }, projects.map(p => {
      const [strand, cls, words] = strandFor(p);
      const level = select(`level-${p.id}`, LEVELS.map(l => [l[0], l[1]]), p.capture_level);
      level.setAttribute('aria-label', `What to save for ${p.name}`);
      return el('li', { class: 'thread' }, el('span', { class: 'strand ' + strand }), el('div', {},
        el('h3', { text: p.name }),
        el('p', { class: cls, text: words + (p.added_by === 'auto' ? ' Added automatically.' : '') }),
        el('p', { class: 'path', text: p.roots.join(', ') }),
        el('div', { class: 'row' }, level,
          el('button', { type: 'button', text: 'Change what is saved', onclick: () => act(`/api/projects/${p.id}/capture`, { level: level.value }, 'Changed.') }),
          el('button', { type: 'button', text: p.paused ? 'Resume saving' : 'Pause saving', onclick: () => act(`/api/projects/${p.id}/${p.paused ? 'resume' : 'pause'}`, {}, p.paused ? 'Saving again.' : 'Paused.') })),
        el('details', {}, el('summary', { text: 'Delete' }),
          el('div', { class: 'row' },
            el('button', { type: 'button', class: 'danger', text: 'Delete saved conversations', 'aria-label': `Delete saved conversations of ${p.name}`, onclick: () => forget({ kind: 'captured', project: p.id }, removal, () => show('projects')) }),
            el('button', { type: 'button', class: 'danger', text: 'Delete the whole project', 'aria-label': `Delete project ${p.name}`, onclick: () => forget({ kind: 'project', project: p.id }, removal, () => show('projects')) })))));
    })), removal);
  } else {
    view.append(el('p', { class: 'quiet', text: 'No projects yet, so nothing is being saved.' }));
  }

  const path = el('input', { type: 'text', id: 'add-path', required: true, autocomplete: 'off', placeholder: 'C:\\Work\\my-project' });
  const name = el('input', { type: 'text', id: 'add-name', autocomplete: 'off' });
  const level = choice('add-level', 'What to save', LEVELS, '');
  view.append(el('h2', { text: 'Add a project' }), el('div', { class: 'table' },
    el('form', { onsubmit: e => { e.preventDefault(); act('/api/projects', { path: path.value, level: level.value(), name: name.value }, 'Project added.'); } },
      field('Folder', path, 'The full path. Everything inside it counts as this project.'), level,
      field('Name', name, 'Optional. The folder name is used if you leave this empty.'),
      el('div', {}, el('button', { type: 'submit', class: 'primary', text: 'Add project' })))));

  if (!set.ok) return;
  const a = set.data.auto_add;
  const mode = choice('mode', 'When Claude Code runs in a folder that is not on the list above', [
    ['ask', 'Remind me', 'Save nothing. Say once that the folder can be added.'],
    ['manual', 'Ignore it', 'Save nothing and say nothing.'],
    ['auto', 'Add it automatically', 'Start saving straight away, at the level below.'],
  ], a.mode);
  const autoLevel = choice('auto-level', 'For automatically added folders, save', LEVELS.slice(0, 3), a.level || '');
  view.append(el('h2', { text: 'Folders SessionKeep has not seen' }), el('div', { class: 'table' },
    el('form', { onsubmit: e => { e.preventDefault(); act('/api/settings/auto-add', { mode: mode.value(), level: mode.value() === 'auto' ? autoLevel.value() : '' }, 'Saved.'); } },
      mode, autoLevel,
      el('p', { class: 'hint', text: 'With automatic adding, any folder you open Claude Code in is saved unless it is excluded below. Exclude client or private folders first.' }),
      el('div', {}, el('button', { type: 'submit', class: 'primary', text: 'Save this choice' })))));

  const ex = el('input', { type: 'text', id: 'ex-path', required: true, autocomplete: 'off', placeholder: 'C:\\Clients' });
  const list = set.data.exclusions || [];
  view.append(el('h2', { text: 'Never save these folders' }),
    el('p', { class: 'quiet', text: 'Nothing in an excluded folder, or below it, is saved or added. This wins over everything else on this page.' }),
    list.length ? el('ul', { class: 'threads' }, list.map(p => el('li', { class: 'thread' }, el('span', { class: 'strand off' }),
      el('div', { class: 'row' }, el('p', { class: 'path', text: p }),
        el('button', { type: 'button', class: 'link', text: 'Stop excluding', 'aria-label': `Stop excluding ${p}`, onclick: () => act('/api/exclusions', { path: p, remove: true }, 'No longer excluded.') })))))
      : el('p', { class: 'quiet', text: 'No folders are excluded.' }),
    el('div', { class: 'table' }, el('form', { onsubmit: e => { e.preventDefault(); act('/api/exclusions', { path: ex.value }, 'Excluded.'); } },
      field('Folder to exclude', ex), el('div', {}, el('button', { type: 'submit', text: 'Exclude this folder' })))));
}

// ---- History --------------------------------------------------------------------------------

async function historyView(projectId, sessionId) {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  if (!projects.length) { lede.textContent = 'Sessions appear here once a project is being saved.'; return; }
  if (sessionId) return sessionView(projectId, sessionId);

  lede.textContent = 'Every saved session, newest first.';
  const proj = projectSelect('history-project', true);
  if (projectId) proj.value = projectId;
  const out = el('div', { 'aria-live': 'polite' });
  const load = async () => {
    const q = proj.value === '*' ? 'all=1' : 'project=' + encodeURIComponent(proj.value);
    const r = await api('GET', `/api/sessions?${q}&limit=100`);
    if (!r.ok) return say(problem(r), 'bad');
    say('');
    out.replaceChildren(r.data.length ? el('ul', { class: 'threads' }, r.data.map(x =>
      el('li', { class: 'thread' }, el('span', { class: 'strand ' + (x.state === 'ended' ? 'off' : 'on') }),
        el('div', {}, el('p', { class: 'quiet small', text: `${x.project_name}, started ${when(x.started_at)}, ${plural(x.text_records, 'piece of text', 'pieces of text')}${x.state === 'ended' ? '' : ', may still be running'}` }),
          el('p', { class: 'said', text: x.first_prompt || '(no text was saved in this session)' }),
          el('a', { href: `#/history/${x.project_id}/${x.id}`, text: 'Read this session' })))))
      : el('p', { class: 'quiet', text: 'No sessions saved here yet.' }));
  };
  proj.addEventListener('change', load);
  view.append(el('div', { class: 'row' }, field('Project', proj)), out);
  await load();
}

async function sessionView(projectId, sessionId) {
  const r = await api('GET', `/api/sessions/${sessionId}?project=${encodeURIComponent(projectId)}`);
  if (!r.ok) return say(problem(r), 'bad');
  const s = r.data.session;
  title.textContent = 'Session';
  lede.textContent = `${s.project_name}, started ${when(s.started_at)}, last activity ${when(s.last_activity_at)}.`;
  const removal = el('div', { 'aria-live': 'polite' });
  view.append(el('p', {}, el('a', { href: '#/history/' + projectId, text: 'Back to all sessions' })), removal,
    el('div', {}, r.data.turns.filter(t => t.text || t.tool).map(t => t.text
      ? el('div', { class: 'turn ' + t.who },
        el('p', { class: 'who' }, t.who === 'user' ? 'You' : t.who === 'assistant' ? 'Claude' : 'Tool', el('small', { text: when(t.observed_at).slice(11) })),
        el('div', {}, el('p', { class: 'said', text: t.text }),
          t.redactions ? el('p', { class: 'quiet small', text: `${plural(t.redactions, 'secret was', 'secrets were')} removed before saving.` }) : null,
          el('button', { type: 'button', class: 'link', text: 'Delete this piece', onclick: () => forget({ kind: 'record', id: t.id, project: projectId }, removal, () => show('history')) })))
      : el('div', { class: 'turn tool' }, el('p', { class: 'who', text: 'Tool' }), el('p', { class: 'said', text: `Claude used ${t.tool}.` })))));
}

// ---- Search ---------------------------------------------------------------------------------

async function searchView() {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  lede.textContent = 'Find anything you or Claude said in a saved session.';
  if (!projects.length) return view.append(el('p', { class: 'quiet', text: 'Add a project first; there is nothing to search yet.' }));
  const q = el('input', { type: 'search', id: 'q', required: true, placeholder: 'A decision, an error message, a file name' });
  const proj = projectSelect('scope', true);
  const who = select('who', [['', 'Anyone'], ['user', 'Me'], ['assistant', 'Claude'], ['tool', 'A tool']], '');
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
    say(d.hits.length ? `${plural(d.hits.length, 'result', 'results')}.` : 'Nothing saved matches that.', d.hits.length ? 'ok' : '');
    if (d.method === 'any') results.append(el('p', { class: 'state-warn', text: 'No piece of text has all of these words. These have some of them.' }));
    if (d.not_searched_pending_events) results.append(el('p', { class: 'state-warn', text: `${plural(d.not_searched_pending_events, 'captured event is', 'captured events are')} not stored yet and could not be searched. Store them from the Overview, then search again.` }));
    for (const h of d.hits) {
      const detail = el('div', {});
      results.append(el('div', { class: 'turn ' + h.who },
        el('p', { class: 'who' }, h.who === 'user' ? 'You' : h.who === 'assistant' ? 'Claude' : 'Tool', el('small', { text: when(h.observed_at) })),
        el('div', {}, el('p', { class: 'said', text: h.snippet.split('**').join('') }),
          el('button', { type: 'button', class: 'link', text: 'Show all of it', onclick: async () => {
            const one = await api('GET', `/api/records/${h.id}?project=${encodeURIComponent(h.project_id)}`);
            if (!one.ok) return say(problem(one), 'bad');
            const x = one.data;
            detail.replaceChildren(el('div', { class: 'quote' },
              el('p', { class: 'quiet small', text: `From ${x.project_name}, ${when(x.observed_at)}.` + (x.redactions ? ` ${plural(x.redactions, 'secret was', 'secrets were')} removed before saving.` : '') + (x.completeness === 'complete' ? '' : ' Only part of it was saved.') }),
              el('p', { class: 'said', text: x.text }),
              el('button', { type: 'button', class: 'link', text: 'Delete this piece', onclick: () => forget({ kind: 'record', id: x.id, project: x.project_id }, detail, () => run('')) })));
          } }), detail)));
    }
    if (d.next_cursor) results.append(el('p', {}, el('button', { type: 'button', text: 'Show more results', onclick: e => { e.target.parentNode.remove(); run(d.next_cursor); } })));
  };
  view.append(el('form', { role: 'search', class: 'searchbox', onsubmit: e => { e.preventDefault(); run(''); } },
    field('Words to look for', q),
    el('div', { class: 'row' }, field('In', proj), field('Said by', who), el('label', { class: 'inline' }, exact, 'Match the exact phrase'),
      el('button', { type: 'submit', class: 'primary', text: 'Search' }))), results);
}

// ---- Instructions ---------------------------------------------------------------------------

async function instructionsView() {
  await loadProjects();
  const r = await api('GET', '/api/instructions?all=1');
  if (!r.ok) return say(problem(r), 'bad');
  lede.textContent = 'Things you asked to be remembered for good. Claude gets them, word for word, at the start of every session.';
  const work = el('div', { id: 'work', 'aria-live': 'polite' });
  const active = r.data.filter(i => i.state === 'active'), retired = r.data.filter(i => i.state !== 'active');
  const row = i => {
    const p = projects.find(x => x.id === i.project_id);
    return el('li', { class: 'thread' }, el('span', { class: 'strand ' + (i.state === 'active' ? 'on' : 'off') }), el('div', {},
      el('p', { class: 'quiet small', text: (i.scope === 'global' ? 'Every project' : (p ? p.name : 'A deleted project')) + (i.revision > 1 ? `, changed ${plural(i.revision - 1, 'time', 'times')}` : '') + (i.state === 'active' ? '' : ', retired') }),
      el('p', { class: 'said', text: i.text }),
      el('div', { class: 'row' },
        i.state === 'active' ? el('button', { type: 'button', class: 'link', text: 'Change the wording', onclick: () => change(i, 'supersede') }) : null,
        i.state === 'active' ? el('button', { type: 'button', class: 'link', text: 'Retire it', onclick: () => change(i, 'archive') }) : null,
        el('button', { type: 'button', class: 'link', text: 'See earlier wording', onclick: () => history(i.id) }))));
  };
  view.append(active.length ? el('ul', { class: 'threads' }, active.map(row)) : el('p', { class: 'quiet', text: 'No instructions yet.' }), work);

  const text = el('textarea', { id: 'new-text', required: true });
  const scope = choice('new-scope', 'Where it applies', [['project', 'One project', 'Only sessions in the project chosen below.'], ['global', 'Every project', 'Every project on the Projects page.']], '');
  const proj = projectSelect('new-project', false);
  const preview = el('div', {});
  const submit = async confirm => {
    const body = { text: text.value, scope: scope.value(), confirm };
    if (scope.value() === 'project') body.project = proj.value;
    const res = await api('POST', '/api/instructions', body);
    if (res.ok) { await show('instructions'); return say('Remembered.', 'ok'); }
    if (res.status === 409 && res.data && res.data.saved === false) {
      say('Not saved yet. Read it once more, then confirm.');
      preview.replaceChildren(el('div', { class: 'preview' },
        el('h3', { text: res.data.scope === 'global' ? 'Remember this in every project?' : 'Remember this in the chosen project?' }),
        el('p', { class: 'said', text: res.data.text }),
        el('button', { type: 'button', class: 'primary', text: 'Remember it', onclick: () => submit(true) })));
      return preview.querySelector('button').focus();
    }
    say(problem(res), 'bad');
  };
  view.append(el('h2', { text: 'Add an instruction' }), el('div', { class: 'table' },
    el('form', { onsubmit: e => { e.preventDefault(); submit(false); } },
      field('What should Claude always know or do', text, 'Your own words. They are saved and repeated exactly.'), scope, field('Project', proj),
      el('div', {}, el('button', { type: 'submit', class: 'primary', text: 'Check before saving' })))), preview);
  if (retired.length) view.append(el('details', {}, el('summary', { text: `Retired instructions (${retired.length})` }), el('ul', { class: 'threads' }, retired.map(row))));

  async function history(id) {
    const h = await api('GET', `/api/instructions/${id}`);
    if (!h.ok) return say(problem(h), 'bad');
    work.replaceChildren(el('h2', { text: 'Earlier wording' }), ...h.data.versions.map(v => el('div', { class: 'quote' },
      el('p', { class: 'quiet small', text: `${v.status === 'active' ? 'In use now' : v.status === 'superseded' ? 'Replaced' : 'Retired'}, written ${when(v.created_at)}${v.reason ? '. Reason: ' + v.reason : ''}` }), el('p', { class: 'said', text: v.text }))));
  }
  function change(i, action) {
    const t = el('textarea', { id: 'chg-text' }); t.value = i.text;
    const reason = el('input', { type: 'text', id: 'chg-reason' });
    const go = async () => {
      const res = await api('POST', `/api/instructions/${i.id}/${action}`, { text: action === 'supersede' ? t.value : '', reason: reason.value, confirm: true });
      if (res.ok) { await show('instructions'); say(action === 'archive' ? 'Retired. Claude no longer gets it.' : 'Changed. The earlier wording is kept.', 'ok'); } else say(problem(res), 'bad');
    };
    work.replaceChildren(el('div', { class: 'preview' },
      el('h3', { text: action === 'archive' ? 'Retire this instruction?' : 'Change the wording' }),
      el('p', { class: 'said', text: i.text }),
      el('form', { onsubmit: e => { e.preventDefault(); go(); } },
        action === 'supersede' ? field('New wording', t) : null, field('Why', reason, 'Optional. Kept with the history.'),
        el('div', { class: 'row' }, el('button', { type: 'submit', class: 'primary', text: action === 'archive' ? 'Retire it' : 'Save the new wording' }),
          el('button', { type: 'button', text: 'Leave it as it is', onclick: () => work.replaceChildren() })))));
    work.querySelector('textarea, input').focus();
  }
}

// ---- Pick up work ---------------------------------------------------------------------------

async function resumeView() {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  lede.textContent = 'Where each project stood when you last stopped, and whether the code has moved since.';
  if (!projects.length) return view.append(el('p', { class: 'quiet', text: 'Add a project first.' }));
  const proj = projectSelect('resume-project', false);
  const out = el('div', { 'aria-live': 'polite' });
  const load = async () => {
    const r = await api('GET', '/api/resume?project=' + encodeURIComponent(proj.value));
    if (!r.ok) return say(problem(r), 'bad');
    const d = r.data, c = d.checkpoints[0], g = d.git_now, warnings = d.warnings || [];
    say('');
    out.replaceChildren(
      el('h2', { text: 'Last checkpoint' }),
      c ? el('div', { class: 'quote' },
        el('p', { class: 'quiet small', text: `Written ${when(c.created_at)}` + (c.git_repo ? `, on branch ${c.branch}, with ${plural(c.changed_files, 'uncommitted file', 'uncommitted files')}.` : ', outside Git.') }),
        el('p', { class: 'said', text: c.note }))
        : el('p', { class: 'quiet', text: 'No checkpoint yet. Write one below before you stop, or ask Claude to save your progress.' }),
      ...warnings.map(w => el('p', { class: 'state-bad', text: 'Changed since then: ' + w + '.' })),
      c && !warnings.length ? el('p', { class: 'state-ok', text: g.repo ? 'The code is exactly as it was: same branch, same commit, same uncommitted work.' : 'Nothing to compare: this folder is not a Git repository.' }) : null,
      el('h2', { text: 'Last things said' }),
      ...(d.recent.length ? d.recent.map(e => el('div', { class: 'turn ' + e.who },
        el('p', { class: 'who' }, e.who === 'user' ? 'You' : 'Claude', el('small', { text: when(e.observed_at) })), el('p', { class: 'said', text: e.text })))
        : [el('p', { class: 'quiet', text: 'Nothing saved yet.' })]),
      d.not_included_pending_events ? el('p', { class: 'state-warn', text: `${plural(d.not_included_pending_events, 'captured event is', 'captured events are')} not stored yet and missing here.` }) : null);
  };
  proj.addEventListener('change', load);
  const note = el('textarea', { id: 'cp-note', required: true });
  view.append(el('div', { class: 'row' }, field('Project', proj)), out,
    el('h2', { text: 'Write a checkpoint' }), el('div', { class: 'table' },
      el('form', { onsubmit: async e => { e.preventDefault(); const r = await api('POST', '/api/checkpoints', { note: note.value, project: proj.value }); if (r.ok) { note.value = ''; await load(); say('Checkpoint saved.', 'ok'); } else say(problem(r), 'bad'); } },
        field('What is done, what comes next, anything blocking', note, 'Write it for someone with no memory of today. The branch and commit are recorded for you.'),
        el('div', {}, el('button', { type: 'submit', class: 'primary', text: 'Save checkpoint' })))));
  await load();
}

// ---- Health ---------------------------------------------------------------------------------

const CHECK_NAMES = { build: 'Program', data_location: 'Where data is kept', settings: 'Settings file', installation: 'Installed copy', plugin_hooks: 'Claude Code connection', sqlite_engine: 'Database engine' };

async function diagnostics() {
  const r = await api('GET', '/api/doctor');
  if (!r.data) return say(problem(r), 'bad');
  lede.textContent = r.data.healthy ? 'Everything checked is working.' : 'Something needs attention.';
  view.append(el('ul', { class: 'threads' }, r.data.checks.map(c => el('li', { class: 'thread' }, el('span', { class: 'strand ' + (c.status === 'ok' ? 'on' : 'bad') }),
    el('div', {}, el('h3', { text: CHECK_NAMES[c.name] || c.name }), el('p', { class: c.status === 'ok' ? 'state-ok' : 'state-bad', text: c.status === 'ok' ? 'Working' : 'Not working' }), el('p', { class: 'path', text: c.detail }))))),
    el('p', { class: 'quiet small', text: 'Not checked here: ' + r.data.not_checked + '.' }));
}

// ---- Routing --------------------------------------------------------------------------------

const views = { overview: ['Overview', overview], projects: ['Projects', projectsView], history: ['History', historyView], search: ['Search', searchView],
  instructions: ['Instructions', instructionsView], resume: ['Pick up work', resumeView], diagnostics: ['Health', diagnostics] };
let current = 'overview';

async function show(route) {
  const [name, ...args] = String(route || '').split('/');
  current = views[name] ? name : 'overview';
  const [label, render] = views[current];
  title.textContent = label;
  lede.textContent = '';
  document.title = label + ' - SessionKeep';
  for (const a of document.querySelectorAll('#nav a')) {
    if (a.dataset.view === current) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  }
  view.replaceChildren();
  say('Loading...');
  await render(...args);
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
