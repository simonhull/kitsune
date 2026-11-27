/** What Choose artwork says: its captions, notes, confirm and toasts. Pure. */

import type { ArtistImageCandidates, ArtistImageKind, ImageCandidate } from './types.ts';

/** A strip tab's label. */
export const kindLabels: Record<ArtistImageKind, string> = {
	thumb: 'Thumb',
	background: 'Background',
	logo: 'Logo',
	banner: 'Banner'
};

/** A source as a caption names it. */
const sourceNames: Record<ImageCandidate['source'], string> = {
	fanart: 'FANART.TV',
	wikimedia: 'COMMONS',
	folder: 'YOUR FILES',
	upload: 'UPLOADED'
};

/** An MBID as the picker shows it: its first four and last two, in capitals. */
export function shortMbid(mbid: string): string {
	return mbid.length <= 8 ? mbid : `${mbid.slice(0, 4)}…${mbid.slice(-2)}`;
}

/** The line under the title: who it is, and what identifies them at the sources. */
export function identityLine(name: string, mbid: string | undefined, phone: boolean): string {
	const who = name.toUpperCase();
	if (mbid === undefined) {
		return `${who} · NO MUSICBRAINZ ID`;
	}
	const id = shortMbid(mbid).toUpperCase();
	return phone ? `${who} · ${id}` : `${who} · BY MUSICBRAINZ ID ${id}`;
}

/** "3840 × 2160", or "" when the source doesn't say. */
function size({ width, height }: Pick<ImageCandidate, 'width' | 'height'>): string {
	return width > 0 && height > 0 ? `${String(width)} × ${String(height)}` : '';
}

/** When a pin was made, as a caption says it: today, yesterday, else the day. */
export function pinnedWhen(at: string | undefined, now: Date): string {
	if (at === undefined) {
		return '';
	}
	const then = new Date(at);
	if (Number.isNaN(then.getTime())) {
		return '';
	}
	const day = (d: Date): number =>
		Math.floor((d.getTime() - d.getTimezoneOffset() * 60_000) / 86_400_000);
	const ago = day(now) - day(then);
	if (ago <= 0) {
		return 'TODAY';
	}
	if (ago === 1) {
		return 'YESTERDAY';
	}
	return then.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' }).toUpperCase();
}

/** One part of a caption line: words, or a link to the license. */
export interface CaptionPart {
	text: string;
	href?: string;
	/** The fox's words: the chosen one's state. */
	fox?: boolean;
}

/**
 * A candidate's two caption lines. The first says where it came from and its size (or, for a
 * Commons picture, whose it is); the second what it is now (pinned, chosen), else its language and
 * licence. A phone has room for the source alone on the first, so its size goes to the second.
 */
export function captions(
	c: ImageCandidate,
	now: Date,
	phone: boolean
): [CaptionPart[], CaptionPart[]] {
	const source = sourceNames[c.source];
	const author = c.source === 'wikimedia' ? c.credit?.author?.toUpperCase() : undefined;
	const second = author ?? size(c);
	const first: CaptionPart[] = [
		{ text: phone || second === '' ? source : `${source} · ${second}` }
	];
	if (c.chosen) {
		if (!c.pinned) {
			return [first, [{ text: 'CHOSEN · AUTOMATIC', fox: true }]];
		}
		const head = c.pinnedBy === undefined ? 'PINNED' : `PINNED BY ${c.pinnedBy.toUpperCase()}`;
		const when = pinnedWhen(c.pinnedAt, now);
		return [first, [{ text: when === '' ? head : `${head} · ${when}`, fox: true }]];
	}
	if (phone && second !== '') {
		return [first, [{ text: second }]];
	}
	const license = c.credit?.license;
	if (license !== undefined) {
		return [
			first,
			[
				c.credit?.licenseUrl === undefined
					? { text: license.toUpperCase() }
					: { text: `${license.toUpperCase()} ›`, href: c.credit.licenseUrl }
			]
		];
	}
	if (c.preview?.alpha === true) {
		return [first, [{ text: 'TRANSPARENT PNG' }]];
	}
	if (c.source === 'fanart') {
		return [first, [{ text: c.lang === undefined ? 'NO WORDS' : c.lang.toUpperCase() }]];
	}
	return [first, []];
}

/** The mono note beside Automatic. */
export function autoNote(pinned: boolean): string {
	return pinned
		? 'PINNED: A REFRESH NEVER CHANGES IT · AUTOMATIC UNPINS'
		: 'FILES FIRST, THEN FANART.TV, THEN THE REST';
}

/** What the sources' states say, once each, in the order they matter. */
export function sourceNotes(set: Pick<ArtistImageCandidates, 'held' | 'sources'>): string[] {
	if (set.held) {
		return ['HELD BACK BY NOT THIS ARTIST · ASKED AGAIN ONCE ITS MUSICBRAINZ ID CHANGES'];
	}
	const notes: string[] = [];
	const add = (note: string): void => {
		if (!notes.includes(note)) {
			notes.push(note);
		}
	};
	for (const { state, words, source } of set.sources) {
		switch (state) {
			case 'off':
				add(
					source === 'fanart'
						? 'FANART.TV IS OFF · HEALTH › METADATA TURNS IT ON'
						: 'WIKIPEDIA IS OFF · HEALTH › METADATA TURNS IT ON'
				);
				break;
			case 'unavailable':
				add('NO MUSICBRAINZ ID · MATCH ITS ALBUMS TO FETCH ART');
				break;
			case 'failed':
				add(
					(
						words ?? `${source === 'fanart' ? 'fanart.tv' : 'Wikipedia'} didn't answer`
					).toUpperCase()
				);
				break;
			case 'held':
				add('HELD BACK BY NOT THIS ARTIST');
				break;
			case 'answered':
		}
	}
	return notes;
}

/** The upload tile's caption. */
export const uploadCaption = 'JPEG, PNG OR WEBP · YOURS';

/** What the picker says while the sources are asked, and with nothing to show. */
export const askingWords = 'Asking the sources…';
export const nothingWords = 'Nothing to choose from yet. Upload one of yours.';

/** The words of Not this artist's confirm. */
export function confirmWords(
	name: string,
	mbid: string | undefined
): { kicker: string; title: string; body: string } {
	const by = mbid === undefined ? 'its MusicBrainz ID' : `the MusicBrainz ID ${shortMbid(mbid)}`;
	return {
		kicker: `NOT THIS ARTIST · ${name.toUpperCase()}`,
		title: 'Take this art and bio away?',
		body: `Kitsune found them by ${by} in your files. It forgets what it fetched, and asks again only once that ID changes. Art from your own files stays.`
	};
}

/** One part of a toast's words: bold where it names the artist. */
export interface ToastPart {
	text: string;
	bold?: boolean;
}

/** What each change says once it's done. */
export function doneWords(
	what: 'pinned' | 'unpinned' | 'uploaded' | 'refresh' | 'rejected',
	name: string,
	kind: ArtistImageKind
): ToastPart[] {
	const who = { text: name, bold: true };
	const word = kindLabels[kind].toLowerCase();
	switch (what) {
		case 'pinned':
			return [{ text: `Pinned ` }, who, { text: `’s ${word}` }];
		case 'uploaded':
			return [{ text: 'Pinned your image as ' }, who, { text: `’s ${word}` }];
		case 'unpinned':
			return [who, { text: `’s ${word} is automatic again` }];
		case 'refresh':
			return [{ text: 'Looking again for ' }, who, { text: ': new art comes in as it’s found' }];
		case 'rejected':
			return [who, { text: ' is held back: what the sources gave is gone' }];
	}
}

/** What a refusal says: the server's words when it gave any, else what its status means. */
export function failedWords(status: number, detail: string, fallback: string): string {
	if (detail !== '') {
		return detail.charAt(0).toUpperCase() + detail.slice(1);
	}
	switch (status) {
		case 0:
			return 'Kitsune couldn’t reach the server';
		case 413:
			return 'That image is larger than 32 MB';
		case 415:
			return 'That isn’t a JPEG, PNG or WebP';
		case 422:
			return 'That image can’t be read';
		case 503:
			return 'The source didn’t give the image';
	}
	return fallback;
}
