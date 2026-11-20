// Generated from schema/*.schema.json by packages/types/generate.mjs; a drift
// check in CI regenerates and diffs. Do not edit by hand.
export type Tier = 'report' | 'info';
export type PageStatus = 'ok' | 'diverged' | 'failed' | 'unreachable';

export interface Box { baseline: [number, number, number, number] | null; engine: [number, number, number, number] | null; }
export interface Delta { dw: number; dh: number; dx: number; dy: number; dln: number; }
export interface Evidence {
  mechanism: string | null;
  consensus: 'baseline-odd' | 'engine-odd' | 'all-differ' | null;
  perception: number | null;
  ink: number | null;
  intent: string[] | null;
  calibration?: { explained: boolean; by: Array<{ model: string; expected?: number; tol?: number; dw?: number; dh?: number }> };
}
export interface Cause {
  id: string;
  tier: Tier;
  title: string;
  kinds: string[];
  consequences: string[];
  selector: string;
  boxes: Box;
  delta: Delta;
  kb: string | null;
  kbDetail?: { id: string; title: string; explanation: string; fix: string };
  evidence: Evidence;
  score: number | null;
  count: number;
  instances: string[];
}
export interface Result {
  host: { os: string; arch: string; id: string };
  page: { slug: string; url: string };
  baseline: { engine: string; version: string };
  engine: { engine: string; family: string; version: string; profile: string };
  status: PageStatus;
  warnings: string[];
  stats: { matchRate: number; report: number; info: number; ms: number };
  shots?: { baseline: { src: string; w: number; h: number }; engine: { src: string; w: number; h: number } };
  findings?: string;
  causes: Cause[];
  intent: Array<{ engine: 'baseline' | 'engine'; selector: string; relations: string[]; px: number; keptBy: string[] }>;
  blocked: { baseline: string[]; engine: string[] };
  dynamic: { baseline: string[]; engine: string[] };
  audit?: { baseline: Array<{ check: string; selector: string; box: number[]; severity: string }>; engine: Array<{ check: string; selector: string; box: number[]; severity: string }> };
}
export interface Summary {
  $schema: string;
  tool: { name: 'omnisnap'; version: string };
  generated: string;
  job: Record<string, unknown>;
  hosts: Array<{ machine: { os: string; arch: string; id: string }; engines: unknown[] }>;
  skipped: Array<{ host?: string; engine: string; reason: string }>;
  pages: Array<{ slug: string; url: string; report: number; info: number }>;
  results: Result[];
}
export interface RunOptions {
  pages: Array<string | { url: string; slug?: string }>;
  engines?: string[];
  baseline?: string;
  runs?: number;
  out?: string;
  viewport?: { width: number; height: number; deviceScaleFactor?: number };
  timeouts?: { navigationMs?: number; scriptMs?: number };
  probes?: boolean;
  fontParity?: boolean;
  /** Do not treat a page under 50 elements as a bot wall (planted and test pages). */
  allowSmallPages?: boolean;
  cwd?: string;
  onProgress?: (event: unknown) => void;
}
export class OmnisnapError extends Error { code: number | null; stderr: string; args: string[]; }
export function run(options: RunOptions): Promise<Summary>;
export function browsers(): Promise<Array<{ id: string; name: string; family: string; protocol: string; executable: string; version: string; source: string; notes?: string[] }>>;
export function merge(bundles: string[], out: string, options?: { onProgress?: (event: unknown) => void }): Promise<Summary>;
export function report(bundle: string): Promise<string>;
export function resolveBinary(): string;
export function runBinary(args: string[], options?: { onProgress?: (event: unknown) => void; cwd?: string; env?: Record<string, string> }): Promise<{ code: number; stdout: string; stderr: string; done: unknown }>;
