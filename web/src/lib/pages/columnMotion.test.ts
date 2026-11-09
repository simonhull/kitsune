import { readFileSync } from 'node:fs';
import { describe, expect, it } from 'vitest';
import { columnSpecs } from './columns.ts';
import { motionIds, motionRule } from './columnMotion.ts';

// Read as a file: Vitest hands a stylesheet's import back empty.
const motion = readFileSync(new URL('columnMotion.css', import.meta.url), 'utf8');
const ids = ['number', 'title', ...columnSpecs.map(({ id }) => id)];

describe('column motion', () => {
	it('has a rule for every column a list places', () => {
		expect([...motionIds].sort()).toEqual([...ids].sort());
	});

	it.each(ids)('moves %s by its own rule, and holds it above the lane', (id) => {
		expect(motionRule(id as never)).toBe(
			`.song-list[data-moving] [data-col='${id}'] { translate: 0px 0; transition-delay: 0ms; }`
		);
		expect(motion).toContain(`.song-list[data-held='${id}'] [data-col='${id}'] {`);
		expect(motion).toContain(`[data-phase='drag'][data-held='${id}'] [data-col='${id}']`);
	});
});
