// The npm wrapper in a fresh project, as a user gets it: run() over the
// planted pages (Task 83). Copied into the fresh project and run there, so
// `import 'omnisnap'` resolves from its node_modules exactly as a user's
// code does (the package is ESM-only: its exports have no require entry):
//   cp test/wrappers/run-planted.mjs <project>/ && cd <project>
//   node run-planted.mjs <planted-dir> <base-url> <out> <engines>
import { readdirSync } from 'node:fs';
import { run, resolveBinary } from 'omnisnap';

const [planted, base, out, engines] = process.argv.slice(2);
if (process.env.OMNISNAP_BINARY) throw new Error('OMNISNAP_BINARY is set: this checks the installed platform package');
const pages = readdirSync(planted).filter((f) => /^\d\d-.*\.html$/.test(f)).sort()
  .map((f) => `${base}/${f.replace(/\.html$/, '')}`);
console.log(`binary: ${resolveBinary()}`);
const summary = await run({ pages, engines: engines.split(','), out, allowSmallPages: true });
console.log(`${summary.results.length} results in ${out}`);
