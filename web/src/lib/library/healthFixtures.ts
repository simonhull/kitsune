import {
	ServerError,
	type IssueCounts,
	type LibraryFacts,
	type ScanIssue,
	type ScanIssues,
	type ScanMode,
	type ScanResult,
	type ScanStatus,
	type TrackWarned,
	type User
} from './source.ts';

/** Where the Health page opens: `?source=fixtures&health=…`, and `&role=user` for a non-admin. */
export type HealthFixture = 'idle' | 'starting' | 'running' | 'clean' | 'mixed' | 'failed';

const fixtures: readonly HealthFixture[] = [
	'idle',
	'starting',
	'running',
	'clean',
	'mixed',
	'failed'
];

/** Reads the state from the URL. Anything else opens on the board's. */
export function healthFixture(name: string | null): HealthFixture {
	return fixtures.find((fixture) => fixture === name) ?? 'idle';
}

/** What the real library holds: a Full scan reads them all. */
const fileCount = 7602;
/** How fast the real library reads, in files a millisecond. */
const fullPace = fileCount / 414_000;
/** How long the survey takes before a Full scan knows its total. */
const walkMs = 2000;
/** What a Quick scan finds to read, and how fast. */
const quickFiles = 12;
const quickPace = 6 / 1000;
/** How far the board's scan has got: 3,120 of 7,602. */
const boardElapsedMs = Math.ceil(3120 / fullPace);
const minuteMs = 60_000;

const wavs: readonly string[] = [
	'Low Tide Choir/Demos/01 Breakwater.wav',
	'Low Tide Choir/Demos/02 Panes.wav',
	'Mara Quill/Rough Mixes/Lowlands.wav',
	'Nadia Brandt/Live/Orbitals.wav',
	'Unsorted/Untitled 3.wav',
	'Unsorted/Untitled 4.wav',
	...Array.from({ length: 10 }, (_, index) => `Unsorted/Bounce ${String(index + 1)}.wav`)
];

function unsupported(path: string): ScanIssue {
	return { path, kind: 'unsupported', message: 'unsupported format' };
}

const boardIssues = wavs.map(unsupported);

const boardWarning: TrackWarned = {
	trackId: 'glasshouse-66-7',
	path: 'Low Tide Choir/Glasshouse/07 Low Tide.flac',
	messages: ['ID3v2 frame TDRC holds a date that could not be parsed']
};

const mixedIssues: ScanIssue[] = [
	...boardIssues.slice(0, 5),
	unsupported('Unsorted/Tape Transfer.aiff'),
	{
		path: 'Vanta Park/Red Signal/03 Static.flac',
		kind: 'corrupted',
		message: 'corrupted file: FLAC stream header is truncated'
	},
	{
		path: 'Vanta Park/Red Signal/04 Static (Reprise).flac',
		kind: 'corrupted',
		message: 'corrupted file: FLAC stream header is truncated'
	},
	{
		path: 'A Very Long Artist Name Indeed/An Even Longer Album Name With Parentheses (Deluxe Edition) [Disc 2]/17 The Longest Song Title Anyone Has Tagged.mp3',
		kind: 'corrupted',
		message: 'corrupted file: no MPEG frame found'
	},
	{
		path: 'Ilse Varga/Quiet Machines/01 Hum.flac',
		kind: 'unreadable',
		message: 'permission denied'
	}
];

const mixedWarnings: TrackWarned[] = [
	boardWarning,
	{
		trackId: 'orbitals-14-2',
		path: 'Nadia Brandt/Orbitals/02 Apogee.flac',
		messages: ['Vorbis comment TRACKNUMBER is not a number', 'embedded picture is empty']
	}
];

function tally({ issues, warnings }: ScanIssues): IssueCounts {
	const of = (wanted: ScanIssue['kind']): number =>
		issues.filter(({ kind }) => kind === wanted).length;
	return {
		unreadable: of('unreadable'),
		corrupted: of('corrupted'),
		unsupported: of('unsupported'),
		warnings: warnings.length
	};
}

function listFor(fixture: HealthFixture): ScanIssues {
	if (fixture === 'clean') {
		return { issues: [], warnings: [] };
	}
	return fixture === 'mixed'
		? { issues: mixedIssues, warnings: mixedWarnings }
		: { issues: boardIssues, warnings: [boardWarning] };
}

/** A scan on the clock. */
interface Run {
	mode: ScanMode;
	startedAt: number;
	/** How long it walks before it has a total. */
	walkMs: number;
	total: number;
	pace: number;
}

function runOf(mode: ScanMode, startedAt: number): Run {
	return mode === 'full'
		? { mode, startedAt, walkMs, total: fileCount, pace: fullPace }
		: { mode, startedAt, walkMs, total: quickFiles, pace: quickPace };
}

function resultOf(run: Run, endedAt: number): ScanResult {
	return {
		mode: run.mode,
		outcome: 'finished',
		startedAt: new Date(run.startedAt).toISOString(),
		endedAt: new Date(endedAt).toISOString(),
		seen: fileCount,
		read: run.total,
		added: 0,
		updated: 0,
		moved: 0,
		missing: 0,
		deleted: 0,
		issues: 0,
		skippedRoots: 0
	};
}

function openingLast(fixture: HealthFixture, now: number): ScanResult {
	const startedAt = now - 19 * minuteMs;
	const finished = resultOf(runOf('full', startedAt), now - 12 * minuteMs);
	return fixture === 'failed'
		? {
				...finished,
				outcome: 'failed',
				read: 2214,
				error: 'write batch: database or disk is full'
			}
		: finished;
}

function openingRun(fixture: HealthFixture, now: number): Run | null {
	if (fixture === 'running') {
		return { ...runOf('full', now - boardElapsedMs), walkMs: 0 };
	}
	return fixture === 'starting' ? runOf('full', now) : null;
}

export interface HealthScans {
	facts(): Pick<LibraryFacts, 'issues' | 'issueCounts' | 'scan'>;
	/** Rejects as the server would: 403 for a non-admin, 409 with a scan running. */
	start(mode: ScanMode): ScanStatus;
	issues(): ScanIssues;
}

/**
 * The scanner through the contract, as a function of the clock. A scan from the buttons runs at the
 * real library's pace, so a Full one takes the same seven minutes.
 */
export function createHealthFixture(
	fixture: HealthFixture,
	role: User['role'],
	clock: () => number = Date.now
): HealthScans {
	const list = listFor(fixture);
	let run = openingRun(fixture, clock());
	let last = openingLast(fixture, clock());

	function forCaller(result: ScanResult): ScanResult {
		const { error, ...rest } = result;
		return role === 'admin' && error !== undefined ? { ...rest, error } : rest;
	}

	function status(): ScanStatus {
		const now = clock();
		if (run !== null) {
			const reading = now - run.startedAt - run.walkMs;
			const read = Math.floor(reading * run.pace);
			if (read >= run.total) {
				last = resultOf(run, run.startedAt + run.walkMs + run.total / run.pace);
				run = null;
			} else {
				return {
					state: 'running',
					mode: run.mode,
					read: Math.max(0, read),
					total: reading < 0 ? 0 : run.total,
					startedAt: new Date(run.startedAt).toISOString(),
					last: forCaller(last)
				};
			}
		}
		return { state: 'idle', read: 0, total: 0, last: forCaller(last) };
	}

	return {
		facts() {
			const issueCounts = tally(list);
			return { issues: list.issues.length, issueCounts, scan: status() };
		},
		start(mode) {
			if (role !== 'admin') {
				throw new ServerError(403);
			}
			if (status().state === 'running') {
				throw new ServerError(409);
			}
			run = runOf(mode, clock());
			return status();
		},
		issues() {
			if (role !== 'admin') {
				throw new ServerError(403);
			}
			return list;
		}
	};
}
