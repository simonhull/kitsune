/**
 * The suite's test: a signed-in page with the probe installed, and `measure`, which times one
 * interaction and keeps the sample. Each test asserts its p95 against its budget at the end.
 */
import { test as base, expect, type Page } from '@playwright/test';
import { readFileSync } from 'node:fs';
import { join } from 'node:path';
import { budgets, multiplier, percentile } from './budgets.ts';
import type { Manifest } from './library.ts';
import { armOnLoadKey, installProbe, type InputType, type Wait } from './probe.ts';
import { writeResult } from './report.ts';
import { libraries, libraryDir, user } from './servers.ts';

/** How many times each interaction is timed. */
export const runs = 20;
/** How long anything measured may take before the run counts as broken, not slow. */
const brokenMs = 10_000;
/** Prints every sample as it's taken. */
const verbose = process.env['KITSUNE_PERF_VERBOSE'] !== undefined;

/** One interaction's samples, kept as the test goes and checked against its budget at the end. */
export class Samples {
	readonly id: string;
	readonly values: number[] = [];

	constructor(id: string) {
		if (budgets.browser[id] === undefined) {
			throw new Error(`no budget for ${id} in perf/budgets.json`);
		}
		this.id = id;
	}

	add(value: number): void {
		this.values.push(value);
	}
}

export interface Perf {
	library: string;
	manifest: Manifest;
	/** Starts waiting for `wait`, runs `act`, and returns the ms from `input` to `wait`. */
	time(
		samples: Samples,
		input: InputType,
		wait: Wait,
		act: () => Promise<unknown>
	): Promise<number>;
	/** Loads `path` as a new page, timing from the start of the load to `wait`. */
	load(samples: Samples, path: string, wait: Wait): Promise<number>;
	samples(id: string): Samples;
}

async function signIn(page: Page): Promise<void> {
	const response = await page.request.post('/api/v1/auth/login', {
		data: { name: user.name, password: user.password, cookie: true }
	});
	expect(response.status()).toBe(200);
}

export const test = base.extend<{ perf: Perf }>({
	perf: async ({ page }, use, info) => {
		const library = libraries.find(({ name }) => name === info.project.name);
		if (library === undefined) {
			throw new Error(`no library for project ${info.project.name}`);
		}
		const manifest = JSON.parse(
			readFileSync(join(libraryDir(library), 'music', 'manifest.json'), 'utf8')
		) as Manifest;
		await page.addInitScript(installProbe);
		await signIn(page);
		const kept: Samples[] = [];
		await use({
			library: library.name,
			manifest,
			async time(samples, input, wait, act) {
				await page.evaluate((w) => {
					window.perf.arm(w);
				}, wait);
				await act();
				const value = await page.evaluate(([type, timeout]) => window.perf.settle(type, timeout), [
					input,
					brokenMs
				] as const);
				samples.add(value);
				if (verbose) {
					console.log(`${library.name} ${samples.id}: ${value.toFixed(1)} ms`);
				}
				return value;
			},
			async load(samples, path, wait) {
				await page.evaluate(
					([key, w]) => {
						sessionStorage.setItem(key, JSON.stringify(w));
					},
					[armOnLoadKey, wait] as const
				);
				await page.goto(path);
				const value = await page.evaluate(
					(timeout) => window.perf.settle('navigation', timeout),
					brokenMs
				);
				samples.add(value);
				return value;
			},
			samples(id) {
				const samples = new Samples(id);
				kept.push(samples);
				return samples;
			}
		});
		for (const samples of kept) {
			writeResult({ id: samples.id, library: library.name, samples: samples.values });
		}
		for (const samples of kept) {
			const budget = (budgets.browser[samples.id]?.p95Ms ?? 0) * multiplier;
			const p95 = percentile(samples.values, 95);
			expect
				.soft(p95, `${samples.id} p95 on ${library.name}: ${p95.toFixed(1)} ms`)
				.toBeLessThanOrEqual(budget);
		}
	}
});

export { expect };
