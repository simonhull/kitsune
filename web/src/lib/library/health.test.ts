import { describe, expect, it } from 'vitest';
import {
	allClear,
	filesState,
	issueGroups,
	lastScan,
	loudnessLine,
	minutesLeft,
	refusalLine,
	scanHelp,
	scanLine,
	scanPercent,
	splitPath,
	staleFor,
	standing,
	steadyMinutes,
	timeLeft,
	warningLine
} from './health';
import type { IssueCounts, LibraryFacts, ScanIssue, ScanResult, ScanStatus } from './source';

const noCounts: IssueCounts = { unreadable: 0, corrupted: 0, unsupported: 0, warnings: 0 };

const facts: LibraryFacts = {
	albums: 927,
	tracks: 7586,
	durationMs: 0,
	roots: 1,
	issues: 0,
	issueCounts: noCounts,
	scan: { state: 'idle', read: 0, total: 0 }
};

const started = Date.parse('2026-11-05T12:00:00Z');

function running(read: number, total: number): ScanStatus {
	return {
		state: 'running',
		mode: 'full',
		read,
		total,
		startedAt: new Date(started).toISOString()
	};
}

const finished: ScanResult = {
	mode: 'full',
	outcome: 'finished',
	startedAt: new Date(started).toISOString(),
	endedAt: new Date(started + 416_000).toISOString(),
	seen: 7602,
	read: 7602,
	added: 0,
	updated: 0,
	moved: 0,
	missing: 0,
	deleted: 0,
	issues: 0,
	skippedRoots: 0
};

const ended = started + 416_000;

describe('scanLine', () => {
	it('says the scan is starting until it has a total', () => {
		expect(scanLine(running(0, 0))).toBe('FULL SCAN · STARTING');
	});

	it('counts once it has one', () => {
		expect(scanLine(running(3120, 7602))).toBe('FULL SCAN · 3,120 OF 7,602');
		expect(scanLine({ ...running(1, 3), mode: 'quick' })).toBe('QUICK SCAN · 1 OF 3');
	});
});

describe('scanPercent', () => {
	it('is the board’s 41 at 3,120 of 7,602', () => {
		expect(scanPercent(running(3120, 7602))).toBe(41);
	});

	it('claims nothing without a total, and never passes 100', () => {
		expect(scanPercent(running(0, 0))).toBe(0);
		expect(scanPercent(running(11, 10))).toBe(100);
	});
});

describe('minutesLeft', () => {
	it('stays silent until there is a total', () => {
		expect(minutesLeft(running(0, 0), started + 60_000)).toBeNull();
	});

	it('stays silent for the first seconds, however fast they went', () => {
		expect(minutesLeft(running(3000, 7602), started + 9999)).toBeNull();
	});

	it('stays silent for the first files, however long they took', () => {
		expect(minutesLeft(running(100, 7602), started + 60_000)).toBeNull();
	});

	it('stays silent on a clock behind the server’s', () => {
		expect(minutesLeft(running(3000, 7602), started - 60_000)).toBeNull();
	});

	it('works it out from the rate so far', () => {
		expect(minutesLeft(running(3120, 7602), started + 170_000)).toBeCloseTo(4.07, 2);
	});

	it('never goes below nothing', () => {
		expect(minutesLeft(running(7610, 7602), started + 416_000)).toBe(0);
	});
});

describe('steadyMinutes', () => {
	it('rounds the first estimate', () => {
		expect(steadyMinutes(null, 7.6)).toBe(8);
	});

	it('holds through the wobble at a rounding edge', () => {
		let shown = steadyMinutes(null, 7.51);
		for (const minutes of [7.49, 7.52, 7.48, 7.55, 7.45]) {
			shown = steadyMinutes(shown, minutes);
			expect(shown).toBe(8);
		}
	});

	it('follows a real change, down or up', () => {
		expect(steadyMinutes(8, 7.2)).toBe(7);
		expect(steadyMinutes(4, 6)).toBe(6);
	});

	it('forgets when there is nothing to say', () => {
		expect(steadyMinutes(8, null)).toBeNull();
	});
});

describe('timeLeft', () => {
	it('stays silent with no estimate', () => {
		expect(timeLeft(running(10, 7602), null)).toBeNull();
	});

	it('says the minutes', () => {
		expect(timeLeft(running(3120, 7602), 4)).toBe('ABOUT 4 MIN LEFT');
		expect(timeLeft(running(10, 200_000), 1200)).toBe('ABOUT 1,200 MIN LEFT');
	});

	it('gives up on minutes at the end', () => {
		expect(timeLeft(running(7500, 7602), 0)).toBe('UNDER A MINUTE LEFT');
	});

	it('does not count down to a zero while the last writes finish', () => {
		expect(timeLeft(running(7602, 7602), 0)).toBe('FINISHING UP');
		expect(timeLeft(running(0, 0), null)).toBeNull();
	});
});

describe('lastScan', () => {
	it('says what it was, when, and how long it took', () => {
		expect(lastScan(finished, ended + 180_000)).toEqual({
			title: 'FULL SCAN · 3 MIN AGO',
			took: 'TOOK 7 MIN',
			detail: '7,602 FILES READ'
		});
	});

	it('reads the hours and days as they pass', () => {
		expect(lastScan(finished, ended + 30_000).title).toBe('FULL SCAN · JUST NOW');
		expect(lastScan(finished, ended + 3 * 3_600_000).title).toBe('FULL SCAN · 3 H AGO');
		expect(lastScan(finished, ended + 72 * 3_600_000).title).toBe('FULL SCAN · 3 DAYS AGO');
	});

	it('copes with a Quick scan shorter than a second', () => {
		const quick: ScanResult = {
			...finished,
			mode: 'quick',
			endedAt: new Date(started + 370).toISOString()
		};
		expect(lastScan(quick, started + 500)).toMatchObject({
			title: 'QUICK SCAN · JUST NOW',
			took: 'TOOK UNDER A SECOND'
		});
	});

	it('names only what changed', () => {
		const changed = { ...finished, mode: 'quick', added: 3, updated: 2, missing: 1 } as const;
		expect(lastScan(changed, ended).detail).toBe('7,602 FILES · 3 ADDED · 2 UPDATED · 1 MISSING');
	});

	it('says a quiet Quick scan changed nothing', () => {
		const quiet = { ...finished, mode: 'quick', read: 0 } as const;
		expect(lastScan(quiet, ended).detail).toBe('7,602 FILES · NOTHING CHANGED');
	});

	it('does not call every track a Full scan rewrites a change', () => {
		const full = { ...finished, updated: 7586, added: 1 };
		expect(lastScan(full, ended).detail).toBe('7,602 FILES READ · 1 ADDED');
	});

	it('says when a folder was skipped, which is never "nothing changed"', () => {
		const skipped = { ...finished, skippedRoots: 2 };
		expect(lastScan(skipped, ended).detail).toBe(
			'7,602 FILES READ · 2 FOLDERS SKIPPED: LOOKED OFFLINE'
		);
		expect(lastScan({ ...skipped, mode: 'quick' }, ended).detail).toBe(
			'7,602 FILES · 2 FOLDERS SKIPPED: LOOKED OFFLINE'
		);
	});

	it('says a scan failed, and why for an admin', () => {
		const failed: ScanResult = { ...finished, outcome: 'failed', error: 'disk full' };
		expect(lastScan(failed, ended + 60_000)).toEqual({
			title: 'FULL SCAN FAILED · 1 MIN AGO',
			took: null,
			detail: 'disk full'
		});
	});

	it('says a scan failed to anyone else, with no reason to give', () => {
		const failed: ScanResult = { ...finished, outcome: 'failed' };
		expect(lastScan(failed, ended).detail).toBe('It stopped before the end. An admin can see why.');
	});
});

describe('standing', () => {
	const wavs = { ...facts, issues: 16, issueCounts: { ...noCounts, unsupported: 16 } };

	it('agrees with the figure, as the board has it', () => {
		expect(standing(wavs)).toEqual({
			headline: "files Kitsune can't read yet.",
			detail: 'Everything else is in: 7,586 tracks, 927 albums.'
		});
	});

	it('is singular at 1', () => {
		const one = { ...facts, issues: 1, issueCounts: { ...noCounts, unsupported: 1 } };
		expect(standing(one).headline).toBe("file Kitsune can't read yet.");
	});

	it('is proud of a clean library', () => {
		expect(standing(facts)).toEqual({
			headline: 'Every file is in.',
			detail: '7,586 tracks, 927 albums. Nothing left out.'
		});
	});

	it('claims nothing about a scan still running', () => {
		const scanning: LibraryFacts = { ...facts, scan: running(10, 100) };
		expect(standing(scanning).headline).toBe('Nothing left out so far.');
		expect(allClear(scanning).note).toContain('So far');
	});

	it('has nothing to be proud of in an empty library', () => {
		const empty = { ...facts, albums: 0, tracks: 0 };
		expect(standing(empty).headline).toBe('Nothing to read yet.');
		expect(standing({ ...empty, roots: 0 }).detail).toContain('No music folders');
		expect(allClear(empty).title).toBe('Nothing to report');
		expect(standing({ ...wavs, albums: 0, tracks: 0 }).detail).toBe('Nothing else has been found.');
	});
});

describe('scanHelp', () => {
	it('says the Full scan will not touch what is listed, when something is', () => {
		const help = scanHelp({ ...facts, issues: 2 });
		expect(help).toContain("won't change the files below");
		expect(scanHelp({ ...facts, issueCounts: { ...noCounts, warnings: 1 } })).toBe(help);
	});

	it('has no files below to mention on a clean library', () => {
		expect(scanHelp(facts)).toBe(
			'Quick looks for new and changed files. Reading everything takes minutes.'
		);
	});
});

describe('refusalLine', () => {
	it('has a sentence for every refusal', () => {
		for (const refusal of ['forbidden', 'stopping', 'unavailable'] as const) {
			expect(refusalLine(refusal)).toMatch(/\.$/);
		}
	});
});

describe('issueGroups', () => {
	const wav = (path: string): ScanIssue => ({ path, kind: 'unsupported', message: 'nope' });

	it('names a group of one format as the board does', () => {
		const [group] = issueGroups({ ...noCounts, unsupported: 2 }, [wav('a.wav'), wav('b/c.WAV')]);
		expect(group).toMatchObject({ title: 'WAV files', tally: '2 · NOT SUPPORTED YET' });
		expect(group?.files).toHaveLength(2);
	});

	it('falls back on the kind for mixed formats and for anyone with no paths', () => {
		const counts = { ...noCounts, unsupported: 2 };
		expect(issueGroups(counts, [wav('a.wav'), wav('b.aiff')]).at(0)?.title).toBe(
			'Unsupported files'
		);
		expect(issueGroups(counts, []).at(0)).toMatchObject({
			title: 'Unsupported files',
			tally: '2 · NOT SUPPORTED YET',
			files: []
		});
	});

	it('leaves out empty kinds, and puts what can be fixed first', () => {
		const counts = { unreadable: 1, corrupted: 1, unsupported: 1, warnings: 5 };
		expect(issueGroups(counts, []).map(({ kind }) => kind)).toEqual([
			'unreadable',
			'corrupted',
			'unsupported'
		]);
		expect(issueGroups(noCounts, [])).toEqual([]);
	});

	it('never says a Full scan fixes anything', () => {
		const counts = { unreadable: 1, corrupted: 1, unsupported: 1, warnings: 0 };
		for (const { note } of issueGroups(counts, [])) {
			expect(note).not.toMatch(/full|read every/i);
		}
	});
});

describe('warningLine', () => {
	it('agrees at 1 and N', () => {
		expect(warningLine(1)).toBe('1 track read fine but left a note');
		expect(warningLine(1200)).toBe('1,200 tracks read fine but left a note');
	});
});

describe('filesState', () => {
	it('shows a loaded list, and nothing to anyone but an admin', () => {
		expect(filesState(true, 'ready', 3, false)).toBe('shown');
		expect(filesState(true, 'ready', 3, true)).toBe('shown');
		expect(filesState(true, 'unavailable', 3, false)).toBe('shown');
		expect(filesState(false, 'loading', 0, false)).toBe('hidden');
		expect(filesState(false, 'unavailable', 0, true)).toBe('hidden');
	});

	it('says the files are coming while they load, or while a list behind the counts is asked for', () => {
		expect(filesState(true, 'loading', 0, false)).toBe('loading');
		expect(filesState(true, 'ready', 0, false)).toBe('loading');
	});

	it('says when they could not be loaded', () => {
		expect(filesState(true, 'unavailable', 0, false)).toBe('failed');
		expect(filesState(true, 'missing', 0, false)).toBe('failed');
	});

	it('gives up on a list still behind the counts after asking again', () => {
		expect(filesState(true, 'ready', 0, true)).toBe('failed');
	});
});

describe('staleFor', () => {
	const file = (kind: ScanIssue['kind']): ScanIssue => ({ path: 'a/b.wav', kind, message: '' });

	it('is null while the list holds every counted kind', () => {
		expect(staleFor({ ...noCounts, unsupported: 16 }, [file('unsupported')])).toBeNull();
		expect(staleFor(noCounts, [])).toBeNull();
	});

	it('names the counts a list is behind, so it is asked for again once per count', () => {
		const counts = { ...noCounts, corrupted: 1, unsupported: 16 };
		expect(staleFor(counts, [file('corrupted')])).toBe('0:1:16');
		expect(staleFor({ ...counts, unsupported: 17 }, [file('corrupted')])).toBe('0:1:17');
	});
});

describe('splitPath', () => {
	it('puts the folders first, then the name', () => {
		expect(splitPath('Low/Things We Lost in the Fire/01 Sunflower.wav')).toEqual({
			folder: 'Low / Things We Lost in the Fire',
			name: '01 Sunflower.wav'
		});
	});

	it('has no folder for a file at the top of a root', () => {
		expect(splitPath('loose.wav')).toEqual({ folder: '', name: 'loose.wav' });
	});
});

describe('loudnessLine', () => {
	const none = { done: 0, total: 0, ungained: 0, measured: 0, unmeasurable: 0 };

	it('says nothing when every song has ReplayGain, or the server does not say', () => {
		expect(loudnessLine(undefined)).toBeNull();
		expect(loudnessLine(none)).toBeNull();
	});

	it('counts the songs without ReplayGain, measured and not', () => {
		expect(loudnessLine({ ...none, ungained: 1200, measured: 1000, unmeasurable: 40 })).toBe(
			"1,200 songs have no ReplayGain: 1,000 measured, 40 can't be measured yet."
		);
		expect(loudnessLine({ ...none, ungained: 1, measured: 1 })).toBe(
			'1 song has no ReplayGain: 1 measured.'
		);
	});

	it('says how far the pass has got while it runs', () => {
		expect(loudnessLine({ ...none, ungained: 9, measured: 2, done: 3, total: 9 })).toBe(
			'9 songs have no ReplayGain: 2 measured. Measuring in the background, 3 of 9.'
		);
	});
});
