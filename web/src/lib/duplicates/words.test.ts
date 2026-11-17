import { describe, expect, it } from 'vitest';
import { keeperOf } from './fixture.ts';
import type { DuplicateCopy, DuplicateCounts, MergePlan } from './types.ts';
import {
	checkedNote,
	chipWords,
	countsLine,
	evidenceWords,
	lengthsWords,
	mergedWords,
	movesLines,
	shortAlbum,
	whyWords
} from './words.ts';

const counts: DuplicateCounts = {
	groups: 51,
	songs: 104,
	pairs: 1,
	audio: 0,
	recording: 35,
	probably: 16,
	dismissed: 0,
	library: 7604,
	checked: 4210
};

function copy(track: string, over: Partial<DuplicateCopy>): DuplicateCopy {
	return {
		track,
		title: 'Creed',
		album: { id: `a-${track}`, title: track, artist: 'Rich Mullins' },
		path: `${track}.flac`,
		root: 0,
		format: 'flac',
		bitrate: 1000,
		lossless: true,
		durationMs: 321_000,
		plays: 0,
		stars: 0,
		raters: 0,
		playlists: 0,
		addedAt: '2025-03-21T00:00:00Z',
		inPlace: true,
		original: true,
		...over
	};
}

const empty: MergePlan = {
	plays: 0,
	listens: 0,
	stars: { come: 0, stay: 0 },
	loves: 0,
	hearts: 0,
	entries: 0,
	playlists: [],
	otherPlaylists: 0,
	excluded: 0,
	runs: 0,
	queues: 0
};

describe('the list', () => {
	it('counts as board G does', () => {
		expect(countsLine(counts)).toBe('51 GROUPS · 104 SONGS · 1 ALBUM PAIR · 7,604 SONGS CHECKED');
		expect(countsLine({ ...counts, pairs: 0 }, true)).toBe('51 GROUPS · 104 SONGS');
	});

	it('says how far the fingerprint pass has got, and nothing once it is done', () => {
		expect(checkedNote(counts)).toBe(
			'4,210 OF 7,604 SONGS CHECKED FOR EXACT COPIES · THE REST AS THE FINGERPRINT PASS REACHES THEM'
		);
		expect(checkedNote({ ...counts, checked: 7604 })).toBe('');
	});

	it('names each confidence', () => {
		expect(chipWords('audio')).toBe('SAME AUDIO');
		expect(chipWords('recording')).toBe('SAME RECORDING');
		expect(chipWords('probably')).toBe('SAME SONG, PROBABLY');
	});
});

describe('a group', () => {
	it('words its evidence', () => {
		expect(lengthsWords([321_000, 322_000])).toBe('5:21 AND 5:22');
		expect(lengthsWords([168_000, 168_400])).toBe('2:48');
		expect(evidenceWords('recording', 'Rich Mullins', [321_000, 322_000])).toBe(
			'RICH MULLINS · BOTH NAME ONE MUSICBRAINZ RECORDING · 5:21 AND 5:22'
		);
		expect(evidenceWords('audio', '', [1, 2])).toBe(
			'SAME AUDIO · THE FINGERPRINT PASS FOUND THE SAME SOUND IN BOTH FILES'
		);
	});

	it('keeps Creed on the album it is from over the best-of, as board H now has it', () => {
		const copies = [
			copy('liturgy', { bitrate: 957, plays: 1 }),
			copy('songs', { bitrate: 1022, plays: 2, original: false })
		];
		const { keep, reasons } = keeperOf(copies);
		expect(keep).toBe(0);
		expect(reasons).toEqual(['original']);
		expect(whyWords(reasons, copies, 'liturgy', 'liturgy')).toBe('ORIGINAL ALBUM');
		expect(whyWords(reasons, copies, 'liturgy', 'songs')).toBe(
			'YOUR CHOICE · THE OTHER IS ON THE ORIGINAL ALBUM'
		);
	});

	it('gives a higher bitrate its margin', () => {
		const copies = [
			copy('flac', { bitrate: 1034 }),
			copy('mp3', { bitrate: 320, lossless: false })
		];
		const { reasons } = keeperOf(copies);
		expect(whyWords(reasons, copies, 'flac', 'flac')).toBe('LOSSLESS · HIGHER BITRATE (+223%)');
	});

	it('shortens an album for a button', () => {
		expect(shortAlbum('A Liturgy, a Legacy & a Ragamuffin Band')).toBe('A Liturgy');
		expect(shortAlbum('Songs')).toBe('Songs');
	});
});

describe('the confirm', () => {
	it('words what moves, never naming another person’s stars', () => {
		const plan: MergePlan = {
			...empty,
			plays: 38,
			listens: 41,
			stars: { yours: { keeper: 4, other: 3.5, after: 4 }, come: 1, stay: 0 },
			loves: 1,
			entries: 2,
			playlists: [{ id: 'p', name: 'Falling Up, Rested and rated' }],
			otherPlaylists: 1,
			excluded: 1,
			runs: 1
		};
		expect(movesLines(plan, { name: 'Wish', plays: 2, stars: 4 })).toEqual([
			'38 plays and their listens, so Wish has 40 plays',
			'Stars: your 4★ stays (the other had 3½★). 1 other person’s stars come over. Last.fm gets 1 love, as any rating over the line would',
			'2 playlist entries: Falling Up, Rested and rated, 1 playlist of someone else’s',
			'Shuffle: excluded for 1 person; 1 Combine place moves too'
		]);
	});

	it('says your stars stay when the copy going had none', () => {
		expect(movesLines(empty, { name: 'A Liturgy', plays: 1, stars: 5, other: 'Songs' })[1]).toBe(
			'Stars: your 5★ stays (Songs had none)'
		);
		expect(movesLines(empty, { name: 'A Liturgy', plays: 1, stars: 0 })[1]).toBe(
			'No stars to move'
		);
	});

	it('words the toast', () => {
		expect(mergedWords('The Best It’s Gonna Get', 'Wish Upon a Blackstar', 1)).toBe(
			'Kept The Best It’s Gonna Get · Wish Upon a Blackstar. 1 copy moved to the Trash'
		);
	});
});
