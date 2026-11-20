#!/usr/bin/env node
// The `omnisnap` bin shim: runs the platform binary with the same arguments,
// so npm users get the CLI and the API from one package.
import { spawn } from 'node:child_process';
import { resolveBinary } from '../index.js';

const binary = resolveBinary();
const child = spawn(binary, process.argv.slice(2), { stdio: 'inherit' });
// A missing binary is an 'error' event, not an exit: without a handler node
// dies with an unhandled-error stack trace.
child.on('error', (err) => {
  process.stderr.write(`omnisnap: cannot run ${binary}: ${err.message}\n` +
    `install the platform package for ${process.platform}-${process.arch} (@omnisnap/${process.platform}-${process.arch}), or set OMNISNAP_BINARY\n`);
  process.exit(127);
});
child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
