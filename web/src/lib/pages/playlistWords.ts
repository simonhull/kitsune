/**
 * What playlists say, and what they show of themselves: the words of their cards, heads, menus and
 * toasts, and the albums their mosaic draws. Pure, so each rule is a test.
 */
import type { PlaylistAlbum, PlaylistSummary } from '#lib/library/source.ts';

/** What an empty name makes, and the first of the names after it: "New playlist 2" and on. */
export const newPlaylistName = 'New playlist';

/** Past this many playlists, Add to playlist has a field to find one. */
export const filterFrom = 8;

function count(n: number, one: string, many: string): string {
	return `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
}

/**
 * The covers a mosaic shows: four albums make a 2×2, and fewer than four give the first alone, to
 * fill it. None is an empty playlist, which shows its placeholder.
 */
export function mosaicOf(albums: readonly PlaylistAlbum[]): PlaylistAlbum[] {
	if (albums.length >= 4) {
		return albums.slice(0, 4);
	}
	return albums.slice(0, 1);
}

/** How long a playlist plays, as its lines say it: "11 H 12 MIN", or "42 MIN". */
export function lengthOf(durationMs: number): string {
	const minutes = Math.round(durationMs / 60_000);
	const hours = Math.floor(minutes / 60);
	return hours === 0
		? `${String(Math.max(1, minutes))} MIN`
		: `${String(hours)} H ${String(minutes % 60)} MIN`;
}

/**
 * A card's line: "151 SONGS · 11 H 12 MIN", or what to do with an empty one. A smart playlist's is
 * "19 SONGS · 1 H 40 MIN · LIVE", since it's worked out as it's read.
 */
export function cardLine({
	songCount,
	durationMs,
	smart = false
}: Pick<PlaylistSummary, 'songCount' | 'durationMs'> & { smart?: boolean }): string {
	if (smart) {
		return songCount === 0
			? 'NOTHING MATCHES YET · LIVE'
			: `${count(songCount, 'SONG', 'SONGS')} · ${lengthOf(durationMs)} · LIVE`;
	}
	if (songCount === 0) {
		return 'EMPTY · ADD SONGS FROM ANY ⋯ MENU';
	}
	return `${count(songCount, 'SONG', 'SONGS')} · ${lengthOf(durationMs)}`;
}

/**
 * A playlist's head: "PLAYLIST · 151 SONGS · 14 ALBUMS · 11 H 12 MIN · FROM NAVIDROME"; a smart
 * one's "SMART PLAYLIST · 19 SONGS · 4 ALBUMS · 1 H 40 MIN · LIVE".
 */
export function headLine(
	p: Pick<PlaylistSummary, 'songCount' | 'albumCount' | 'durationMs' | 'source'> & {
		smart?: boolean;
	}
): string {
	const smart = p.smart === true;
	const parts = [smart ? 'SMART PLAYLIST' : 'PLAYLIST', count(p.songCount, 'SONG', 'SONGS')];
	if (p.songCount > 0) {
		parts.push(count(p.albumCount, 'ALBUM', 'ALBUMS'), lengthOf(p.durationMs));
	}
	if (p.source === 'navidrome') {
		parts.push('FROM NAVIDROME');
	}
	if (smart) {
		parts.push('LIVE');
	}
	return parts.join(' · ');
}

/** A phone's head, which has less room: "PLAYLIST · 151 SONGS · 11 H". */
export function phoneHeadLine({
	songCount,
	durationMs,
	smart = false
}: Pick<PlaylistSummary, 'songCount' | 'durationMs'> & { smart?: boolean }): string {
	const hours = Math.floor(Math.round(durationMs / 60_000) / 60);
	const length = hours === 0 ? lengthOf(durationMs) : `${String(hours)} H`;
	if (smart) {
		// The phone board: "SMART · 19 SONGS · 1 H 40 · LIVE".
		const minutes = Math.round(durationMs / 60_000);
		const short =
			hours === 0 ? `${String(minutes)} MIN` : `${String(hours)} H ${String(minutes % 60)}`;
		return `SMART · ${count(songCount, 'SONG', 'SONGS')} · ${short} · LIVE`;
	}
	return songCount === 0
		? 'PLAYLIST · EMPTY'
		: `PLAYLIST · ${count(songCount, 'SONG', 'SONGS')} · ${length}`;
}

/** The Playlists page's line: "3 PLAYLISTS · YOURS ALONE", or "2 PLAYLISTS · 1 SMART · YOURS ALONE". */
export function countLine(playlists: number, smart = 0): string {
	const parts = [count(playlists, 'PLAYLIST', 'PLAYLISTS')];
	if (smart > 0) {
		parts.push(`${smart.toLocaleString('en-US')} SMART`);
	}
	return `${parts.join(' · ')} · YOURS ALONE`;
}

/** How long ago something was, as the rules card says it: "11 DAYS AGO", "YESTERDAY", "NEVER". */
export function agoWords(at: string | undefined, now: number): string {
	if (at === undefined) {
		return 'NEVER';
	}
	const days = Math.round((now - Date.parse(at)) / 86_400_000);
	return days <= 0 ? 'TODAY' : days === 1 ? 'YESTERDAY' : `${String(days)} DAYS AGO`;
}

/** What the rules card's count says beside it. */
export function matchWord(songs: number): string {
	return songs === 1 ? 'SONG MATCHES' : 'SONGS MATCH';
}

/**
 * The rules card's line: "1 H 40 MIN · 4 ALBUMS", with "· 23 MATCH, THE LIMIT KEEPS 19" when the
 * limit cuts, and "· 7 FEWER THAN SAVED" against the saved rules.
 */
export function liveLine(
	p: { songCount: number; matchCount: number; albumCount: number; durationMs: number },
	saved: number | undefined
): string {
	const parts = [
		p.songCount === 0 ? '0 MIN' : lengthOf(p.durationMs),
		count(p.albumCount, 'ALBUM', 'ALBUMS')
	];
	if (p.matchCount > p.songCount) {
		parts.push(
			`${p.matchCount.toLocaleString('en-US')} MATCH, THE LIMIT KEEPS ${p.songCount.toLocaleString('en-US')}`
		);
	}
	if (saved !== undefined && saved !== p.songCount) {
		const by = Math.abs(p.songCount - saved);
		parts.push(
			`${by.toLocaleString('en-US')} ${p.songCount > saved ? 'MORE' : 'FEWER'} THAN SAVED`
		);
	}
	return parts.join(' · ');
}

/** A rules card row's line: "CAPTIVA · 11 DAYS AGO · 6 PLAYS". */
export function liveRowLine(
	album: string,
	lastPlayedAt: string | undefined,
	plays: number,
	now: number
): string {
	return [album.toUpperCase(), agoWords(lastPlayedAt, now), count(plays, 'PLAY', 'PLAYS')]
		.filter((part) => part !== '')
		.join(' · ');
}

/** The rules card's foot: "+ 6 MORE · COUNTED BY THE SERVER AS YOU TYPE". */
export function liveFoot(songs: number, shown: number): string {
	const more = songs - shown;
	return `${more > 0 ? `+ ${more.toLocaleString('en-US')} MORE · ` : ''}COUNTED BY THE SERVER AS YOU TYPE`;
}

/** The dock's line: "4 rules · not saved yet", or "Nothing changed". */
export function dockLine(rules: number, changed: boolean): string {
	return changed
		? `${String(rules)} ${rules === 1 ? 'rule' : 'rules'} · not saved yet`
		: 'Nothing changed';
}

/** The phone's limit pill: "Limit · 50 songs ›", or "Limit · off ›". */
export function limitPill(limit: { on: boolean; count: number; unit: string }): string {
	if (!limit.on) {
		return 'Limit · off';
	}
	return `Limit · ${limit.count.toLocaleString('en-US')} ${limit.unit === 'gb' ? 'GB' : limit.unit}`;
}

/** A smart playlist's foot, under its songs. */
export const smartFoot =
	'LIVE: RATE OR PLAY A SONG AND IT JOINS OR LEAVES · THE RULES SET THE ORDER, SO NOTHING DRAGS';

/** What an empty name makes: "New playlist", or the first "New playlist N" nobody has. */
export function nameFor(typed: string, taken: readonly string[]): string {
	const name = typed.trim();
	if (name !== '') {
		return name;
	}
	const names = new Set(taken.map((each) => each.trim().toLowerCase()));
	if (!names.has(newPlaylistName.toLowerCase())) {
		return newPlaylistName;
	}
	for (let n = 2; ; n += 1) {
		const next = `${newPlaylistName} ${String(n)}`;
		if (!names.has(next.toLowerCase())) {
			return next;
		}
	}
}

/**
 * What a playlist in Add to playlist says of the songs being added: "ALREADY IN IT" when it holds
 * them all, "1 OF THE 3 ALREADY IN IT" when some, and nothing when none, or while that's not known.
 */
export function holdsWords(holds: number | undefined, of: number): string | null {
	if (holds === undefined || holds === 0 || of === 0) {
		return null;
	}
	if (holds >= of) {
		return 'ALREADY IN IT';
	}
	return `${String(holds)} OF THE ${String(of)} ALREADY IN IT`;
}

/** The phone sheet's line for a playlist: its size, and what it holds of the songs, shorter. */
export function sheetLine(
	p: Pick<PlaylistSummary, 'songCount'>,
	holds: number | undefined,
	of: number
): string {
	const size = p.songCount === 0 ? 'EMPTY' : count(p.songCount, 'SONG', 'SONGS');
	if (holds === undefined || holds === 0 || of === 0) {
		return size;
	}
	return holds >= of
		? `${size} · ALREADY IN IT`
		: `${size} · ${String(holds)} OF THE ${String(of)} IN IT`;
}

/** The line over Add to playlist's list, in a popover or a sheet: "ADD 3 SONGS TO". */
export function addHeading(songs: number): string {
	return `ADD ${count(songs, 'SONG', 'SONGS')} TO`;
}

/**
 * What Add to playlist lists, in its order: most recently added to first, then the newest made. A
 * smart or file playlist is left out, since nothing is added to one.
 */
export function byLastAdded(playlists: readonly PlaylistSummary[]): PlaylistSummary[] {
	const at = (p: PlaylistSummary): number => Date.parse(p.addedAt ?? p.createdAt);
	return playlists
		.filter(({ smart, source }) => !smart && source !== 'file')
		.toSorted((a, b) => at(b) - at(a) || a.name.localeCompare(b.name));
}

/** The playlists whose names hold what's typed, ignoring case. */
export function matching(playlists: readonly PlaylistSummary[], typed: string): PlaylistSummary[] {
	const want = typed.trim().toLowerCase();
	return want === ''
		? [...playlists]
		: playlists.filter(({ name }) => name.toLowerCase().includes(want));
}

/** A toast's words: the bold parts and the plain ones, in order. */
export interface ToastPart {
	text: string;
	bold?: boolean;
}

/**
 * What an add's toast says: "2 songs added to *Falling Up* · 1 was already in it", "*Falling Up*
 * has it already", "All 3 already in *Falling Up*".
 */
export function addedWords(added: number, skipped: number, name: string): ToastPart[] {
	if (added === 0) {
		return skipped === 1
			? [{ text: name, bold: true }, { text: ' has it already' }]
			: [{ text: `All ${String(skipped)} already in ` }, { text: name, bold: true }];
	}
	const parts: ToastPart[] = [
		{ text: `${count(added, 'song', 'songs')} added to ` },
		{ text: name, bold: true }
	];
	if (skipped > 0) {
		parts.push({ text: ` · ${String(skipped)} ${skipped === 1 ? 'was' : 'were'} already in it` });
	}
	return parts;
}

/** The toast's button that adds the skipped songs after all, when there are some. */
export function againWords(skipped: number): string | null {
	if (skipped === 0) {
		return null;
	}
	return skipped === 1 ? 'Add it again' : 'Add them again';
}

/** What a take-out's toast says: "*Searchlights* taken out of Falling Up", or "3 songs taken out of…". */
export function removedWords(titles: readonly string[], name: string): ToastPart[] {
	const [only] = titles;
	if (titles.length === 1 && only !== undefined) {
		return [{ text: only, bold: true }, { text: ` taken out of ${name}` }];
	}
	return [{ text: `${count(titles.length, 'song', 'songs')} taken out of ${name}` }];
}
