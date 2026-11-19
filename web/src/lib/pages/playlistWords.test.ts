import { describe, expect, it } from 'vitest';
import type { PlaylistAlbum, PlaylistSummary } from '#lib/library/source.ts';
import {
	addedWords,
	addHeading,
	againWords,
	agoWords,
	byLastAdded,
	cardLine,
	countLine,
	dockLine,
	headLine,
	holdsWords,
	limitPill,
	liveFoot,
	liveLine,
	liveRowLine,
	matching,
	matchWord,
	mosaicOf,
	nameFor,
	phoneHeadLine,
	removedWords,
	sheetLine,
	undoneWords
} from './playlistWords.ts';

const album = (id: string): PlaylistAlbum => ({ id, title: id, artist: 'A', compilation: false });

function summary(over: Partial<PlaylistSummary> = {}): PlaylistSummary {
	return {
		id: 'p',
		name: 'Falling Up',
		source: 'kitsune',
		smart: false,
		songCount: 151,
		albumCount: 14,
		durationMs: (11 * 60 + 12) * 60_000,
		mosaic: [],
		createdAt: '2026-11-01T00:00:00Z',
		updatedAt: '2026-11-01T00:00:00Z',
		...over
	};
}

describe('mosaicOf', () => {
	it('draws four albums as a 2×2, in the order given', () => {
		const four = ['a', 'b', 'c', 'd', 'e'].map(album);
		expect(mosaicOf(four).map(({ id }) => id)).toEqual(['a', 'b', 'c', 'd']);
	});

	it('fills the card with the first cover when there are fewer than four, and none for an empty one', () => {
		expect(mosaicOf(['a', 'b', 'c'].map(album)).map(({ id }) => id)).toEqual(['a']);
		expect(mosaicOf([album('a')]).map(({ id }) => id)).toEqual(['a']);
		expect(mosaicOf([])).toEqual([]);
	});
});

describe('lines', () => {
	it('says how big a playlist is, and what to do with an empty one', () => {
		expect(cardLine(summary())).toBe('151 SONGS · 11 H 12 MIN');
		expect(cardLine(summary({ songCount: 1, durationMs: 200_000 }))).toBe('1 SONG · 3 MIN');
		expect(cardLine(summary({ songCount: 0, durationMs: 0 }))).toBe(
			'EMPTY · ADD SONGS FROM ANY ⋯ MENU'
		);
	});

	it('heads a playlist, saying where an imported one came from', () => {
		expect(headLine(summary({ source: 'navidrome' }))).toBe(
			'PLAYLIST · 151 SONGS · 14 ALBUMS · 11 H 12 MIN · FROM NAVIDROME'
		);
		expect(headLine(summary({ songCount: 0 }))).toBe('PLAYLIST · 0 SONGS');
		expect(phoneHeadLine(summary())).toBe('PLAYLIST · 151 SONGS · 11 H');
	});

	it('counts the playlists', () => {
		expect(countLine(1)).toBe('1 PLAYLIST · YOURS ALONE');
		expect(countLine(3)).toBe('3 PLAYLISTS · YOURS ALONE');
	});
});

describe('nameFor', () => {
	it('keeps a name typed, trimmed', () => {
		expect(nameFor('  Road trip ', ['New playlist'])).toBe('Road trip');
	});

	it('names an empty one New playlist, then New playlist 2 and on, whatever the case', () => {
		expect(nameFor('', [])).toBe('New playlist');
		expect(nameFor('   ', ['new playlist'])).toBe('New playlist 2');
		expect(nameFor('', ['New playlist', 'New playlist 2', 'New playlist 4'])).toBe(
			'New playlist 3'
		);
	});
});

describe('holdsWords', () => {
	it('says ALREADY IN IT when it holds every song, and 1 OF THE 3 when some', () => {
		expect(holdsWords(1, 1)).toBe('ALREADY IN IT');
		expect(holdsWords(3, 3)).toBe('ALREADY IN IT');
		expect(holdsWords(1, 3)).toBe('1 OF THE 3 ALREADY IN IT');
	});

	it('says nothing when it holds none, or while that is not known', () => {
		expect(holdsWords(0, 3)).toBeNull();
		expect(holdsWords(undefined, 3)).toBeNull();
	});

	it('heads a popover or a sheet with how many songs go in', () => {
		expect(addHeading(3)).toBe('ADD 3 SONGS TO');
		expect(addHeading(1)).toBe('ADD 1 SONG TO');
	});

	it('is shorter on the phone sheet, after the playlist’s size', () => {
		expect(sheetLine(summary(), 1, 3)).toBe('151 SONGS · 1 OF THE 3 IN IT');
		expect(sheetLine(summary(), 3, 3)).toBe('151 SONGS · ALREADY IN IT');
		expect(sheetLine(summary({ songCount: 0 }), undefined, 3)).toBe('EMPTY');
	});
});

describe('the toasts', () => {
	const plain = (parts: { text: string; bold?: boolean }[]): string =>
		parts.map(({ text, bold }) => (bold === true ? `*${text}*` : text)).join('');

	it('says what an add did, and what it left out', () => {
		expect(plain(addedWords(2, 1, 'Falling Up'))).toBe(
			'2 songs added to *Falling Up* · 1 was already in it'
		);
		expect(plain(addedWords(1, 0, 'Falling Up'))).toBe('1 song added to *Falling Up*');
		expect(plain(addedWords(3, 2, 'Mix'))).toBe('3 songs added to *Mix* · 2 were already in it');
		expect(plain(addedWords(0, 1, 'Mix'))).toBe('*Mix* has it already');
		expect(plain(addedWords(0, 3, 'Mix'))).toBe('All 3 already in *Mix*');
	});

	it('offers to add the skipped ones after all', () => {
		expect(againWords(0)).toBeNull();
		expect(againWords(1)).toBe('Add it again');
		expect(againWords(2)).toBe('Add them again');
	});

	it('says what a take-out took', () => {
		expect(plain(removedWords(['Searchlights'], 'Falling Up'))).toBe(
			'*Searchlights* taken out of Falling Up'
		);
		expect(plain(removedWords(['a', 'b', 'c'], 'Falling Up'))).toBe(
			'3 songs taken out of Falling Up'
		);
	});
});

describe('the submenu’s order', () => {
	it('lists the most recently added to first, then the newest made', () => {
		const lists = [
			summary({ id: 'old', createdAt: '2026-01-01T00:00:00Z' }),
			summary({ id: 'added', addedAt: '2026-11-10T00:00:00Z' }),
			summary({ id: 'new', createdAt: '2026-11-05T00:00:00Z' })
		];
		expect(byLastAdded(lists).map(({ id }) => id)).toEqual(['added', 'new', 'old']);
	});

	it('finds a playlist by any part of its name', () => {
		const lists = [summary({ id: 'a', name: 'Falling Up' }), summary({ id: 'b', name: 'Road' })];
		expect(matching(lists, 'fall').map(({ id }) => id)).toEqual(['a']);
		expect(matching(lists, ' ').map(({ id }) => id)).toEqual(['a', 'b']);
	});
});

describe('smart playlists', () => {
	it('say they are smart and live on their cards, heads and the count line', () => {
		const smart = summary({ smart: true, songCount: 19, albumCount: 4, durationMs: 100 * 60_000 });
		expect(cardLine(smart)).toBe('19 SONGS · 1 H 40 MIN · LIVE');
		expect(cardLine({ ...smart, songCount: 0 })).toBe('NOTHING MATCHES YET · LIVE');
		expect(headLine(smart)).toBe('SMART PLAYLIST · 19 SONGS · 4 ALBUMS · 1 H 40 MIN · LIVE');
		expect(phoneHeadLine(smart)).toBe('SMART · 19 SONGS · 1 H 40 · LIVE');
		expect(countLine(2, 1)).toBe('2 PLAYLISTS · 1 SMART · YOURS ALONE');
	});

	it('are left out of Add to playlist', () => {
		const lists = [summary({ id: 'a' }), summary({ id: 's', smart: true })];
		expect(byLastAdded(lists).map(({ id }) => id)).toEqual(['a']);
	});

	it('word the live card', () => {
		const p = { songCount: 19, matchCount: 23, albumCount: 4, durationMs: 100 * 60_000 };
		expect(matchWord(1)).toBe('SONG MATCHES');
		expect(matchWord(19)).toBe('SONGS MATCH');
		expect(liveLine(p, undefined)).toBe('1 H 40 MIN · 4 ALBUMS · 23 MATCH, THE LIMIT KEEPS 19');
		expect(liveLine({ ...p, matchCount: 19 }, 26)).toBe(
			'1 H 40 MIN · 4 ALBUMS · 7 FEWER THAN SAVED'
		);
		expect(liveLine({ ...p, matchCount: 19 }, 19)).toBe('1 H 40 MIN · 4 ALBUMS');
		const now = Date.parse('2026-11-15T12:00:00Z');
		expect(liveRowLine('Captiva', '2026-11-04T12:00:00Z', 6, now)).toBe(
			'CAPTIVA · 11 DAYS AGO · 6 PLAYS'
		);
		expect(liveRowLine('Captiva', undefined, 1, now)).toBe('CAPTIVA · NEVER · 1 PLAY');
		expect(agoWords('2026-11-14T12:00:00Z', now)).toBe('YESTERDAY');
		expect(liveFoot(19, 13)).toBe('+ 6 MORE · COUNTED BY THE SERVER AS YOU TYPE');
		expect(liveFoot(3, 3)).toBe('COUNTED BY THE SERVER AS YOU TYPE');
	});

	it('word the dock and the phone limit pill', () => {
		expect(dockLine(4, true)).toBe('4 rules · not saved yet');
		expect(dockLine(1, true)).toBe('1 rule · not saved yet');
		expect(dockLine(4, false)).toBe('Nothing changed');
		expect(limitPill({ on: true, count: 50, unit: 'songs' })).toBe('Limit · 50 songs');
		expect(limitPill({ on: false, count: 50, unit: 'songs' })).toBe('Limit · off');
	});
});

describe('undoneWords', () => {
	it('says what Undo took back, an add or a take-out', () => {
		expect(undoneWords('add', 3, 'Late drives')).toEqual([
			{ text: '3 songs taken out of ' },
			{ text: 'Late drives', bold: true }
		]);
		expect(undoneWords('takeOut', 1, 'Late drives')).toEqual([
			{ text: '1 song put back in ' },
			{ text: 'Late drives', bold: true }
		]);
	});
});
