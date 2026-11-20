'use strict';
// Finds the SessionKeep program for this computer. No dependencies; nothing here uses the network.
const fs = require('fs');
const path = require('path');

const root = path.join(__dirname, '..');

// Folder name inside vendor/ for a Node platform and CPU, or null when SessionKeep is not built for it.
function target(platform, arch) {
  const os = { win32: 'windows', linux: 'linux', darwin: 'darwin' }[platform];
  const cpu = { x64: 'amd64', arm64: 'arm64' }[arch];
  return os && cpu ? `${os}-${cpu}` : null;
}

function binaryName(platform) {
  return platform === 'win32' ? 'sessionkeep.exe' : 'sessionkeep';
}

// The program shipped inside this package, or null when this package has none for this computer.
function bundled(platform = process.platform, arch = process.arch, base = root) {
  const t = target(platform, arch);
  if (!t) return null;
  const p = path.join(base, 'vendor', t, binaryName(platform));
  return fs.existsSync(p) ? p : null;
}

// Where `sessionkeep install` puts the program. Mirrors internal/install.Dir in the Go code.
function installDir(platform = process.platform, env = process.env) {
  const override = env.SESSIONKEEP_INSTALL_DIR;
  if (override) return path.isAbsolute(override) ? override : null;
  if (platform === 'win32') {
    return env.LOCALAPPDATA && path.isAbsolute(env.LOCALAPPDATA) ? path.join(env.LOCALAPPDATA, 'Programs', 'SessionKeep') : null;
  }
  if (!env.HOME || !path.isAbsolute(env.HOME)) return null;
  return platform === 'darwin'
    ? path.join(env.HOME, 'Library', 'Application Support', 'SessionKeep', 'app')
    : path.join(env.HOME, '.local', 'lib', 'sessionkeep');
}

// The installed copy, but only when it is the same version as this package. Using it for
// everyday commands keeps this package's own files free, so npm can replace them during an
// update even while the dashboard is running.
function installed(version, platform = process.platform, env = process.env) {
  const dir = installDir(platform, env);
  if (!dir) return null;
  try {
    const rec = JSON.parse(fs.readFileSync(path.join(dir, 'install.json'), 'utf8'));
    if (rec.version === version && typeof rec.binary === 'string' && fs.existsSync(rec.binary)) return rec.binary;
  } catch (_) { /* not installed, or unreadable: fall back to the bundled program */ }
  return null;
}

// Which program a command should run. `install` and `uninstall` always run the bundled one:
// install copies it into place, and uninstall must not run from the folder it removes.
function choose(args, version, platform = process.platform, arch = process.arch, env = process.env, base = root) {
  const own = bundled(platform, arch, base);
  if (args[0] === 'install' || args[0] === 'uninstall') return own;
  return installed(version, platform, env) || own;
}

const failedMarker = path.join(root, 'setup-failed.txt');

module.exports = { root, target, binaryName, bundled, installDir, installed, choose, failedMarker };
