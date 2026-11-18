/**
 * Splits the suite's files into shards of about equal time, so CI can run it on several runners at
 * once, each still with one worker. Playwright's own `--shard` splits by test count, and these tests
 * run from 3 s to 2 minutes, so its shards came out two to four times apart.
 *
 * `KITSUNE_PERF_SHARD=i/n` runs shard `i` of `n`: every shard sorts the files by `seconds`, longest
 * first, and gives each to the shard with the least so far, so they all agree on the split and every
 * file runs in exactly one. A file missing from `seconds` counts as `unknownSeconds` and still runs.
 */
import { readdirSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

/**
 * About how long each file takes on CI on the 50k library, from a run's log. They only steer the
 * split: refresh them when the shards drift apart.
 */
const seconds: Record<string, number> = {
	'album.perf.ts': 74,
	'artist.perf.ts': 116,
	'columns.perf.ts': 82,
	'continues.perf.ts': 127,
	'cover.perf.ts': 23,
	'filePlaylists.perf.ts': 48,
	'home.perf.ts': 71,
	'library.perf.ts': 120,
	'organize.perf.ts': 100,
	'playlists.perf.ts': 45,
	'resume.perf.ts': 6,
	'search.perf.ts': 4,
	'shuffle.perf.ts': 21,
	'smartPlaylists.perf.ts': 30,
	'trash.perf.ts': 24
};
const unknownSeconds = 60;

/** One shard of the suite: `index` of `total`, from 1. */
export interface Shard {
	index: number;
	total: number;
}

/** The shard `KITSUNE_PERF_SHARD` names, or undefined when the whole suite runs. */
export const shard: Shard | undefined = parseShard(process.env['KITSUNE_PERF_SHARD']);

function parseShard(value: string | undefined): Shard | undefined {
	if (value === undefined || value === '') {
		return undefined;
	}
	const [index, total] = value.split('/').map(Number);
	if (
		index === undefined ||
		total === undefined ||
		!Number.isInteger(index) ||
		!Number.isInteger(total) ||
		index < 1 ||
		index > total
	) {
		throw new Error(`KITSUNE_PERF_SHARD must be i/n with 1 ≤ i ≤ n, not ${value}`);
	}
	return { index, total };
}

/** The suite's files in `shard`, in name order, as Playwright would run them unsharded. */
export function shardFiles({ index, total }: Shard): string[] {
	const dir = fileURLToPath(new URL('.', import.meta.url));
	const files = readdirSync(dir).filter((name) => name.endsWith('.perf.ts'));
	const weight = (file: string) => seconds[file] ?? unknownSeconds;
	const sorted = [...files].sort((a, b) => weight(b) - weight(a) || (a < b ? -1 : 1));
	const loads = Array.from({ length: total }, () => 0);
	const mine: string[] = [];
	for (const file of sorted) {
		const least = loads.indexOf(Math.min(...loads));
		loads[least] = (loads[least] ?? 0) + weight(file);
		if (least === index - 1) {
			mine.push(file);
		}
	}
	return mine.sort();
}
