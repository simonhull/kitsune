import { describe, expect, it } from 'vitest';
import { choreographyFor } from './choreography';
import { pick, type Candidate, type Origins } from './pairs';
import { albumExit } from './path';

const album = '/album/glasshouse-66';

describe('albumExit', () => {
	it('goes back when the album is right behind Now playing', () => {
		expect(albumExit(album, album)).toBe('back');
	});

	it('takes the place of Now playing when entered from Home', () => {
		expect(albumExit('/', album)).toBe('replace');
	});

	it('takes the place of Now playing when entered from another album', () => {
		expect(albumExit('/album/quiet-machines-24', album)).toBe('replace');
	});

	it('takes the place of Now playing after a reload', () => {
		expect(albumExit(null, album)).toBe('replace');
	});
});

describe('the way up from Now playing', () => {
	const { travel, leaving, motion } = choreographyFor('now-playing', 'album');
	const cover = 'cover:glasshouse-66';
	const hero: Candidate = { id: cover, slot: 'hero' };
	const island: Candidate = { id: cover, slot: 'island' };

	it('is the same journey however it was reached: direction comes from depth, not history', () => {
		expect(travel?.sourceSlots).toEqual(['poster']);
		expect(leaving.to.y).toBeGreaterThan(0);
		expect(leaving.ripple).toBeUndefined();
		expect(motion.token).toBe('--morph');
	});

	it('settles the cover into the hero, over the island', () => {
		const slots = travel?.targetSlots ?? [];
		expect(pick([island, hero], { ids: [cover], slots }).get(cover)).toBe(hero);
	});

	it('falls back to the island only when the hero is off screen', () => {
		const slots = travel?.targetSlots ?? [];
		const eligible = (candidate: Candidate) => candidate !== hero;
		expect(pick([island, hero], { ids: [cover], slots, eligible }).get(cover)).toBe(island);
	});
});

describe('going on to Home afterwards', () => {
	const { travel } = choreographyFor('album', 'home');
	const slots = travel?.targetSlots ?? [];
	const cover = 'cover:glasshouse-66';
	const tile: Candidate = { id: cover, slot: 'card' };
	const repeat: Candidate = { id: cover, slot: 'card' };
	const island: Candidate = { id: cover, slot: 'island' };

	it('needs no card origin: the card on screen takes the cover', () => {
		const picked = pick([tile, repeat, island], {
			ids: [cover],
			slots,
			origins: new Map() satisfies Origins,
			visible: (candidate) => candidate === repeat
		});
		expect(picked.get(cover)).toBe(repeat);
	});

	it('condenses into the island when the album has no card', () => {
		expect(pick([island], { ids: [cover], slots }).get(cover)).toBe(island);
	});
});
