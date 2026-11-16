/**
 * What file playlists say: their cards, heads, the FROM line, the card of lines that found no song,
 * and Export's words. The server sends facts (a reason, a file name); the words are made here. Pure,
 * so each rule is a test.
 */
import type {
	ExistingFile,
	MissingLine,
	MissingReason,
	PlaylistSaved,
	PlaylistSummary
} from '#lib/library/source.ts';
import { agoWords, lengthOf } from './playlistWords.ts';

function count(n: number, one: string, many: string): string {
	return `${n.toLocaleString('en-US')} ${n === 1 ? one : many}`;
}

/** Whether a playlist comes from a file in the library. */
export function isFile(p: Pick<PlaylistSummary, 'source'>): boolean {
	return p.source === 'file';
}

/** The tag's words: the extension as written, "M3U8" or "M3U". */
export function tagWords(format: 'm3u' | 'm3u8'): string {
	return format.toUpperCase();
}

/**
 * A file card's line, in two parts: "19 SONGS · 1 H 19 MIN · " and "3 NOT FOUND", which is in the
 * fox. The second is empty when every line found its song.
 */
export function fileCardLine(
	p: Pick<PlaylistSummary, 'songCount' | 'durationMs' | 'file'>
): [string, string] {
	const lost = p.file?.notFound ?? 0;
	const size =
		p.songCount === 0
			? 'NO SONGS FOUND'
			: `${count(p.songCount, 'SONG', 'SONGS')} · ${lengthOf(p.durationMs)}`;
	return lost === 0 ? [size, ''] : [`${size} · `, `${lost.toLocaleString('en-US')} NOT FOUND`];
}

/** A file playlist's head: "FILE PLAYLIST · 19 SONGS · 7 ALBUMS · 1 H 19 MIN · EVERYONE HERE". */
export function fileHeadLine(
	p: Pick<PlaylistSummary, 'songCount' | 'albumCount' | 'durationMs'>
): string {
	const parts = ['FILE PLAYLIST', count(p.songCount, 'SONG', 'SONGS')];
	if (p.songCount > 0) {
		parts.push(count(p.albumCount, 'ALBUM', 'ALBUMS'), lengthOf(p.durationMs));
	}
	parts.push('EVERYONE HERE');
	return parts.join(' · ');
}

/** A phone's: "FILE · 19 SONGS · 1 H 19 · EVERYONE". */
export function filePhoneHeadLine(p: Pick<PlaylistSummary, 'songCount' | 'durationMs'>): string {
	const minutes = Math.round(p.durationMs / 60_000);
	const hours = Math.floor(minutes / 60);
	const short =
		hours === 0 ? `${String(minutes)} MIN` : `${String(hours)} H ${String(minutes % 60)}`;
	return `FILE · ${count(p.songCount, 'SONG', 'SONGS')} · ${short} · EVERYONE`;
}

/** When the file was read, after its path: "READ 2 DAYS AGO", "READ TODAY". */
export function readWords(readAt: string, now: number): string {
	return `READ ${agoWords(readAt, now)}`;
}

/** The pill that opens the card: "3 SONGS NOT FOUND", or on a phone "3 NOT FOUND". */
export function notFoundPill(lost: number, phone: boolean): string {
	return phone
		? `${lost.toLocaleString('en-US')} NOT FOUND`
		: `${count(lost, 'SONG', 'SONGS')} NOT FOUND`;
}

/** The card's heading: "3 lines found no song". */
export function missingHeading(lost: number): string {
	return `${count(lost, 'line', 'lines')} found no song`;
}

/** The card's line beside it: "THE OTHER 19 PLAY IN THE FILE'S ORDER · FIX THE FILE AND …". */
export function missingLead(found: number, phone: boolean): string {
	const fix = 'FIX THE FILE AND KITSUNE READS IT AGAIN';
	if (phone || found === 0) {
		return fix;
	}
	return `THE OTHER ${found.toLocaleString('en-US')} PLAY IN THE FILE'S ORDER · ${fix}`;
}

/** A line's reason, as its chip says it. */
export function reasonWords(reason: MissingReason | 'pastLimit'): string {
	switch (reason) {
		case 'noSong':
			return 'NO SONG AT THIS PATH';
		case 'outside':
			return 'OUTSIDE THE LIBRARY';
		case 'notFile':
			return 'NOT A FILE';
		case 'trashed':
			return 'IN THE TRASH';
		case 'pastLimit':
			return 'PAST THE LIMIT';
	}
}

/** The line as the card shows it: a web address says it is one. */
export function missingText(line: Pick<MissingLine, 'text' | 'reason'>): string {
	return line.reason === 'notFile' ? `${line.text} (a web address)` : line.text;
}

/** The `#EXTINF` under it: "Falling Up - Golden Arrows · 4:06", or "" with none. */
export function extinfWords(line: Pick<MissingLine, 'title' | 'seconds'>): string {
	const length =
		line.seconds === undefined || line.seconds <= 0
			? ''
			: `${String(Math.floor(line.seconds / 60))}:${String(line.seconds % 60).padStart(2, '0')}`;
	return [line.title ?? '', length].filter((part) => part !== '').join(' · ');
}

/** The folder a path names its file in, as a person reads it: "2009 - Fangs!" is "Fangs!". */
function folderOf(text: string): string {
	const parts = text.split('/');
	const folder = parts.at(-2) ?? '';
	return folder.replace(/^\d{4} - /, '');
}

/** A line's plain hint: what's wrong, and what would fix it. Shorter on a phone. */
export function hintFor(line: MissingLine, phone: boolean): string {
	switch (line.reason) {
		case 'noSong': {
			if (line.otherExtension === undefined) {
				return phone
					? 'Nothing in the library is here.'
					: 'No song in the library is at this path.';
			}
			const folder = folderOf(line.text);
			const where = folder === '' ? 'Its folder' : folder;
			if (phone) {
				return `${where} has ${line.otherExtension}.`;
			}
			const dot = line.text.lastIndexOf('.');
			const named = dot < 0 ? '' : line.text.slice(dot);
			return named === ''
				? `${where} has ${line.otherExtension}.`
				: `${where} has ${line.otherExtension}: the file names the ${named}.`;
		}
		case 'outside':
			return phone
				? 'Only the end of an absolute path is shown.'
				: 'An absolute path in no library folder. Only its end is shown.';
		case 'notFile':
			return phone
				? 'Streams aren’t songs in the library.'
				: 'Kitsune plays songs in its library, not streams.';
		case 'trashed':
			return 'Restore it and it’s back here.';
	}
}

/** The row for lines past what Kitsune reads: counted only. */
export function pastLimitWords(lines: number): { text: string; hint: string } {
	return {
		text: `${count(lines, 'more line', 'more lines')}`,
		hint: 'Kitsune reads a file’s first 10,000 songs.'
	};
}

/** The Playlists page's line: "4 PLAYLISTS · 1 SMART · 2 FROM FILES". */
export function countWithFiles(playlists: number, smart: number, files: number): string {
	const parts = [count(playlists, 'PLAYLIST', 'PLAYLISTS')];
	if (smart > 0) {
		parts.push(`${smart.toLocaleString('en-US')} SMART`);
	}
	if (files > 0) {
		parts.push(`${files.toLocaleString('en-US')} FROM FILES`);
		return parts.join(' · ');
	}
	return `${parts.join(' · ')} · YOURS ALONE`;
}

/** The longest a file name's stem may be, in bytes, as the server cuts it. */
const mostNameBytes = 120;

/**
 * The file a playlist downloads as, by the server's rule: no `/`, `\` or control character, no
 * leading dot, at most 120 bytes cut on a character, "playlist" if nothing is left, then ".m3u8".
 */
export function exportFileName(name: string): string {
	let stem = name.replace(/[/\\\p{Cc}]/gu, '').trim();
	stem = stem.replace(/^\.+/, '');
	const encoder = new TextEncoder();
	let bytes = 0;
	let cut = '';
	for (const char of stem) {
		bytes += encoder.encode(char).length;
		if (bytes > mostNameBytes) {
			break;
		}
		cut += char;
	}
	stem = cut.trim();
	return `${stem === '' ? 'playlist' : stem}.m3u8`;
}

/** Export's head: "EXPORT · 151 SONGS AS .M3U8". */
export function exportHead(songs: number): string {
	return `EXPORT · ${count(songs, 'SONG', 'SONGS')} AS .M3U8`;
}

/** The Download card's line: "Falling Up.m3u8 · 151 LINES · #EXTINF WITH ARTIST - TITLE". */
export function downloadLine(name: string, songs: number): string {
	return `${exportFileName(name)} · ${count(songs, 'LINE', 'LINES')} · #EXTINF WITH ARTIST - TITLE`;
}

/** A folder as Save shows it: "music / Playlists", its library folder by its own name. */
export function folderLabel(root: string, folder: string): string {
	const rootName =
		root
			.split('/')
			.filter((part) => part !== '')
			.at(-1) ?? '/';
	return [rootName, ...folder.split('/').filter((part) => part !== '' && part !== '.')].join(' / ');
}

/** Splits a folder the picker gave into the root it's in and the path under it, or null. */
export function underRoot(
	path: string,
	roots: readonly string[]
): { root: string; folder: string } | null {
	const trim = (each: string): string => (each.length > 1 ? each.replace(/\/+$/, '') : each);
	const at = trim(path);
	const root = roots
		.map(trim)
		.filter((each) => at === each || at.startsWith(each === '/' ? '/' : `${each}/`))
		.toSorted((a, b) => b.length - a.length)
		.at(0);
	if (root === undefined) {
		return null;
	}
	return { root, folder: at.slice(root.length).replace(/^\/+/, '') };
}

/** How long ago a file changed, in words: "3 weeks ago", "yesterday". */
export function changedAgo(at: string, now: number): string {
	const days = Math.floor((now - Date.parse(at)) / 86_400_000);
	if (days <= 0) {
		return 'today';
	}
	if (days === 1) {
		return 'yesterday';
	}
	if (days < 14) {
		return `${String(days)} days ago`;
	}
	if (days < 60) {
		return `${String(Math.round(days / 7))} weeks ago`;
	}
	return `${String(Math.round(days / 30))} months ago`;
}

/** A sentence's parts, the bold ones marked. */
export interface Part {
	text: string;
	bold?: boolean;
}

/**
 * The overwrite confirm's words: "Kitsune reads it as *Falling Up*, a file playlist of 148 songs,
 * last changed 3 weeks ago. Replacing it changes that playlist for everyone."
 */
export function existsWords(exists: ExistingFile, now: number): Part[] {
	const when = changedAgo(exists.modifiedAt, now);
	if (exists.playlistName === undefined) {
		return [
			{
				text: `It was last changed ${when}. Replacing it changes it for everyone who reads this folder.`
			}
		];
	}
	const size =
		exists.songCount === undefined ? '' : ` of ${count(exists.songCount, 'song', 'songs')}`;
	return [
		{ text: 'Kitsune reads it as ' },
		{ text: exists.playlistName, bold: true },
		{
			text: `, a file playlist${size}, last changed ${when}. Replacing it changes that playlist for everyone.`
		}
	];
}

/** The confirm's second button: "Save as Falling Up 2.m3u8". */
export function saveAsWords(freeName: string): string {
	return `Save as ${freeName}`;
}

/** The Saved card's words, with what was left out, if anything. */
export function savedWords(saved: PlaylistSaved, root: string): { head: string; body: string } {
	const where = folderLabel(root, saved.folder);
	const left =
		saved.leftOut === 0
			? ''
			: ` ${count(saved.leftOut, 'song is', 'songs are')} in another library folder and ${saved.leftOut === 1 ? 'was' : 'were'} left out.`;
	return {
		head: `Saved ${saved.name}`,
		body: `In ${where}. It shows under From files in a moment, when the library reads it.${left}`
	};
}

/** A file page's foot, under its songs. */
export const fileFoot =
	'FROM A FILE, SO EVERYONE HERE SEES IT AND NOTHING DRAGS · TO CHANGE IT, EDIT THE FILE OR COPY IT TO A PLAYLIST OF YOUR OWN';

/** Its ⋯ menu's note. */
export const fileMenuNote =
	'NO RENAME, DRAG OR TAKE OUT: IT IS THE FILE’S. COPY IT TO MAKE IT YOURS.';
