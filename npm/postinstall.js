'use strict';
// Runs after `npm install -g`. It does what a person would otherwise type next:
//
//     sessionkeep install --enable-plugin
//
// which copies the program to its per-user folder, generates the Claude Code plugin and turns it
// on with Claude Code's own two plugin commands. It never fails the npm install: if setup cannot
// finish, the reason is kept and shown the next time `sessionkeep` is run.
//
// It does nothing for a non-global install (a project dependency, npx) or when
// SESSIONKEEP_SKIP_SETUP is set. `npm install --ignore-scripts` skips it entirely.
const fs = require('fs');
const { spawnSync } = require('child_process');
const { bundled, failedMarker } = require('./lib/locate');

function shouldRun(env) {
  return env.npm_config_global === 'true' && !env.SESSIONKEEP_SKIP_SETUP;
}

function main() {
  if (!shouldRun(process.env)) return;
  const program = bundled();
  let failure = '';
  if (!program) {
    failure = `This package has no program for ${process.platform} ${process.arch}.`;
  } else {
    const r = spawnSync(program, ['install', '--enable-plugin'], { encoding: 'utf8', windowsHide: true, timeout: 5 * 60 * 1000 });
    const out = `${r.stdout || ''}${r.stderr || ''}`.trim();
    if (out) console.log(out);
    if (r.error) failure = r.error.message;
    else if (r.status !== 0) failure = out.split(/\r?\n/).slice(-12).join('\n');
  }
  try {
    if (failure) fs.writeFileSync(failedMarker, failure + '\n');
    else fs.rmSync(failedMarker, { force: true });
  } catch (_) { /* a read-only package folder: the message above is all there is */ }
  if (failure) console.log('\nSessionKeep setup did not finish. Run: sessionkeep install --enable-plugin');
}

if (require.main === module) main();
module.exports = { shouldRun };
