/** The words the Organize page says, worked out here so they can be held to the boards. */

import { defaultTemplate } from './template.ts';
import type {
	FileKind,
	LeftReason,
	OrganizeAlbum,
	OrganizeClaimant,
	OrganizeCounts,
	OrganizeRun
} from './types.ts';

const n = (count: number): string => count.toLocaleString('en-US');
const songs = (count: number): string => `${n(count)} ${count === 1 ? 'song' : 'songs'}`;
const SONGS = (count: number): string => `${n(count)} ${count === 1 ? 'SONG' : 'SONGS'}`;

/** A preset: what fills the field, and its name in the menu. */
export interface Preset {
	name: string;
	template: string;
}

/** The presets, the library's own shape first. Each fills the field, which stays editable. */
export const presets: readonly Preset[] = [
	{ name: 'Artist / Year - Album / NN - Title', template: defaultTemplate },
	{ name: 'Artist / Album / NN Title', template: '<Album Artist>/<Album>/<Track> <Title>' },
	{
		name: 'MusicBee: Artist / Album / D-NN Title',
		template: '<Album Artist>/<Album>/[<Disc>-]<Track> <Title>'
	},
	{
		name: 'Artist / Year - Album / NN. Title',
		template: '<Album Artist>/[<Year> - ]<Album>/<Track>. <Title>'
	}
];

/** A template's name when it's a preset, else the template itself. */
export function templateName(template: string): string {
	return presets.find((p) => p.template === template)?.name ?? template;
}

/** The mono line under the title: "7,604 SONGS · 5,070 IN PLACE · 2,534 WOULD MOVE · 66 IN CONFLICT". */
export function countsLine(c: OrganizeCounts, compact = false): string {
	const parts = [
		...(compact ? [] : [SONGS(c.songs)]),
		`${n(c.inPlace)} IN PLACE`,
		`${n(c.moves)} WOULD MOVE`,
		`${n(c.conflicts)} IN CONFLICT`
	];
	if (c.skipped > 0) {
		parts.push(`${n(c.skipped)} SKIPPED`);
	}
	return parts.join(' · ');
}

/** "Organizing 12 songs from Fangs!", for a preview of some songs. */
export function scopeWords(scope: { songs: number; albums: number; album?: string }): string {
	if (scope.album !== undefined) {
		return `Organizing ${songs(scope.songs)} from ${scope.album}`;
	}
	return `Organizing ${songs(scope.songs)} from ${n(scope.albums)} ${scope.albums === 1 ? 'album' : 'albums'}`;
}

/** The dock's words: "Move 2,534 songs", and what stays where it is. */
export function dockWords(
	moves: number,
	c: OrganizeCounts,
	picked: boolean
): { title: string; line: string } {
	const stays = [`${n(c.conflicts)} IN CONFLICT STAY WHERE THEY ARE`];
	if (!picked) {
		stays.push(`${n(c.inPlace)} ALREADY IN PLACE`);
	}
	return { title: `Move ${songs(moves)}`, line: stays.join(' · ') };
}

/** The toast when the songs a person picked are already in place. */
export function inPlaceWords(count: number): string {
	return count === 1
		? 'This song is already where the template puts it.'
		: `These ${n(count)} songs are already where the template puts them.`;
}

/** A song's chip in the preview. */
export const fileChip: Record<FileKind, string> = {
	rename: 'RENAME',
	move: 'MOVE',
	conflict: 'CONFLICT',
	skipped: 'SKIPPED',
	inPlace: 'IN PLACE'
};

/** An album's chip: what the songs listed do together, else what its songs do. */
export function albumChip(
	album: OrganizeAlbum,
	listed: readonly { kind: FileKind }[] = []
): string {
	const kinds = new Set(listed.map(({ kind }) => kind));
	const [only] = kinds;
	const kind = kinds.size === 1 && only !== undefined ? only : album.kind;
	switch (kind) {
		case 'move':
			return 'NEW FOLDER';
		case 'mixed':
			return listed.some((f) => f.kind === 'conflict') ? 'CONFLICT' : 'MOVES';
		default:
			return fileChip[kind];
	}
}

/** Why a song stays where it is, in the preview's and a run's mono. */
export const reasonWords: Record<LeftReason, string> = {
	claimed: 'ANOTHER SONG IS THERE OR WANTS IT',
	unknownFile: 'A FILE KITSUNE DOESN’T PLAY IS THERE',
	noTags: 'NO TAGS FOR THE TEMPLATE',
	tooLong: 'THE PATH WOULD BE TOO LONG',
	otherFilesystem: 'IN ANOTHER FILESYSTEM',
	changed: 'CHANGED SINCE THE PREVIEW',
	missing: 'FILE IS MISSING',
	failed: 'THE MOVE FAILED · THE LOG SAYS WHY',
	movedSince: 'MOVED AGAIN SINCE THE RUN',
	taken: 'SOMETHING IS AT ITS OLD PATH'
};

/** The mono line under an album: who, when, how many, and what happens. */
export function albumLine(album: OrganizeAlbum, year?: number): string {
	const parts = [album.artist.toUpperCase()];
	if (year !== undefined && year > 0) {
		parts.push(String(year));
	}
	parts.push(SONGS(album.songs));
	if (album.caseOnly === true) {
		parts.push('CASE ONLY, DONE IN TWO STEPS');
	} else if (album.kind === 'rename') {
		parts.push('SAME FOLDER');
	} else if (album.fromFolder !== undefined && !album.fromFolder.includes('/')) {
		parts.push('NO ALBUM FOLDER YET');
	}
	return parts.join(' · ');
}

/** "+ cover.jpg, Fangs!.cue": what comes along. */
export function companionsLine(companions: readonly { from: string }[]): string {
	return `+ ${companions.map(({ from }) => from.slice(from.lastIndexOf('/') + 1)).join(', ')}`;
}

/** A claimant's facts on a conflict card: "FLAC · 1,020 KB/S · 5:21 · ADDED MAR 2025 · ALREADY THERE". */
export function claimantFacts(c: OrganizeClaimant): string {
	const seconds = Math.round(c.durationMs / 1000);
	const length = `${String(Math.floor(seconds / 60))}:${String(seconds % 60).padStart(2, '0')}`;
	const added = new Date(c.addedAt)
		.toLocaleDateString('en-US', { month: 'short', year: 'numeric', timeZone: 'UTC' })
		.toUpperCase();
	const parts = [c.format.toUpperCase()];
	if (c.bitrate !== undefined) {
		parts.push(`${n(c.bitrate)} KB/S`);
	}
	parts.push(length);
	if (c.plays > 0) {
		parts.push(`${n(c.plays)} ${c.plays === 1 ? 'PLAY' : 'PLAYS'}`);
	}
	if (c.stars > 0) {
		parts.push(`${String(c.stars)}★`);
	}
	parts.push(`ADDED ${added}`);
	parts.push(c.here ? 'ALREADY THERE' : 'WANTS TO MOVE THERE');
	return parts.join(' · ');
}

/** "3 MIN 12 S", "45 S", "1 H 2 MIN". */
export function tookWords(ms: number): string {
	const seconds = Math.max(0, Math.round(ms / 1000));
	if (seconds < 60) {
		return `${String(seconds)} S`;
	}
	const minutes = Math.floor(seconds / 60);
	if (minutes < 60) {
		return `${String(minutes)} MIN ${String(seconds % 60)} S`;
	}
	return `${String(Math.floor(minutes / 60))} H ${String(minutes % 60)} MIN`;
}

/** "TODAY · 21:14", "YESTERDAY · 09:02", "NOV 12 · 21:14": when, by the browser's clock. */
export function whenWords(iso: string, now: number): string {
	const at = new Date(iso);
	const time = at.toLocaleTimeString('en-GB', { hour: '2-digit', minute: '2-digit' });
	const day = (d: Date): number =>
		Math.floor((d.getTime() - d.getTimezoneOffset() * 60_000) / 86_400_000);
	const ago = day(new Date(now)) - day(at);
	if (ago === 0) {
		return `TODAY · ${time}`;
	}
	if (ago === 1) {
		return `YESTERDAY · ${time}`;
	}
	return `${at.toLocaleDateString('en-US', { month: 'short', day: 'numeric' }).toUpperCase()} · ${time}`;
}

/** The run head's title: "Moving 1,214 of 2,534", or for an undo "Putting back …". */
export function headWords(run: Pick<OrganizeRun, 'kind' | 'done' | 'total'>): string {
	return `${run.kind === 'undo' ? 'Putting back' : 'Moving'} ${n(run.done)} of ${n(run.total)}`;
}

/** The pace so far, and what's left at it: "640 SONGS A MINUTE · ABOUT 2 MIN LEFT". */
export function paceWords(done: number, total: number, elapsedMs: number): string {
	if (done <= 0 || elapsedMs <= 0) {
		return 'STARTING';
	}
	const perMinute = (done / elapsedMs) * 60_000;
	const leftMinutes = (total - done) / perMinute;
	const left =
		total - done <= 0
			? 'ALMOST DONE'
			: leftMinutes < 1
				? 'UNDER A MINUTE LEFT'
				: `ABOUT ${String(Math.round(leftMinutes))} MIN LEFT`;
	return `${left} · ${n(Math.round(perMinute))} SONGS A MINUTE`;
}

/** The finished card: kicker, title and body. */
export function doneWords(
	run: OrganizeRun,
	now: number
): { kicker: string; title: string; body: string } {
	const when = whenWords(run.finishedAt ?? run.startedAt, now);
	const took =
		run.finishedAt === undefined
			? ''
			: tookWords(Date.parse(run.finishedAt) - Date.parse(run.startedAt));
	const conflicts =
		run.conflicts === 0
			? ''
			: `${n(run.conflicts)} in conflict ${run.conflicts === 1 ? 'stayed where it is' : 'stayed where they are'}; open Conflicts to compare them. `;
	const failed =
		run.failed === 0 ? 'Nothing failed. ' : `${songs(run.failed)} couldn’t move and stayed put. `;
	const left =
		run.left === 0
			? ''
			: `${songs(run.left)} stayed where ${run.left === 1 ? 'it was' : 'they were'}, named below. `;
	if (run.kind === 'undo') {
		return {
			kicker: `PUT BACK · ${when}${took === '' ? '' : ` · TOOK ${took}`}`,
			title: `Put back ${songs(run.moved)}`,
			body: `${left}Every song that could go back is where it was before the run, and the folders it made are gone. Plays, stars and playlists never moved.`
		};
	}
	if (run.state === 'undone') {
		return {
			kicker: `UNDONE · ${when}`,
			title: `Put back ${songs(run.moved)}`,
			body: 'Every song is where it was before the run, and the folders it made are gone. Your plays, stars and playlists never moved.'
		};
	}
	const stopped = run.state === 'stopped';
	return {
		kicker: stopped
			? `STOPPED · ${when} · AT AN ALBUM’S END`
			: `FINISHED · ${when}${took === '' ? '' : ` · TOOK ${took}`}`,
		title: `Moved ${songs(run.moved)}`,
		body: stopped
			? `The rest stay where they were. Preview again to carry on: the songs already moved count as in place. ${conflicts}`.trim()
			: `${conflicts}${failed}${left}Every song kept its plays, stars and playlist places.`.trim()
	};
}

/** "Moved 2,468 songs into 312 folders · 66 conflicts stayed · 0 failed". */
export function summaryWords(run: OrganizeRun): string {
	return [
		`Moved ${songs(run.moved)} into ${n(run.folders)} ${run.folders === 1 ? 'folder' : 'folders'}`,
		`${n(run.conflicts)} ${run.conflicts === 1 ? 'conflict' : 'conflicts'} stayed`,
		`${n(run.failed)} failed`
	].join(' · ');
}

/** A run in the runs list: what it did, and where it stands. */
export function runRowWords(run: OrganizeRun): { what: string; state: string } {
	const what =
		run.kind === 'undo'
			? `Put back ${songs(run.moved)}`
			: run.kind === 'edit'
				? `Moved ${songs(run.moved)} after a tag edit`
				: `Moved ${songs(run.moved)} · ${templateName(run.template)}`;
	const state =
		run.state === 'running'
			? 'RUNNING'
			: run.undoable
				? 'UNDO ›'
				: run.state === 'undone'
					? 'UNDONE'
					: run.state === 'stopped'
						? 'STOPPED'
						: 'DONE';
	return { what, state };
}

/** "Put 2,468 songs back where they were?" */
export function undoAsk(run: OrganizeRun): string {
	return `Put ${songs(run.moved)} back where ${run.moved === 1 ? 'it was' : 'they were'}?`;
}

/** The nav's note while a run goes: "ORGANIZING · 48%". */
export function navNote(done: number, total: number): string {
	return `ORGANIZING · ${String(total <= 0 ? 0 : Math.floor((done / total) * 100))}%`;
}

/** One part of a path, and whether it changed. */
export interface PathPart {
	text: string;
	changed: boolean;
}

/**
 * The parts of an old and a new path, split at folders, each marked where it changes: a folder
 * that stays the same at the same depth is unchanged, and so is the file's name when it and the
 * depth both are. The old one's changed parts are struck through; the new one's are in the fox.
 */
export function pathDiff(from: string, to: string): { old: PathPart[]; new: PathPart[] } {
	const a = from.split('/');
	const b = to.split('/');
	const sameDepth = a.length === b.length;
	const mark = (mine: string[], theirs: string[]): PathPart[] =>
		mine.map((s, i) => {
			const last = i === mine.length - 1;
			return {
				text: s + (last ? '' : '/'),
				changed: !(s === theirs[i] && (!last || sameDepth))
			};
		});
	return { old: mark(a, b), new: mark(b, a) };
}
