// SessionKeep dashboard. Plain modules, no dependencies, nothing loaded from elsewhere.
// Every piece of data is put on the page with textContent, never as markup.

const $ = id => document.getElementById(id);
const view = $('view'), title = $('title'), lede = $('lede'), notice = $('notice'), crumb = $('crumb'), actions = $('actions'), dialog = $('dialog');
let csrf = '';
let projects = [];
let scope = '*';       // the project chosen in the sidebar; '*' is every project
let current = 'overview';
let selectors = {};    // per page: show another row in the pane beside the list without redrawing the page

function el(tag, props = {}, ...children) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(props)) {
    if (v == null || v === false) continue;
    if (k === 'class') node.className = v;
    else if (k === 'text') node.textContent = v;
    else if (k.startsWith('on')) node.addEventListener(k.slice(2), k === 'onclick' || k === 'onsubmit' ? busyWhile(v) : v);
    else if (v === true) node.setAttribute(k, '');
    else node.setAttribute(k, v);
  }
  for (const c of children.flat(Infinity)) {
    if (c == null || c === false) continue;
    node.append(c.nodeType ? c : document.createTextNode(String(c)));
  }
  return node;
}

// While a click or a submitted form is still being worked on, its button turns in place and
// cannot be pressed again. A handler takes part by returning its promise.
function busyWhile(handler) {
  return function (e) {
    const r = handler.call(this, e);
    if (!r || typeof r.then !== 'function') return r;
    const b = e.type === 'submit' ? (e.submitter || e.currentTarget.querySelector('button[type=submit]')) : e.currentTarget;
    if (!b || b.disabled) return r;
    b.disabled = true;
    b.classList.add('busy');
    b.setAttribute('aria-busy', 'true');
    const done = () => { b.disabled = false; b.classList.remove('busy'); b.removeAttribute('aria-busy'); };
    r.then(done, done);
    return r;
  };
}

// A moving bar with a sentence, shown where the result will appear.
function working(text) {
  return el('div', { class: 'working' }, el('span', { class: 'bar', 'aria-hidden': 'true' }), el('span', { text }));
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

// ---- small helpers --------------------------------------------------------------------------

const enc = encodeURIComponent;
const when = iso => (iso || '').replace('T', ' ').slice(0, 16);
const dateOnly = iso => (iso || '').slice(0, 10);
const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;
const clip = (s, n) => s.length > n ? s.slice(0, n - 1).trimEnd() + '\u2026' : s;
const MONTHS = ['Jan', 'Feb', 'Mar', 'Apr', 'May', 'Jun', 'Jul', 'Aug', 'Sep', 'Oct', 'Nov', 'Dec'];
const two = n => String(n).padStart(2, '0');

// "Today 10:14", "Yesterday 16:30", "13 Nov 19:05", in this computer's time.
function nice(iso) {
  const d = new Date(String(iso || '').replace(/(\.\d{3})\d+/, '$1'));
  if (isNaN(d)) return when(iso);
  const clockText = `${two(d.getHours())}:${two(d.getMinutes())}`;
  const days = Math.round((new Date().setHours(0, 0, 0, 0) - new Date(d).setHours(0, 0, 0, 0)) / 86400000);
  if (days === 0) return 'Today ' + clockText;
  if (days === 1) return 'Yesterday ' + clockText;
  return `${d.getDate()} ${MONTHS[d.getMonth()]}${d.getFullYear() === new Date().getFullYear() ? '' : ' ' + d.getFullYear()} ${clockText}`;
}
const clock = iso => nice(iso).split(' ').pop();

const route = () => location.hash.replace(/^#\/?/, '') || 'overview';
const reload = () => show(route(), true);
const inScope = pid => scope === '*' || scope === pid;
const scoped = () => projects.filter(p => inScope(p.id));
const projectName = id => (projects.find(p => p.id === id) || { name: 'a deleted project' }).name;
const scopeName = () => scope === '*' ? 'all projects' : projectName(scope);

const chip = (kind, text) => el('span', { class: 'chip ' + kind, text });
const dotword = (kind, text) => el('span', { class: 'dotword ' + kind, text });
const btn = (text, cls, onclick, label) => el('button', { type: 'button', class: cls || 'btn', text: text, onclick, 'aria-label': label });
const card = (heading, ...children) => el('section', { class: 'card' }, heading ? el('h2', { text: heading }) : null, children);

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

// Buttons of which exactly one is pressed.
function seg(label, options, value, onpick, cls) {
  const group = el('div', { class: cls || 'seg', role: 'group', 'aria-label': label });
  const paint = v => { for (const b of group.children) b.setAttribute('aria-pressed', String(b.dataset.v === v)); };
  for (const [v, text] of options) group.append(el('button', { type: 'button', 'data-v': v, text: text, onclick: () => { paint(v); return onpick(v); } }));
  paint(value);
  return group;
}

// Marks the row with this id as the one shown beside the list.
function mark(list, id) {
  for (const a of list.querySelectorAll('a.pick')) {
    if (a.dataset.id === id) a.setAttribute('aria-current', 'true'); else a.removeAttribute('aria-current');
  }
}

// Text as paragraphs, with fenced code kept in its own block.
function prose(text) {
  const out = [];
  String(text).split('```').forEach((part, i) => {
    if (i % 2) out.push(el('pre', { text: part.replace(/^[^\n]*\n/, '').replace(/\n$/, '') }));
    else if (part.trim()) out.push(el('p', { class: 'said', text: part.trim() }));
  });
  return out;
}

const LEVELS = [
  ['conversation', 'Conversation', 'What you ask and what Claude answers. The usual choice.'],
  ['extended', 'Conversation and tool output', 'Also short excerpts of what tools printed.'],
  ['metadata', 'Activity only', 'Times and tool names. No text at all.'],
  ['off', 'Nothing', 'Registered, but nothing is saved.'],
];
const levelName = v => (LEVELS.find(l => l[0] === v) || [v, v])[1];

function fillScope() {
  const s = $('scope');
  if (scope !== '*' && !projects.some(p => p.id === scope)) scope = '*';
  s.replaceChildren(el('option', { value: '*', text: 'All projects' }), ...projects.map(p => el('option', { value: p.id, text: p.name })));
  s.value = scope;
}

async function loadProjects() {
  const r = await api('GET', '/api/projects');
  projects = r.ok ? r.data : [];
  fillScope();
  const saving = projects.filter(p => !p.paused && p.capture_level !== 'off').length;
  $('saving').textContent = saving ? `Saving in ${plural(saving, 'project', 'projects')}` : 'Saving nothing';
  $('saving').parentNode.classList.toggle('idle', !saving);
  return r;
}

function projectSelect(id) {
  const opts = projects.map(p => [p.id, p.name]);
  return select(id, opts, scope !== '*' ? scope : opts.length ? opts[0][0] : '');
}

// [kind, word, sentence] for how a project is being saved.
function savingState(p) {
  if (p.paused) return ['warn', 'Paused', `Paused. It was saving: ${levelName(p.capture_level).toLowerCase()}.`];
  if (p.capture_level === 'off') return ['off', 'Off', 'Saving nothing.'];
  return ['ok', 'Saving', `Saving: ${levelName(p.capture_level).toLowerCase()}.`];
}

function badge(id, n, kind, words) {
  const b = $(id);
  b.textContent = n ? (words || String(n)) : '';
  b.className = 'count ' + kind;
}

function openDialog(heading, body, buttons) {
  dialog.replaceChildren(el('h2', { id: 'dialog-title', text: heading }), ...body, el('div', { class: 'row end' }, buttons));
  dialog.showModal();
  dialog.querySelector('button').focus();
}

// Deletion always shows what would be removed first; nothing is deleted until confirmed.
async function forget(body, after) {
  const r = await api('POST', '/api/forget', body);
  if (r.status !== 409 || !r.data || r.data.deleted !== false) return say(problem(r), 'bad');
  const w = r.data.would_remove;
  const rows = [['Sessions', w.sessions], ['Pieces of text', w.text_records], ['Events', w.events], ['Instructions', w.instructions], ['Checkpoints', w.checkpoints], ['Projects', w.projects]].filter(x => x[1]);
  openDialog('Delete permanently?', [
    el('p', { class: 'said', text: r.data.target }),
    el('p', { class: 'quiet', text: 'This removes it from this computer. It cannot be undone.' }),
    el('div', { class: 'tally' }, el('p', { class: 'head', text: 'Will be deleted' }),
      rows.length ? rows.map(([k, v]) => el('p', {}, el('span', { text: k }), el('strong', { text: String(v) }))) : el('p', { text: 'Nothing is stored for it.' })),
    el('p', { class: 'quiet small', text: 'Exports you made earlier still contain it, and so do Claude Code\u2019s own session files.' }),
  ], [
    btn('Keep everything', 'btn', () => dialog.close()),
    btn('Delete permanently', 'danger solid', async () => {
      const done = await api('POST', '/api/forget', { ...body, confirm: true });
      dialog.close();
      if (!done.ok) return say(problem(done), 'bad');
      await after();
      say('Deleted.', 'ok');
    }),
  ]);
}

async function act(path, body, done) {
  const r = await api('POST', path, body);
  if (r.ok) { await reload(); say(done, 'ok'); } else say(problem(r), 'bad');
  return r;
}

// A checkpoint note written as "Done: / Next: / Blocking:" lines is shown in those three parts.
function noteParts(note) {
  const parts = {};
  let key = '';
  for (const line of String(note || '').split('\n')) {
    const m = /^(done|next|blocking)\s*:\s*(.*)$/i.exec(line.trim());
    if (m) { key = m[1].toLowerCase(); parts[key] = m[2]; } else if (key && line.trim()) parts[key] += ' ' + line.trim();
  }
  return Object.keys(parts).length ? parts : null;
}

// ---- What needs attention (also fills the counts in the sidebar) ----------------------------

async function attention(status) {
  const items = [], cps = [];
  let facts = 0;
  if (status.spool_pending) {
    items.push({ kind: 'bad', tag: 'Not stored', title: `${plural(status.spool_pending, 'captured event is', 'captured events are')} not stored yet`,
      detail: `Waiting since ${nice(status.spool_oldest)}. They cannot be searched until they are stored.`, action: 'Store them now', primary: true,
      run: async () => { const r = await api('POST', '/api/ingest'); if (r.ok) { await reload(); say(`Stored ${plural(r.data.ingested, 'event', 'events')}.`, 'ok'); } else say(problem(r), 'bad'); } });
  }
  await Promise.all(projects.map(async p => {
    const [rs, cp] = await Promise.all([api('GET', '/api/research?project=' + enc(p.id)), api('GET', '/api/resume?project=' + enc(p.id))]);
    for (const c of (rs.ok && rs.data.claims) || []) {
      if (c.status !== 'disputed' && c.status !== 'stale') continue;
      facts++;
      items.push(c.status === 'disputed'
        ? { kind: 'bad', tag: 'Disputed', title: c.subject, detail: `${p.name}: the source page no longer contains the quoted words.`, action: 'Look at the fact', href: `#/research/${p.id}/${c.id}` }
        : { kind: 'warn', tag: 'Out of date', title: c.subject, detail: `${p.name}: it should have been checked again by ${dateOnly(c.expires_at)}.`, action: 'Look at the fact', href: `#/research/${p.id}/${c.id}` });
    }
    if (cp.ok && cp.data.checkpoints[0]) {
      const w = cp.data.warnings || [];
      cps.push({ p, c: cp.data.checkpoints[0], w });
      if (w.length) items.push({ kind: 'warn', tag: 'Code moved', title: `${p.name} changed since its last checkpoint`, detail: w.join('; ') + '.', action: 'See what changed', href: '#/resume/' + p.id });
    }
    if (p.paused) items.push({ kind: 'warn', tag: 'Paused', title: `${p.name} is paused`, detail: 'Nothing is saved there until you resume.', action: 'Resume saving', run: () => act(`/api/projects/${p.id}/resume`, {}, 'Saving again.') });
  }));
  const order = { bad: 0, warn: 1, off: 2 };
  items.sort((a, b) => order[a.kind] - order[b.kind]);
  cps.sort((a, b) => a.c.created_at < b.c.created_at ? 1 : -1);
  badge('n-overview', items.length, items.some(i => i.kind === 'bad') ? 'bad' : 'warn');
  badge('n-research', facts, 'bad');
  return { items, cps };
}

async function healthBadge() {
  const r = await api('GET', '/api/doctor');
  const failing = r.data ? r.data.checks.filter(c => c.status !== 'ok').length : 0;
  badge('n-health', failing, 'bad', `${failing} not working`);
}

function checkpointCard({ p, c, w }) {
  const parts = noteParts(c.note);
  return el('article', { class: 'card' },
    el('div', { class: 'between' }, el('strong', { class: 'mono', text: p.name }), el('span', { class: 'quiet small', text: nice(c.created_at) })),
    c.git_repo ? el('p', { class: 'path', text: 'branch ' + c.branch }) : null,
    el('p', { class: 'next' }, parts && parts.next ? el('span', { class: 'quiet', text: 'Next: ' }) : null, clip(parts && parts.next ? parts.next : c.note, 180)),
    w.length ? dotword('warn', 'Code changed since') : dotword('ok', 'Code is exactly as it was'),
    el('a', { href: '#/resume/' + p.id, text: 'Pick up here' }));
}

// ---- Overview -------------------------------------------------------------------------------

async function overview() {
  const [st, , ses] = await Promise.all([api('GET', '/api/status'), loadProjects(), api('GET', '/api/sessions?all=1&limit=5')]);
  if (!st.ok) return say(problem(st), 'bad');
  const s = st.data;
  if (!projects.length) {
    lede.textContent = 'Nothing is being saved yet.';
    return view.append(el('div', { class: 'empty' },
      el('h2', { text: 'No project added yet' }),
      el('p', { class: 'quiet', text: 'SessionKeep only saves conversations in folders you add. Right now nothing is being saved.' }),
      el('a', { class: 'go', href: '#/projects', text: 'Add your first project' }),
      el('p', { class: 'quiet small' }, 'Or in a terminal, inside the folder: ', el('code', { text: 'sessionkeep project add .' }))));
  }
  const saving = projects.filter(p => !p.paused && p.capture_level !== 'off');
  lede.textContent = `Saving works in ${saving.length} of ${plural(projects.length, 'project', 'projects')}.`;
  const heading = el('h2', { text: 'Needs attention' });
  const att = el('div', { 'aria-live': 'polite' }, working('Looking for things that need you.'));
  const cps = el('div', { class: 'cards' }, working('Reading the last checkpoints.'));
  view.append(el('div', { class: 'cols' },
    el('div', { class: 'col-main' },
      el('section', { class: 'card flush' }, el('div', { class: 'cardhead' }, heading, el('span', { class: 'quiet small', text: 'Each has one action. Nothing happens until you press it.' })), att),
      el('section', {}, el('div', { class: 'sechead' }, el('h2', { text: 'Pick up where you stopped' }), el('a', { href: '#/resume', text: 'Open Pick up work' })), cps)),
    el('aside', { class: 'col-side' },
      el('section', { class: 'card side-card' }, el('h2', { text: 'Projects' }), projects.map(p => { const [kind, word] = savingState(p); return el('div', { class: 'kv' }, el('span', { class: 'mono', text: p.name }), dotword(kind, word)); })),
      el('section', { class: 'card side-card' }, el('h2', { text: 'Stored on this computer' }), el('dl', { class: 'counts' },
        [['projects', s.stored.projects], ['sessions', s.stored.sessions], ['pieces of text', s.stored.evidence], ['events', s.stored.events], ['gaps on record', s.stored.capture_gaps], ['unreadable files set aside', s.quarantined_files]]
          .map(([k, v]) => el('div', {}, el('dt', { text: k }), el('dd', { text: Number(v || 0).toLocaleString('en') }))))),
      el('section', { class: 'card side-card' }, el('h2', { text: 'Latest sessions' }),
        ses.ok && ses.data.length ? ses.data.map(x => el('a', { class: 'latest', href: `#/history/${x.project_id}/${x.id}/read` },
          el('span', { class: 'top' }, el('span', { class: 'mono', text: x.project_name }), el('span', { text: nice(x.started_at) })),
          el('span', { class: 'line', text: x.first_prompt || '(no text was saved in this session)' })))
          : el('p', { class: 'quiet small', text: 'No session has been saved yet.' })))));

  const found = await attention(s);
  if (!att.isConnected) return;
  const paint = () => {
    heading.textContent = `Needs attention (${found.items.length})`;
    att.replaceChildren(...(found.items.length ? found.items.map(a => el('div', { class: 'attn' },
      el('span', { class: 'tag chip ' + a.kind, text: a.tag }),
      el('div', { class: 'what' }, el('b', { text: a.title }), el('span', { text: a.detail })),
      a.href ? el('a', { href: a.href, text: a.action }) : btn(a.action, a.primary ? 'btn primary' : 'btn', a.run)))
      : [el('p', { class: 'working', text: 'Nothing needs you right now.' })]));
  };
  paint();
  cps.replaceChildren(...(found.cps.length ? found.cps.map(checkpointCard)
    : [el('p', { class: 'quiet', text: 'No checkpoint has been written yet. Write one on the Pick up work page before you stop, or ask Claude to save your progress.' })]));

  // Claude Code takes a moment to answer, so accounts are added after the rest.
  const ac = await api('GET', '/api/accounts/plugin');
  if (!att.isConnected || !ac.ok) return;
  for (const a of ac.data.accounts || []) {
    if (a.plugin === 'off') found.items.push({ kind: 'off', tag: 'Off', title: `SessionKeep is off for the account "${a.name}"`, detail: 'Sessions started with that account are not saved.', action: 'Open settings', href: '#/settings' });
  }
  paint();
  badge('n-overview', found.items.length, found.items.some(i => i.kind === 'bad') ? 'bad' : 'warn');
}

// ---- Pick up work ---------------------------------------------------------------------------

async function resumeView(pid) {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  if (!projects.length) return view.append(el('p', { class: 'quiet', text: 'Add a project first.' }));
  if (pid && projects.some(p => p.id === pid)) { scope = pid; fillScope(); }
  lede.textContent = 'Where the work stood when you last stopped, and whether the code has moved since.';
  if (scope === '*') {
    const all = await Promise.all(projects.map(async p => { const r = await api('GET', '/api/resume?project=' + enc(p.id)); return r.ok && r.data.checkpoints[0] ? { p, c: r.data.checkpoints[0], w: r.data.warnings || [] } : null; }));
    const have = all.filter(Boolean);
    return view.append(el('p', { class: 'quiet', text: 'Choose a project in the sidebar to see what was said last and to write a checkpoint.' }),
      have.length ? el('div', { class: 'cards' }, have.map(checkpointCard)) : el('p', { class: 'quiet', text: 'No checkpoint has been written in any project yet.' }));
  }
  const p = projects.find(x => x.id === scope);
  crumb.textContent = `${p.name}, ${p.roots.join(', ')}`;
  const r = await api('GET', '/api/resume?project=' + enc(p.id));
  if (!r.ok) return say(problem(r), 'bad');
  const d = r.data, c = d.checkpoints[0], warnings = d.warnings || [], parts = c && noteParts(c.note);

  const done = el('textarea', { id: 'cp-done', rows: '2' });
  const next = el('textarea', { id: 'cp-next', rows: '2' });
  const blocking = el('textarea', { id: 'cp-blocking', rows: '2' });
  const save = async e => {
    e.preventDefault();
    const note = [['Done', done], ['Next', next], ['Blocking', blocking]].filter(x => x[1].value.trim()).map(x => `${x[0]}: ${x[1].value.trim()}`).join('\n');
    if (!note) return say('Write at least one of the three before saving.', 'bad');
    const res = await api('POST', '/api/checkpoints', { note, project: p.id });
    if (res.ok) { await reload(); say('Checkpoint saved.', 'ok'); } else say(problem(res), 'bad');
  };

  view.append(el('div', { class: 'cols' },
    el('div', { class: 'col-main' },
      el('section', { class: 'card flush' },
        el('div', { class: 'cardhead' }, el('h2', { text: 'Last checkpoint' }),
          c ? el('span', { class: 'quiet small', text: nice(c.created_at) + (c.git_repo ? `, branch ${c.branch}, ${plural(c.changed_files, 'uncommitted file', 'uncommitted files')}` : ', outside Git') }) : null),
        !c ? el('p', { class: 'working', text: 'No checkpoint yet. Write one here before you stop, or ask Claude to save your progress.' })
          : parts ? el('dl', { class: 'cp' }, [['Done', parts.done, ''], ['Next', parts.next, 'next'], ['Blocking', parts.blocking, '']].filter(x => x[1]).map(([k, v, cls]) => [el('dt', { text: k }), el('dd', { class: cls, text: v })]))
            : el('div', { class: 'cardbody' }, el('p', { class: 'said', text: c.note })),
        c && warnings.length ? el('p', { class: 'banner bad' }, el('strong', { text: 'Changed since then:' }), el('span', { text: warnings.join('; ') + '.' })) : null,
        c && !warnings.length ? el('p', { class: 'banner ok' }, el('strong', { text: d.git_now.repo ? 'The code is exactly as it was.' : 'Nothing to compare.' }),
          el('span', { class: 'quiet', text: d.git_now.repo ? 'Same branch, same commit, same uncommitted work.' : 'This folder is not a Git repository.' })) : null),
      el('section', {}, el('div', { class: 'sechead' }, el('h2', { text: 'The last things said here' })),
        d.recent.length ? el('dl', { class: 'lastsaid' }, d.recent.map(e => [el('dt', { text: `${e.who === 'user' ? 'You' : 'Claude'} ${nice(e.observed_at)}` }), el('dd', { class: 'said ' + (e.who === 'user' ? 'you' : ''), text: e.text })]))
          : el('p', { class: 'quiet', text: 'Nothing saved yet.' }),
        d.not_included_pending_events ? el('p', { class: 'banner warn' }, el('strong', { text: 'Missing here:' }), el('span', { text: `${plural(d.not_included_pending_events, 'captured event is', 'captured events are')} not stored yet. Store them from the Overview.` })) : null)),
    el('div', { class: 'col-side' }, card('Write a new checkpoint',
      el('form', { onsubmit: save },
        field('What is done', done), field('What comes next', next), field('What is blocking', blocking),
        el('p', { class: 'hint', text: 'Write it for someone with no memory of today. It is saved with the current branch and commit, and Claude sees it at the start of the next session here.' }),
        el('button', { type: 'submit', class: 'primary', text: 'Save checkpoint' }))))));
}

// ---- History --------------------------------------------------------------------------------

function summaryBox(m) {
  return m ? el('div', { class: 'summary' },
    el('h3', { text: `Summary written by a model (${m.model}), not what was said` }),
    el('p', { class: 'said', text: m.text }),
    el('p', { class: 'quiet small', text: `Written ${nice(m.created_at)} from ${m.source_records_given_to_model} of ${plural(m.source_records, 'piece of text', 'pieces of text')}.` })) : null;
}

function turnNode(t, del, hit) {
  if (!t.text) return el('div', { class: 'turn tool' }, el('span', { text: `Claude used ${t.tool || 'a tool'}` }), el('span', { class: 'at', text: clock(t.observed_at) }));
  const who = t.who === 'user' ? 'You' : t.who === 'assistant' ? 'Claude' : 'Tool output';
  const meta = el('div', { class: 'meta' }, el('span', { text: `${who}, ${clock(t.observed_at)}` }),
    del ? el('button', { type: 'button', class: 'link quiet', text: 'Delete this piece', onclick: () => del(t) }) : null);
  const secret = t.redactions ? el('p', { class: 'secret', text: `${plural(t.redactions, 'secret was', 'secrets were')} removed before saving` }) : null;
  const node = t.who === 'user'
    ? el('div', { class: 'turn user' }, meta, el('div', { class: 'bubble said', text: t.text }), secret)
    : el('div', { class: 'turn ' + t.who }, el('span', { class: 'avatar', 'aria-hidden': 'true', text: t.who === 'assistant' ? 'C' : 'T' }), el('div', { class: 'body' }, meta, prose(t.text), secret));
  if (t.id) node.dataset.id = t.id;
  if (hit && t.id === hit) node.classList.add('hit');
  return node;
}

const sessionState = x => x.state === 'ended' ? chip('', 'Ended') : chip('ok', 'May still be running');

async function historyView(pid, sid, mode, rid) {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  if (!projects.length) { lede.textContent = 'Sessions appear here once a project is being saved.'; return; }
  if (sid && mode === 'read') return sessionView(pid, sid, rid);
  // Every saved session, read 200 at a time.
  const all = [];
  for (let page = 0; page < 100; page++) {
    const r = await api('GET', `/api/sessions?${scope === '*' ? 'all=1' : 'project=' + enc(scope)}&limit=200&offset=${all.length}`);
    if (!r.ok) return say(problem(r), 'bad');
    all.push(...r.data);
    if (r.data.length < 200) break;
  }
  lede.textContent = `All ${plural(all.length, 'saved session', 'saved sessions')} in ${scopeName()}, newest first. They stay here when Claude Code clears or deletes its own copy.`;
  view.className = 'wide';
  let chosen = '', filter = 'all';
  const list = el('div', { class: 'rows' });
  const pane = el('section', { class: 'pane-read', 'aria-label': 'Session preview', 'aria-live': 'polite' });
  const shown = () => all.filter(x => filter === 'all' || (filter === 'ended') === (x.state === 'ended'));

  const preview = async x => {
    pane.replaceChildren(working('Opening the session.'));
    const one = await api('GET', `/api/sessions/${x.id}?project=${enc(x.project_id)}`);
    if (chosen !== x.id) return;
    if (!one.ok) return pane.replaceChildren(el('p', { class: 'quiet', text: problem(one) }));
    const turns = one.data.turns.filter(t => t.text);
    pane.replaceChildren(el('div', { class: 'reading' },
      el('div', { class: 'between' }, el('p', { class: 'crumb', text: `${x.project_name}, ${nice(x.started_at)}, ${plural(x.text_records, 'piece of text', 'pieces of text')}` }), sessionState(x)),
      el('h2', { text: clip(x.first_prompt || '(no text was saved in this session)', 240) }),
      summaryBox(one.data.summary),
      turns.slice(0, 3).map(t => turnNode(t)),
      turns.length > 3 ? el('p', { class: 'quiet small', text: `${plural(turns.length - 3, 'more piece', 'more pieces')} in this session.` }) : null,
      el('div', { class: 'row' }, el('a', { class: 'go', href: `#/history/${x.project_id}/${x.id}/read`, text: 'Read the whole conversation' })),
      el('p', { class: 'quiet small', text: 'The up and down arrow keys move through the list without leaving this page.' })));
  };
  const choose = id => {
    const x = shown().find(v => v.id === id) || shown()[0];
    if (!x) { chosen = ''; return pane.replaceChildren(el('p', { class: 'quiet', text: 'No sessions here.' })); }
    chosen = x.id;
    mark(list, x.id);
    return preview(x);
  };
  const paint = () => {
    list.replaceChildren(...shown().map(x => el('a', { class: 'pick', href: `#/history/${x.project_id}/${x.id}`, 'data-id': x.id },
      el('span', { class: 'top' }, el('span', { class: 'name', text: x.project_name }), el('span', { text: nice(x.started_at) })),
      el('span', { class: 'main', text: clip(x.first_prompt || '(no text was saved in this session)', 160) }),
      el('span', { class: 'sub' }, el('span', { text: plural(x.text_records, 'piece', 'pieces') }), el('span', { class: x.state === 'ended' ? '' : 'word-ok', text: x.state === 'ended' ? 'Ended' : 'May still be running' })))));
    return choose(chosen);
  };
  list.addEventListener('keydown', e => {
    if (e.key !== 'ArrowDown' && e.key !== 'ArrowUp') return;
    const rows = [...list.children], at = rows.findIndex(a => a.dataset.id === chosen);
    const to = rows[at + (e.key === 'ArrowDown' ? 1 : -1)];
    if (!to) return;
    e.preventDefault();
    to.focus();
    history.replaceState(null, '', to.getAttribute('href'));
    choose(to.dataset.id);
  });
  view.append(all.length ? el('div', { class: 'split' },
    el('section', { class: 'pane-list', 'aria-label': 'Sessions' },
      el('div', { class: 'tools' }, seg('Show', [['all', 'All'], ['running', 'Running'], ['ended', 'Ended']], 'all', v => { filter = v; paint(); })), list), pane)
    : el('p', { class: 'working', text: 'No sessions saved here yet.' }));
  chosen = sid || '';
  paint();
  selectors.history = args => { if (args[2] === 'read') return false; choose(args[1] || ''); return true; };
}

async function sessionView(pid, sid, rid) {
  const r = await api('GET', `/api/sessions/${sid}?project=${enc(pid)}`);
  if (!r.ok) return say(problem(r), 'bad');
  const s = r.data.session;
  title.textContent = 'Session';
  crumb.textContent = `${s.project_name}, ${nice(s.started_at)}, ${plural(s.text_records, 'piece of text', 'pieces of text')}, ${s.state === 'ended' ? 'ended' : 'may still be running'}`;
  lede.textContent = clip(s.first_prompt || '', 200);
  actions.append(el('a', { class: 'go plain', href: `#/history/${pid}/${sid}`, text: 'Back to History' }));
  view.className = 'wide';
  const turns = r.data.turns.filter(t => t.text || t.tool);
  const step = 60;
  let shown = Math.min(turns.length, Math.max(step, turns.findIndex(t => t.id === rid) + 1));
  const flow = el('div', { class: 'reading' }), more = el('p', { class: 'more' });
  const del = t => forget({ kind: 'record', id: t.id, project: pid }, reload);
  const paint = () => {
    flow.replaceChildren(summaryBox(r.data.summary) || '', ...turns.slice(0, shown).map(t => turnNode(t, del, rid)), more);
    more.replaceChildren(`Pieces 1 to ${shown} of ${turns.length}. `, shown < turns.length ? btn(`Show the next ${Math.min(step, turns.length - shown)}`, 'link', () => { shown = Math.min(turns.length, shown + step); paint(); }) : '');
  };
  const jump = t => { const at = turns.indexOf(t); if (at >= shown) { shown = at + 1; paint(); } const n = flow.querySelector(`[data-id="${t.id}"]`); if (n) n.scrollIntoView({ block: 'start' }); };
  paint();
  const asked = turns.filter(t => t.who === 'user' && t.id).slice(0, 14);
  const files = [...new Set(turns.flatMap(t => (t.text || '').match(/[\w.\-]+(?:[\\\/][\w.\-]+)+\.[a-z0-9]{1,5}\b/gi) || []))].slice(0, 10);
  view.append(el('div', { class: 'split' },
    el('section', { class: 'pane-read', 'aria-label': 'Conversation' }, flow),
    el('aside', { class: 'pane-aside', 'aria-label': 'In this session' },
      asked.length ? el('div', {}, el('h3', { text: 'Jump to what you asked' }), asked.map(t => el('button', { type: 'button', class: 'jump', text: `${clock(t.observed_at)}  ${clip(t.text.replace(/\s+/g, ' '), 44)}`, onclick: () => jump(t) }))) : null,
      files.length ? el('div', {}, el('h3', { text: 'Files mentioned' }), files.map(f => el('span', { class: 'path', text: f }))) : null,
      el('p', { class: 'quiet small', text: 'What you typed is in grey bubbles. Claude writes on the page. Tool use is one quiet line. Code is in a box.' }))));
  if (rid) { const n = flow.querySelector('.hit'); if (n) n.scrollIntoView({ block: 'center' }); }
}

// ---- Search ---------------------------------------------------------------------------------

async function searchView() {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  lede.textContent = 'Find anything you or Claude said in a saved session.';
  if (!projects.length) return view.append(el('p', { class: 'quiet', text: 'Add a project first; there is nothing to search yet.' }));
  const q = el('input', { type: 'search', id: 'q', required: true, placeholder: 'A decision, an error message, a file name' });
  const exact = el('input', { type: 'checkbox', id: 'exact' });
  let who = '';
  const results = el('div', { class: 'col-main results', 'aria-live': 'polite' });
  const around = el('aside', { class: 'col-side', 'aria-label': 'The whole piece', 'aria-live': 'polite' });

  const open = async h => {
    around.replaceChildren(el('section', { class: 'card around' }, working('Opening it.')));
    const one = await api('GET', `/api/records/${h.id}?project=${enc(h.project_id)}`);
    if (!one.ok) { around.replaceChildren(); return say(problem(one), 'bad'); }
    const x = one.data;
    const box = el('section', { class: 'card around' },
      el('h3', { text: `${x.who === 'user' ? 'You' : x.who === 'assistant' ? 'Claude' : 'A tool'}, ${nice(x.observed_at)}, in ${x.project_name}` }),
      el('div', { class: (x.who === 'user' ? 'bubble' : 'plain') + ' said', text: x.text }),
      x.redactions || x.completeness !== 'complete' ? el('p', { class: 'quiet small', text: (x.redactions ? `${plural(x.redactions, 'secret was', 'secrets were')} removed before saving. ` : '') + (x.completeness === 'complete' ? '' : 'Only part of it was saved.') }) : null);
    around.replaceChildren(box);
    const list = x.session_id ? await api('GET', `/api/sessions?project=${enc(x.project_id)}&limit=100`) : null;
    const s = list && list.ok ? list.data.find(v => v.host_session_id === x.session_id) : null;
    box.append(el('div', { class: 'row' },
      s ? el('a', { class: 'go', href: `#/history/${s.project_id}/${s.id}/read/${x.id}`, text: 'Open the session here' }) : null,
      btn('Delete this piece', 'link quiet', () => forget({ kind: 'record', id: x.id, project: x.project_id }, () => { around.replaceChildren(); return run(''); }))));
  };

  const run = async cursor => {
    const p = new URLSearchParams({ q: q.value });
    if (scope === '*') p.set('all', '1'); else p.set('project', scope);
    if (who) p.set('who', who);
    if (exact.checked) p.set('exact', '1');
    if (cursor) p.set('cursor', cursor);
    const r = await api('GET', '/api/search?' + p);
    if (!cursor) { results.replaceChildren(); around.replaceChildren(); }
    if (!r.ok) return say(problem(r), 'bad');
    const d = r.data, sums = d.summaries || [];
    say(d.hits.length ? `${plural(d.hits.length, 'result', 'results')}${d.next_cursor ? ' so far' : ''}.` : '', 'ok');
    if (d.not_searched_pending_events) results.append(el('p', { class: 'banner warn' }, el('strong', { text: 'Not searched:' }),
      el('span', { text: `${plural(d.not_searched_pending_events, 'captured event is', 'captured events are')} not stored yet. Store them from the Overview, then search again.` })));
    if (!d.hits.length && !cursor) results.append(el('section', { class: 'card centered' }, el('h2', { text: `Nothing saved matches \u201c${q.value}\u201d` }),
      el('p', { class: 'quiet', text: (exact.checked ? 'Nobody said these words in this order. Turn off Exact phrase, or ' : 'Try other words for the same thing, or ') + (scope === '*' ? 'ask who said it differently.' : 'choose All projects in the sidebar.') })));
    if (sums.length) results.append(el('div', { class: 'summary' },
      el('h3', { text: 'Session summaries that match' + (d.summaries_partial ? ' some of the words' : '') + ', written by a model' }),
      sums.map(h => el('p', {}, el('a', { href: `#/history/${h.project_id}/${h.session_id}/read`, text: h.snippet.split('**').join('') })))));
    if (d.method === 'any' && d.hits.length) results.append(el('h3', { text: 'Partial matches: these contain some of the words, not all of them' }));
    for (const h of d.hits) {
      results.append(el('article', { class: 'card result' },
        el('div', { class: 'row small quiet' }, el('span', { class: 'whochip ' + h.who, text: h.who === 'user' ? 'You' : h.who === 'assistant' ? 'Claude' : 'Tool' }),
          el('span', { text: nice(h.observed_at) }), el('span', { class: 'mono', text: projectName(h.project_id) })),
        el('p', { class: 'text' }, h.snippet.split('**').map((part, i) => i % 2 ? el('mark', { text: part }) : part)),
        el('div', { class: 'row' }, btn('Show all of it', 'link', () => open(h)))));
    }
    if (d.next_cursor) results.append(el('p', {}, el('button', { type: 'button', class: 'btn', text: 'Show more results', onclick: e => { e.target.parentNode.remove(); return run(d.next_cursor); } })));
  };

  view.append(el('section', { class: 'card searchcard' }, el('form', { role: 'search', onsubmit: e => { e.preventDefault(); return run(''); } },
    field('Words to look for', q),
    el('div', { class: 'row' }, el('span', { class: 'quiet small', text: `Searching in ${scopeName()}.` }),
      seg('Who said it', [['', 'Anyone'], ['user', 'Me'], ['assistant', 'Claude'], ['tool', 'A tool']], '', v => { who = v; return q.value ? run('') : null; }),
      el('label', { class: 'inline' }, exact, 'Exact phrase'),
      el('button', { type: 'submit', class: 'primary', text: 'Search' })))),
    el('div', { class: 'cols top' }, results, around));
  q.focus();
}

// ---- Instructions ---------------------------------------------------------------------------

async function instructionsView(sel) {
  await loadProjects();
  const r = await api('GET', '/api/instructions?all=1');
  if (!r.ok) return say(problem(r), 'bad');
  lede.textContent = 'Claude receives these word for word at the start of every session.';
  const mine = r.data.filter(i => i.scope === 'global' || inScope(i.project_id));
  const active = mine.filter(i => i.state === 'active'), retired = mine.filter(i => i.state !== 'active');
  const where = i => i.scope === 'global' ? 'every project' : projectName(i.project_id);
  let tab = sel && retired.some(i => i.id === sel) ? 'retired' : 'active';
  const list = el('div', { class: 'col-list' });
  const pane = el('div', { class: 'col-detail', 'aria-live': 'polite' });

  const change = (i, action, work) => {
    const t = el('textarea', { id: 'chg-text' }); t.value = i.text;
    const reason = el('input', { type: 'text', id: 'chg-reason' });
    const go = async () => {
      const res = await api('POST', `/api/instructions/${i.id}/${action}`, { text: action === 'supersede' ? t.value : '', reason: reason.value, confirm: true });
      if (res.ok) { await reload(); say(action === 'archive' ? 'Retired. Claude no longer gets it.' : 'Changed. The earlier wording is kept.', 'ok'); } else say(problem(res), 'bad');
    };
    work.replaceChildren(el('div', { class: 'confirm' },
      el('h3', { text: action === 'archive' ? 'Retire this instruction?' : 'Change the wording' }),
      el('form', { onsubmit: e => { e.preventDefault(); return go(); } },
        action === 'supersede' ? field('New wording', t) : null, field('Why', reason, 'Optional. Kept with the history.'),
        el('div', { class: 'row' }, el('button', { type: 'submit', class: 'primary', text: action === 'archive' ? 'Retire it' : 'Save the new wording' }),
          btn('Leave it as it is', 'btn', () => work.replaceChildren())))));
    work.querySelector('textarea, input').focus();
  };

  const detail = async id => {
    const i = mine.find(x => x.id === id);
    if (!i) return pane.replaceChildren();
    const work = el('div', {}), earlier = el('div', {}, working('Reading the earlier wording.'));
    pane.replaceChildren(el('section', { class: 'card' },
      el('div', { class: 'row' }, i.state === 'active' ? chip('accent', 'In use') : chip('', 'Retired'), chip('plain', i.scope === 'global' ? 'every project' : where(i) + ' only')),
      el('div', {}, el('h3', { text: 'Exact words Claude receives' }), el('blockquote', { class: 'exact said', text: i.text })),
      i.state === 'active' ? el('div', { class: 'row' }, btn('Change the wording', 'btn', () => change(i, 'supersede', work)), btn('Retire this instruction', 'btn', () => change(i, 'archive', work))) : null,
      work, earlier));
    const h = await api('GET', `/api/instructions/${id}`);
    if (!earlier.isConnected) return;
    if (!h.ok) return earlier.replaceChildren(el('p', { class: 'quiet', text: problem(h) }));
    earlier.replaceChildren(el('h3', { text: 'Wording over time' }), el('ol', { class: 'timeline' }, h.data.versions.map(v => el('li', {},
      el('p', { class: 'quiet small', text: `${when(v.created_at)}, ${v.status === 'active' ? 'in use now' : v.status === 'superseded' ? 'replaced' : 'retired'}` }),
      el('p', { class: (v.status === 'active' ? 'cur' : 'old') + ' said', text: v.text }),
      v.reason ? el('p', { class: 'small', text: 'Reason: ' + v.reason }) : null))));
  };

  const add = () => {
    mark(list, '');
    const text = el('textarea', { id: 'new-text', required: true, rows: '3' });
    const where2 = choice('new-scope', 'Where it applies', [['project', 'One project', 'Only sessions in the project chosen below.'], ['global', 'Every project', 'Every project on the Projects page.']], scope === '*' ? '' : 'project');
    const proj = projectSelect('new-project');
    const check = el('div', {});
    const submit = async confirm => {
      const body = { text: text.value, scope: where2.value(), confirm };
      if (body.scope === 'project') body.project = proj.value;
      const res = await api('POST', '/api/instructions', body);
      if (res.ok) { await reload(); return say('Remembered. Claude gets it from the next session.', 'ok'); }
      if (res.status === 409 && res.data && res.data.saved === false) {
        say('Not saved yet. Read it once more, then confirm.');
        check.replaceChildren(el('div', { class: 'confirm' },
          el('h3', { text: res.data.scope === 'global' ? 'Step 2 of 2: remember this in every project?' : 'Step 2 of 2: remember this in the chosen project?' }),
          el('p', { class: 'cur said mono', text: res.data.text }),
          el('div', { class: 'row' }, btn('Confirm and send it from the next session', 'btn primary', () => submit(true)), btn('Edit again', 'btn', () => { check.replaceChildren(); text.focus(); }))));
        return check.querySelector('button').focus();
      }
      say(problem(res), 'bad');
    };
    pane.replaceChildren(card('Add an instruction', el('form', { onsubmit: e => { e.preventDefault(); return submit(false); } },
      field('What should Claude always know or do', text, 'Your own words. They are saved and repeated exactly.'), where2, field('Project', proj),
      el('button', { type: 'submit', class: 'primary start', text: 'Check before saving' })), check));
    text.focus();
  };

  const paint = () => {
    const rows = tab === 'active' ? active : retired;
    list.replaceChildren(seg('Which instructions', [['active', `In use (${active.length})`], ['retired', `Retired (${retired.length})`]], tab, v => { tab = v; paint(); }, 'tabs'),
      ...(rows.length ? rows.map(i => el('a', { class: 'pick boxed', href: '#/instructions/' + i.id, 'data-id': i.id },
        el('span', { class: 'main', text: i.text }),
        el('span', { class: 'sub' }, el('span', { class: 'mono', text: where(i) }), el('span', { text: i.state === 'active' ? 'In use' : 'Retired' }),
          el('span', { text: i.revision > 1 ? plural(i.revision - 1, 'earlier wording', 'earlier wordings') : 'Never changed' }))))
        : [el('p', { class: 'quiet', text: tab === 'active' ? 'No instructions yet. Add one, or tell Claude "remember that..." in a session.' : 'Nothing has been retired.' })]));
    const first = rows.find(i => i.id === sel) || rows[0];
    if (first) { mark(list, first.id); detail(first.id); } else pane.replaceChildren();
  };
  actions.append(btn('Add an instruction', 'primary', add));
  view.append(el('div', { class: 'cols' }, list, pane));
  paint();
  selectors.instructions = args => { if (!args[0] || !mine.some(i => i.id === args[0])) return false; sel = args[0]; mark(list, sel); detail(sel); return true; };
}

// ---- Research -------------------------------------------------------------------------------

const CLAIM = {
  verified: ['ok', 'Checked and current', c => `Checked against its source on ${dateOnly(c.verified_at)}. Check again after ${dateOnly(c.expires_at)}.`],
  stale: ['warn', 'Out of date', c => `Last checked on ${dateOnly(c.verified_at)}; it should have been checked again by ${dateOnly(c.expires_at)}.`],
  disputed: ['bad', 'Disputed', c => `The page no longer contains the quoted words (last checked ${dateOnly(c.verified_at)}).`],
  candidate: ['off', 'Not checked', () => 'It was written down without a source.'],
  withdrawn: ['off', 'Withdrawn', () => 'Withdrawn. It is kept for the record.'],
  superseded: ['off', 'Replaced', () => 'Replaced by a newer fact.'],
};

function versionNote(v) {
  if (!v) return null;
  const uses = `this project uses ${v.package} ${(v.installed || []).join(' and ')} (${v.file})`;
  const words = {
    match: ['', 'Version:', `${uses}, which is the version this fact is about.`],
    other_major: ['warn', 'Not about this project\u2019s version:', `the fact is about ${v.package} ${v.claimed}, ${uses}.`],
    other_version: ['warn', 'Different version:', `the fact is about ${v.package} ${v.claimed}, ${uses}.`],
    mixed: ['warn', 'Several versions:', `the fact is about ${v.package} ${v.claimed}, ${uses}.`],
    not_found: ['', 'Version not compared:', `${v.package} is not in this project\u2019s lock files.`],
  }[v.result];
  if (!words) return null;
  return words[0] ? el('p', { class: 'banner warn' }, el('strong', { text: words[1] }), el('span', { text: words[2] })) : el('p', { class: 'quiet small', text: `${words[1]} ${words[2]}` });
}

async function researchView(pid, sel) {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  lede.textContent = 'Facts Claude looked up on the web, each with the page and the exact words that back it.';
  if (!projects.length) return view.append(el('p', { class: 'quiet', text: 'Add a project first; facts are kept per project.' }));
  if (pid && projects.some(p => p.id === pid) && !inScope(pid)) { scope = pid; fillScope(); }
  const got = await Promise.all(scoped().map(async p => { const r = await api('GET', `/api/research?project=${enc(p.id)}&all=1`); return r.ok ? (r.data.claims || []).map(c => ({ ...c, project: p })) : []; }));
  const all = got.flat();
  const open = c => c.status !== 'withdrawn' && c.status !== 'superseded';
  const groups = [['verified', 'ok'], ['stale', 'warn'], ['disputed', 'bad'], ['candidate', 'off'], ['withdrawn', 'gone']];
  let filter = sel && all.some(c => c.id === sel && !open(c)) ? 'withdrawn' : '';
  const inGroup = (c, g) => g === 'withdrawn' ? !open(c) : c.status === g;
  const filters = el('div', { class: 'filters', role: 'group', 'aria-label': 'Filter by state' });
  const list = el('div', { class: 'card flush rows' });
  const pane = el('div', { class: 'col-detail', 'aria-live': 'polite' });

  const run = async (c, action, body, done) => {
    const res = await api('POST', `/api/research/${enc(c.id)}/${action}`, { project: c.project.id, ...body });
    if (!res.ok) return say(problem(res), 'bad');
    await show(`research/${c.project.id}/${c.id}`, true);
    say(done(res.data), 'ok');
  };
  const detail = id => {
    const c = all.find(x => x.id === id);
    if (!c) return pane.replaceChildren();
    const [kind, word, sentence] = CLAIM[c.status] || CLAIM.candidate;
    const sourced = c.state === 'verified' || c.state === 'disputed';
    const failed = (c.last_checks || []).filter(k => k.result === 'unavailable');
    pane.replaceChildren(el('section', { class: 'card fact' },
      el('div', { class: 'row' }, chip(kind === 'off' ? '' : kind, word), c.applies_to ? chip('plain', c.applies_to) : null),
      versionNote(c.version_check),
      el('div', {}, el('p', { class: 'quiet small', text: c.subject }), el('h2', { class: 'said', text: c.statement })),
      el('p', { class: 'quiet small', text: sentence(c) }),
      (c.evidence || []).map(e => el('figure', { class: 'quoted' }, el('figcaption', { text: 'Exact words on the page' }), el('blockquote', { class: 'said', text: '\u201c' + e.quote + '\u201d' }),
        el('p', { class: 'path' }, 'From ', e.found_in.url.startsWith('https:') ? el('a', { href: e.found_in.url, rel: 'noopener noreferrer', target: '_blank', text: e.found_in.url }) : e.found_in.url, `, read on ${dateOnly(e.last_confirmed_in.fetched_at)}.`))),
      failed.length ? el('p', { class: 'banner warn' }, el('strong', { text: 'Could not reach the page:' }), el('span', { text: `the last check on ${dateOnly(failed[0].checked_at)} failed (${failed[0].detail}).` })) : null,
      el('dl', { class: 'facts-meta' },
        el('div', {}, el('dt', { text: 'Project' }), el('dd', { class: 'mono', text: c.project.name })),
        el('div', {}, el('dt', { text: 'Kind of fact' }), el('dd', { text: c.category })),
        c.note ? el('div', {}, el('dt', { text: 'Note' }), el('dd', { text: c.note })) : null),
      open(c) ? el('div', { class: 'row' },
        sourced ? btn('Check the page again', 'btn primary', () => run(c, 'check', {}, d => { const s = ((d.checked || [])[0] || {}).status; return s === 'verified' ? 'The page still says so.' : s === 'disputed' ? 'The page no longer contains the quoted words.' : 'The page could not be reached; nothing changed.'; }), `Check the page again for: ${c.subject}`) : null,
        btn('Withdraw this fact', 'btn danger', () => run(c, 'withdraw', { reason: 'withdrawn from the dashboard' }, () => 'Withdrawn. It is kept, marked as withdrawn.'), `Withdraw the fact: ${c.subject}`)) : null,
      sourced && open(c) ? el('p', { class: 'quiet small', text: 'Checking opens the page from this computer. It works only while fetching is turned on (sessionkeep research on).' }) : null));
  };
  const paint = () => {
    filters.replaceChildren(...groups.map(([g, cls]) => el('button', { type: 'button', class: cls, 'aria-pressed': String(filter === g), text: `${g === 'withdrawn' ? 'Withdrawn or replaced' : CLAIM[g][1]} ${all.filter(c => inGroup(c, g)).length}`, onclick: () => { filter = filter === g ? '' : g; paint(); } })));
    const rows = all.filter(c => filter ? inGroup(c, filter) : open(c));
    list.replaceChildren(...(rows.length ? rows.map(c => { const [kind, word] = CLAIM[c.status] || CLAIM.candidate;
      return el('a', { class: 'pick', href: `#/research/${c.project.id}/${c.id}`, 'data-id': c.id },
        el('span', { class: 'top' }, el('span', { text: c.subject }), el('span', { class: 'word-' + kind, text: word })),
        el('span', { class: 'main', text: c.statement }),
        el('span', { class: 'sub mono' }, el('span', { text: c.project.name }), el('span', { text: c.verified_at ? 'checked ' + dateOnly(c.verified_at) : 'never checked, no source' }))); })
      : [el('p', { class: 'working', text: all.length ? 'No facts in this state.' : 'No facts are recorded here. Claude records them when it looks something up with the research skill.' })]));
    const first = rows.find(c => c.id === sel) || rows[0];
    if (first) { mark(list, first.id); detail(first.id); } else pane.replaceChildren();
  };
  view.append(filters, el('div', { class: 'cols' }, el('div', { class: 'col-list' }, list), pane));
  paint();
  selectors.research = args => { if (!args[1] || !all.some(c => c.id === args[1])) return false; sel = args[1]; mark(list, sel); detail(sel); return true; };
}

// ---- Projects -------------------------------------------------------------------------------

async function projectsView(sel) {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  lede.textContent = 'SessionKeep saves only inside these folders, at the level you choose for each.';
  const list = el('div', { class: 'card flush rows' });
  const pane = el('div', { class: 'col-side', 'aria-live': 'polite' });
  const again = () => show('projects', true);

  const detail = id => {
    const p = projects.find(x => x.id === id);
    if (!p) return pane.replaceChildren();
    const [, , words] = savingState(p);
    const level = choice('lvl-' + p.id, 'What is saved', LEVELS, p.capture_level);
    pane.replaceChildren(el('section', { class: 'card' },
      el('div', {}, el('h2', { class: 'mono', text: p.name }), el('p', { class: 'path', text: p.roots.join(', ') }), el('p', { class: 'quiet small', text: words + (p.added_by === 'auto' ? ' Added automatically.' : '') })),
      level,
      el('div', { class: 'row' }, btn('Change what is saved', 'btn', () => act(`/api/projects/${p.id}/capture`, { level: level.value() }, 'Changed.')),
        btn(p.paused ? 'Resume saving' : 'Pause saving', p.paused ? 'btn primary' : 'btn', () => act(`/api/projects/${p.id}/${p.paused ? 'resume' : 'pause'}`, {}, p.paused ? 'Saving again.' : 'Paused.'))),
      el('p', { class: 'quiet small', text: 'Both of these show what would go and ask before deleting.' }),
      el('div', { class: 'row' }, btn('Delete saved conversations', 'btn danger', () => forget({ kind: 'captured', project: p.id }, again), `Delete saved conversations of ${p.name}`),
        btn('Delete the whole project', 'btn danger', () => forget({ kind: 'project', project: p.id }, again), `Delete project ${p.name}`))));
  };

  const path = el('input', { type: 'text', id: 'add-path', class: 'code', required: true, autocomplete: 'off', placeholder: 'C:\\Work\\my-project' });
  const name = el('input', { type: 'text', id: 'add-name', autocomplete: 'off' });
  const addLevel = select('add-level', LEVELS.map(l => [l[0], l[1]]), 'conversation');
  const addCard = card('Add a project', el('form', { onsubmit: e => { e.preventDefault(); return act('/api/projects', { path: path.value, level: addLevel.value, name: name.value }, 'Project added.'); } },
    field('Folder', path, 'The full path. Everything inside it counts as this project.'),
    el('div', { class: 'pair' }, field('What is saved', addLevel), field('Name', name, 'Optional. The folder name is used if empty.')),
    el('button', { type: 'submit', class: 'strong start', text: 'Add project and start saving' })));

  // Earlier sessions: check first, import only after the numbers were shown.
  const found = el('div', { 'aria-live': 'polite' });
  const describe = d => [
    el('p', {}, `${d.imported ? 'Imported' : 'Found'} ${plural(d.turns, 'piece of text', 'pieces of text')} from ${plural(d.sessions, 'earlier session', 'earlier sessions')}` + (d.imported ? ` (${d.stored} new).` : '.')),
    (d.projects || []).map(p => el('div', { class: 'listrow' }, el('span', { class: 'mono', text: p.name }), el('span', { text: plural(p.turns, 'piece', 'pieces') }))),
    el('p', { class: 'quiet small', text: `Left out: ${plural(d.sessions_captured_live_skipped, 'session', 'sessions')} already saved as they happened, ${d.turns_in_excluded_folders} in excluded folders, ${d.turns_in_paused_or_text_free_projects} in paused or text-free projects, ${d.turns_in_unregistered_folders} in folders that are not projects. ${plural(d.secrets_removed, 'secret', 'secrets')} removed.` }),
    d.unregistered_folders && d.unregistered_folders.length ? el('details', {}, el('summary', { text: 'Folders with earlier sessions that are not projects' }),
      el('ul', {}, d.unregistered_folders.map(f => el('li', { class: 'path', text: `${f.name} (${f.turns})` })))) : null,
  ];
  const importHistory = async confirm => {
    found.replaceChildren(working(confirm ? 'Importing earlier sessions. This can take a minute.' : 'Reading Claude Code\u2019s session files. This can take a minute.'));
    const r = await api('POST', '/api/history/import', { confirm });
    if (r.ok) { found.replaceChildren(...describe(r.data).flat().filter(Boolean)); return say('Earlier sessions imported.', 'ok'); }
    if (r.status !== 409 || !r.data) { found.replaceChildren(); return say(problem(r), 'bad'); }
    say('Nothing is imported yet.');
    found.replaceChildren(...describe(r.data).flat().filter(Boolean), r.data.turns ? btn(`Import ${plural(r.data.sessions, 'session', 'sessions')}`, 'btn primary', () => importHistory(true)) : '');
  };
  const importCard = card('Import earlier Claude Code sessions',
    el('p', { class: 'quiet small', text: 'Bring in what you and Claude said before SessionKeep was installed, for the projects above. Claude\u2019s private reasoning, tool output and images are never imported. Nothing is imported until you confirm.' }),
    btn('Check what can be imported', 'btn start', () => importHistory(false)), found);

  actions.append(btn('Import earlier sessions', 'btn', () => { importCard.scrollIntoView({ block: 'center' }); importCard.querySelector('button').focus(); }),
    btn('Add a project', 'primary', () => { addCard.scrollIntoView({ block: 'center' }); path.focus(); }));
  list.replaceChildren(...projects.map(p => { const [kind, word] = savingState(p);
    return el('a', { class: 'pick project', href: '#/projects/' + p.id, 'data-id': p.id },
      el('span', { class: 'who' }, el('strong', { text: p.name }), el('span', { class: 'path', text: p.roots.join(', ') + (p.added_by === 'auto' ? ' (added automatically)' : '') })),
      dotword(kind, word), el('span', { class: 'quiet small', text: levelName(p.capture_level) })); }));
  view.append(el('div', { class: 'stack' },
    projects.length ? el('div', { class: 'cols' }, el('div', { class: 'col-detail' }, list), pane) : el('p', { class: 'quiet', text: 'No projects yet, so nothing is being saved.' }),
    el('div', { class: 'pair wide' }, addCard, importCard)));
  const first = projects.find(p => p.id === sel) || projects.find(p => p.id === scope) || projects[0];
  if (first) { mark(list, first.id); detail(first.id); }
  selectors.projects = args => { if (!args[0] || !projects.some(p => p.id === args[0])) return false; mark(list, args[0]); detail(args[0]); return true; };
}

// ---- Settings -------------------------------------------------------------------------------

async function settingsView() {
  const [set, sm, ac] = await Promise.all([api('GET', '/api/settings'), api('GET', '/api/summaries'), api('GET', '/api/accounts')]);
  lede.textContent = 'Accounts, summaries, and what happens in folders that are not projects.';
  view.className = 'narrow';
  const stack = el('div', { class: 'stack' });
  view.append(stack);

  // Which Claude Code accounts have SessionKeep turned on. Claude Code takes a moment to answer.
  const accounts = el('div', { 'aria-live': 'polite' }, working('Asking Claude Code where SessionKeep is turned on.'));
  stack.append(el('section', { class: 'card flush' }, el('div', { class: 'cardhead' }, el('h2', { text: 'Claude Code accounts on this computer' }),
    el('span', { class: 'quiet small', text: 'Every account it is on for saves into the same memory and reads from it.' })), accounts));
  const loadAccounts = async () => {
    const r = await api('GET', '/api/accounts/plugin');
    if (!accounts.isConnected) return;
    if (!r.ok) return accounts.replaceChildren(el('p', { class: 'working', text: problem(r) }));
    const list = r.data.accounts || [];
    const turn = async (a, on) => {
      const res = await api('POST', '/api/accounts/plugin', { name: a.name, on });
      if (!res.ok) return say(problem(res), 'bad');
      await loadAccounts();
      say(on ? `Turned on for "${a.name}". Restart Claude Code there for it to load.` : `Turned off for "${a.name}". What is already saved stays.`, 'ok');
    };
    accounts.replaceChildren(...(list.length ? list.map(a => {
      const on = a.plugin === 'on', known = on || a.plugin === 'off';
      return el('div', { class: 'setrow' },
        el('div', { class: 'who' }, el('b', { text: a.name }), el('span', { class: 'path', text: (a.command ? `You start it with "${a.command}".` : 'No short command was found for it.') + (a.last_used ? ` Last used ${a.last_used}.` : ' Never used.') })),
        on ? dotword('ok', 'SessionKeep on') : known ? dotword('off', 'SessionKeep off') : dotword('warn', `Could not be asked: ${a.plugin_note || 'no reason given'}`),
        known ? (on ? btn('Turn SessionKeep off', 'btn', () => turn(a, false), `Turn SessionKeep off for the account ${a.name}`) : btn('Turn SessionKeep on', 'btn primary', () => turn(a, true), `Turn SessionKeep on for the account ${a.name}`)) : null);
    }) : [el('p', { class: 'working', text: 'No Claude Code account was found on this computer.' })]));
  };
  loadAccounts();

  if (sm.ok) {
    const d = sm.data;
    const on = el('input', { type: 'checkbox', id: 'sum-on', checked: d.enabled });
    const models = [['sonnet', 'Sonnet (recommended)'], ['opus', 'Opus (uses more of your plan)'], ['haiku', 'Haiku (lightest)']];
    if (!models.some(m => m[0] === d.model)) models.push([d.model, d.model]);
    const model = select('summary-model', models, d.model);
    const list = ac.ok ? ac.data.accounts : [];
    const chosen = list.find(a => a.chosen);
    const acct = select('summary-account', [['', 'Choose an account']].concat(list.map(a => [a.name, a.name + (a.command && a.command !== a.name ? ` (you start it with "${a.command}")` : '')])), chosen ? chosen.name : '');
    const meter = el('span', {});
    meter.style.width = Math.min(100, d.daily_limit ? 100 * d.requests_today / d.daily_limit : 0) + '%';
    stack.append(card('', el('form', { onsubmit: async e => {
      e.preventDefault();
      if (list.length > 1 && acct.value && (!chosen || chosen.name !== acct.value)) {
        const r = await api('POST', '/api/accounts', { name: acct.value });
        if (!r.ok) return say(problem(r), 'bad');
      }
      return act('/api/summaries', { enabled: on.checked, model: model.value }, 'Saved.');
    } },
      el('div', { class: 'between' }, el('h2', { text: 'Session summaries' }), el('label', { class: 'inline' }, on, 'On')),
      el('p', { class: 'quiet small', text: 'After a session ends, Claude Code on this computer writes a short summary in the background, under your own sign-in. Each one uses a little of your plan. A summary is always shown as a summary, never as what was said. While this is off, no session text is given to a model.' }),
      el('div', { class: 'pair' }, list.length > 1 ? field('Account', acct, 'Summaries use the plan of this account.') : null, field('Model', model)),
      el('div', {}, el('p', { class: 'between small' }, el('span', { text: 'Requests used today' }), el('strong', { text: `${d.requests_today} of ${d.daily_limit}` })), el('div', { class: 'meter' }, meter)),
      el('p', { class: 'quiet small', text: `${plural(d.summaries, 'summary', 'summaries')} written so far. ${plural(d.sessions_waiting, 'finished session is', 'finished sessions are')} waiting.` + (d.program ? '' : ' Claude Code was not found on this computer, so none can be written.') }),
      el('button', { type: 'submit', class: 'primary start', text: 'Save this choice' }))));
  }

  if (!set.ok) return say(problem(set), 'bad');
  const a = set.data.auto_add;
  const mode = choice('mode', 'When Claude Code runs in a folder that is not a project', [
    ['ask', 'Remind me, and save nothing', 'Says once that the folder can be added.'],
    ['manual', 'Ignore it, and save nothing', 'Says nothing.'],
    ['auto', 'Add it as a project automatically and start saving', 'At the level chosen below. Exclude client or private folders first.'],
  ], a.mode);
  const autoLevel = select('auto-level', LEVELS.slice(0, 3).map(l => [l[0], l[1]]), a.level || 'conversation');
  stack.append(card('', el('form', { onsubmit: e => { e.preventDefault(); return act('/api/settings/auto-add', { mode: mode.value(), level: mode.value() === 'auto' ? autoLevel.value : '' }, 'Saved.'); } },
    mode, field('For automatically added folders, save', autoLevel),
    el('button', { type: 'submit', class: 'primary start', text: 'Save this choice' }))));

  const ex = el('input', { type: 'text', id: 'ex-path', class: 'code', required: true, autocomplete: 'off', placeholder: 'C:\\Clients' });
  const never = set.data.exclusions || [];
  stack.append(card('Folders that are never saved',
    el('p', { class: 'quiet small', text: 'Nothing in one of these folders, or below it, is saved or added. This wins over everything else.' }),
    never.length ? never.map(p => el('div', { class: 'never' }, el('span', { class: 'path', text: p }), btn('Remove from list', 'link quiet', () => act('/api/exclusions', { path: p, remove: true }, 'No longer excluded.'), `Stop excluding ${p}`)))
      : el('p', { class: 'quiet', text: 'No folders are excluded.' }),
    el('form', { onsubmit: e => { e.preventDefault(); return act('/api/exclusions', { path: ex.value }, 'Excluded.'); } },
      field('Folder to never save', ex), el('button', { type: 'submit', class: 'btn start', text: 'Add folder' }))));
}

// ---- Health ---------------------------------------------------------------------------------

const CHECK_NAMES = { build: 'Program', data_location: 'Where data is kept', settings: 'Settings file', installation: 'Installed copy', plugin_hooks: 'Claude Code connection', sqlite_engine: 'Database engine' };

async function diagnostics() {
  const r = await api('GET', '/api/doctor');
  if (!r.data) return say(problem(r), 'bad');
  const checks = r.data.checks, working2 = checks.filter(c => c.status === 'ok').length;
  lede.textContent = `${working2} of ${plural(checks.length, 'check is', 'checks are')} working.`;
  badge('n-health', checks.length - working2, 'bad', `${checks.length - working2} not working`);
  actions.append(btn('Run the checks again', 'btn', reload));
  view.append(el('div', { class: 'cards checks' }, checks.map(c => el('article', { class: 'card' + (c.status === 'ok' ? '' : ' alarm') },
    el('div', { class: 'between' }, el('h2', { text: CHECK_NAMES[c.name] || c.name }), c.status === 'ok' ? chip('ok', 'Working') : chip('bad', 'Not working')),
    el('p', { class: 'detail', text: c.detail })))),
    el('p', { class: 'quiet small', text: 'Not checked here: ' + r.data.not_checked + '.' }));
}

// ---- Routing --------------------------------------------------------------------------------

const views = { overview: ['Overview', overview], resume: ['Pick up work', resumeView], history: ['History', historyView], search: ['Search', searchView],
  instructions: ['Instructions', instructionsView], research: ['Research', researchView], projects: ['Projects', projectsView],
  settings: ['Settings', settingsView], diagnostics: ['Health', diagnostics] };

async function show(target, force) {
  const [name, ...args] = String(target || '').split('/');
  const next = views[name] ? name : 'overview';
  if (!force && next === current && selectors[next] && selectors[next](args)) return;
  current = next;
  selectors = {};
  const [label, render] = views[current];
  title.textContent = label;
  lede.textContent = '';
  crumb.textContent = '';
  actions.replaceChildren();
  document.title = label + ' - SessionKeep';
  for (const a of document.querySelectorAll('#nav a')) {
    if (a.dataset.view === current) a.setAttribute('aria-current', 'page'); else a.removeAttribute('aria-current');
  }
  $('shell').classList.remove('open');
  $('menu').setAttribute('aria-expanded', 'false');
  view.className = '';
  view.replaceChildren();
  say('Loading...');
  await render(...args);
  if (notice.textContent === 'Loading...') say('');
}

window.addEventListener('hashchange', () => { show(route()); if (!selectors[current]) $('main').focus(); });

$('scope').addEventListener('change', e => {
  scope = e.target.value;
  history.replaceState(null, '', '#/' + current);
  show(current, true);
});
$('menu').addEventListener('click', () => $('menu').setAttribute('aria-expanded', String($('shell').classList.toggle('open'))));
$('theme').addEventListener('click', () => {
  const root = document.documentElement;
  const dark = root.dataset.theme ? root.dataset.theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  root.dataset.theme = dark ? 'light' : 'dark';
  $('theme').textContent = dark ? 'Switch to dark theme' : 'Switch to light theme';
});
if (matchMedia('(prefers-color-scheme: dark)').matches) $('theme').textContent = 'Switch to light theme';
document.addEventListener('keydown', e => {
  if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey || /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || dialog.open) return;
  e.preventDefault();
  if (current === 'search') { const q = $('q'); if (q) q.focus(); } else location.hash = '#/search';
});

(async () => {
  const s = await api('GET', '/api/session');
  if (!s.ok) { title.textContent = 'Not signed in'; return say(problem(s), 'bad'); }
  csrf = s.data.csrf;
  $('version').textContent = s.data.version;
  await show(route(), true);
  healthBadge();
  if (current !== 'overview') { const st = await api('GET', '/api/status'); if (st.ok) attention(st.data); }
})();
