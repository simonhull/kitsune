import { describe, expect, it } from 'vitest';
import { pick, recordOrigins, type Candidate, type Origins } from './pairs';

interface Named extends Candidate {
	name: string;
}

const cover = 'cover:a';
const title = 'title:a';

const tile: Named = { id: cover, slot: 'card', name: 'tile' };
const repeat: Named = { id: cover, slot: 'card', name: 'repeat' };
const island: Named = { id: cover, slot: 'island', name: 'island' };
const repeatTitle: Named = { id: title, slot: 'card', name: 'repeat title' };
const other: Named = { id: 'cover:b', slot: 'card', name: 'other' };

const page = [tile, repeat, island, repeatTitle, other];

describe('pick', () => {
	it('resolves exactly one candidate per id', () => {
		const picked = pick(page, { ids: [cover, title], slots: ['card'] });
		expect(picked.size).toBe(2);
		expect(picked.get(cover)).toBe(tile);
		expect(picked.get(title)).toBe(repeatTitle);
	});

	it('lets the clicked element win', () => {
		const picked = pick(page, {
			ids: [cover],
			slots: ['card'],
			clicked: (candidate) => candidate === repeat
		});
		expect(picked.get(cover)).toBe(repeat);
	});

	it('prefers the clicked element to the origin', () => {
		const origins: Origins = new Map([[cover, { slot: 'card', ordinal: 0 }]]);
		const picked = pick(page, {
			ids: [cover],
			slots: ['card'],
			origins,
			clicked: (candidate) => candidate === repeat
		});
		expect(picked.get(cover)).toBe(repeat);
	});

	it('returns to the card the journey started from', () => {
		const origins: Origins = new Map([[cover, { slot: 'card', ordinal: 1 }]]);
		expect(pick(page, { ids: [cover], slots: ['card'], origins }).get(cover)).toBe(repeat);
	});

	it('prefers what is on screen when there is no click or origin', () => {
		const picked = pick(page, {
			ids: [cover],
			slots: ['card'],
			visible: (candidate) => candidate === repeat
		});
		expect(picked.get(cover)).toBe(repeat);
	});

	it('falls back to the first candidate when nothing is on screen', () => {
		expect(pick(page, { ids: [cover], slots: ['card'], visible: () => false }).get(cover)).toBe(
			tile
		);
	});

	it('takes slots in order', () => {
		expect(pick(page, { ids: [cover], slots: ['card', 'island'] }).get(cover)).toBe(tile);
	});

	it('falls through to the island when there is no card', () => {
		expect(pick([island], { ids: [cover], slots: ['card', 'island'] }).get(cover)).toBe(island);
	});

	it('passes over what cannot travel, to the next slot', () => {
		const hero: Named = { id: cover, slot: 'hero', name: 'hero' };
		const picked = pick([hero, island], {
			ids: [cover],
			slots: ['hero', 'island'],
			eligible: (candidate) => candidate !== hero
		});
		expect(picked.get(cover)).toBe(island);
	});

	it('finds nothing for an album that is neither on the page nor playing', () => {
		expect(pick([other], { ids: [cover], slots: ['card', 'island'] }).size).toBe(0);
	});

	it('does not pair a title with a card that never had one', () => {
		const origins: Origins = new Map([[title, { slot: 'card', ordinal: null }]]);
		expect(pick(page, { ids: [title], slots: ['card'], origins }).size).toBe(0);
	});

	it('survives an origin that no longer exists', () => {
		const origins: Origins = new Map([[cover, { slot: 'card', ordinal: 9 }]]);
		expect(pick(page, { ids: [cover], slots: ['card'], origins }).get(cover)).toBe(tile);
	});
});

describe('recordOrigins', () => {
	it('remembers the ordinal among siblings, and the absence of a title', () => {
		const origins: Origins = new Map();
		const sources = new Map([[cover, repeat]]);
		recordOrigins(page, [cover, title], 'card', sources, origins);
		expect(origins.get(cover)).toEqual({ slot: 'card', ordinal: 1 });
		expect(origins.get(title)).toEqual({ slot: 'card', ordinal: null });
	});
});
