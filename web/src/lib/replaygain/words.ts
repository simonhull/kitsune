/** What the ReplayGain card, the album ⋯, its confirm and its toast say, held to the boards here. */

import { tookWords, whenWords } from '#lib/organize/words.ts';
import type { GainAlbum, GainLeft, GainLeftReason, GainRun, GainRunAlbum } from './types.ts';

const n = (count: number): string => count.toLocaleString('en-US');
const plural = (count: number, one: string, many: string): string =>
	`${n(count)} ${count === 1 ? one : many}`;
const songs = (count: number): string => plural(count, 'song', 'songs');
const files = (count: number): string => plural(count, 'file', 'files');

/** "FLAC", "FLAC AND MP3", "FLAC, MP3 AND WAV". */
export function formatsWords(formats: readonly string[]): string {
	const upper = formats.map((format) => format.toUpperCase());
	const last = upper.at(-1);
	if (last === undefined) {
		return '';
	}
	return upper.length === 1 ? last : `${upper.slice(0, -1).join(', ')} AND ${last}`;
}

/** "ON 90 ALBUMS · FLAC AND MP3". */
export function whereLine(albums: number, formats: readonly string[]): string {
	const on = `ON ${plural(albums, 'ALBUM', 'ALBUMS')}`;
	const kinds = formatsWords(formats);
	return kinds === '' ? on : `${on} · ${kinds}`;
}

/** "+ 87 MORE", the albums past the chips; empty when the chips are all of them. */
export function moreWords(albums: number, shown: number): string {
	return albums > shown ? `+ ${n(albums - shown)} MORE` : '';
}

/** A phone's one line for the biggest albums: "BIGGEST: RAYMAN 3 (292), RAYMAN 2 (167)". */
export function biggestLine(biggest: readonly GainAlbum[]): string {
	if (biggest.length === 0) {
		return '';
	}
	return `BIGGEST: ${biggest.map((a) => `${a.title.toUpperCase()} (${n(a.songs)})`).join(', ')}`;
}

/** Under the number: what it counts. */
export function levelledWords(count: number): string {
	return count === 1 ? 'song is levelled only in Kitsune' : 'songs are levelled only in Kitsune';
}

/** The card's button: "Write ReplayGain to 1,614 files". */
export function writeWords(count: number): string {
	return `Write ReplayGain to ${files(count)}`;
}

/** What the loudness pass hasn't got to: "212 STILL BEING MEASURED · WRITE THEM ONCE THEY ARE". */
export function waitingLine(waiting: number, unmeasurable: number): string {
	const parts: string[] = [];
	if (waiting > 0) {
		parts.push(
			`${n(waiting)} STILL BEING MEASURED · WRITE ${waiting === 1 ? 'IT ONCE IT IS' : 'THEM ONCE THEY ARE'}`
		);
	}
	if (unmeasurable > 0) {
		parts.push(`${n(unmeasurable)} CAN’T BE MEASURED YET`);
	}
	return parts.join(' · ');
}

/** The run head's title: "Writing 612 of 1,614", or for an undo "Taking out 12 of 1,609". */
export function headWords(run: Pick<GainRun, 'kind' | 'done' | 'total'>): string {
	return `${run.kind === 'undo' ? 'Taking out' : 'Writing'} ${n(run.done)} of ${n(run.total)}`;
}

/** Rounded as a person says a rate: two figures, so 1,034 is "1,000". */
function roughly(value: number): number {
	if (value < 100) {
		return Math.max(1, Math.round(value));
	}
	const unit = 10 ** (Math.floor(Math.log10(value)) - 1);
	return Math.round(value / unit) * unit;
}

/** The pace so far and what's left at it: "ABOUT 1 MIN LEFT · ABOUT 1,000 FILES A MINUTE". */
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
				: `ABOUT ${n(Math.round(leftMinutes))} MIN LEFT`;
	return `${left} · ABOUT ${n(roughly(perMinute))} ${roughly(perMinute) === 1 ? 'FILE' : 'FILES'} A MINUTE`;
}

/** The head's last line: the album in hand, and what Stop does. */
export function nowLine(title: string): string {
	const rest = 'STOP FINISHES THIS ALBUM, THEN STOPS · MUSIC KEEPS PLAYING';
	return title === '' ? rest : `NOW · ${title.toUpperCase()} · ${rest}`;
}

/** The nav's note while a run goes: "REPLAYGAIN · 38%". */
export function navNote(done: number, total: number): string {
	return `REPLAYGAIN · ${String(total <= 0 ? 0 : Math.floor((done / total) * 100))}%`;
}

/** An album row's mono line: "ARCHITECTS · 2025 · FLAC · 12 SONGS". */
export function albumLine(album: GainAlbum): string {
	const parts = [album.artist.toUpperCase()];
	if (album.year !== undefined && album.year > 0) {
		parts.push(String(album.year));
	}
	const kinds = formatsWords(album.formats);
	if (kinds !== '') {
		parts.push(kinds);
	}
	parts.push(plural(album.songs, 'SONG', 'SONGS'));
	return parts.join(' · ');
}

/** An album row's chip in a run. */
export function albumChip(state: GainRunAlbum['state'], undo = false): string {
	switch (state) {
		case 'written':
			return undo ? 'TAKEN OUT ✓' : 'WRITTEN ✓';
		case 'writing':
			return undo ? 'TAKING OUT' : 'WRITING';
		case 'waiting':
			return 'WAITING';
		case 'left':
			return 'LEFT AS IT WAS';
	}
}

/** Why a song was left: its chip, and what happens to it next. */
export const leftWords: Record<GainLeftReason, { chip: string; then: string }> = {
	changed: {
		chip: 'CHANGED SINCE THE SCAN',
		then: 'THE NEXT SCAN READS IT, THE PASS MEASURES IT AGAIN'
	},
	unwritable: { chip: 'CAN’T BE WRITTEN', then: 'THE FILE WOULDN’T TAKE IT · THE LOG SAYS WHY' },
	stale: { chip: 'MEASURE OUT OF DATE', then: 'ITS ALBUM CHANGED · IT WAITS FOR THE PASS' },
	trashed: {
		chip: 'IN THE TRASH',
		then: 'TRASHED WHILE THE RUN WENT · RESTORED, IT CAN BE WRITTEN'
	},
	missing: { chip: 'FILE IS MISSING', then: 'THE NEXT SCAN LOOKS FOR IT' },
	retagged: {
		chip: 'RETAGGED SINCE',
		then: 'ITS REPLAYGAIN ISN’T WHAT THE RUN WROTE · IT KEEPS ITS OWN'
	},
	interrupted: { chip: 'THE SERVER STOPPED', then: 'IT STOPPED BEFORE THIS ONE WAS WRITTEN' },
	failed: { chip: 'THE WRITE FAILED', then: 'THE LOG SAYS WHY' }
};

/** Under the first few: "+ 2 MORE · CHANGED SINCE THE SCAN", each reason once, most first. */
export function moreLeftLine(rest: readonly GainLeft[], beyond = 0): string {
	const total = rest.length + beyond;
	if (total <= 0) {
		return '';
	}
	const counts = new Map<GainLeftReason, number>();
	for (const { reason } of rest) {
		counts.set(reason, (counts.get(reason) ?? 0) + 1);
	}
	const reasons = [...counts.entries()]
		.sort((a, b) => b[1] - a[1])
		.map(([reason]) => leftWords[reason].chip);
	return [`+ ${n(total)} MORE`, ...reasons].join(' · ');
}

/** "LEFT AS THEY WERE · 5". */
export function leftHeading(count: number): string {
	return `${count === 1 ? 'LEFT AS IT WAS' : 'LEFT AS THEY WERE'} · ${n(count)}`;
}

/** What a finished run's card says: its kicker, title and body. */
export function doneWords(
	run: GainRun,
	now: number
): { kicker: string; title: string; body: string } {
	const when = whenWords(run.finishedAt ?? run.startedAt, now);
	const took =
		run.finishedAt === undefined
			? ''
			: ` · TOOK ${tookWords(Date.parse(run.finishedAt) - Date.parse(run.startedAt))}`;
	if (run.kind === 'undo') {
		return {
			kicker: `UNDONE · ${when}${took}`,
			title: `Took ReplayGain out of ${files(run.written)}`,
			body: 'Every file is as it was before the run. Kitsune plays them by its own measure again, as it did.'
		};
	}
	if (run.state === 'undone') {
		return {
			kicker: `UNDONE · ${when}`,
			title: `Wrote ReplayGain to ${songs(run.written)}`,
			body: 'This run was undone: its files are as they were, and Kitsune plays them by its own measure, as it did.'
		};
	}
	if (run.state === 'stopped') {
		return {
			kicker: `STOPPED · ${when} · AT AN ALBUM’S END`,
			title: `Wrote ReplayGain to ${songs(run.written)}`,
			body: 'The albums it reached carry their own level now; the rest are as they were. Write again to carry on.'
		};
	}
	return {
		kicker: `FINISHED · ${when}${took}`,
		title: `Wrote ReplayGain to ${songs(run.written)}`,
		body: 'Kitsune plays them as it did; every other player levels them now too. Their plays, stars and fingerprints are untouched.'
	};
}

/** A run in the runs list: what it did, and where it stands. */
export function runRowWords(run: GainRun): { what: string; state: string } {
	const left = run.left + run.failed;
	const what =
		run.kind === 'undo'
			? `Took it out of ${files(run.written)}`
			: `Wrote ${songs(run.written)}${left > 0 ? `, ${n(left)} left` : ''}`;
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

/** "Take ReplayGain out of 1,609 files?" */
export function undoAsk(run: GainRun): string {
	return `Take ReplayGain out of ${files(run.written)}?`;
}

/** A gain as the confirm's mono has it: "−10.31 DB", "+1.20 DB". */
export function dB(gain: number): string {
	const sign = gain < 0 ? '−' : '+';
	return `${sign}${Math.abs(gain).toFixed(2)} DB`;
}

/** The album ⋯ item's note: "12 SONGS MEASURED · WRITES THEM TO THE FILES". */
export function menuNote(count: number): string {
	return count === 1
		? '1 SONG MEASURED · WRITES IT TO THE FILE'
		: `${n(count)} SONGS MEASURED · WRITES THEM TO THE FILES`;
}

/** What the album confirm says of what it writes. */
export interface ConfirmWords {
	kicker: string;
	title: string;
	/** "−10.31 DB · PEAK 1.071", when the album's pooled measure is known. */
	album: string;
	button: string;
}

export function confirmWords(
	count: number,
	formats: readonly string[],
	albumGain: number | undefined,
	albumPeak: number | undefined
): ConfirmWords {
	const kinds = formatsWords(formats);
	const album = [
		...(albumGain === undefined ? [] : [dB(albumGain)]),
		...(albumPeak === undefined ? [] : [`PEAK ${albumPeak.toFixed(3)}`])
	].join(' · ');
	return {
		kicker: [
			`WRITE REPLAYGAIN`,
			plural(count, 'FILE', 'FILES'),
			...(kinds === '' ? [] : [kinds])
		].join(' · '),
		title:
			count === 1
				? 'Write Kitsune’s measure into this file?'
				: `Write Kitsune’s measure into these ${n(count)} files?`,
		album,
		button: `Write ${files(count)}`
	};
}

/** The toast after an album's run: "Wrote ReplayGain to 12 files of", then the album's name. */
export function wroteToast(written: number, left: number): string {
	if (written === 0) {
		return left === 0
			? 'Nothing was written to'
			: 'Its files changed since the scan: nothing was written to';
	}
	return `Wrote ReplayGain to ${files(written)} of`;
}

/** The toast after an album's undo. */
export function tookOutToast(written: number): string {
	return `Took ReplayGain out of ${files(written)} of`;
}

/** A refusal as a sentence: the server's detail with a capital and a full stop, or a fallback. */
export function problemWords(detail: string, fallback: string): string {
	if (detail === '') {
		return fallback;
	}
	const text = detail.charAt(0).toUpperCase() + detail.slice(1);
	return /[.!?]$/.test(text) ? text : `${text}.`;
}

/** How each format holds ReplayGain, as audiometa writes it. */
const schemes: Record<string, string> = {
	FLAC: 'VORBIS COMMENTS',
	OGG: 'VORBIS COMMENTS',
	MP3: 'ID3 TXXX',
	WAV: 'ID3 TXXX',
	AIFF: 'ID3 TXXX',
	M4A: 'FREEFORM ITEMS',
	OPUS: 'R128 TAGS'
};

/** "FLAC: VORBIS COMMENTS · MP3: ID3 TXXX": the tags each format gets, formats sharing one together. */
export function tagsLine(formats: readonly string[]): string {
	const shown = formats.length === 0 ? ['FLAC', 'MP3'] : formats.map((f) => f.toUpperCase());
	const byScheme = new Map<string, string[]>();
	for (const format of shown) {
		const scheme = schemes[format] ?? 'ITS OWN TAGS';
		byScheme.set(scheme, [...(byScheme.get(scheme) ?? []), format]);
	}
	return [...byScheme.entries()]
		.map(([scheme, kinds]) => `${formatsWords(kinds)}: ${scheme}`)
		.join(' · ');
}
