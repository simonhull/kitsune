/**
 * Turns the samples the tests wrote into one table: median and p95 per interaction and library,
 * against its budget. It goes to the console, to `results/table.md`, and on CI to the job summary.
 */
import { appendFileSync, existsSync, readdirSync, readFileSync, writeFileSync } from 'node:fs';
import { join } from 'node:path';
import { budgets, multiplier, percentile } from './budgets.ts';
import { libraries, resultsDir } from './servers.ts';

/** One interaction measured on one library. */
export interface Result {
	id: string;
	library: string;
	samples: number[];
}

export function resultPath(library: string, id: string): string {
	return join(resultsDir, `${library}--${id}.json`);
}

export function writeResult(result: Result): void {
	writeFileSync(resultPath(result.library, result.id), JSON.stringify(result));
}

function readResults(): Map<string, Result> {
	const results = new Map<string, Result>();
	if (!existsSync(resultsDir)) {
		return results;
	}
	for (const file of readdirSync(resultsDir).filter((name) => name.endsWith('.json'))) {
		const result = JSON.parse(readFileSync(join(resultsDir, file), 'utf8')) as Result;
		results.set(`${result.library}--${result.id}`, result);
	}
	return results;
}

const ms = (value: number) => (Number.isNaN(value) ? '–' : value.toFixed(1));

export function writeReport(): void {
	const results = readResults();
	const lines = [
		`| Interaction | ${libraries.map((l) => `${l.name} median | ${l.name} p95`).join(' | ')} | Budget (p95) |`,
		`|---|${libraries.map(() => '--:|--:').join('|')}|--:|`
	];
	for (const [id, budget] of Object.entries(budgets.browser)) {
		const cells = libraries.map((library) => {
			const samples = results.get(`${library.name}--${id}`)?.samples ?? [];
			const p95 = percentile(samples, 95);
			const over = p95 > budget.p95Ms * multiplier ? ' ✗' : '';
			return `${ms(percentile(samples, 50))} | ${ms(p95)}${over}`;
		});
		lines.push(`| ${budget.interaction} | ${cells.join(' | ')} | ${String(budget.p95Ms)} ms |`);
	}
	const scale = multiplier === 1 ? '' : ` Budgets are multiplied by ${String(multiplier)} here.`;
	const table = `### Browser, ms (20 runs each)\n\n${lines.join('\n')}\n\n${scale}\n`;
	writeFileSync(join(resultsDir, 'table.md'), table);
	console.log(`\n${table}`);
	const summary = process.env['GITHUB_STEP_SUMMARY'];
	if (summary) {
		appendFileSync(summary, `${table}\n`);
	}
}
