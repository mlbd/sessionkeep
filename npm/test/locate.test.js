'use strict';
const test = require('node:test');
const assert = require('node:assert');
const fs = require('fs');
const os = require('os');
const path = require('path');
const { target, bundled, installDir, installed, choose } = require('../lib/locate');
const { shouldRun, setupEnv } = require('../postinstall');

function tmp() { return fs.mkdtempSync(path.join(os.tmpdir(), 'sessionkeep-npm-')); }

test('platform names map to the folders the build script writes', () => {
  assert.strictEqual(target('win32', 'x64'), 'windows-amd64');
  assert.strictEqual(target('darwin', 'arm64'), 'darwin-arm64');
  assert.strictEqual(target('linux', 'x64'), 'linux-amd64');
  assert.strictEqual(target('freebsd', 'x64'), null);
  assert.strictEqual(target('linux', 'ia32'), null);
});

test('a package without a program for this computer says so instead of guessing', () => {
  const base = tmp();
  fs.mkdirSync(path.join(base, 'vendor', 'windows-amd64'), { recursive: true });
  fs.writeFileSync(path.join(base, 'vendor', 'windows-amd64', 'sessionkeep.exe'), '');
  assert.ok(bundled('win32', 'x64', base).endsWith('sessionkeep.exe'));
  assert.strictEqual(bundled('linux', 'x64', base), null);
  assert.strictEqual(choose(['status'], '1.0.0', 'linux', 'x64', {}, base), null);
});

test('the macOS and Linux programs are made executable before they are started', { skip: process.platform === 'win32' && 'Windows has no executable bit' }, () => {
  const base = tmp();
  const dir = path.join(base, 'vendor', 'linux-amd64');
  fs.mkdirSync(dir, { recursive: true });
  const p = path.join(dir, 'sessionkeep');
  fs.writeFileSync(p, '', { mode: 0o644 });
  fs.chmodSync(p, 0o644);
  assert.strictEqual(bundled('linux', 'x64', base), p);
  assert.strictEqual(fs.statSync(p).mode & 0o777, 0o755);
});

test('the install folder matches the Go program', () => {
  assert.strictEqual(installDir('win32', { LOCALAPPDATA: 'C:\\L' }), path.join('C:\\L', 'Programs', 'SessionKeep'));
  assert.strictEqual(installDir('win32', {}), null);
  assert.strictEqual(installDir('linux', { HOME: path.resolve('/h') }), path.join(path.resolve('/h'), '.local', 'lib', 'sessionkeep'));
  assert.strictEqual(installDir('darwin', { HOME: path.resolve('/h') }), path.join(path.resolve('/h'), 'Library', 'Application Support', 'SessionKeep', 'app'));
  assert.strictEqual(installDir('linux', { SESSIONKEEP_INSTALL_DIR: 'relative' }), null);
});

test('everyday commands use the installed copy only when it is the same version', () => {
  const base = tmp();
  const dir = tmp();
  fs.mkdirSync(path.join(base, 'vendor', 'windows-amd64'), { recursive: true });
  const own = path.join(base, 'vendor', 'windows-amd64', 'sessionkeep.exe');
  fs.writeFileSync(own, '');
  const bin = path.join(dir, 'sessionkeep.exe');
  fs.writeFileSync(bin, '');
  fs.writeFileSync(path.join(dir, 'install.json'), JSON.stringify({ version: '1.2.3', binary: bin }));
  const env = { SESSIONKEEP_INSTALL_DIR: dir };

  assert.strictEqual(installed('1.2.3', 'win32', env), bin);
  assert.strictEqual(choose(['status'], '1.2.3', 'win32', 'x64', env, base), bin);
  assert.strictEqual(choose(['status'], '1.2.4', 'win32', 'x64', env, base), own, 'an older installed copy must not be used');
  assert.strictEqual(choose(['install'], '1.2.3', 'win32', 'x64', env, base), own, 'install copies the bundled program');
  assert.strictEqual(choose(['uninstall'], '1.2.3', 'win32', 'x64', env, base), own, 'uninstall must not run from the folder it removes');

  fs.writeFileSync(path.join(dir, 'install.json'), '{ damaged');
  assert.strictEqual(choose(['status'], '1.2.3', 'win32', 'x64', env, base), own);
  fs.writeFileSync(path.join(dir, 'install.json'), JSON.stringify({ version: '1.2.3', binary: path.join(dir, 'gone.exe') }));
  assert.strictEqual(choose(['status'], '1.2.3', 'win32', 'x64', env, base), own);
});

test('setup runs only for a global install that was not told to skip it', () => {
  assert.strictEqual(shouldRun({ npm_config_global: 'true' }), true);
  assert.strictEqual(shouldRun({}), false);
  assert.strictEqual(shouldRun({ npm_config_global: 'false' }), false);
  assert.strictEqual(shouldRun({ npm_config_global: 'true', SESSIONKEEP_SKIP_SETUP: '1' }), false);
});

test('setup never looks up claude in a node_modules/.bin folder npm put on PATH (M7)', () => {
  const win = setupEnv({
    Path: 'C:\\node_modules\\.bin;D:\\proj\\node_modules\\.bin\\;C:\\Program Files\\nodejs;C:\\Users\\u\\.local\\bin;;C:\\NODE_MODULES\\.BIN',
    npm_config_global: 'true',
  }, 'win32');
  assert.strictEqual(win.Path, 'C:\\Program Files\\nodejs;C:\\Users\\u\\.local\\bin;');
  assert.strictEqual(win.npm_config_global, 'true');

  const unix = setupEnv({ PATH: '/home/u/node_modules/.bin:/usr/local/bin:/opt/node_modules/.bin/:/usr/bin', Path: '/x/node_modules/.bin' }, 'linux');
  assert.strictEqual(unix.PATH, '/usr/local/bin:/usr/bin');
  assert.strictEqual(unix.Path, '/x/node_modules/.bin', 'only PATH is a search path on Unix');

  // A folder that merely contains the words is kept.
  assert.strictEqual(setupEnv({ PATH: '/srv/node_modules/.bin/tools:/a/node_modules' }, 'linux').PATH, '/srv/node_modules/.bin/tools:/a/node_modules');
});
