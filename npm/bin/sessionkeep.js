#!/usr/bin/env node
'use strict';
// Starts the SessionKeep program with the same arguments, input and output, and returns its exit code.
const fs = require('fs');
const { spawnSync } = require('child_process');
const { choose, installed, failedMarker } = require('../lib/locate');
const { version } = require('../package.json');

const args = process.argv.slice(2);
const program = choose(args, version);
if (!program) {
  console.error(`sessionkeep: this package has no program for ${process.platform} ${process.arch}.`);
  process.exit(69);
}

if (args[0] !== 'install' && fs.existsSync(failedMarker)) {
  let why = '';
  try { why = fs.readFileSync(failedMarker, 'utf8').trim(); } catch (_) { /* ignore */ }
  console.error(`sessionkeep: setup did not finish when this package was installed.\n${why}\nRun: sessionkeep install --enable-plugin\n`);
} else if (!['install', 'uninstall', 'version', 'help', undefined].includes(args[0]) && !installed(version)) {
  // npm was told not to run install scripts, or an older copy is still installed.
  console.error(`sessionkeep: version ${version} is not set up yet. Run: sessionkeep install --enable-plugin\n`);
}

const r = spawnSync(program, args, { stdio: 'inherit', windowsHide: true });
if (r.error) {
  console.error(`sessionkeep: could not start ${program}: ${r.error.message}`);
  process.exit(69);
}
if (args[0] === 'install' && r.status === 0) {
  try { fs.rmSync(failedMarker, { force: true }); } catch (_) { /* ignore */ }
}
process.exit(r.status === null ? 1 : r.status);
