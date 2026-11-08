/**
 * The budgets, from `perf/budgets.json` at the repository root, the one file that holds them, and
 * the arithmetic every number in the suite goes through.
 */
import { readFileSync } from 'node:fs';
import { fileURLToPath } from 'node:url';

export interface Budget {
	interaction: string;
	p95Ms: number;
}

export interface Budgets {
	ciMultiplier: number;
	browser: Record<string, Budget>;
	server: Record<string, Budget>;
}

export const budgets = JSON.parse(
	readFileSync(fileURLToPath(new URL('../../perf/budgets.json', import.meta.url)), 'utf8')
) as Budgets;

/**
 * What every budget is multiplied by here: 1 on a developer's machine, `ciMultiplier` on CI, where
 * runners are slower and shared. `KITSUNE_PERF_MULTIPLIER` overrides both.
 */
export const multiplier = Number(
	process.env['KITSUNE_PERF_MULTIPLIER'] ?? (process.env['CI'] ? budgets.ciMultiplier : 1)
);

/** The nearest-rank percentile: a value that was measured, never one interpolated between two. */
export function percentile(samples: readonly number[], p: number): number {
	const sorted = [...samples].sort((a, b) => a - b);
	const rank = Math.max(1, Math.ceil((p / 100) * sorted.length));
	return sorted[rank - 1] ?? Number.NaN;
}
