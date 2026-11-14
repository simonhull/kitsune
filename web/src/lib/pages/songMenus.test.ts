import { describe, expect, it } from 'vitest';
import { Selection } from './selection.svelte.ts';
import { factsLine, menuTarget, pickedFacts, rowMenuItems } from './songActions.ts';

function pickedOf(ids: string[]): Selection {
	const selection = new Selection();
	for (const id of ids) {
		selection.toggle(id);
	}
	return selection;
}

const song = { kind: 'song' as const, artistId: 'ar1' };
const everywhere = { editable: true, removable: false };

describe('rowMenuItems', () => {
	it('gives a song the queue actions first, and no Play: a click on the row plays', () => {
		expect(rowMenuItems(song, everywhere)).toEqual([
			'playNext',
			'addToQueue',
			'rating',
			'editTags',
			'goToAlbum',
			'goToArtist',
			'fileInfo'
		]);
	});

	it('leads songs picked with Play, then Play next and Add to queue, and goes nowhere', () => {
		expect(rowMenuItems({ kind: 'picked' }, everywhere)).toEqual([
			'play',
			'playNext',
			'addToQueue',
			'rating',
			'editTags'
		]);
	});

	it('leaves out the page it is on', () => {
		expect(rowMenuItems(song, { ...everywhere, here: 'album' })).not.toContain('goToAlbum');
		expect(rowMenuItems(song, { ...everywhere, here: 'album' })).toContain('goToArtist');
		expect(rowMenuItems(song, { ...everywhere, here: 'artist' })).not.toContain('goToArtist');
		expect(rowMenuItems(song, { ...everywhere, here: 'artist' })).toContain('goToAlbum');
	});

	it('adds Remove from queue in Up next, and Edit tags only for someone who may', () => {
		const queue = rowMenuItems(song, { editable: false, removable: true });
		expect(queue).toContain('remove');
		expect(queue).not.toContain('editTags');
	});

	it('ends with Move to Trash only for someone who may, for one song or songs picked', () => {
		expect(rowMenuItems(song, everywhere)).not.toContain('trash');
		expect(rowMenuItems(song, { ...everywhere, trashable: false })).not.toContain('trash');
		expect(rowMenuItems(song, { ...everywhere, trashable: true }).at(-1)).toBe('trash');
		expect(rowMenuItems({ kind: 'picked' }, { ...everywhere, trashable: true }).at(-1)).toBe(
			'trash'
		);
	});

	it('has no Go to artist for a song with nobody to go to', () => {
		expect(rowMenuItems({ kind: 'song' }, everywhere)).not.toContain('goToArtist');
	});
});

describe('menuTarget', () => {
	const facts = (id: string): { albumId: string; durationMs: number } => ({
		albumId: id.startsWith('a') ? 'A' : 'B',
		durationMs: 60_000
	});
	const one = (
		id: string
	): { albumId: string; title: string; line: string; artistId: undefined } => ({
		albumId: 'A',
		title: id,
		line: '',
		artistId: undefined
	});

	it('is the songs picked when it opens on one of several picked', () => {
		const target = menuTarget('a1', pickedOf(['a1', 'b1']), () => ['a1', 'b1'], facts, one);
		expect(target).toEqual({
			kind: 'picked',
			count: 2,
			line: '2 ALBUMS · 2:00',
			trackIds: ['a1', 'b1']
		});
	});

	it('is the song alone when it opens on one not picked, or picked alone', () => {
		const never = (): string[] => {
			throw new Error('asked for the songs picked');
		};
		expect(menuTarget('a2', pickedOf(['a1', 'b1']), never, facts, one)?.kind).toBe('song');
		expect(menuTarget('a1', pickedOf(['a1']), never, facts, one)?.kind).toBe('song');
	});

	it('says the facts as the bar does', () => {
		expect(factsLine(pickedFacts(['a1', 'a2'], facts))).toBe('1 ALBUM · 2:00');
	});
});
