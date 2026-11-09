import { count, emptyLine, issuesNoun, plural } from './home.ts';
import type { Load } from './library.svelte.ts';
import type {
	IssueCounts,
	IssueKind,
	LibraryFacts,
	LoudnessStatus,
	ScanIssue,
	ScanMode,
	ScanRefusal,
	ScanResult,
	ScanStatus
} from './source.ts';

const secondMs = 1000;
const minuteMs = 60 * secondMs;
const hourMs = 60 * minuteMs;
const dayMs = 24 * hourMs;

/** Before this long, the walk and the first slow reads are most of what a rate would measure. */
const settledMs = 10 * secondMs;
/** Before this share of the files, one big album can swing the estimate by minutes. */
const settledShare = 0.02;
/** How far an estimate may wander, in minutes, before the one on screen changes. */
const steadyWithin = 0.75;

const modeNames: Record<ScanMode, string> = { quick: 'QUICK SCAN', full: 'FULL SCAN' };

/** Where a running scan has got to, in the words above the bar. */
export function scanLine({ mode = 'full', read, total }: ScanStatus): string {
	const name = modeNames[mode];
	return total === 0 ? `${name} · STARTING` : `${name} · ${count(read)} OF ${count(total)}`;
}

/** How much of a running scan is done, in whole percent. */
export function scanPercent({ read, total }: ScanStatus): number {
	return total === 0 ? 0 : Math.min(100, Math.floor((read / total) * 100));
}

/** Minutes left of a running scan, from the rate so far, once there's enough of it to say. */
export function minutesLeft({ read, total, startedAt }: ScanStatus, nowMs: number): number | null {
	if (total === 0 || startedAt === undefined) {
		return null;
	}
	const elapsed = nowMs - Date.parse(startedAt);
	if (elapsed < settledMs || read < total * settledShare) {
		return null;
	}
	return (elapsed * Math.max(0, total - read)) / read / minuteMs;
}

/**
 * Holds a shown estimate until the real one has clearly moved. Files arrive in batches and the clock
 * ticks between them, so a raw estimate near a rounding edge flips every second.
 */
export function steadyMinutes(shown: number | null, minutes: number | null): number | null {
	if (minutes === null) {
		return null;
	}
	const moved = shown === null || Math.abs(minutes - shown) >= steadyWithin;
	return moved ? Math.round(minutes) : shown;
}

/** What's left of a running scan. Null is honest silence. */
export function timeLeft({ read, total }: ScanStatus, minutes: number | null): string | null {
	if (total > 0 && read >= total) {
		return 'FINISHING UP';
	}
	if (minutes === null) {
		return null;
	}
	return minutes < 1 ? 'UNDER A MINUTE LEFT' : `ABOUT ${count(minutes)} MIN LEFT`;
}

function ago(ms: number): string {
	if (ms < minuteMs) {
		return 'JUST NOW';
	}
	if (ms < hourMs) {
		return `${String(Math.floor(ms / minuteMs))} MIN AGO`;
	}
	return ms < 2 * dayMs
		? `${String(Math.floor(ms / hourMs))} H AGO`
		: `${String(Math.floor(ms / dayMs))} DAYS AGO`;
}

function took(ms: number): string {
	if (ms < secondMs) {
		return 'TOOK UNDER A SECOND';
	}
	return ms < minuteMs
		? `TOOK ${String(Math.round(ms / secondMs))} S`
		: `TOOK ${String(Math.round(ms / minuteMs))} MIN`;
}

/** The last scan to end, in the slot a running one's progress takes. */
export interface LastScan {
	title: string;
	/** How long it ran. A failed scan doesn't say. */
	took: string | null;
	detail: string;
}

/** A Full scan rewrites every track it reads, so its `updated` says nothing about what changed. */
function changes({ mode, added, updated, moved, missing, deleted }: ScanResult): string[] {
	const counted: [number, string][] = [
		[added, 'ADDED'],
		[mode === 'quick' ? updated : 0, 'UPDATED'],
		[moved, 'MOVED'],
		[missing, 'MISSING'],
		[deleted, 'DELETED']
	];
	return counted.filter(([value]) => value > 0).map(([value, word]) => `${count(value)} ${word}`);
}

function outcome(last: ScanResult): string {
	const skipped =
		last.skippedRoots === 0
			? []
			: [`${plural(last.skippedRoots, 'FOLDER', 'FOLDERS')} SKIPPED: LOOKED OFFLINE`];
	const changed = changes(last);
	const quiet = changed.length === 0 && skipped.length === 0;
	return (
		last.mode === 'full'
			? [`${plural(last.read, 'FILE', 'FILES')} READ`, ...skipped, ...changed]
			: [plural(last.seen, 'FILE', 'FILES'), ...skipped, ...(quiet ? ['NOTHING CHANGED'] : changed)]
	).join(' · ');
}

/** What the last scan did. Only an administrator's copy of a failure says why. */
export function lastScan(last: ScanResult, nowMs: number): LastScan {
	const when = ago(nowMs - Date.parse(last.endedAt));
	const name = modeNames[last.mode];
	if (last.outcome === 'failed') {
		return {
			title: `${name} FAILED · ${when}`,
			took: null,
			detail: last.error ?? 'It stopped before the end. An admin can see why.'
		};
	}
	return {
		title: `${name} · ${when}`,
		took: took(Date.parse(last.endedAt) - Date.parse(last.startedAt)),
		detail: outcome(last)
	};
}

/** The fox panel's words under the figure. They must agree with it at 0, 1 and N. */
export function standing(facts: LibraryFacts): { headline: string; detail: string } {
	const { issues, tracks, albums, scan } = facts;
	const running = scan.state === 'running';
	const library = `${plural(tracks, 'track', 'tracks')}, ${plural(albums, 'album', 'albums')}`;
	if (issues > 0) {
		return {
			headline: `${issuesNoun(facts.issueCounts)}.`,
			detail: tracks > 0 ? `Everything else is in: ${library}.` : 'Nothing else has been found.'
		};
	}
	if (tracks === 0) {
		return { headline: 'Nothing to read yet.', detail: emptyLine(facts, running) };
	}
	return running
		? { headline: 'Nothing left out so far.', detail: `${library}, and still looking.` }
		: { headline: 'Every file is in.', detail: `${library}. Nothing left out.` };
}

/** The issues panel with nothing to list: a result, said calmly. */
export function allClear({ tracks, roots, scan }: LibraryFacts): { title: string; note: string } {
	if (tracks === 0) {
		return { title: 'Nothing to report', note: 'When a scan finds trouble, it shows up here.' };
	}
	const folders = plural(roots, 'folder', 'folders');
	return scan.state === 'running'
		? { title: 'Nothing needs you', note: `So far, every file in your ${folders} has gone in.` }
		: {
				title: 'Nothing needs you',
				note: `Kitsune read every file in your ${folders}. If that changes, it will show up here.`
			};
}

/** Why the Full button is there, and that it won't touch what's listed. */
export function scanHelp({ issues, issueCounts }: LibraryFacts): string {
	const quick = 'Quick looks for new and changed files.';
	return issues + issueCounts.warnings > 0
		? `${quick} Reading everything takes minutes and won't change the files below.`
		: `${quick} Reading everything takes minutes.`;
}

/**
 * What Health says of levelling: how many songs have no ReplayGain, and how many of those Kitsune
 * has measured or can't yet. Null when every song has ReplayGain, or the server doesn't say.
 */
export function loudnessLine(status: LoudnessStatus | undefined): string | null {
	if (status === undefined || status.ungained === 0) {
		return null;
	}
	const { ungained, measured, unmeasurable, done, total } = status;
	const songs = plural(ungained, 'song has', 'songs have');
	const cant = unmeasurable === 0 ? '' : `, ${count(unmeasurable)} can't be measured yet`;
	const running =
		total > 0 ? ` Measuring in the background, ${count(done)} of ${count(total)}.` : '';
	return `${songs} no ReplayGain: ${count(measured)} measured${cant}.${running}`;
}

const refusals: Record<ScanRefusal, string> = {
	forbidden: 'Only an admin can start a scan.',
	stopping: "The server is shutting down, so the scan didn't start.",
	unavailable: "The server didn't answer, so no scan started."
};

/** Why a scan didn't start. */
export function refusalLine(refusal: ScanRefusal): string {
	return refusals[refusal];
}

/** One kind of issue, as the panel shows it. */
export interface IssueGroup {
	kind: IssueKind;
	title: string;
	tally: string;
	note: string;
	/** Empty for a non-admin, or until the list has loaded. */
	files: ScanIssue[];
}

const kinds: Record<IssueKind, Omit<IssueGroup, 'files' | 'kind' | 'tally'> & { word: string }> = {
	unreadable: {
		title: "Files Kitsune couldn't open",
		word: 'COULD NOT OPEN',
		note: 'A disk or permission problem got in the way. Kitsune tries them again on every scan.'
	},
	corrupted: {
		title: 'Damaged files',
		word: 'DAMAGED',
		note: "Something inside them is broken, so they aren't in your library. Kitsune looks again when a file changes."
	},
	unsupported: {
		title: 'Unsupported files',
		word: 'NOT SUPPORTED YET',
		note: "They're safe where they are. Kitsune just can't read them, so they aren't in your library."
	}
};

/** One extension across every file names the group, as the board does: `WAV files`. */
function formatOf(files: readonly ScanIssue[]): string | null {
	const extensions = new Set(
		files.map(({ path }) => path.slice(path.lastIndexOf('.') + 1).toUpperCase())
	);
	return extensions.size === 1 ? ([...extensions].at(0) ?? null) : null;
}

/** Groups what's wrong by kind: what someone can fix first, what only a release can last. */
export function issueGroups(counts: IssueCounts, files: readonly ScanIssue[]): IssueGroup[] {
	const order: IssueKind[] = ['unreadable', 'corrupted', 'unsupported'];
	return order
		.filter((kind) => counts[kind] > 0)
		.map((kind) => {
			const { title, word, note } = kinds[kind];
			const ofKind = files.filter((file) => file.kind === kind);
			const format = kind === 'unsupported' ? formatOf(ofKind) : null;
			return {
				kind,
				title: format === null ? title : `${format} files`,
				tally: `${count(counts[kind])} · ${word}`,
				note,
				files: ofKind
			};
		});
}

/** The note about tracks that read with warnings. They play, so this is never an alarm. */
export function warningLine(warnings: number): string {
	return `${plural(warnings, 'track', 'tracks')} read fine but left a note`;
}

/** What sits under a group's heading in place of its files, or that they're there. */
export type FilesState = 'hidden' | 'loading' | 'failed' | 'shown';

/**
 * Whether a group can show its files. A list holding none of a kind the counts have is behind
 * them, and is asked for once more (see `staleFor`): that reads as loading until the answer comes,
 * and as failed if it is still behind (`gaveUp`), rather than loading forever.
 */
export function filesState(
	admin: boolean,
	load: Load<unknown>['kind'],
	files: number,
	gaveUp: boolean
): FilesState {
	if (!admin) {
		return 'hidden';
	}
	if (files > 0) {
		return 'shown';
	}
	if (load === 'loading' || (load === 'ready' && !gaveUp)) {
		return 'loading';
	}
	return 'failed';
}

/**
 * The counts a loaded list has fallen behind, as a key to ask again for once, or null. A scan
 * finds files between loads, so the counts can have a kind the list holds none of.
 */
export function staleFor(counts: IssueCounts, files: readonly ScanIssue[]): string | null {
	const kinds: IssueKind[] = ['unreadable', 'corrupted', 'unsupported'];
	const behind = kinds.some(
		(kind) => counts[kind] > 0 && !files.some((file) => file.kind === kind)
	);
	return behind ? kinds.map((kind) => String(counts[kind])).join(':') : null;
}

/** A path under a root as a list shows it: the folders it sits in, then its own name. */
export function splitPath(path: string): { folder: string; name: string } {
	const parts = path.split('/').filter((part) => part !== '');
	return { folder: parts.slice(0, -1).join(' / '), name: parts.at(-1) ?? path };
}
