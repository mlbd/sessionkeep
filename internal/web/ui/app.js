// SessionKeep dashboard. Plain modules, no dependencies, nothing loaded from elsewhere.
// Every piece of data is put on the page with textContent, never as markup.

const $ = id => document.getElementById(id);
const view = $('view'), title = $('title'), lede = $('lede'), notice = $('notice'), crumb = $('crumb'), actions = $('actions'), dialog = $('dialog');
let csrf = '';
let projects = [];
let scope = '*';       // the open project's id; '*' when looking at every project
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
const projectHref = (id, page = 'overview', ...more) => '#/' + ['p', id, page, ...more].join('/');
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
  for (const [v, label, meaning, disabled] of options) {
    const input = el('input', { type: 'radio', name, value: v, checked: v === value, disabled: !!disabled });
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

async function loadProjects() {
  const r = await api('GET', '/api/projects');
  projects = r.ok ? r.data : [];
  const saving = projects.filter(p => !p.paused && p.capture_level !== 'off').length;
  $('saving').textContent = saving ? `Saving in ${plural(saving, 'project', 'projects')}` : 'Saving nothing';
  $('saving').parentNode.classList.toggle('idle', !saving);
  return r;
}

// [kind, word, sentence] for how a project is being saved.
function savingState(p) {
  if (p.paused) return ['warn', 'Paused', `Paused. It was saving: ${levelName(p.capture_level).toLowerCase()}.`];
  if (p.capture_level === 'off') return ['off', 'Off', 'Saving nothing.'];
  return ['ok', 'Saving', `Saving: ${levelName(p.capture_level).toLowerCase()}.`];
}

function badge(id, n, kind, words) {
  const b = $(id);
  if (!b) return; // the counter is not on this level's menu
  b.textContent = n ? (words || String(n)) : '';
  b.className = 'count ' + kind;
}

function openDialog(heading, body, buttons) {
  dialog.replaceChildren(el('h2', { id: 'dialog-title', text: heading }), ...body, el('div', { class: 'row end' }, buttons));
  dialog.showModal();
  (dialog.querySelector('.row.end button') || dialog.querySelector('button')).focus();
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

// Brings one project up to date with what Claude Code has on this computer: first stores the
// events that are still waiting, then looks in Claude Code's own session files for anything
// that is not saved yet. What it found is shown before anything is imported.
async function sync(p, justAdded) {
  say(`Looking for sessions of ${p.name} that are not saved yet. This can take a minute.`);
  const waiting = await api('POST', '/api/ingest');
  const stored = waiting.ok && waiting.data ? waiting.data.ingested || 0 : 0;
  const r = await api('POST', '/api/history/import', { project: p.id, confirm: false });
  if (r.status !== 409 || !r.data) return say(problem(r), 'bad');
  const d = r.data;
  if (!d.turns) {
    if (stored) await reload();
    if (justAdded) return say(`Project added. Claude Code has no earlier sessions of ${p.name} to bring in.`, 'ok');
    return say((stored ? `Stored ${plural(stored, 'waiting event', 'waiting events')}. ` : '') + `Nothing else to bring in: everything Claude Code still has for ${p.name} is already saved.`, 'ok');
  }
  say(justAdded ? 'Project added. Its earlier sessions are not imported yet.' : 'Nothing is imported yet.');
  openDialog(justAdded ? `Also bring in the earlier sessions of ${p.name}?` : `Bring ${p.name} up to date?`, [
    el('p', {}, `Claude Code has ${plural(d.turns, 'piece of text', 'pieces of text')} from ${plural(d.sessions, 'session', 'sessions')} that SessionKeep has not saved.`),
    el('p', { class: 'quiet small', text: `Left out: ${plural(d.sessions_captured_live_skipped, 'session', 'sessions')} already saved as they happened, ${d.turns_in_excluded_folders} pieces in excluded folders, ${d.turns_in_paused_or_text_free_projects} in paused or text-free projects. Claude\u2019s private reasoning, tool output and images are never imported.` }),
  ], [
    btn('Not now', 'btn', () => { dialog.close(); say(''); }),
    btn(`Import ${plural(d.sessions, 'session', 'sessions')}`, 'btn primary', async () => {
      const done = await api('POST', '/api/history/import', { project: p.id, confirm: true });
      dialog.close();
      if (!done.ok) return say(problem(done), 'bad');
      await reload();
      say(`Imported ${plural(done.data.stored, 'piece of text', 'pieces of text')} from ${plural(done.data.sessions, 'session', 'sessions')}.`, 'ok');
    }),
  ]);
}

// ---- Sharing a project through the cloud ----------------------------------------------------

// Which storage is saved on this computer and which projects are shared through it. A project
// is never sent anywhere unless sharing was turned on for it here.
let team = { storage: { set: false }, storages: { personal: { set: false }, shared: { set: false } }, projects: {} };
async function loadTeam() {
  const r = await api('GET', '/api/team/overview');
  if (r.ok && r.data && r.data.storages) team = r.data;
  return r;
}

// The two storages saved once on this computer. Personal is used unless another is chosen.
const STORAGES = {
  personal: { title: 'Personal storage', what: 'Your own bucket. Projects you share go here unless you choose otherwise.' },
  shared: { title: 'Shared storage', what: 'A bucket you share with others, such as your company\'s. Each person who uses it has their own keys to it.' },
};
// How a shared project's place is named on the Projects page.
const placeOf = shared => shared.storage === 'personal' ? 'your personal storage' : shared.storage === 'shared' ? 'the shared storage' : 'a bucket of its own';

// A line inside a dialog for what went wrong, so that what was typed is not lost.
const alertLine = () => el('p', { class: 'oops', role: 'alert' });
const countLine = c => `${plural(c.instructions, 'instruction', 'instructions')}, ${plural(c.checkpoints, 'checkpoint', 'checkpoints')} and ${plural(c.pieces_of_text, 'piece of text', 'pieces of text')} from ${plural(c.sessions, 'session', 'sessions')}`;

// The fields of a bucket and its keys. The keys are kept protected on this computer and no page
// shows them again.
function bucketFields(prefix, s) {
  const f = {
    endpoint: el('input', { type: 'text', id: prefix + '-endpoint', class: 'code', autocomplete: 'off', value: s.endpoint || '' }),
    bucket: el('input', { type: 'text', id: prefix + '-bucket', class: 'code', autocomplete: 'off', value: s.bucket || '' }),
    access: el('input', { type: 'password', id: prefix + '-access', class: 'code', autocomplete: 'off' }),
    secret: el('input', { type: 'password', id: prefix + '-secret', class: 'code', autocomplete: 'off' }),
    from: el('input', { type: 'text', id: prefix + '-from', autocomplete: 'off', value: s.as || '' }),
  };
  f.nodes = [
    field('Storage address', f.endpoint, 'Cloudflare shows it when you create the R2 API token. It ends in r2.cloudflarestorage.com.'),
    field('Bucket name', f.bucket),
    field('Access Key ID', f.access),
    field('Secret Access Key', f.secret, 'Both keys are kept protected on this computer and are never shown again.'),
    field('Your name', f.from, 'How colleagues see who sent something.'),
  ];
  f.values = () => ({ endpoint: f.endpoint.value.trim(), bucket: f.bucket.value.trim(), from: f.from.value.trim(), access_key: f.access.value, secret_key: f.secret.value });
  return f;
}

// A storage saved once: the personal one or the shared one.
function cloudStorage(slot = 'personal', then) {
  const s = team.storages[slot] || {};
  const info = STORAGES[slot];
  const f = bucketFields('cs', s);
  const wrong = alertLine();
  openDialog(s.saved ? `Change the ${info.title.toLowerCase()}` : `Set up the ${info.title.toLowerCase()}`, [
    el('p', { class: 'quiet small', text: `${info.what} A storage bucket in Cloudflare R2 or another S3-compatible service. Only projects you turn sharing on for are ever sent there, and they are encrypted first.` }),
    ...f.nodes,
    wrong,
  ], [
    btn('Cancel', 'btn', () => dialog.close()),
    btn('Save storage', 'btn primary', async () => {
      const r = await api('POST', '/api/team/storage', { storage: slot, ...f.values() });
      if (!r.ok) { wrong.textContent = problem(r); return; }
      await loadTeam();
      dialog.close();
      say(`The bucket answered and the ${info.title.toLowerCase()} is saved. Nothing was sent.`, 'ok');
      return then ? then() : reload();
    }),
  ]);
}

function forgetStorage(slot = 'personal') {
  const name = STORAGES[slot].title.toLowerCase();
  openDialog(`Forget the ${name}?`, [
    el('p', { text: 'This computer forgets the bucket and its keys. Projects already shared through it keep working until you stop sharing them. Nothing in the bucket is touched.' }),
  ], [
    btn('Keep it', 'btn', () => dialog.close()),
    btn('Forget storage', 'btn danger', async () => {
      const r = await api('POST', '/api/team/storage', { forget: true, storage: slot });
      dialog.close();
      if (!r.ok) return say(problem(r), 'bad');
      await loadTeam();
      await reload();
      say(`The ${name} is forgotten.`, 'ok');
    }),
  ]);
}

// Turning sharing on for one project, on its Sharing page: where it is kept (the personal storage
// unless another is chosen), a shared name and a passphrase.
function shareForm(p) {
  const st = team.storages;
  const noneSaved = !st.personal.set && !st.shared.set;
  const slug = p.name.toLowerCase().replace(/[^a-z0-9]+/g, '-').replace(/^-+|-+$/g, '') || 'project';
  const notYet = 'Not set up yet: set it up in Settings, Cloud storage.';
  const where = choice('sh-store', 'Where it is kept', [
    ['personal', 'Personal storage', st.personal.set ? `Your own bucket ${st.personal.bucket}.` : notYet, !st.personal.set],
    ['shared', 'Shared storage', st.shared.set ? `The bucket ${st.shared.bucket} you share with others.` : notYet, !st.shared.set],
    ['custom', 'A bucket of its own', 'Another bucket for this project only, such as one a client or another team gave you.'],
  ], st.personal.set ? 'personal' : st.shared.set ? 'shared' : 'custom');
  const own = bucketFields('sc', { as: st.personal.as || st.shared.as });
  const ownBox = el('div', { class: 'subfields' }, own.nodes);
  const paint = () => { ownBox.hidden = where.value() !== 'custom'; };
  where.addEventListener('change', paint);
  paint();
  const shared = el('input', { type: 'text', id: 'sh-name', class: 'code', autocomplete: 'off', value: slug });
  const pass = el('input', { type: 'password', id: 'sh-pass', autocomplete: 'new-password' });
  pass.setAttribute('aria-label', 'Passphrase for this project');
  // Show what is being typed, so a mistyped passphrase is caught before it is used.
  const reveal = el('button', { type: 'button', class: 'btn', 'aria-pressed': 'false', 'aria-label': 'Show the passphrase', text: 'Show' });
  reveal.addEventListener('click', e => {
    e.preventDefault();
    const shown = pass.type === 'password';
    pass.type = shown ? 'text' : 'password';
    reveal.textContent = shown ? 'Hide' : 'Show';
    reveal.setAttribute('aria-pressed', String(shown));
    reveal.setAttribute('aria-label', shown ? 'Hide the passphrase' : 'Show the passphrase');
    pass.focus();
  });
  const wrong = alertLine();
  const submit = async e => {
    e.preventDefault();
    const storage = where.value();
    const body = { project: p.id, name: shared.value.trim(), passphrase: pass.value, storage };
    if (storage === 'custom') Object.assign(body, own.values());
    const r = await api('POST', '/api/team/connect', body);
    if (!r.ok) { wrong.textContent = problem(r); return; }
    await loadTeam();
    await reload();
    say(`${p.name} is now shared as "${r.data.name}". ` + (r.data.bundles_there ? `${plural(r.data.bundles_there, 'bundle is', 'bundles are')} already in the bucket for it.` : 'The bucket holds nothing for it yet.') + ' Press Sync now to receive and send.', 'ok');
  };
  return el('section', { class: 'card narrowcol' },
    el('h2', { text: `Share ${p.name} through the cloud` }),
    el('p', { class: 'quiet', text: 'Its instructions, checkpoints and conversations can then be sent, encrypted on this computer, to the bucket you choose, and colleagues who share the same project can receive them. Nothing is sent until you press Sync now.' }),
    noneSaved ? el('div', { class: 'banner warn' }, el('span', { text: 'No storage is saved on this computer yet. Set up your own bucket once, or give this project a bucket of its own below.' }),
      btn('Set up personal storage', 'btn', () => cloudStorage('personal', reload))) : null,
    el('form', { onsubmit: submit },
      where,
      ownBox,
      field('Shared project name', shared, 'Colleagues must type exactly the same name. Lower-case letters, digits and dashes.'),
      field('Passphrase for this project', el('div', { class: 'inputrow' }, pass, reveal), `At least ${team.min_passphrase || 8} characters. Make one up if you are the first to share this project; otherwise type the one your colleague gave you. Keep it somewhere safe: only a computer connected to this project can show it again, and anyone who has it and can reach the bucket can read what is sent.`),
      wrong,
      el('button', { type: 'submit', class: 'primary start', text: 'Turn sharing on' })));
}

// One press for both directions: what colleagues sent is received, then what is new here is
// sent. What would happen is shown first, with every received instruction in full.
async function teamSync(p) {
  say(`Looking at the bucket for ${p.name}. This can take a minute.`);
  const r = await api('POST', '/api/team/sync', { project: p.id, confirm: false });
  if (r.ok) return say(`${p.name} is in step: nothing to receive and nothing new to send.`, 'ok');
  if (r.status !== 409 || !r.data || !r.data.receive) return say(problem(r), 'bad');
  say('Nothing is received or sent yet.');
  const get = r.data.receive, send = r.data.send, told = get.instructions || [];
  const toSend = send.counts.instructions + send.counts.checkpoints + send.counts.pieces_of_text;
  openDialog(`Sync ${p.name}?`, [
    el('p', {}, el('strong', { text: 'To receive: ' }), get.bundles ? `${countLine(get.counts)}, sent by ${(get.from || []).join(', ') || 'colleagues'}.` + (get.counts.sessions && !get.sessions_taken ? ' The sessions would be left out: this project is paused or does not save conversation text.' : '') : 'nothing new.'),
    told.length ? el('p', { text: 'These received instructions would be given to Claude in every session of this project, as if you had recorded them. Read them:' }) : null,
    told.map(i => el('blockquote', { class: 'exact said', text: i.text })),
    el('p', {}, el('strong', { text: 'To send: ' }), toSend ? `${countLine(send.counts)}.` : 'nothing new.'),
    el('p', { class: 'quiet small', text: 'What you send is encrypted on this computer with the passphrase of this project before it leaves. Your folder paths are shortened and secrets are removed; other names or paths you typed in a conversation are not. It cannot be taken back from colleagues who received it.' }),
  ].flat().filter(Boolean), [
    btn('Not now', 'btn', () => { dialog.close(); say(''); }),
    btn('Receive and send', 'btn primary', async () => {
      const done = await api('POST', '/api/team/sync', { project: p.id, confirm: true });
      dialog.close();
      if (!done.ok) return say(problem(done), 'bad');
      await reload();
      const got = done.data.receive.stored;
      say((got ? `Received ${plural(got.pieces_of_text, 'piece of text', 'pieces of text')}, ${plural(got.instructions, 'instruction', 'instructions')} and ${plural(got.checkpoints, 'checkpoint', 'checkpoints')}. ` : '') + (done.data.send.sent ? 'Your new work is sent, encrypted.' : 'Nothing new to send.'), 'ok');
    }),
  ]);
}

// The passphrase stored on this computer for a shared project, read back after a confirmation,
// for someone who forgot it (D131). It is fetched only when confirmed and is not kept in the page.
async function showPassphrase(p) {
  const shared = team.projects[p.id] || {};
  openDialog(`Show the passphrase of ${p.name}?`, [
    el('p', { text: `It opens everything sent for "${shared.name || p.name}" in the bucket. Anyone who sees it and can reach the bucket can read this project's instructions, checkpoints and conversations.` }),
    el('p', { class: 'quiet small', text: 'Make sure nobody else can see your screen. Give it to a colleague only in person or through a password manager, not in the same message as the storage keys.' }),
  ], [
    btn('Cancel', 'btn', () => dialog.close()),
    btn('Show it', 'btn primary', async () => {
      const r = await api('POST', '/api/team/passphrase', { project: p.id, confirm: true });
      if (!r.ok || !r.data || typeof r.data.passphrase !== 'string') { dialog.close(); return say(problem(r), 'bad'); }
      const secret = el('blockquote', { class: 'exact', text: r.data.passphrase });
      const copied = el('span', { class: 'quiet small', role: 'status' });
      const canCopy = typeof navigator !== 'undefined' && navigator.clipboard && navigator.clipboard.writeText;
      openDialog(`Passphrase of ${p.name}`, [
        el('p', { class: 'quiet small', text: `Shared as "${r.data.name}".` }),
        secret,
        copied,
      ], [
        canCopy ? btn('Copy', 'btn', async () => {
          try { await navigator.clipboard.writeText(r.data.passphrase); copied.textContent = 'Copied. Clear it from the clipboard when you are done.'; } catch (_) { copied.textContent = 'Could not copy; select the text instead.'; }
        }) : null,
        btn('Close', 'btn primary', () => dialog.close()),
      ].filter(Boolean));
    }),
  ]);
}

async function stopSharing(p) {
  const r = await api('POST', '/api/team/disconnect', { project: p.id, confirm: false });
  if (r.status !== 409) return say(problem(r), 'bad');
  openDialog(`Stop sharing ${p.name}?`, [
    el('p', { text: 'This computer forgets the passphrase and what it sent and received for this project. Nothing more is sent or received. Everything saved here stays.' }),
    el('p', { class: 'quiet small', text: 'What is already in the bucket stays there until you delete it in the storage service, and colleagues keep what they received. To share again later you need the shared name and the passphrase: if you are not sure you remember it, keep sharing and use Show passphrase first.' }),
  ], [
    btn('Keep sharing', 'btn', () => dialog.close()),
    btn('Stop sharing', 'btn danger', async () => {
      const done = await api('POST', '/api/team/disconnect', { project: p.id, confirm: true });
      dialog.close();
      if (!done.ok) return say(problem(done), 'bad');
      await loadTeam();
      await reload();
      say(`${p.name} is no longer shared.`, 'ok');
    }),
  ]);
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
        ? { kind: 'bad', tag: 'Disputed', title: c.subject, detail: `${p.name}: the source page no longer contains the quoted words.`, action: 'Look at the fact', href: projectHref(p.id, 'research', c.id) }
        : { kind: 'warn', tag: 'Out of date', title: c.subject, detail: `${p.name}: it should have been checked again by ${dateOnly(c.expires_at)}.`, action: 'Look at the fact', href: projectHref(p.id, 'research', c.id) });
    }
    if (cp.ok && cp.data.checkpoints[0]) {
      const w = cp.data.warnings || [];
      cps.push({ p, c: cp.data.checkpoints[0], w });
      if (w.length) items.push({ kind: 'warn', tag: 'Code moved', title: `${p.name} changed since its last checkpoint`, detail: w.join('; ') + '.', action: 'See what changed', href: projectHref(p.id, 'resume') });
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

// ---- Overview (every project) ---------------------------------------------------------------

const minutesOf = x => Math.max(0, (new Date(x.last_activity_at || x.started_at) - new Date(x.started_at)) / 60000) || 0;
const sizeOf = x => { const m = minutesOf(x); return m < 20 ? 'short' : m < 60 ? 'mid' : 'long'; };
const dayKey = d => `${d.getFullYear()}-${two(d.getMonth() + 1)}-${two(d.getDate())}`;
const sessionLine = x => x.first_prompt || '(no text was saved in this session)';
const sessionHref = x => projectHref(x.project_id, 'sessions', x.id, 'read');

// Where everything starts when there is no project yet.
function noProjectsYet() {
  return el('div', { class: 'empty' },
    el('h2', { text: 'No project added yet' }),
    el('p', { class: 'quiet', text: 'SessionKeep only saves conversations in folders you add. Right now nothing is being saved.' }),
    el('a', { class: 'go', href: '#/projects', text: 'Add your first project' }),
    el('p', { class: 'quiet small' }, 'Or in a terminal, inside the folder: ', el('code', { text: 'sessionkeep project add .' })));
}

// Sessions of the last seven days, one row per project and one block per session; a block is
// wider the longer the session lasted.
function weekGrid(sessions) {
  const today = new Date(); today.setHours(0, 0, 0, 0);
  const days = [...Array(7)].map((_, i) => new Date(today.getTime() - (6 - i) * 86400000));
  const byCell = {};
  for (const x of sessions) {
    const d = new Date(String(x.started_at || '').replace(/(\.\d{3})\d+/, '$1'));
    if (isNaN(d)) continue;
    (byCell[x.project_id + '/' + dayKey(d)] = byCell[x.project_id + '/' + dayKey(d)] || []).push(x);
  }
  const DAYNAMES = ['Sun', 'Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat'];
  const grid = el('div', { class: 'weekgrid', role: 'table', 'aria-label': 'Sessions in the last seven days' });
  grid.append(el('span', { class: 'wk-corner', role: 'columnheader', text: 'Project' }),
    ...days.map((d, i) => el('span', { class: 'wk-day' + (i === 6 ? ' today' : ''), role: 'columnheader', text: i === 6 ? 'Today' : `${DAYNAMES[d.getDay()]} ${d.getDate()}` })));
  for (const p of projects) {
    grid.append(el('a', { class: 'wk-name', role: 'rowheader', href: projectHref(p.id), text: p.name }));
    for (const [i, d] of days.entries()) {
      const xs = byCell[p.id + '/' + dayKey(d)] || [];
      const cell = el('span', { class: 'wk-cell' + (i === 6 ? ' today' : ''), role: 'cell', 'aria-label': xs.length ? plural(xs.length, 'session', 'sessions') : 'no sessions' });
      for (const x of xs.slice(0, 6)) cell.append(el('a', { class: 'wk-s ' + sizeOf(x), href: sessionHref(x), title: `${clock(x.started_at)}, ${Math.round(minutesOf(x))} min: ${clip(sessionLine(x), 80)}`, 'aria-label': `${clock(x.started_at)}: ${clip(sessionLine(x), 60)}` }));
      if (xs.length > 6) cell.append(el('span', { class: 'wk-more', text: '+' + (xs.length - 6) }));
      grid.append(cell);
    }
  }
  return grid;
}

// One thing that needs the person, with one action.
function needRow(a) {
  return el('li', { class: 'need' },
    el('span', { class: 'need-bar ' + a.kind, 'aria-hidden': 'true' }),
    el('div', {}, el('b', { text: a.title }), el('span', { class: 'quiet small', text: a.detail })),
    a.href ? el('a', { class: 'btn', href: a.href, text: a.action }) : btn(a.action, a.primary ? 'btn primary' : 'btn', a.run));
}

async function overviewAll() {
  const [st, , ses, doc] = await Promise.all([api('GET', '/api/status'), loadProjects(), api('GET', '/api/sessions?all=1&limit=200'), api('GET', '/api/doctor')]);
  if (!st.ok) return say(problem(st), 'bad');
  if (!projects.length) { lede.textContent = 'Nothing is being saved yet.'; return view.append(noProjectsYet()); }
  const s = st.data, sessions = ses.ok ? ses.data : [];
  const saving = projects.filter(p => !p.paused && p.capture_level !== 'off');
  const checks = doc.data ? doc.data.checks : [], failing = checks.filter(c => c.status !== 'ok').length;
  const weekAgo = Date.now() - 7 * 86400000;
  const thisWeek = sessions.filter(x => new Date(x.started_at) >= weekAgo).length;

  const statusLine = el('div', { class: 'statusline' },
    el('span', {}, el('span', { class: 'dot ' + (saving.length ? 'ok' : 'off'), 'aria-hidden': 'true' }), el('b', { text: `${saving.length} of ${projects.length}` }), ' projects saving'),
    el('span', {}, el('b', { text: Number(s.stored.sessions || 0).toLocaleString('en') }), ` sessions kept, ${thisWeek} this week`),
    el('a', { href: '#/health' }, el('span', { class: 'dot ' + (failing ? 'bad' : 'ok'), 'aria-hidden': 'true' }), 'Health: ', el('b', { text: failing ? `${failing} of ${checks.length}` : `all ${checks.length}` }), failing ? ' checks not working' : ' checks working'));
  const cont = el('section', { class: 'continue', 'aria-label': 'Continue where you stopped' }, working('Finding where you stopped.'));
  const needsHead = el('h2', { text: 'Needs you' });
  const needs = el('ul', { class: 'needs', 'aria-live': 'polite' }, el('li', {}, working('Looking for things that need you.')));
  view.append(el('div', { class: 'stack' }, statusLine, cont,
    el('section', { class: 'card' }, el('div', { class: 'between' }, el('h2', { text: 'This week' }),
      el('span', { class: 'legend' }, el('i', { class: 'wk-s short', 'aria-hidden': 'true' }), 'short session', el('i', { class: 'wk-s long', 'aria-hidden': 'true' }), 'long session')), weekGrid(sessions)),
    el('div', { class: 'pair' },
      el('section', { class: 'card' }, el('div', { class: 'between' }, needsHead, el('span', { class: 'quiet small', text: 'Nothing happens until you press a button.' })), needs),
      el('section', { class: 'card' }, el('div', { class: 'between' }, el('h2', { text: 'Latest sessions' }), el('a', { class: 'more-link', href: '#/search', text: 'Search all sessions' })),
        sessions.length ? el('ul', { class: 'latestlist' }, sessions.slice(0, 6).map(x => el('li', {}, el('a', { href: sessionHref(x) },
          el('span', { class: 't', text: clip(sessionLine(x), 90) }), el('span', { class: 'p', text: x.project_name }),
          el('span', { class: 'w' + (x.state === 'ended' ? '' : ' live'), text: x.state === 'ended' ? nice(x.started_at) : 'May still be running' })))))
          : el('p', { class: 'quiet small', text: 'No session has been saved yet.' })))));

  const found = await attention(s);
  if (!cont.isConnected) return;
  // The session most likely to be picked up again: the newest one, with its project's last checkpoint.
  const latest = sessions[0], lp = latest && projects.find(p => p.id === latest.project_id);
  if (!lp) cont.replaceChildren(el('p', { text: 'No session has been saved yet. Start Claude Code in one of your projects.' }));
  else {
    const cp = found.cps.find(x => x.p.id === lp.id), parts = cp && noteParts(cp.c.note);
    cont.replaceChildren(el('div', { class: 'cont-main' },
      el('p', { class: 'cont-where' }, el('span', { class: 'dot ' + (latest.state === 'ended' ? 'ok' : 'live'), 'aria-hidden': 'true' }),
        el('b', { text: lp.name }), latest.state === 'ended' ? ` last session ${nice(latest.last_activity_at || latest.started_at)}` : ` has a session that may still be running, started ${nice(latest.started_at)}`),
      el('h2', { class: 'cont-title', text: clip(sessionLine(latest), 110) }),
      cp ? el('dl', { class: 'cont-steps' }, parts ? [['Done', parts.done], ['Next', parts.next], ['Blocking', parts.blocking]].filter(x => x[1]).map(([k, v]) => el('div', {}, el('dt', { text: k }), el('dd', { text: clip(v, 120) })))
        : el('div', {}, el('dt', { text: 'Checkpoint' }), el('dd', { text: clip(cp.c.note, 160) })))
        : el('p', { class: 'cont-none', text: 'No checkpoint in this project yet. Write one on its Pick up work page before you stop.' })),
    el('div', { class: 'cont-acts' }, el('a', { class: 'btn primary', href: projectHref(lp.id, 'resume'), text: 'Pick up work' }), el('a', { class: 'btn', href: projectHref(lp.id), text: `Open ${lp.name}` })));
  }
  const paint = () => {
    needsHead.textContent = found.items.length ? `Needs you (${found.items.length})` : 'Needs you';
    needs.replaceChildren(...(found.items.length ? found.items.map(needRow) : [el('li', { class: 'quiet', text: 'Nothing needs you right now.' })]));
  };
  paint();
  // Claude Code takes a moment to answer, so accounts are added after the rest.
  const ac = await api('GET', '/api/accounts/plugin');
  if (!needs.isConnected || !ac.ok) return;
  for (const a of ac.data.accounts || []) {
    if (a.plugin === 'off') found.items.push({ kind: 'off', tag: 'Off', title: `SessionKeep is off for the account "${a.name}"`, detail: 'Sessions started with that account are not saved.', action: 'Open settings', href: '#/settings/claude' });
  }
  paint();
  badge('n-overview', found.items.length, found.items.some(i => i.kind === 'bad') ? 'bad' : 'warn');
}

// ---- Overview (one project) -----------------------------------------------------------------

async function projectHome(p) {
  const [ses, cp, ins] = await Promise.all([api('GET', `/api/sessions?project=${enc(p.id)}&limit=200`), api('GET', '/api/resume?project=' + enc(p.id)), api('GET', '/api/instructions?all=1'), loadTeam()]);
  const sessions = ses.ok ? ses.data : [];
  const [kind, word, sentence] = savingState(p);
  const shared = team.projects[p.id];
  const own = ins.ok ? ins.data.filter(i => i.state === 'active' && i.scope !== 'global' && i.project_id === p.id).length : 0;
  const global = ins.ok ? ins.data.filter(i => i.state === 'active' && i.scope === 'global').length : 0;
  const c = cp.ok ? cp.data.checkpoints[0] : null, parts = c && noteParts(c.note), warnings = cp.ok ? cp.data.warnings || [] : [];
  if (p.paused) view.append(el('p', { class: 'banner warn' }, el('strong', { text: 'Saving is paused here.' }), el('span', { text: 'Nothing new is kept until you resume. ' }), el('a', { href: projectHref(p.id, 'settings'), text: 'Resume saving' })));
  view.append(el('div', { class: 'stack' },
    el('div', { class: 'statusline' },
      el('span', {}, dotword(kind, word), ' ', sentence),
      el('span', {}, el('b', { text: sessions.length >= 200 ? '200+' : String(sessions.length) }), ' sessions saved'),
      el('span', {}, sessions[0] ? ['Last session ', el('b', { text: nice(sessions[0].last_activity_at || sessions[0].started_at) })] : 'No session yet'),
      el('a', { href: projectHref(p.id, 'sharing') }, shared ? `Shared through ${placeOf(shared)}` : 'Not shared')),
    el('div', { class: 'pair' },
      el('section', { class: 'card' }, el('div', { class: 'between' }, el('h2', { text: 'Pick up where you stopped' }), el('a', { class: 'more-link', href: projectHref(p.id, 'resume'), text: 'Open' })),
        !c ? el('p', { class: 'quiet', text: 'No checkpoint yet. Write one on Pick up work before you stop, or ask Claude to save your progress.' })
          : parts ? el('dl', { class: 'cp' }, [['Done', parts.done], ['Next', parts.next], ['Blocking', parts.blocking]].filter(x => x[1]).map(([k, v]) => [el('dt', { text: k }), el('dd', { text: v })]))
            : el('p', { class: 'said', text: c.note }),
        c ? (warnings.length ? dotword('warn', 'The code changed since') : dotword('ok', 'The code is as it was')) : null,
        c ? el('p', { class: 'quiet small', text: 'Written ' + nice(c.created_at) }) : null),
      el('section', { class: 'card' }, el('div', { class: 'between' }, el('h2', { text: 'Latest sessions' }), el('a', { class: 'more-link', href: projectHref(p.id, 'sessions'), text: 'All sessions' })),
        sessions.length ? el('ul', { class: 'latestlist' }, sessions.slice(0, 5).map(x => el('li', {}, el('a', { href: sessionHref(x) },
          el('span', { class: 't', text: clip(sessionLine(x), 90) }), el('span', { class: 'p', text: plural(x.text_records, 'piece of text', 'pieces of text') }),
          el('span', { class: 'w' + (x.state === 'ended' ? '' : ' live'), text: x.state === 'ended' ? nice(x.started_at) : 'May still be running' })))))
          : el('p', { class: 'quiet', text: 'No session has been saved here yet.' }))),
    el('section', { class: 'card' }, el('h2', { text: 'What Claude is told here' }),
      el('p', {}, `${plural(own, 'instruction', 'instructions')} for this project and ${plural(global, 'instruction', 'instructions')} for all projects. `,
        el('a', { href: projectHref(p.id, 'instructions'), text: 'See this project’s' }), ', or ', el('a', { href: '#/instructions', text: 'the ones for all projects' }), '.'),
      el('p', { class: 'quiet small', text: p.recall === 'automatic' ? 'Earlier sessions reach Claude automatically.' : 'Claude looks into earlier sessions when you refer to them.' }))));
}

// ---- Recap ----------------------------------------------------------------------------------

// What was done in a day, or in the last seven: sessions, checkpoints and commits per project.
async function recapView(day) {
  const DAYS = [['today', 'Today'], ['yesterday', 'Yesterday'], ['week', 'Last 7 days']];
  if (!DAYS.some(d => d[0] === day)) day = 'today';
  actions.append(seg('Which days', DAYS, day, v => { location.hash = projectHref(scope, 'recap', v); }));
  const r = await api('GET', `/api/recap?day=${day}&${scope === '*' ? 'all=1' : 'project=' + enc(scope)}`);
  if (!r.ok) return say(problem(r), 'bad');
  const d = r.data;
  lede.textContent = `${plural(d.sessions, 'session', 'sessions')} and ${plural(d.requests, 'request', 'requests')} in ${scopeName()}, ${d.label}.`;
  if (!d.projects.length) {
    return view.append(el('div', { class: 'empty' },
      el('h2', { text: 'Nothing was saved in that time' }),
      el('p', { class: 'quiet', text: 'No session, checkpoint or commit falls in these days. Choose other days above, or another project in the sidebar.' })));
  }
  const stamp = iso => day === 'week' ? nice(iso) : clock(iso);
  const line = (at, text) => el('div', { class: 'recap-line' }, el('span', { class: 'mono', text: stamp(at) }), el('span', { text: text }));
  view.append(el('div', { class: 'stack' }, d.projects.map(p => {
    const requests = p.sessions.reduce((n, x) => n + x.requests, 0);
    return el('section', { class: 'card' },
      el('div', { class: 'sechead' }, el('h2', { class: 'mono', text: p.name }),
        el('span', { class: 'quiet small', text: `${plural(p.sessions.length, 'session', 'sessions')}, ${plural(requests, 'request', 'requests')}, ${plural(p.checkpoints.length, 'checkpoint', 'checkpoints')}, ${plural(p.commits.length, 'commit', 'commits')}` })),
      p.sessions.map(x => el('a', { class: 'latest', href: `#/p/${p.project_id}/sessions/${x.id}/read` },
        el('span', { class: 'top' }, el('span', { text: `${stamp(x.from)} to ${clock(x.to)}` }),
          el('span', { text: plural(x.requests, 'request', 'requests') + (x.failed_commands ? `, ${plural(x.failed_commands, 'failed command', 'failed commands')}` : '') })),
        el('span', { class: 'line wrap', text: x.first_request || '(no text was saved in this session)' }),
        x.summary ? el('span', { class: 'line wrap quiet', text: 'Summary, written by a model: ' + clip(x.summary.replace(/\s+/g, ' '), 320) })
          : x.last_reply ? el('span', { class: 'line wrap quiet', text: 'Last reply: ' + x.last_reply }) : null,
        x.files && x.files.length ? el('span', { class: 'line wrap quiet small', text: 'Files: ' + x.files.join(', ') + (x.more_files ? ` and ${x.more_files} more` : '') }) : null)),
      p.checkpoints.length ? [el('h3', { text: 'Checkpoints' }), p.checkpoints.map(c => line(c.created_at, clip(c.note.replace(/\s+/g, ' '), 240)))] : null,
      p.commits.length ? [el('h3', { text: 'Commits' }), p.commits.map(c => line(c.at, `${c.id}  ${c.subject}`))] : null);
  })));
}

// ---- Pick up work ---------------------------------------------------------------------------

async function resumeView(pid) {
  const pr = await loadProjects();
  if (!pr.ok) return say(problem(pr), 'bad');
  if (!projects.length) return view.append(el('p', { class: 'quiet', text: 'Add a project first.' }));
  lede.textContent = 'Where the work stood when you last stopped, and whether the code has moved since.';
  const p = projects.find(x => x.id === scope);
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
  if (here) actions.append(btn('Bring in missing sessions', 'btn', () => sync(here), `Bring in sessions of ${here.name} that Claude Code has and SessionKeep does not`));
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
      el('div', { class: 'row' }, el('a', { class: 'go', href: `#/p/${x.project_id}/sessions/${x.id}/read`, text: 'Read the whole conversation' })),
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
    list.replaceChildren(...shown().map(x => el('a', { class: 'pick', href: `#/p/${x.project_id}/sessions/${x.id}`, 'data-id': x.id },
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
  selectors.sessions = args => { if (args[1] === 'read') return false; choose(args[0] || ''); return true; };
}

async function sessionView(pid, sid, rid) {
  const r = await api('GET', `/api/sessions/${sid}?project=${enc(pid)}`);
  if (!r.ok) return say(problem(r), 'bad');
  const s = r.data.session;
  title.textContent = 'Session';
  crumb.textContent = `${s.project_name}, ${nice(s.started_at)}, ${plural(s.text_records, 'piece of text', 'pieces of text')}, ${s.state === 'ended' ? 'ended' : 'may still be running'}`;
  lede.textContent = clip(s.first_prompt || '', 200);
  actions.append(el('a', { class: 'go plain', href: `#/p/${pid}/sessions/${sid}`, text: 'Back to Sessions' }));
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
      s ? el('a', { class: 'go', href: `#/p/${s.project_id}/sessions/${s.id}/read/${x.id}`, text: 'Open the session here' }) : null,
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
      el('p', { class: 'quiet', text: (exact.checked ? 'Nobody said these words in this order. Turn off Exact phrase, or ' : 'Try other words for the same thing, or ') + (scope === '*' ? 'ask who said it differently.' : 'use Search everything to look in every project.') })));
    if (sums.length) results.append(el('div', { class: 'summary' },
      el('h3', { text: 'Session summaries that match' + (d.summaries_partial ? ' some of the words' : '') + ', written by a model' }),
      sums.map(h => el('p', {}, el('a', { href: `#/p/${h.project_id}/sessions/${h.session_id}/read`, text: h.snippet.split('**').join('') })))));
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
  const mine = r.data.filter(i => scope === '*' ? i.scope === 'global' : i.scope !== 'global' && i.project_id === scope);
  const globalCount = r.data.filter(i => i.scope === 'global' && i.state === 'active').length;
  if (scope !== '*') lede.textContent = `Claude receives these word for word at the start of every session in ${scopeName()}. It also gets ${plural(globalCount, 'instruction', 'instructions')} for all projects.`;
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

  const add = prefill => {
    mark(list, '');
    const text = el('textarea', { id: 'new-text', required: true, rows: '3' });
    if (typeof prefill === 'string') text.value = prefill;
    const where2 = choice('new-scope', 'Where it applies', scope === '*' ? [['global', 'Every project', 'Sessions in every project.']] : [['project', 'This project', `Only sessions in ${scopeName()}.`]], scope === '*' ? 'global' : 'project');
    const check = el('div', {});
    const submit = async confirm => {
      const body = { text: text.value, scope: where2.value(), confirm };
      if (body.scope === 'project') body.project = scope;
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
      field('What should Claude always know or do', text, 'Your own words. They are saved and repeated exactly.'), where2,
      el('button', { type: 'submit', class: 'primary start', text: 'Check before saving' })), check));
    text.focus();
  };

  const paint = () => {
    const rows = tab === 'active' ? active : retired;
    list.replaceChildren(seg('Which instructions', [['active', `In use (${active.length})`], ['retired', `Retired (${retired.length})`]], tab, v => { tab = v; paint(); }, 'tabs'),
      ...(rows.length ? rows.map(i => el('a', { class: 'pick boxed', href: (scope === '*' ? '#/instructions/' : projectHref(scope, 'instructions') + '/') + i.id, 'data-id': i.id },
        el('span', { class: 'main', text: i.text }),
        el('span', { class: 'sub' }, el('span', { class: 'mono', text: where(i) }), el('span', { text: i.state === 'active' ? 'In use' : 'Retired' }),
          el('span', { text: i.revision > 1 ? plural(i.revision - 1, 'earlier wording', 'earlier wordings') : 'Never changed' }))))
        : [el('p', { class: 'quiet', text: tab === 'active' ? 'No instructions yet. Add one, or tell Claude "remember that..." in a session.' : 'Nothing has been retired.' })]));
    const first = rows.find(i => i.id === sel) || rows[0];
    if (first) { mark(list, first.id); detail(first.id); } else pane.replaceChildren();
  };
  // Requests made in nearly the same words in several sessions; each can be put into the form.
  const repeated = async () => {
    mark(list, '');
    const heading = 'Things you keep telling Claude';
    pane.replaceChildren(card(heading, working('Reading your requests.')));
    const r = await api('GET', `/api/suggestions?${scope === '*' ? 'all=1' : 'project=' + enc(scope)}`);
    if (!r.ok) return pane.replaceChildren(card(heading, el('p', { class: 'quiet', text: problem(r) })));
    const found = r.data.suggestions;
    pane.replaceChildren(card(heading,
      el('p', { class: 'quiet small', text: `Requests you made in nearly the same words in three or more sessions of ${scopeName()}, and that no instruction covers. They are found by comparing words, not by a model, so read each one. Nothing is saved until you add it.` }),
      found.length ? found.map(x => el('div', { class: 'confirm' },
        el('p', { class: 'said', text: x.text }),
        el('div', { class: 'row' }, btn('Use as an instruction', 'btn', () => add(x.text)),
          el('span', { class: 'quiet small', text: `In ${plural(x.sessions, 'session', 'sessions')}, last ${nice(x.last_at)}` }))))
        : el('p', { class: 'quiet', text: 'Nothing is repeated often enough yet.' })));
  };
  actions.append(btn('Things you repeat', 'btn', repeated), btn('Add an instruction', 'primary', () => add()));
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
    await show(`p/${c.project.id}/research/${c.id}`, true);
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
      return el('a', { class: 'pick', href: projectHref(c.project.id, 'research', c.id), 'data-id': c.id },
        el('span', { class: 'top' }, el('span', { text: c.subject }), el('span', { class: 'word-' + kind, text: word })),
        el('span', { class: 'main', text: c.statement }),
        el('span', { class: 'sub mono' }, el('span', { text: c.project.name }), el('span', { text: c.verified_at ? 'checked ' + dateOnly(c.verified_at) : 'never checked, no source' }))); })
      : [el('p', { class: 'working', text: all.length ? 'No facts in this state.' : 'No facts are recorded here. Claude records them when it looks something up with the research skill.' })]));
    const first = rows.find(c => c.id === sel) || rows[0];
    if (first) { mark(list, first.id); detail(first.id); } else pane.replaceChildren();
  };
  view.append(filters, el('div', { class: 'cols' }, el('div', { class: 'col-list' }, list), pane));
  paint();
  selectors.research = args => { if (!args[0] || !all.some(c => c.id === args[0])) return false; sel = args[0]; mark(list, sel); detail(sel); return true; };
}

// ---- Choosing a folder ----------------------------------------------------------------------

// The parts of a path, each with the path up to it, for the clickable path at the top.
function pathParts(path) {
  if (/^[a-z]:\\/i.test(path) || path.includes('\\')) {
    const parts = path.split('\\').filter(Boolean);
    return parts.map((x, i) => [i ? x : x + '\\', i ? parts.slice(0, i + 1).join('\\') : x + '\\']);
  }
  const parts = path.split('/').filter(Boolean);
  return [['/', '/'], ...parts.map((x, i) => [x, '/' + parts.slice(0, i + 1).join('/')])];
}

// A window for choosing a folder on this computer. SessionKeep's own server lists the folders
// (a web page cannot see full paths), so it works the same on Windows, macOS and Linux. It
// resolves to the chosen path, or to '' when nothing was chosen.
function chooseFolder(start) {
  return new Promise(resolve => {
    let current = null, done = false;
    const finish = path => { if (done) return; done = true; dialog.classList.remove('wide'); resolve(path); };
    const places = el('nav', { class: 'pk-places', 'aria-label': 'Places' });
    const crumbs = el('div', { class: 'pk-crumbs' });
    const list = el('ul', { class: 'pk-list', 'aria-live': 'polite' });
    const chosen = el('p', { class: 'pk-chosen path' });
    const go = async path => {
      list.replaceChildren(el('li', {}, working('Reading the folder.')));
      const r = await api('GET', '/api/folders' + (path ? '?path=' + enc(path) : ''));
      if (!r.ok) return list.replaceChildren(el('li', { class: 'pk-empty', text: problem(r) }));
      current = r.data;
      chosen.textContent = 'Selected: ' + current.path;
      places.replaceChildren(el('p', { class: 'pk-head', text: 'Places' }), ...current.places.map(pl => btn(pl.name, 'pk-place' + (pl.path === current.path ? ' on' : ''), () => go(pl.path), `Go to ${pl.path}`)));
      crumbs.replaceChildren(btn('Up', 'btn', () => current.parent ? go(current.parent) : null, 'Go up one folder'),
        ...pathParts(current.path).map(([label, to], i) => [i ? el('span', { class: 'pk-sep', 'aria-hidden': 'true', text: '›' }) : null, btn(label, 'pk-crumb', () => go(to), `Go to ${to}`)]).flat().filter(Boolean));
      list.replaceChildren(...(current.folders.length ? current.folders.map(f => {
        const b = el('button', { type: 'button', class: 'pk-row', text: f.name, onclick: () => go(f.path) });
        b.prepend(icon('folder') || '');
        if (f.project) b.append(el('span', { class: 'pk-tag done', text: 'Already a project' }));
        else if (f.excluded) b.append(el('span', { class: 'pk-tag off', text: 'Never saved' }));
        else if (f.git) b.append(el('span', { class: 'pk-tag', text: 'Git repository' }));
        return el('li', {}, b);
      }) : [el('li', { class: 'pk-empty', text: 'No folders inside. Choose this folder, or go up.' })]));
    };
    dialog.classList.add('wide');
    openDialog('Choose a project folder', [
      el('div', { class: 'pk' }, places, el('div', { class: 'pk-main' }, crumbs, list)),
      chosen,
    ], [
      btn('Cancel', 'btn', () => { dialog.close(); finish(''); }),
      btn('Use this folder', 'btn primary', () => { const p = current ? current.path : ''; dialog.close(); finish(p); }),
    ]);
    dialog.addEventListener('close', () => finish(''), { once: true });
    go(start || '');
  });
}

// A folder path field with a button that opens the folder chooser.
function folderField(id, label, hint, value) {
  const input = el('input', { type: 'text', id, class: 'code', required: true, autocomplete: 'off', placeholder: 'Type or paste a path, or choose a folder' });
  input.setAttribute('aria-label', label);
  if (value) input.value = value;
  const pick = btn('Choose folder', 'btn', async () => { const p = await chooseFolder(input.value.trim()); if (p) { input.value = p; input.focus(); } }, `Choose the folder for: ${label}`);
  return { input, node: el('div', { class: 'fieldwrap' }, el('label', { for: id, class: 'lbl', text: label }), el('div', { class: 'inputrow' }, input, pick), hint ? el('span', { class: 'hint', text: hint }) : null) };
}

// ---- Projects (every project) ---------------------------------------------------------------

async function projectsAll() {
  const [pr, ses] = await Promise.all([loadProjects(), api('GET', '/api/sessions?all=1&limit=200'), loadTeam()]);
  if (!pr.ok) return say(problem(pr), 'bad');
  const last = {};
  for (const x of ses.ok ? ses.data : []) if (!last[x.project_id]) last[x.project_id] = x;

  const folder = folderField('add-path', 'Folder', 'Everything inside it counts as this project. Works on Windows, macOS and Linux.');
  const name = el('input', { type: 'text', id: 'add-name', autocomplete: 'off' });
  const addLevel = select('add-level', LEVELS.map(l => [l[0], l[1]]), 'conversation');
  // A new project that saves text is offered its earlier Claude Code sessions straight away.
  const addProject = async e => {
    e.preventDefault();
    const saveText = addLevel.value === 'conversation' || addLevel.value === 'extended';
    const r = await act('/api/projects', { path: folder.input.value, level: addLevel.value, name: name.value }, 'Project added.');
    const added = r.ok && r.data ? projects.find(p => p.id === r.data.id) : null;
    if (added && saveText) await sync(added, true);
  };
  const addCard = card('Add a project', el('form', { onsubmit: addProject },
    folder.node,
    el('div', { class: 'pair' }, field('What is saved', addLevel), field('Name', name, 'Optional. The folder name is used if empty.')),
    el('button', { type: 'submit', class: 'primary start', text: 'Add project and start saving' })));

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
    found.replaceChildren(working(confirm ? 'Importing earlier sessions. This can take a minute.' : 'Reading Claude Code’s session files. This can take a minute.'));
    const r = await api('POST', '/api/history/import', { confirm });
    if (r.ok) { found.replaceChildren(...describe(r.data).flat().filter(Boolean)); return say('Earlier sessions imported.', 'ok'); }
    if (r.status !== 409 || !r.data) { found.replaceChildren(); return say(problem(r), 'bad'); }
    say('Nothing is imported yet.');
    found.replaceChildren(...describe(r.data).flat().filter(Boolean), r.data.turns ? btn(`Import ${plural(r.data.sessions, 'session', 'sessions')}`, 'btn primary', () => importHistory(true)) : '');
  };
  const importCard = card('Import earlier Claude Code sessions',
    el('p', { class: 'quiet small', text: 'Bring in what you and Claude said before SessionKeep was installed, for the projects above. Claude’s private reasoning, tool output and images are never imported. Nothing is imported until you confirm.' }),
    btn('Check what can be imported', 'btn start', () => importHistory(false)), found);

  actions.append(btn('Add a project', 'primary', () => { addCard.scrollIntoView({ block: 'center' }); folder.input.focus(); }));
  const table = projects.length ? el('div', { class: 'card flush' }, el('table', { class: 'projtable' },
    el('thead', {}, el('tr', {}, ['Project', 'Saving', 'What is saved', 'Last session', 'Sharing', ''].map(h => el('th', { scope: 'col', text: h })))),
    el('tbody', {}, projects.map(p => {
      const [kind, word] = savingState(p), x = last[p.id], sh = team.projects[p.id];
      return el('tr', {},
        el('td', {}, el('a', { class: 'pname', href: projectHref(p.id), text: p.name }), el('span', { class: 'path', text: p.roots.join(', ') + (p.added_by === 'auto' ? ' (added automatically)' : '') })),
        el('td', {}, dotword(kind, word)),
        el('td', { class: 'small', text: levelName(p.capture_level) }),
        el('td', { class: 'small quiet', text: x ? nice(x.last_activity_at || x.started_at) : 'Never' }),
        el('td', {}, sh ? chip('accent', placeOf(sh)) : el('span', { class: 'quiet small', text: 'This computer only' })),
        el('td', { class: 'right' }, el('a', { class: 'btn', href: projectHref(p.id), text: 'Open', 'aria-label': `Open ${p.name}` })));
    })))) : noProjectsYet();
  view.append(el('div', { class: 'stack' }, table, el('div', { class: 'pair' }, addCard, importCard)));
}

// ---- Sharing (one project) ------------------------------------------------------------------

async function sharingPage(p) {
  await loadTeam();
  const shared = team.projects[p.id];
  if (shared) {
    return view.append(el('div', { class: 'stack' },
      el('section', { class: 'card' },
        el('div', { class: 'between' }, el('h2', { text: `Shared as "${shared.name}"` }), chip('accent', placeOf(shared))),
        el('p', { class: 'quiet', text: `In the bucket ${shared.bucket}${shared.as ? `; colleagues see you as "${shared.as}"` : ''}. Nothing is sent or received until you press Sync now.` }),
        el('div', { class: 'row' }, btn('Sync now', 'btn primary', () => teamSync(p), `Receive and send the work on ${p.name}`)),
        el('p', { class: 'quiet small', text: 'Sync first shows what would be received and sent, including every instruction a colleague sent, word for word.' })),
      el('div', { class: 'pair' },
        el('section', { class: 'card' }, el('h2', { text: 'Passphrase' }),
          el('p', { class: 'quiet small', text: 'Forgot it, or a colleague needs it? This computer is connected, so it can show it after you confirm.' }),
          el('div', { class: 'row' }, btn('Show passphrase', 'btn', () => showPassphrase(p), `Show the passphrase of ${p.name}`))),
        el('section', { class: 'card' }, el('h2', { text: 'Stop sharing' }),
          el('p', { class: 'quiet small', text: 'This computer forgets the passphrase and stops sending and receiving. Everything saved here stays, and so does what is in the bucket.' }),
          el('div', { class: 'row' }, btn('Stop sharing', 'btn danger', () => stopSharing(p), `Stop sharing ${p.name}`))))));
  }
  view.append(shareForm(p));
}

// ---- Project settings -----------------------------------------------------------------------

async function projectSettings(p) {
  const level = choice('lvl-' + p.id, 'What is saved', LEVELS, p.capture_level);
  const recall = choice('rec-' + p.id, 'How Claude gets earlier sessions', [
    ['asked', 'When asked', 'Claude gets your instructions and the latest checkpoint, and looks into earlier sessions when you refer to them.'],
    ['automatic', 'Automatically', 'Also: every new session is told what the latest sessions were about, and with every request Claude is handed the saved pieces that share its words. They are picked by matching words, so some will be beside the point.']], p.recall || 'asked');
  const to = folderField('move-to', 'New folder', 'Renamed or moved the folder? Choose where it is now; its saved sessions stay with it.');
  const newName = el('input', { type: 'text', id: 'move-name', autocomplete: 'off' });
  const gone = () => { location.hash = '#/projects'; };
  view.append(el('div', { class: 'stack narrowcol' },
    el('section', { class: 'card' }, level,
      el('div', { class: 'row' }, btn('Save what is saved', 'btn primary', () => act(`/api/projects/${p.id}/capture`, { level: level.value() }, 'Changed.')),
        btn(p.paused ? 'Resume saving' : 'Pause saving', 'btn', () => act(`/api/projects/${p.id}/${p.paused ? 'resume' : 'pause'}`, {}, p.paused ? 'Saving again.' : 'Paused.')))),
    el('section', { class: 'card' }, recall,
      el('div', { class: 'row' }, btn('Save how Claude gets them', 'btn primary', () => act(`/api/projects/${p.id}/recall`, { mode: recall.value() }, 'Changed. It applies from the next request.')))),
    el('section', { class: 'card' }, el('h2', { text: 'Folder' }),
      el('p', { class: 'path', text: p.roots.join(', ') }),
      el('form', { onsubmit: async e => { e.preventDefault(); const r = await act(`/api/projects/${p.id}/move`, { to: to.input.value, name: newName.value }, 'Moved. Everything saved stays with the project.'); if (r.ok) await loadProjects(); } },
        to.node, field('New name', newName, 'Optional.'),
        el('button', { type: 'submit', class: 'btn start', text: 'Move project' }))),
    el('section', { class: 'card dangerzone' }, el('h2', { text: 'Delete' }),
      el('p', { class: 'quiet small', text: 'Both show exactly what would be removed and ask before deleting.' }),
      el('div', { class: 'row' }, btn('Delete saved conversations', 'btn danger', () => forget({ kind: 'captured', project: p.id }, reload), `Delete saved conversations of ${p.name}`),
        btn('Delete the whole project', 'btn danger', () => forget({ kind: 'project', project: p.id }, async () => { await loadProjects(); gone(); }), `Delete project ${p.name}`)))));
}

// ---- Settings (every project) ---------------------------------------------------------------

const SETTING_TABS = [['folders', 'New folders'], ['claude', 'Claude Code'], ['summaries', 'Summaries'], ['cloud', 'Cloud storage'], ['research', 'Web research']];

async function settingsView(_, args) {
  let tab = (args && args[0]) || 'folders';
  if (!SETTING_TABS.some(t => t[0] === tab)) tab = 'folders';
  view.className = 'narrow';
  const box = el('div', { class: 'stack' });
  view.append(seg('Settings section', SETTING_TABS, tab, v => { location.hash = '#/settings/' + v; }, 'tabs'), box);
  await ({ folders: settingsFolders, claude: settingsClaude, summaries: settingsSummaries, cloud: settingsCloud, research: settingsResearch })[tab](box);
}

// What happens in folders that are not projects, and folders that are never saved.
async function settingsFolders(box) {
  const set = await api('GET', '/api/settings');
  if (!set.ok) return say(problem(set), 'bad');
  const a = set.data.auto_add;
  const mode = choice('mode', 'When Claude Code runs in a folder that is not a project', [
    ['ask', 'Remind me, and save nothing', 'Says once that the folder can be added.'],
    ['manual', 'Ignore it, and save nothing', 'Says nothing.'],
    ['auto', 'Add it as a project automatically and start saving', 'At the level chosen below. Exclude client or private folders first.'],
  ], a.mode);
  const autoLevel = select('auto-level', LEVELS.slice(0, 3).map(l => [l[0], l[1]]), a.level || 'conversation');
  box.append(card('', el('form', { onsubmit: e => { e.preventDefault(); return act('/api/settings/auto-add', { mode: mode.value(), level: mode.value() === 'auto' ? autoLevel.value : '' }, 'Saved.'); } },
    mode, field('For automatically added folders, save', autoLevel),
    el('button', { type: 'submit', class: 'primary start', text: 'Save this choice' }))));
  const ex = folderField('ex-path', 'Folder to never save', 'Nothing in it, or below it, is saved or added. This wins over everything else.');
  const never = set.data.exclusions || [];
  box.append(card('Folders that are never saved',
    never.length ? never.map(p => el('div', { class: 'never' }, el('span', { class: 'path', text: p }), btn('Remove from list', 'link quiet', () => act('/api/exclusions', { path: p, remove: true }, 'No longer excluded.'), `Stop excluding ${p}`)))
      : el('p', { class: 'quiet', text: 'No folders are excluded.' }),
    el('form', { onsubmit: e => { e.preventDefault(); return act('/api/exclusions', { path: ex.input.value }, 'Excluded.'); } },
      ex.node, el('button', { type: 'submit', class: 'btn start', text: 'Add folder' }))));
}

// Which Claude Code accounts have SessionKeep turned on. Claude Code takes a moment to answer.
async function settingsClaude(box) {
  const accounts = el('div', { 'aria-live': 'polite' }, working('Asking Claude Code where SessionKeep is turned on.'));
  box.append(el('section', { class: 'card flush' }, el('div', { class: 'cardhead' }, el('h2', { text: 'Claude Code accounts on this computer' }),
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
  await loadAccounts();
}

async function settingsSummaries(box) {
  const [sm, ac] = await Promise.all([api('GET', '/api/summaries'), api('GET', '/api/accounts')]);
  if (!sm.ok) return say(problem(sm), 'bad');
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
  box.append(card('', el('form', { onsubmit: async e => {
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

// The personal and the shared storage, saved once and used when a project is shared.
async function settingsCloud(box) {
  await loadTeam();
  const used = slot => Object.values(team.projects).filter(x => x.storage === slot).length;
  const own = Object.values(team.projects).filter(x => x.storage === 'custom').length;
  const storageCard = slot => {
    const s = team.storages[slot];
    const info = STORAGES[slot];
    const row = (label, value, mono) => el('div', { class: 'kv' }, el('span', { class: 'quiet', text: label }), el('span', { class: mono ? 'mono' : '', text: value }));
    const host = (s.endpoint || '').replace(/^[a-z]+:\/\//i, '').replace(/\/+$/, '');
    return el('section', { class: 'card storagecard' },
      el('div', { class: 'between' }, el('h2', { text: info.title }), s.set ? chip('ok', 'Set up') : chip('', 'Not set up')),
      el('p', { class: 'quiet small', text: info.what }),
      s.set ? el('div', { class: 'kvlist' },
        row('Bucket', s.bucket, true),
        row('Address', host, true),
        row('Used by', plural(used(slot), 'project', 'projects')),
        s.as ? row('Colleagues see you as', s.as) : null) : null,
      s.set && !s.saved ? el('p', { class: 'quiet small', text: 'Found in a project you shared earlier, and used from there. Change saves it here with its own keys.' }) : null,
      el('div', { class: 'row' }, btn(s.set ? 'Change' : 'Set up', s.set ? 'btn' : 'btn primary', () => cloudStorage(slot), `${s.set ? 'Change' : 'Set up'} the ${info.title.toLowerCase()}`),
        s.saved ? btn('Forget', 'btn', () => forgetStorage(slot), `Forget the ${info.title.toLowerCase()}`) : null));
  };
  box.append(el('p', { class: 'quiet small', text: 'Projects stay on this computer unless you turn sharing on for them, one by one, on each project’s Sharing page. A shared project goes to your personal storage unless you choose the shared storage or a bucket of its own; it is sent, encrypted, only when you press Sync now.' }),
    el('div', { class: 'pair' }, storageCard('personal'), storageCard('shared')),
    own ? el('p', { class: 'quiet small', text: `${plural(own, 'project uses', 'projects use')} a bucket of its own; it is kept with that project, not here.` }) : null);
}

// Whether SessionKeep may fetch a page to check a fact. Turning it on is a choice about the
// network, so what it means is said next to the switch.
async function settingsResearch(box) {
  const r = await api('GET', '/api/research/fetching');
  if (!r.ok) return say(problem(r), 'bad');
  const on = !!r.data.enabled;
  box.append(card('Web research',
    el('p', {}, on ? dotword('ok', 'Fetching pages is on') : dotword('off', 'Fetching pages is off')),
    el('p', { class: 'quiet small', text: 'When it is on, SessionKeep may open a page Claude cites, from this computer, to check the exact words before a fact is marked as checked, and again when you press Check the page again. The site sees your network address, as with any visit; no project text is sent. Nothing is fetched unless a command or a button asks.' }),
    el('div', { class: 'row' }, on ? btn('Turn fetching off', 'btn', () => act('/api/research/fetching', { on: false }, 'Fetching pages is off.'))
      : btn('Turn fetching on', 'btn primary', () => act('/api/research/fetching', { on: true }, 'Fetching pages is on.')))));
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

// ---- Routing: everything, or one project ----------------------------------------------------
//
// There are two levels and every page belongs to exactly one of them:
//   #/<page>                  about every project (no project is open)
//   #/p/<project id>/<page>   about that one project only

const GLOBAL_VIEWS = {
  overview: ['Overview', overviewAll, 'Where your work stands across every project, and what needs you.'],
  projects: ['Projects', projectsAll, 'SessionKeep saves only inside these folders, at the level you choose for each. Open a project to see everything about it.'],
  search: ['Search everything', () => searchView(), 'Find anything you or Claude said, in every project at once. To search one project, open it and use its Search.'],
  instructions: ['Instructions for all', (_, a) => instructionsView(a[0]), 'Claude receives these word for word at the start of every session, in every project. Instructions for one project are inside that project.'],
  settings: ['Settings', settingsView, 'These apply to every project. Settings for one project are in that project.'],
  health: ['Health', diagnostics, ''],
};
const PROJECT_VIEWS = {
  overview: ['Overview', projectHome, 'Where this project stands, at a glance.'],
  resume: ['Pick up work', p => resumeView(p.id), 'Where the work stood when you last stopped, and whether the code has moved since.'],
  recap: ['Recap', (p, a) => recapView(a[0]), ''],
  sessions: ['Sessions', (p, a) => historyView(p.id, a[0], a[1], a[2]), ''],
  search: ['Search', () => searchView(), ''],
  instructions: ['Instructions', (p, a) => instructionsView(a[0]), ''],
  research: ['Research', (p, a) => researchView(p.id, a[0]), 'Facts Claude looked up on the web for this project, each with the page and the exact words that back it.'],
  sharing: ['Sharing', sharingPage, 'Exchange this project’s memory with colleagues through a bucket you choose. Nothing leaves this computer until you press Sync now.'],
  settings: ['Project settings', projectSettings, ''],
};
const GLOBAL_MENU = ['overview', 'projects', 'search', 'instructions', 'settings', 'health'];
const PROJECT_MENU = ['overview', 'resume', 'recap', 'sessions', 'search', 'instructions', 'research', 'sharing', 'settings'];
const COUNTS = { overview: 'n-overview', health: 'n-health' };
let here = null; // the open project, or null when looking at every project

const iconBox = $('icons');
function icon(name) {
  const t = iconBox && iconBox.content && iconBox.content.querySelector(`[data-icon="${name}"]`);
  return t ? t.cloneNode(true) : null;
}

// Links from before the redesign still arrive (bookmarks, and pages that link the old way).
function translate(path) {
  const [n, ...a] = path.split('/');
  const recent = () => scope !== '*' ? scope : (projects[0] || {}).id;
  const into = (pid, page, more) => pid ? ['p', pid, page, ...more].join('/') : 'overview';
  switch (n) {
    case 'history': return a[0] ? into(a[0], 'sessions', a.slice(1)) : into(recent(), 'sessions', []);
    case 'resume': return into(a[0] || recent(), 'resume', []);
    case 'recap': return into(recent(), 'recap', a);
    case 'research': return a[0] ? into(a[0], 'research', a.slice(1)) : into(recent(), 'research', []);
    case 'projects': return a[0] ? into(a[0], 'settings', []) : null;
    case 'diagnostics': return 'health';
  }
  return null;
}

function paintSide(page) {
  const p = here;
  $('navhead').textContent = p ? 'This project' : 'Everything';
  $('sidetop').replaceChildren(...(p ? [
    el('a', { class: 'backlink', href: '#/overview' }, icon('back'), 'All projects'),
    el('div', { class: 'projcard' }, el('strong', { text: p.name }), el('span', { class: 'path', text: p.roots.join(', ') }),
      el('span', { class: 'row' }, dotword(...savingState(p).slice(0, 2)), team.projects[p.id] ? chip('accent', 'Shared') : null)),
  ] : []));
  const ids = p ? PROJECT_MENU : GLOBAL_MENU, table = p ? PROJECT_VIEWS : GLOBAL_VIEWS;
  $('nav').replaceChildren(...ids.map(id => el('li', {}, el('a', { href: p ? projectHref(p.id, id) : '#/' + id, 'aria-current': id === page ? 'page' : null },
    icon(id), table[id][0],
    id === 'search' ? el('kbd', { text: '/' }) : null,
    !p && COUNTS[id] ? el('span', { class: 'count', id: COUNTS[id] }) : null,
    p && id === 'research' ? el('span', { class: 'count', id: 'n-research' }) : null))));
  $('sideprojects').replaceChildren(...(p || !projects.length ? [] : [
    el('p', { class: 'navhead', text: 'Your projects' }),
    el('ul', { class: 'projlist' }, projects.map(x => { const [kind, word] = savingState(x);
      return el('li', {}, el('a', { href: projectHref(x.id) }, el('span', { class: 'pname', text: x.name }), dotword(kind, word))); })),
  ]));
}

async function show(target, force) {
  if (!projects.length) await loadProjects();
  let path = String(target || '');
  const moved = translate(path);
  if (moved !== null && moved !== path) { history.replaceState(null, '', '#/' + moved); path = moved; }
  const parts = path.split('/').filter(x => x !== '');
  let pid = null, name = parts[0] || 'overview', args = parts.slice(1);
  if (name === 'p') { pid = parts[1] || ''; name = parts[2] || 'overview'; args = parts.slice(3); }
  if (pid && !projects.some(p => p.id === pid)) await loadProjects();
  let gone = false;
  if (pid !== null && !projects.some(p => p.id === pid)) { gone = true; pid = null; name = 'overview'; args = []; history.replaceState(null, '', '#/overview'); }
  const table = pid ? PROJECT_VIEWS : GLOBAL_VIEWS;
  if (!table[name]) name = 'overview';
  const key = (pid || '*') + '/' + name;
  if (!force && key === current && selectors[name] && selectors[name](args)) return;
  const entering = (pid || '*') !== String(current).split('/')[0];
  current = key;
  selectors = {};
  here = pid ? projects.find(p => p.id === pid) : null;
  scope = pid || '*';
  if (entering && here) await loadTeam();
  const [label, render, about] = table[name];
  title.textContent = label;
  const tag = $('scopetag');
  tag.className = 'scopetag ' + (here ? 'one' : 'all');
  tag.textContent = here ? 'Only ' + here.name : 'All projects';
  crumb.replaceChildren(...(here ? [el('a', { href: '#/overview', text: 'All projects' }), ' / ', el('a', { href: projectHref(here.id), text: here.name }), ' / ' + label] : ['SessionKeep / ' + label]));
  lede.textContent = about || '';
  actions.replaceChildren();
  document.title = (here ? here.name + ': ' : '') + label + ' - SessionKeep';
  paintSide(name);
  $('shell').classList.remove('open');
  $('menu').setAttribute('aria-expanded', 'false');
  view.className = '';
  view.replaceChildren();
  say('Loading...');
  await render(here, args);
  if (gone) say('That project is no longer here. It may have been deleted.', 'bad');
  else if (notice.textContent === 'Loading...') say('');
}

window.addEventListener('hashchange', () => { show(route()); if (!selectors[String(current).split('/')[1]]) $('main').focus(); });

$('menu').addEventListener('click', () => $('menu').setAttribute('aria-expanded', String($('shell').classList.toggle('open'))));
$('theme').addEventListener('click', () => {
  const root = document.documentElement;
  const dark = root.dataset.theme ? root.dataset.theme === 'dark' : matchMedia('(prefers-color-scheme: dark)').matches;
  root.dataset.theme = dark ? 'light' : 'dark';
  $('theme').textContent = dark ? 'Switch to dark theme' : 'Switch to light theme';
});
if (matchMedia('(prefers-color-scheme: dark)').matches) $('theme').textContent = 'Switch to light theme';
// "/" opens Search: this project's inside a project, every project's otherwise.
document.addEventListener('keydown', e => {
  if (e.key !== '/' || e.ctrlKey || e.metaKey || e.altKey || /^(INPUT|TEXTAREA|SELECT)$/.test(e.target.tagName) || dialog.open) return;
  e.preventDefault();
  if (String(current).endsWith('/search')) { const q = $('q'); if (q) q.focus(); } else location.hash = here ? projectHref(here.id, 'search') : '#/search';
});

(async () => {
  const s = await api('GET', '/api/session');
  if (!s.ok) { title.textContent = 'Not signed in'; return say(problem(s), 'bad'); }
  csrf = s.data.csrf;
  $('version').textContent = s.data.version;
  await loadTeam();
  await show(route(), true);
  healthBadge();
  if (current !== '*/overview') { const st = await api('GET', '/api/status'); if (st.ok) attention(st.data); }
})();
