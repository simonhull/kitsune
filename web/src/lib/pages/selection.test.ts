import { describe, expect, it } from 'vitest';
import { Selection } from './selection.svelte.ts';

/** Two albums' worth of songs, in the list's order. */
const order = ['a1', 'a2', 'a3', 'b1', 'b2', 'b3'];

describe('Selection', () => {
	it('toggles a song, and anchors there', () => {
		const picked = new Selection();
		picked.toggle('a2');
		expect([...picked.ids]).toEqual(['a2']);
		expect(picked.anchor).toBe('a2');
		picked.toggle('a2');
		expect(picked.any).toBe(false);
	});

	it('takes a range across albums from the anchor, either way', () => {
		const picked = new Selection();
		picked.toggle('a2');
		picked.extend('b2', order);
		expect(picked.inOrder(order)).toEqual(['a2', 'a3', 'b1', 'b2']);
		const back = new Selection();
		back.toggle('b3');
		back.extend('a3', order);
		expect(back.inOrder(order)).toEqual(['a3', 'b1', 'b2', 'b3']);
	});

	it('adds a range to what is picked', () => {
		const picked = new Selection();
		picked.toggle('a1');
		picked.toggle('b1');
		picked.extend('b3', order);
		expect(picked.inOrder(order)).toEqual(['a1', 'b1', 'b2', 'b3']);
	});

	it('takes a song alone when there is no anchor to range from', () => {
		const picked = new Selection();
		picked.extend('b1', order);
		expect([...picked.ids]).toEqual(['b1']);
	});

	it('ranges over a thousand rows', () => {
		const many = Array.from({ length: 5000 }, (_, index) => `t${String(index)}`);
		const picked = new Selection();
		picked.toggle('t10');
		picked.extend('t1209', many);
		expect(picked.size).toBe(1200);
	});

	it('selects all the filter shows, and counts what it hides', () => {
		const picked = new Selection();
		picked.toggle('a1');
		const shown = ['a2', 'b2'];
		expect(picked.boxFor(shown)).toBe('none');
		picked.toggleAll(shown);
		expect(picked.boxFor(shown)).toBe('all');
		expect(picked.inOrder(['a2', 'b2'])).toEqual(['a2', 'b2', 'a1']);
		expect(picked.hiddenFrom(new Set(shown))).toBe(1);
		picked.toggleAll(shown);
		expect([...picked.ids]).toEqual(['a1']);
	});

	it('shows some when part of what is shown is picked', () => {
		const picked = new Selection();
		picked.toggle('a1');
		expect(picked.boxFor(['a1', 'a2'])).toBe('some');
	});

	it('extends with the arrows, keeping the anchor', () => {
		const picked = new Selection();
		picked.toggle('a1');
		picked.add('a2');
		picked.add('a3');
		expect(picked.anchor).toBe('a1');
		expect(picked.size).toBe(3);
	});

	it('clears', () => {
		const picked = new Selection();
		picked.only('b2');
		picked.clear();
		expect(picked.any).toBe(false);
		expect(picked.anchor).toBeNull();
	});
});
