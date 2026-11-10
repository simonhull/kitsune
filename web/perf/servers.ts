/**
 * The libraries the suite runs on, and the servers that serve them. Everything lives under the perf
 * directory (`KITSUNE_PERF_DIR`, by default a folder in the system temp directory), never in the
 * repository.
 */
import { tmpdir } from 'node:os';
import { join } from 'node:path';
import type { Shape } from './library.ts';

/** A library size the suite measures. Each is a Playwright project with a server of its own. */
export interface Library {
	name: string;
	songs: number;
	/** The albums whose first two songs are real noise: every one the suite plays is one of them. */
	loudAlbums: number;
	/** The prolific artist an artist page is timed on. */
	prolific: Shape['prolific'];
	port: number;
}

/**
 * Where the servers listen: from `KITSUNE_PERF_PORT` (default 14610), one port a library, so runs
 * from two worktrees at once don't answer each other's requests.
 */
const firstPort = Number(process.env['KITSUNE_PERF_PORT'] ?? '14610');

const allLibraries: Library[] = [
	{
		name: '1k',
		songs: 1000,
		loudAlbums: 60,
		prolific: { albums: 5, perAlbum: 16, guests: 5 },
		port: firstPort
	},
	{
		name: '50k',
		songs: 50_000,
		loudAlbums: 60,
		prolific: { albums: 40, perAlbum: 20, guests: 25 },
		port: firstPort + 1
	}
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
