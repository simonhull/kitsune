import { describe, expect, it } from 'vitest';
import type { OrganizeCounts, OrganizeRun } from './types.ts';
import {
	countsLine,
	dockWords,
	doneWords,
	headWords,
	inPlaceWords,
	navNote,
	paceWords,
	pathDiff,
	runRowWords,
	scopeWords,
	summaryWords,
	tookWords,
	undoAsk
} from './words.ts';

const counts: OrganizeCounts = {
	songs: 7604,
	inPlace: 5070,
	moves: 2534,
	renames: 850,
	newFolders: 1684,
	conflicts: 66,
	skipped: 0,
	folders: 312
};

const run: OrganizeRun = {
	id: 'r',
	kind: 'run',
	state: 'done',
	startedAt: '2026-11-16T21:10:00Z',
	finishedAt: '2026-11-16T21:13:12Z',
	template: '<Album Artist>/[<Year> - ]<Album>/[<Disc>-]<Track> - <Title>',
	total: 2468,
	done: 2468,
	moved: 2468,
	folders: 312,
	conflicts: 66,
	left: 0,
	failed: 0,
	companions: 40,
	undoable: true
};

describe('the counts', () => {
	it('reads as the board’s mono line', () => {
		expect(countsLine(counts)).toBe(
			'7,604 SONGS · 5,070 IN PLACE · 2,534 WOULD MOVE · 66 IN CONFLICT'
		);
		expect(countsLine({ ...counts, skipped: 4 }, true)).toBe(
			'5,070 IN PLACE · 2,534 WOULD MOVE · 66 IN CONFLICT · 4 SKIPPED'
		);
	});

	it('says what a run would move, and what stays', () => {
		expect(dockWords(2534, counts, false)).toEqual({
			title: 'Move 2,534 songs',
			line: '66 IN CONFLICT STAY WHERE THEY ARE · 5,070 ALREADY IN PLACE'
		});
		expect(dockWords(1, counts, true).title).toBe('Move 1 song');
	});

	it('names a scoped preview and an in-place one', () => {
		expect(scopeWords({ songs: 12, albums: 1, album: 'Fangs!' })).toBe(
			'Organizing 12 songs from Fangs!'
		);
		expect(scopeWords({ songs: 30, albums: 3 })).toBe('Organizing 30 songs from 3 albums');
		expect(inPlaceWords(12)).toBe('These 12 songs are already where the template puts them.');
		expect(inPlaceWords(1)).toBe('This song is already where the template puts it.');
	});
});

describe('a run', () => {
	it('has a head, a pace and a note', () => {
		expect(headWords({ kind: 'run', done: 1214, total: 2534 })).toBe('Moving 1,214 of 2,534');
		expect(headWords({ kind: 'undo', done: 3, total: 9 })).toBe('Putting back 3 of 9');
		expect(paceWords(1280, 2560, 120_000)).toBe('ABOUT 2 MIN LEFT · 640 SONGS A MINUTE');
		expect(paceWords(0, 10, 0)).toBe('STARTING');
		expect(navNote(1214, 2534)).toBe('ORGANIZING · 47%');
	});

	it('says how long it took', () => {
		expect(tookWords(192_000)).toBe('3 MIN 12 S');
		expect(tookWords(45_400)).toBe('45 S');
		expect(tookWords(3_720_000)).toBe('1 H 2 MIN');
	});

	it('sums up what it did', () => {
		expect(summaryWords(run)).toBe(
			'Moved 2,468 songs into 312 folders · 66 conflicts stayed · 0 failed'
		);
		const done = doneWords(run, Date.parse('2026-11-16T22:00:00Z'));
		expect(done.title).toBe('Moved 2,468 songs');
		expect(done.kicker).toMatch(/^FINISHED · TODAY · \d\d:\d\d · TOOK 3 MIN 12 S$/);
		expect(doneWords({ ...run, state: 'stopped' }, Date.now()).kicker).toContain(
			'AT AN ALBUM’S END'
		);
		expect(undoAsk(run)).toBe('Put 2,468 songs back where they were?');
	});

	it('lists as the runs list does', () => {
		expect(runRowWords(run)).toEqual({
			what: 'Moved 2,468 songs · Artist / Year - Album / NN - Title',
			state: 'UNDO ›'
		});
		expect(runRowWords({ ...run, undoable: false, state: 'undone' }).state).toBe('UNDONE');
		expect(runRowWords({ ...run, kind: 'undo', undoable: false }).what).toBe(
			'Put back 2,468 songs'
		);
	});
});

describe('pathDiff', () => {
	const changed = (parts: { text: string; changed: boolean }[]): string[] =>
		parts.filter((p) => p.changed).map((p) => p.text);

	it('marks only the file for a rename', () => {
		const d = pathDiff(
			'David Crowder Band/2009 - Church Music/01 Phos Hilaron.flac',
			'David Crowder Band/2009 - Church Music/01 - Phos Hilaron.flac'
		);
		expect(changed(d.old)).toEqual(['01 Phos Hilaron.flac']);
		expect(changed(d.new)).toEqual(['01 - Phos Hilaron.flac']);
	});

	it('marks the new folder and the name for a move one deeper', () => {
		const d = pathDiff(
			'Harry Gregson-Williams/13 Only the Beginning.flac',
			'Harry Gregson-Williams/2005 - Narnia/13 - Only the Beginning.flac'
		);
		expect(changed(d.old)).toEqual(['13 Only the Beginning.flac']);
		expect(changed(d.new)).toEqual(['2005 - Narnia/', '13 - Only the Beginning.flac']);
	});

	it('marks nothing in place', () => {
		const d = pathDiff('A/B/c.flac', 'A/B/c.flac');
		expect(changed(d.old)).toEqual([]);
		expect(changed(d.new)).toEqual([]);
	});
});
