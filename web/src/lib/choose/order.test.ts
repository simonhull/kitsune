import { describe, expect, it } from 'vitest';
import { keepOrder } from './order.ts';
import type { ImageCandidate } from './types.ts';

const c = (id: string, chosen = false): ImageCandidate => ({
	id,
	source: 'fanart',
	width: 0,
	height: 0,
	chosen,
	pinned: chosen
});

describe('keepOrder', () => {
	it('keeps the tiles where the person sees them when a pin moves to the front', () => {
		const shown = [c('a', true), c('b'), c('c')];
		const next = [c('b', true), c('a'), c('c')];
		expect(keepOrder(shown, next).map(({ id }) => id)).toEqual(['a', 'b', 'c']);
		expect(keepOrder(shown, next)[1]?.chosen).toBe(true);
	});

	it('puts new ones after, in the server’s order, and drops gone ones', () => {
		const shown = [c('a'), c('b')];
		expect(keepOrder(shown, [c('upload:1'), c('b'), c('x')]).map(({ id }) => id)).toEqual([
			'b',
			'upload:1',
			'x'
		]);
	});
});
