import { describe, expect, it } from 'vitest';
import { emptyLine, greeting, healthNote, listeningTime, summary, trackCount } from './home';
import type { LibraryFacts } from './source';

const facts: LibraryFacts = {
	albums: 927,
	tracks: 7586,
	durationMs: 0,
	roots: 1,
	issues: 0,
	issueCounts: { unreadable: 0, corrupted: 0, unsupported: 0, warnings: 0 },
	scan: { state: 'idle', read: 0, total: 0 }
};

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
	it('counts 0, 1 and N', () => {
		expect(summary({ ...facts, albums: 0, roots: 0 })).toBe(
			'0 albums · 0 roots watched · no issues'
		);
		expect(summary({ ...facts, albums: 1, issues: 1 })).toBe('1 album · 1 root watched · 1 issue');
		expect(summary({ ...facts, albums: 1284, roots: 2, issues: 16 })).toBe(
			'1,284 albums · 2 roots watched · 16 issues'
		);
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
	it('agrees with the number at 0, 1 and N', () => {
		expect(healthNote(0)).toBe('Every file was read');
		expect(healthNote(1)).toBe('1 file could not be read');
		expect(healthNote(1600)).toBe('1,600 files could not be read');
	});
});

describe('trackCount', () => {
	it('is singular at 1', () => {
		expect(trackCount(1)).toBe('1 track');
		expect(trackCount(7586)).toBe('7,586 tracks');
	});
});

describe('listeningTime', () => {
	it('reads in days and hours', () => {
		expect(listeningTime(1_677_087_013)).toBe('19d 10h');
		expect(listeningTime(86_400_000)).toBe('1d 0h');
		expect(listeningTime(23 * 3_600_000)).toBe('23h');
	});

	it('falls to minutes under an hour, and carries when an hour rounds up', () => {
		expect(listeningTime(0)).toBe('0m');
		expect(listeningTime(42 * 60_000)).toBe('42m');
		expect(listeningTime(47.9 * 3_600_000)).toBe('2d 0h');
	});
});
