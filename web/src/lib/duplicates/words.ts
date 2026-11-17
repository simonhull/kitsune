/** The words the Duplicates pages say, worked out here so they can be held to the boards. */

import { clock } from '#lib/library/format.ts';
import { starsText } from '#lib/library/stars.ts';
import type {
	Confidence,
	DuplicateCopy,
	DuplicateCounts,
	DuplicateFilter,
	DuplicateGroupSummary,
	DuplicatePairSummary,
	KeepReason,
	MergePlan
} from './types.ts';

const n = (count: number): string => count.toLocaleString('en-US');
const plural = (count: number, one: string, many: string): string =>
	`${n(count)} ${count === 1 ? one : many}`;

/** "51 GROUPS · 104 SONGS · 1 ALBUM PAIR · 7,604 SONGS CHECKED". */
export function countsLine(c: DuplicateCounts, compact = false): string {
	const parts = [plural(c.groups, 'GROUP', 'GROUPS'), plural(c.songs, 'SONG', 'SONGS')];
	if (c.pairs > 0) {
		parts.push(plural(c.pairs, 'ALBUM PAIR', 'ALBUM PAIRS'));
	}
	if (!compact) {
		parts.push(`${n(c.library)} SONGS CHECKED`);
	}
	return parts.join(' · ');
}

/** While the fingerprint pass is under way, how far it has got; once it's done, nothing. */
export function checkedNote(c: DuplicateCounts): string {
	if (c.checked >= c.library) {
		return '';
	}
	return `${n(c.checked)} OF ${n(c.library)} SONGS CHECKED FOR EXACT COPIES · THE REST AS THE FINGERPRINT PASS REACHES THEM`;
}

/** The filter chips, with their counts. Same audio, recording and Probably count groups. */
export function filterChips(
	c: DuplicateCounts
): { filter: DuplicateFilter; label: string; count: number }[] {
	return [
		{ filter: 'all', label: 'All', count: c.groups },
		{ filter: 'audio', label: 'Same audio', count: c.audio },
		{ filter: 'recording', label: 'Same recording', count: c.recording },
		{ filter: 'probably', label: 'Probably', count: c.probably },
		{ filter: 'pairs', label: 'Album pairs', count: c.pairs },
		{ filter: 'dismissed', label: 'Dismissed', count: c.dismissed }
	];
}

/** The mono chip a group wears. */
export function chipWords(c: Confidence): string {
	switch (c) {
		case 'audio':
			return 'SAME AUDIO';
		case 'recording':
			return 'SAME RECORDING';
		case 'probably':
			return 'SAME SONG, PROBABLY';
	}
}

/** The lengths of copies: "5:21 AND 5:22", "2:48" when they are one, "0:51, 0:52 AND 0:53". */
export function lengthsWords(durationsMs: readonly number[]): string {
	const each = [...new Set(durationsMs.map((ms) => clock(Math.round(ms / 1000))))];
	if (each.length === 1) {
		return each[0] ?? '';
	}
	return `${each.slice(0, -1).join(', ')} AND ${each.at(-1) ?? ''}`;
}

/** What the evidence says, under a group's title. */
export function evidenceWords(
	c: Confidence,
	artist: string,
	durationsMs: readonly number[]
): string {
	const both = durationsMs.length === 2 ? 'BOTH' : 'ALL';
	switch (c) {
		case 'audio':
			return `SAME AUDIO · THE FINGERPRINT PASS FOUND THE SAME SOUND IN ${both === 'BOTH' ? 'BOTH FILES' : 'EVERY FILE'}`;
		case 'recording':
			return `${artist.toUpperCase()} · ${both} NAME ONE MUSICBRAINZ RECORDING · ${lengthsWords(durationsMs)}`;
		case 'probably':
			return `${artist.toUpperCase()} · THE SAME ARTIST AND TITLE · ${lengthsWords(durationsMs)}`;
	}
}

/** "2 COPIES · FLAC + FLAC". */
export function copiesLine(g: Pick<DuplicateGroupSummary, 'copies' | 'formats'>): string {
	return `${n(g.copies)} COPIES · ${g.formats.map((f) => f.toUpperCase()).join(' + ')}`;
}

/** The facts under a group row's albums: its copies, lengths and plays. */
export function groupFacts(g: DuplicateGroupSummary): string {
	const parts = [copiesLine(g), lengthsWords(g.durationMs)];
	if (g.plays > 0) {
		parts.push(plural(g.plays, 'PLAY', 'PLAYS'));
	}
	return parts.join(' · ');
}

/** A pair card's sentence: whether every song of one album is in the other. */
export function pairSentence(p: DuplicatePairSummary): {
	lead: string;
	a: string;
	mid: string;
	b: string;
} {
	const [first, second] = p.albums;
	if (p.only[1] === 0) {
		return { lead: 'Every song of ', a: second.title, mid: ' is also in ', b: first.title };
	}
	if (p.only[0] === 0) {
		return { lead: 'Every song of ', a: first.title, mid: ' is also in ', b: second.title };
	}
	return { lead: `${n(p.both)} songs of `, a: first.title, mid: ' are also in ', b: second.title };
}

/** A pair card's mono line: "16 SONGS IN BOTH · KEEP ONE ALBUM · THE PLAIN ONE ALSO HOLDS 2 …". */
export function pairLine(p: DuplicatePairSummary): string {
	const parts = [`${n(p.both)} SONGS IN BOTH`, 'KEEP ONE ALBUM'];
	p.only.forEach((count, side) => {
		if (count > 0) {
			const title = p.albums[side]?.title.toUpperCase() ?? '';
			parts.push(`${title} ALSO HOLDS ${n(count)} THE OTHER DOESN’T, NEVER TRASHED`);
		}
	});
	return parts.join(' · ');
}

/** A copy's facts as the group page lists them, label then value. */
export function copyFacts(
	c: DuplicateCopy
): { label: string; value: string; quiet?: boolean; bold?: boolean }[] {
	const format = [c.format.toUpperCase()];
	if (c.sampleRate !== undefined) {
		const khz = c.sampleRate / 1000;
		format.push(
			`${Number.isInteger(khz) ? String(khz) : khz.toFixed(1)} kHz${c.bitDepth === undefined ? '' : ` / ${String(c.bitDepth)}`}`
		);
	}
	const othersRated = c.raters - (c.stars > 0 ? 1 : 0);
	const stars =
		c.stars > 0 ? '★'.repeat(Math.floor(c.stars)) + (c.stars % 1 >= 0.5 ? '½' : '') : 'Not rated';
	return [
		{ label: 'FORMAT', value: format.join(' · ') },
		{
			label: 'BITRATE',
			value: c.bitrate === undefined ? 'Unknown' : `${n(c.bitrate)} kb/s`,
			bold: true,
			quiet: c.bitrate === undefined
		},
		{ label: 'LENGTH', value: clock(Math.round(c.durationMs / 1000)) },
		{ label: 'PLAYS', value: n(c.plays), bold: true },
		{
			label: 'YOUR STARS',
			value:
				othersRated > 0 ? `${stars} · rated by ${plural(c.raters, 'person', 'people')}` : stars,
			bold: true,
			quiet: c.stars === 0
		},
		{
			label: 'PLAYLISTS',
			value: c.playlists === 0 ? 'None' : `In ${n(c.playlists)}`,
			quiet: c.playlists === 0
		},
		{ label: 'ADDED', value: dayWords(c.addedAt) },
		{
			label: 'PLACE',
			value: c.inPlace ? 'In place' : 'Not where the template puts it',
			quiet: !c.inPlace
		}
	];
}

/** "21 Mar 2025". */
export function dayWords(at: string): string {
	return new Date(at).toLocaleDateString('en-GB', {
		day: 'numeric',
		month: 'short',
		year: 'numeric',
		timeZone: 'UTC'
	});
}

/** A copy's facts in two mono lines, for a phone. */
export function copyLines(c: DuplicateCopy): [string, string] {
	const parts = [c.format.toUpperCase()];
	if (c.bitrate !== undefined) {
		parts.push(`${n(c.bitrate)} KB/S`);
	}
	parts.push(clock(Math.round(c.durationMs / 1000)), plural(c.plays, 'PLAY', 'PLAYS'));
	parts.push(c.stars > 0 ? `${starsText(c.stars)}★` : 'NOT RATED');
	if (c.original) {
		parts.push('ORIGINAL ALBUM');
	}
	return [parts.join(' · '), c.path];
}

/** The year line under a copy's album: "1993 · TRACK 5 · A BEST-OF". */
export function albumLine(c: DuplicateCopy): string {
	const parts: string[] = [];
	if (c.album.year !== undefined) {
		parts.push(String(c.album.year));
	}
	if (c.trackNo !== undefined) {
		parts.push(
			c.disc !== undefined && c.disc > 1
				? `DISC ${String(c.disc)} · TRACK ${String(c.trackNo)}`
				: `TRACK ${String(c.trackNo)}`
		);
	}
	if (!c.original) {
		parts.push('NOT ITS ORIGINAL ALBUM');
	}
	return parts.join(' · ');
}

const reasonNames: Record<KeepReason, string> = {
	lossless: 'LOSSLESS',
	original: 'ORIGINAL ALBUM',
	bitrate: 'HIGHER BITRATE',
	inPlace: 'ALREADY ORGANIZED',
	plays: 'MORE PLAYS',
	earlier: 'ADDED FIRST'
};

const otherHas: Record<KeepReason, string> = {
	lossless: 'IS LOSSLESS',
	original: 'IS ON THE ORIGINAL ALBUM',
	bitrate: 'HAS A HIGHER BITRATE',
	inPlace: 'IS ALREADY ORGANIZED',
	plays: 'HAS MORE PLAYS',
	earlier: 'WAS ADDED FIRST'
};

/**
 * Why the copy in the ring is kept: the suggestion's reasons, "LOSSLESS · HIGHER BITRATE (+7%)", or
 * for another chosen by hand, "YOUR CHOICE · THE OTHER IS ON THE ORIGINAL ALBUM".
 */
export function whyWords(
	reasons: readonly KeepReason[],
	copies: readonly DuplicateCopy[],
	suggested: string,
	kept: string
): string {
	if (kept !== suggested) {
		const first = reasons[0];
		const who = copies.length === 2 ? 'THE OTHER' : 'THE SUGGESTED ONE';
		return first === undefined ? 'YOUR CHOICE' : `YOUR CHOICE · ${who} ${otherHas[first]}`;
	}
	if (reasons.length === 0) {
		return 'ALIKE IN EVERYTHING · THE FIRST ONE';
	}
	const keeper = copies.find((c) => c.track === kept);
	return reasons
		.map((r) => {
			if (r !== 'bitrate' || keeper?.bitrate === undefined) {
				return reasonNames[r];
			}
			const next = Math.max(...copies.filter((c) => c.track !== kept).map((c) => c.bitrate ?? 0));
			return next > 0
				? `${reasonNames[r]} (+${String(Math.round((keeper.bitrate / next - 1) * 100))}%)`
				: reasonNames[r];
		})
		.join(' · ');
}

/** A short album name for a button: up to its first comma or colon. */
export function shortAlbum(title: string): string {
	const cut = title.search(/[,:(]/);
	const short = (cut > 0 ? title.slice(0, cut) : title).trim();
	return short === '' ? title : short;
}

/** The keeper as the lines of what moves name it: its short name, plays and the caller's stars. */
export interface KeeperFacts {
	name: string;
	plays: number;
	stars: number;
	/** The copy going, when there is one; "the other" otherwise. */
	other?: string;
}

/** The lines of what moves to the keeper. */
export function movesLines(plan: MergePlan, keeper: KeeperFacts, pair = false): string[] {
	const lines: string[] = [];
	if (plan.plays === 0 && plan.listens === 0) {
		lines.push('No plays or listens to move');
	} else {
		const plays =
			plan.plays === 1 ? '1 play and its listens' : `${n(plan.plays)} plays and their listens`;
		lines.push(
			plan.plays === 0
				? plural(plan.listens, 'listen', 'listens')
				: `${plays}, so ${keeper.name} has ${plural(keeper.plays + plan.plays, 'play', 'plays')}`
		);
	}
	lines.push(starsLine(plan, keeper, pair));
	if (plan.entries === 0) {
		lines.push('No playlist entries to move');
	} else {
		const names = plan.playlists.map((p) => p.name);
		if (plan.otherPlaylists > 0) {
			names.push(
				plural(plan.otherPlaylists, 'playlist of someone else’s', 'playlists of other people’s')
			);
		}
		lines.push(
			`${plural(plan.entries, 'playlist entry', 'playlist entries')}: ${names.join(', ')}`
		);
	}
	lines.push(shuffleLine(plan));
	if (plan.queues > 0) {
		lines.push(
			`${plural(plan.queues, 'remembered queue names', 'remembered queues name')} the keeper instead`
		);
	}
	if (plan.hearts > 0) {
		lines.push(`${plural(plan.hearts, 'heart', 'hearts')} kept from before`);
	}
	return lines;
}

/** What a merge does with stars, the caller's own first. */
function starsLine(plan: MergePlan, keeper: KeeperFacts, pair: boolean): string {
	const parts: string[] = [];
	const yours = plan.stars.yours;
	const going = keeper.other ?? 'the other';
	if (yours === undefined && keeper.stars > 0 && !pair) {
		parts.push(`Stars: your ${starsText(keeper.stars)}★ stays (${going} had none)`);
	} else if (yours !== undefined) {
		if (yours.after === yours.keeper) {
			parts.push(
				`Stars: your ${starsText(yours.keeper)}★ stays (${going} had ${starsText(yours.other)}★)`
			);
		} else {
			const was =
				yours.keeper > 0
					? `over the keeper’s ${starsText(yours.keeper)}★`
					: 'you hadn’t rated the keeper';
			parts.push(`Your ${starsText(yours.after)}★ comes over (${was})`);
		}
	}
	// For one group the caller's own are above, so the rest are other people's.
	const other = pair ? '' : 'other ';
	const { come, stay } = plan.stars;
	if (come > 0) {
		parts.push(
			`${n(come)} ${other}${come === 1 ? 'person’s stars come' : 'people’s stars come'} over`
		);
	}
	if (stay > 0) {
		parts.push(
			`${n(stay)} ${other}${stay === 1 ? 'person keeps' : 'people keep'} their own stars on the keeper`
		);
	}
	if (plan.loves > 0) {
		parts.push(
			`Last.fm gets ${plural(plan.loves, 'love', 'loves')}, as any rating over the line would`
		);
	}
	return parts.length === 0 ? 'No stars to move' : parts.join('. ');
}

/** What a merge does with shuffle's rules. */
function shuffleLine(plan: MergePlan): string {
	if (plan.excluded === 0 && plan.runs === 0) {
		return 'Shuffle: nothing was excluded or combined, so nothing to carry';
	}
	const parts: string[] = [];
	if (plan.excluded > 0) {
		parts.push(`excluded for ${plural(plan.excluded, 'person', 'people')}`);
	}
	if (plan.runs > 0) {
		parts.push(`${plural(plan.runs, 'Combine place moves', 'Combine places move')} too`);
	}
	return `Shuffle: ${parts.join('; ')}`;
}

/** What a merge leaves as it is. */
export function staysLines(keeper: string, copies: number): string[] {
	const files = copies === 1 ? 'The copy’s file goes' : 'The copies’ files go';
	return [
		`${files} to the Trash for 30 days, ${copies === 1 ? 'its' : 'their'} own loudness and fingerprint with ${copies === 1 ? 'it' : 'them'}.`,
		`Restoring ${copies === 1 ? 'it' : 'one'} brings back the file, not its plays or stars. Those stay with ${keeper}.`,
		'Scrobbles Last.fm already has stay as they are.'
	];
}

/** The toast after a merge: "Kept Creed · A Liturgy… 1 copy moved to the Trash". */
export function mergedWords(title: string, album: string, trashed: number): string {
	return `Kept ${title} · ${album}. ${plural(trashed, 'copy', 'copies')} moved to the Trash`;
}
