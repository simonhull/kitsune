import { describe, expect, it } from 'vitest';
import {
	addedLine,
	emptyLine,
	everythingNote,
	greeting,
	healthNote,
	homeRows,
	listeningHours,
	playsLine,
	summary
} from './home';
import type { AlbumEntry, History, LibraryFacts } from './source';

const facts: LibraryFacts = {
	albums: 927,
	tracks: 7585,
	durationMs: 466 * 3_600_000,
	roots: 1,
	issues: 0,
	issueCounts: { unreadable: 0, corrupted: 0, unsupported: 0, warnings: 0 },
	scan: { state: 'idle', read: 0, total: 0 }
};

function entry(id: string): AlbumEntry {
	return { id, title: id, artist: 'Someone', trackCount: 1, compilation: false };
}

describe('greeting', () => {
	it.each([
		[4, 'Good evening, Simon'],
		[5, 'Good morning, Simon'],
		[11, 'Good morning, Simon'],
		[12, 'Good afternoon, Simon'],
		[17, 'Good afternoon, Simon'],
		[18, 'Good evening, Simon'],
		[23, 'Good evening, Simon']
	])('at %i is %s', (hour, want) => {
		expect(greeting(hour, 'Simon')).toBe(want);
	});
});

describe('summary', () => {
	it('counts albums, songs and hours, at 0, 1 and N', () => {
		expect(summary(facts)).toEqual(['927 albums', '7,585 songs', '466 hours']);
		expect(summary({ ...facts, albums: 1, tracks: 1, durationMs: 3_600_000 })).toEqual([
			'1 album',
			'1 song',
			'1 hour'
		]);
		expect(summary({ ...facts, albums: 0, tracks: 0, durationMs: 0 })).toEqual([
			'0 albums',
			'0 songs',
			'0 minutes'
		]);
	});
});

describe('listeningHours', () => {
	it('rounds to hours, and falls to minutes under one', () => {
		expect(listeningHours(1_677_087_013)).toBe('466 hours');
		expect(listeningHours(42 * 60_000)).toBe('42 minutes');
		expect(listeningHours(60_000)).toBe('1 minute');
		expect(listeningHours(59.9 * 60_000)).toBe('60 minutes');
	});
});

describe('emptyLine', () => {
	const empty = { ...facts, albums: 0, tracks: 0 };

	it('tells no roots, a scan under way and a scan that found nothing apart', () => {
		expect(emptyLine({ ...empty, roots: 0 }, false)).toMatch(/^No music folders/);
		expect(emptyLine(empty, true)).toMatch(/^The first scan is running/);
		expect(emptyLine(empty, false)).toMatch(/^The scan found no music/);
	});
});

describe('healthNote', () => {
	const none = facts.issueCounts;

	it('agrees with the number at 0, 1 and N', () => {
		expect(healthNote(0, none)).toBe('Every file was read');
		expect(healthNote(1, { ...none, unsupported: 1 })).toBe("1 file Kitsune can't read yet");
		expect(healthNote(1600, { ...none, unsupported: 1600 })).toBe(
			"1,600 files Kitsune can't read yet"
		);
	});

	it('promises nothing about a file that is damaged', () => {
		expect(healthNote(3, { ...none, unsupported: 2, corrupted: 1 })).toBe(
			"3 files Kitsune can't read"
		);
		expect(healthNote(1, { ...none, unreadable: 1 })).toBe("1 file Kitsune can't read");
	});
});

describe('the tiles’ words', () => {
	it('names the artists only once they are known', () => {
		expect(everythingNote(7585, 916)).toBe('7,585 songs · 916 artists');
		expect(everythingNote(1, 1)).toBe('1 song · 1 artist');
		expect(everythingNote(7585, null)).toBe('7,585 songs');
	});

	it('counts plays', () => {
		expect(playsLine(122)).toBe('122 plays');
		expect(playsLine(1)).toBe('1 play');
	});

	it('gives the year and when an album came, and leaves out what is unknown', () => {
		expect(addedLine({ year: 2013, addedAt: '2026-11-09T12:00:00Z' })).toBe('2013 · added Nov 9');
		expect(addedLine({ year: 0, addedAt: '2026-11-09T12:00:00Z' })).toBe('added Nov 9');
		expect(addedLine({})).toBe('');
	});
});

describe('homeRows', () => {
	const recent = [entry('new'), entry('older'), entry('oldest')];
	const history: History = {
		recent: [entry('b'), entry('a'), entry('c')],
		onRepeat: [
			{ album: entry('a'), plays: 9 },
			{ album: entry('b'), plays: 3 }
		]
	};

	it('leaves the album the player holds out of Recently played, and keeps every new one', () => {
		const rows = homeRows(recent, history, 'a');
		expect(rows.feature).toBeUndefined();
		expect(rows.played.map(({ id }) => id)).toEqual(['b', 'c']);
		expect(rows.onRepeat.map(({ album }) => album.id)).toEqual(['a', 'b']);
		expect(rows.added.map(({ id }) => id)).toEqual(['new', 'older', 'oldest']);
	});

	it('gives the newest album the card while nothing is held, and leaves it out of Just added', () => {
		const rows = homeRows(recent, history, null);
		expect(rows.feature?.id).toBe('new');
		expect(rows.played.map(({ id }) => id)).toEqual(['b', 'a', 'c']);
		expect(rows.added.map(({ id }) => id)).toEqual(['older', 'oldest']);
	});

	it('has no history rows before the history is known, or for someone who never played', () => {
		for (const none of [null, { recent: [], onRepeat: [] }]) {
			const rows = homeRows(recent, none, null);
			expect(rows.played).toEqual([]);
			expect(rows.onRepeat).toEqual([]);
			expect(rows.added).toHaveLength(2);
		}
	});

	it('draws nothing in Recently played when the only album played is the one held', () => {
		const one: History = { recent: [entry('a')], onRepeat: [{ album: entry('a'), plays: 1 }] };
		expect(homeRows(recent, one, 'a').played).toEqual([]);
	});
});
