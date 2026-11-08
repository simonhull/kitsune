/**
 * The libraries the suite runs on, and the servers that serve them. Everything lives under the perf
 * directory (`KITSUNE_PERF_DIR`, by default a folder in the system temp directory), never in the
 * repository.
 */
import { tmpdir } from 'node:os';
import { join } from 'node:path';

/** A library size the suite measures. Each is a Playwright project with a server of its own. */
export interface Library {
	name: string;
	songs: number;
	/** The albums whose first two songs are real noise: every one the suite plays is one of them. */
	loudAlbums: number;
	port: number;
}

const allLibraries: Library[] = [
	{ name: '1k', songs: 1000, loudAlbums: 60, port: 14610 },
	{ name: '50k', songs: 50_000, loudAlbums: 60, port: 14611 }
];

/** The libraries this run measures: all of them, or those `KITSUNE_PERF_LIBRARIES` names (`1k,50k`). */
export const libraries = allLibraries.filter(
	({ name }) => process.env['KITSUNE_PERF_LIBRARIES']?.split(',').includes(name) ?? true
);

export const perfDir = process.env['KITSUNE_PERF_DIR'] ?? join(tmpdir(), 'kitsune-perf');

/** The release build the suite runs: `scripts/perf.sh` builds it there. */
export const serverBinary = process.env['KITSUNE_PERF_SERVER'] ?? join(perfDir, 'kitsune-server');

export const user = { name: 'perf', password: 'perf-suite-password' };

export function libraryDir(library: Library): string {
	return join(perfDir, library.name);
}

export const resultsDir = join(perfDir, 'results');
