#!/usr/bin/env node
// The `omnisnap` bin shim: runs the platform binary with the same arguments,
// so npm users get the CLI and the API from one package.
import { spawn } from 'node:child_process';
import { resolveBinary } from '../index.js';

const child = spawn(resolveBinary(), process.argv.slice(2), { stdio: 'inherit' });
child.on('exit', (code, signal) => {
  if (signal) process.kill(process.pid, signal);
  else process.exit(code ?? 1);
});
