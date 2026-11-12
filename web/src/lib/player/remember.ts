import { clock } from '#lib/library/format.ts';
import type {
	Album,
	DeviceForm,
	PlaybackKind,
	PlaybackRecord,
	PlaybackSave,
	Track
} from '#lib/library/source.ts';
import type { AlbumLookup, ListEntry, Queue } from './queue.ts';

/** The most songs a record holds, across its tracks and later. */
export const mostRemembered = 1000;
/** Past the cap, how many songs behind the current one are kept. */
export const behindRemembered = 100;
/** The furthest into a song a record may be: a day. */
const mostPositionMs = 24 * 60 * 60 * 1000;

/** This browser, as a record names it. */
export interface Client {
	id: string;
	device: DeviceForm;
}

/** Where a shuffle stands: the songs of its current unit from the one playing, and the session's place after them. */
export interface ShuffleAt {
	after: number;
	songs: readonly string[];
}

/** What kind of record a queue is saved as. */
export function kindOf(queue: Queue): PlaybackKind {
	if (queue.shuffle !== undefined) {
		return 'shuffle';
	}
	if (queue.later !== undefined) {
		return 'list';
	}
	return queue.albums === undefined ? 'album' : 'songs';
}

/**
 * The record of a queue, `seconds` into its current song: track IDs only. A shuffle keeps just its
 * current song or run and its session's place after it; one whose session hasn't answered yet is
 * saved as the songs it holds. Past the cap it keeps up to 100 songs behind the current one and
 * fills the rest ahead (behind takes what ahead can't fill), so a very long list comes to rest
 * earlier than it would have.
 */
export function recordOf(
	queue: Queue,
	seconds: number,
	shuffle: ShuffleAt | null,
	client: Client
): PlaybackSave {
	const positionMs = Math.min(Math.max(Math.round(seconds * 1000), 0), mostPositionMs);
	const common = { positionMs, ordered: queue.ordered, clientId: client.id, device: client.device };
	const kind = kindOf(queue);
	if (kind === 'shuffle' && queue.shuffle !== undefined && shuffle !== null) {
		return {
			...common,
			kind,
			tracks: [...shuffle.songs],
			index: 0,
			shuffle: { id: queue.shuffle.id, after: shuffle.after }
		};
	}
	const ids = queue.tracks.map(({ id }) => id);
	const named = queue.later?.map(({ trackId }) => trackId) ?? [];
	// Ahead first: what's behind keeps 100, and more only where there's too little ahead to fill.
	const ahead = ids.length - queue.index - 1 + named.length;
	const behind = Math.max(behindRemembered, mostRemembered - 1 - ahead);
	const from = Math.max(0, queue.index - behind);
	const tracks = ids.slice(from, from + mostRemembered);
	const later = named.slice(0, mostRemembered - tracks.length);
	const saved = kind === 'shuffle' ? 'songs' : kind;
	return {
		...common,
		kind: saved,
		tracks,
		index: queue.index - from,
		...(saved === 'list' && later.length > 0 ? { later } : {}),
		...(queue.continues !== undefined && (saved === 'album' || saved === 'songs')
			? { continues: queue.continues.album }
			: {})
	};
}

/**
 * The queue a record comes back as, with the albums here now, or null when the current song's
 * album isn't here (or no longer holds it). Songs behind come back only where their albums are
 * here; songs ahead go in up to the first whose album isn't, and that one and the rest wait as a
 * list's do, taken in as their albums arrive.
 */
export function restored(record: PlaybackRecord, lookup: AlbumLookup): Queue | null {
	const songAt = (id: string, albumId: string): { album: Album; track: Track } | null => {
		const album = lookup(albumId);
		const track =
			album === undefined || album === 'gone'
				? undefined
				: album.tracks.find((each) => each.id === id);
		return album === undefined || album === 'gone' || track === undefined ? null : { album, track };
	};
	const current = record.tracks.at(record.index);
	const now = current === undefined ? null : songAt(current.id, current.albumId);
	if (now === null) {
		return null;
	}
	const behind = record.tracks
		.slice(0, record.index)
		.flatMap(({ id, albumId }) => songAt(id, albumId) ?? []);
	const ahead: { album: Album; track: Track }[] = [];
	const waiting: ListEntry[] = [];
	for (const { id, albumId } of record.tracks.slice(record.index + 1)) {
		const song = waiting.length === 0 ? songAt(id, albumId) : null;
		if (song !== null) {
			ahead.push(song);
		} else if (waiting.length > 0 || lookup(albumId) === undefined) {
			waiting.push({ trackId: id, albumId });
		}
	}
	for (const { id, albumId } of record.later ?? []) {
		waiting.push({ trackId: id, albumId });
	}
	const songs = [...behind, now, ...ahead];
	const oneAlbum = songs.every(({ album }) => album.id === now.album.id);
	const queue: Queue = {
		album: songs[0]?.album ?? now.album,
		tracks: songs.map(({ track }) => track),
		index: behind.length,
		ordered: record.ordered,
		...(record.kind === 'album' && oneAlbum && waiting.length === 0
			? {}
			: { albums: new Map(songs.map(({ album, track }) => [track.id, album])) }),
		...(waiting.length > 0 ? { later: waiting } : {}),
		...(record.continues === undefined ? {} : { continues: { album: record.continues } }),
		...(record.shuffle === undefined ? {} : { shuffle: { id: record.shuffle.id } })
	};
	return queue;
}

/**
 * Where a record was left, as the tag and Now playing say it: by another browser, with its form
 * when it differs from this one's, or a song gone from the library, whoever saved it. Null when
 * there's nothing to say: this browser saved it.
 */
export interface Left {
	/** The saving browser's form, or `browser` when it's the same as this one's. */
	form: DeviceForm | 'browser';
	/** When it was saved, Unix ms. */
	at: number;
	positionMs: number;
	moved: boolean;
}

export function leftOf(record: PlaybackRecord, here: Client): Left | null {
	if (!record.moved && record.clientId === here.id) {
		return null;
	}
	return {
		form: record.device === here.device ? 'browser' : record.device,
		at: record.updatedAt,
		positionMs: record.positionMs,
		moved: record.moved
	};
}

const minuteMs = 60_000;
const hourMs = 60 * minuteMs;
const dayMs = 24 * hourMs;
const months = [
	'JAN',
	'FEB',
	'MAR',
	'APR',
	'MAY',
	'JUN',
	'JUL',
	'AUG',
	'SEP',
	'OCT',
	'NOV',
	'DEC'
] as const;

/** How long ago something was, as the tag says it: JUST NOW, 12 MIN AGO, 2 H AGO, YESTERDAY, 3 NOV. */
export function ageWords(at: number, now: number): string {
	const since = Math.max(0, now - at);
	if (since < minuteMs) {
		return 'JUST NOW';
	}
	if (since < hourMs) {
		return `${String(Math.floor(since / minuteMs))} MIN AGO`;
	}
	if (since < dayMs) {
		return `${String(Math.floor(since / hourMs))} H AGO`;
	}
	if (since < 2 * dayMs) {
		return 'YESTERDAY';
	}
	const day = new Date(at);
	return `${String(day.getDate())} ${months[day.getMonth()] ?? ''}`;
}

/** The same in full words, for a screen reader: just now, 12 minutes ago, 2 hours ago. */
function ageSpoken(at: number, now: number): string {
	const since = Math.max(0, now - at);
	const count = (n: number, unit: string): string =>
		`${String(n)} ${unit}${n === 1 ? '' : 's'} ago`;
	if (since < minuteMs) {
		return 'just now';
	}
	if (since < hourMs) {
		return count(Math.floor(since / minuteMs), 'minute');
	}
	if (since < dayMs) {
		return count(Math.floor(since / hourMs), 'hour');
	}
	if (since < 2 * dayMs) {
		return 'yesterday';
	}
	const day = new Date(at);
	return `on ${day.toLocaleDateString('en-GB', { day: 'numeric', month: 'long' })}`;
}

const gone = 'THAT ONE’S GONE · NEXT IN LINE';

/** The island's tag: FROM YOUR PHONE · 2 H AGO, FROM ANOTHER BROWSER · 12 MIN AGO, or why a song changed. */
export function tagWords(left: Left, now: number): string {
	if (left.moved) {
		return gone;
	}
	const from = left.form === 'browser' ? 'ANOTHER BROWSER' : `YOUR ${left.form.toUpperCase()}`;
	return `FROM ${from} · ${ageWords(left.at, now)}`;
}

/** What the island says of it to a screen reader: "Left off on your phone, 2 hours ago." */
export function leftSpoken(left: Left, now: number): string {
	if (left.moved) {
		return 'The song you left on is gone from the library. This is the one after it.';
	}
	const where = left.form === 'browser' ? 'in another browser' : `on your ${left.form}`;
	return `Left off ${where}, ${ageSpoken(left.at, now)}.`;
}

/** Now playing's note: "Your phone left this at 2:13, 2 h ago." */
export function leftNote(left: Left, now: number): string {
	if (left.moved) {
		return 'The song you left on is gone from the library. This is the one after it.';
	}
	const who = left.form === 'browser' ? 'Another browser' : `Your ${left.form}`;
	const at = clock(left.positionMs / 1000);
	if (now - left.at >= 2 * dayMs) {
		const day = new Date(left.at);
		return `${who} left this at ${at} on ${day.toLocaleDateString('en-GB', { day: 'numeric', month: 'short' })}.`;
	}
	return `${who} left this at ${at}, ${ageWords(left.at, now).toLowerCase()}.`;
}
