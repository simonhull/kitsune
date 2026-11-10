import { discOf } from '#lib/library/format.ts';
import type { Album, Track } from '#lib/library/source.ts';

/**
 * What the player works through, and how far it has got: one album in some order, or songs picked
 * from several, each with its album.
 */
export interface Queue {
	/** The album it was started from: the first song's. */
	album: Album;
	tracks: readonly Track[];
	index: number;
	/** Whether this is the album's own order, which is what album gain is measured for. */
	ordered: boolean;
	/** For songs from more than one album, each song's, by track ID. Absent for one album. */
	albums?: ReadonlyMap<string, Album>;
	/** The shuffle it plays, which goes on adding to it. */
	shuffle?: ShuffleRef;
	/** Set when it goes on through a list, album by album, past its last song. */
	continues?: Continues;
}

/**
 * A queue going on through a list, the Library's, an album at a time. The list's order is never
 * kept here: it's read when the next album is wanted, so a list that changes is followed.
 */
export interface Continues {
	/** The album of the list the queue took in last: the next one is the album after it. */
	album: string;
	/** Where that album's songs are in the queue, and the album it was put after. */
	ahead?: Ahead;
}

/** The album a list put after a queue: where its songs start, how many, and what it follows. */
export interface Ahead {
	at: number;
	count: number;
	after: string;
}

/**
 * What a queue goes on to past its last song. This is the one place that decides, and the first
 * that applies wins: a repeat, when there is one, goes first. A shuffle owns what comes after it;
 * a queue started from a list goes on through it; anything else comes to rest.
 */
export type Onward = 'shuffle' | 'list' | 'rest';

export function onwardOf({ shuffle, continues }: Queue): Onward {
	if (shuffle !== undefined) {
		return 'shuffle';
	}
	return continues === undefined ? 'rest' : 'list';
}

/**
 * Where the song playing is among the songs of its album around it, from 0, and how many there
 * are: its place in the album, for a queue made of whole albums.
 */
export function albumRun(queue: Queue): { index: number; trackCount: number } {
	const current = queue.tracks.at(queue.index);
	if (current === undefined) {
		return { index: queue.index, trackCount: queue.tracks.length };
	}
	const album = albumOf(queue, current).id;
	const same = (track: Track | undefined): boolean =>
		track !== undefined && albumOf(queue, track).id === album;
	let start = queue.index;
	while (start > 0 && same(queue.tracks.at(start - 1))) {
		start -= 1;
	}
	let end = queue.index + 1;
	while (same(queue.tracks.at(end))) {
		end += 1;
	}
	return { index: queue.index - start, trackCount: end - start };
}

/** Where a queue was started: a list goes on past the album, anywhere else comes to rest after it. */
export type StartedFrom = 'list' | 'elsewhere';

/**
 * Whether an album's play button, started from `from`, would pause the queue rather than start it
 * again: the queue is that album alone, from its first song or any other; or, from a list, the
 * queue goes on through the list and the song playing is the album's.
 */
export function playsAlbum(queue: Queue | null, albumId: string, from: StartedFrom): boolean {
	if (queue === null) {
		return false;
	}
	if (queue.album.id === albumId && queue.albums === undefined) {
		return true;
	}
	const current = queue.tracks.at(queue.index);
	return (
		from === 'list' &&
		queue.continues !== undefined &&
		current !== undefined &&
		albumOf(queue, current).id === albumId
	);
}

/** Marks a queue started from a list at `album`, so it goes on through the list after it. */
export function throughList(queue: Queue): Queue {
	return { ...queue, continues: { album: queue.album.id } };
}

/**
 * The album put after the queue that may still change: one taken in from the list whose first song
 * hasn't begun. Once it has, it's the album playing, and the list goes on from it.
 */
export function pendingAlbum(queue: Queue): Ahead | undefined {
	const ahead = queue.continues?.ahead;
	return ahead !== undefined && queue.index < ahead.at ? ahead : undefined;
}

/**
 * Puts the list's next album after the queue, in the album's own order. The queue keeps whether it
 * plays album gain: each album in it is whole and in order, as when that album plays.
 */
export function continued(queue: Queue, album: Album): Queue {
	const after = queue.continues?.album ?? queue.album.id;
	const at = queue.tracks.length;
	const { ordered } = queue;
	return {
		...spliced(
			queue,
			at,
			album.tracks.map((track) => ({ album, track }))
		),
		ordered,
		continues: { album: album.id, ahead: { at, count: album.tracks.length, after } }
	};
}

/**
 * Takes back the album put after the queue while it hasn't begun, for a list that has changed:
 * the queue goes on from the album before it again. Songs added after it stay.
 */
export function withdrawn(queue: Queue): Queue {
	const ahead = pendingAlbum(queue);
	if (ahead === undefined) {
		return queue;
	}
	const tracks = queue.tracks.toSpliced(ahead.at, ahead.count);
	const albums = new Map(tracks.map((track) => [track.id, albumOf(queue, track)]));
	return { ...queue, tracks, albums, continues: { album: ahead.after } };
}

/**
 * A shuffle, as the queue and its listens name it. Its ID is the client's own until the server's
 * answer gives it the session's, since the first song never waits for that.
 */
export interface ShuffleRef {
	id: string;
}

/** A song to queue, with the album it's on. */
export interface Queued {
	album: Album;
	track: Track;
}

/** The album a song in the queue is on. */
export function albumOf(queue: Queue, track: Track): Album {
	return queue.albums?.get(track.id) ?? queue.album;
}

/**
 * Queues songs picked from anywhere, in the order given. Their own gain plays: album gain is
 * measured for a whole album, in its order. None makes no queue.
 */
export function picked(songs: readonly Queued[]): Queue | null {
	const first = songs.at(0);
	if (first === undefined) {
		return null;
	}
	return {
		album: first.album,
		tracks: songs.map(({ track }) => track),
		index: 0,
		ordered: false,
		albums: new Map(songs.map(({ album, track }) => [track.id, album]))
	};
}

/** Every song's album, as a map, for a queue that's about to hold more than one. */
function albumsOf(queue: Queue): Map<string, Album> {
	return new Map(queue.tracks.map((track) => [track.id, albumOf(queue, track)]));
}

/** Puts songs right after the one playing. The queue's own order is no longer the album's. */
export function playingNext(queue: Queue, songs: readonly Queued[]): Queue {
	return spliced(queue, queue.index + 1, songs);
}

/** Puts a shuffle's next songs at the end of its queue. The order is the shuffle's, as it was. */
export function extended(queue: Queue, songs: readonly Queued[]): Queue {
	const { ordered } = queue;
	return { ...spliced(queue, queue.tracks.length, songs), ordered };
}

/**
 * Puts songs at the end of the queue: ahead of an album a list put after it that hasn't begun,
 * since what someone queues plays before the list goes on.
 */
export function appended(queue: Queue, songs: readonly Queued[]): Queue {
	return spliced(queue, pendingAlbum(queue)?.at ?? queue.tracks.length, songs);
}

function spliced(queue: Queue, at: number, songs: readonly Queued[]): Queue {
	if (songs.length === 0) {
		return queue;
	}
	const albums = albumsOf(queue);
	for (const { album, track } of songs) {
		albums.set(track.id, album);
	}
	const tracks = [...queue.tracks];
	tracks.splice(at, 0, ...songs.map(({ track }) => track));
	return {
		...queue,
		tracks,
		ordered: false,
		albums,
		continues: shifted(queue.continues, at, songs.length)
	};
}

/** Keeps a list's album ahead where its songs are, when songs go in before them. */
function shifted(continues: Continues | undefined, at: number, by: number): Continues | undefined {
	const ahead = continues?.ahead;
	if (continues === undefined || ahead === undefined || at > ahead.at) {
		return continues;
	}
	return { ...continues, ahead: { ...ahead, at: ahead.at + by } };
}

/** Takes songs out of what's still to play. The one playing, and those before it, stay. */
export function without(queue: Queue, ids: readonly string[]): Queue {
	const rest = aheadPlaces(queue).filter((at) => {
		const track = queue.tracks.at(at);
		return track !== undefined && !ids.includes(track.id);
	});
	return rest.length === queue.tracks.length - queue.index - 1 ? queue : rearranged(queue, rest);
}

/**
 * Puts the songs still to play in a new order, `ahead` naming each of them once. Anything it
 * doesn't name keeps its place after them; a name it doesn't hold is passed over.
 */
export function reordered(queue: Queue, ahead: readonly string[]): Queue {
	const rest = aheadPlaces(queue);
	const idAt = (at: number): string | undefined => queue.tracks.at(at)?.id;
	const byId = new Map(rest.map((at) => [idAt(at), at]));
	const named = ahead.flatMap((id) => byId.get(id) ?? []);
	const left = rest.filter((at) => {
		const id = idAt(at);
		return id === undefined || !ahead.includes(id);
	});
	return rearranged(queue, [...named, ...left]);
}

/** Where the songs still to play are in the queue. */
function aheadPlaces(queue: Queue): number[] {
	return Array.from(
		{ length: Math.max(0, queue.tracks.length - queue.index - 1) },
		(_, offset) => queue.index + 1 + offset
	);
}

/**
 * The queue with what's still to play replaced by the songs at `rest`, places in the queue as it
 * was, in their new order. An album a list put after the queue stays withdrawable while its songs
 * are still together and in order; once they aren't, the person has made it theirs, and the list
 * goes on after it as from any album that has begun.
 */
function rearranged(queue: Queue, rest: readonly number[]): Queue {
	const tracks = [
		...queue.tracks.slice(0, queue.index + 1),
		...rest.flatMap((at) => queue.tracks.at(at) ?? [])
	];
	return {
		...queue,
		tracks,
		ordered: false,
		albums: albumsOf(queue),
		continues: regrouped(queue, rest)
	};
}

/** Where a list's pending album is after a rearrangement, if it can still be taken back whole. */
function regrouped(queue: Queue, rest: readonly number[]): Continues | undefined {
	const { continues } = queue;
	const ahead = pendingAlbum(queue);
	if (continues === undefined || ahead === undefined) {
		return continues;
	}
	const kept = rest.flatMap((at, place) =>
		at >= ahead.at && at < ahead.at + ahead.count ? [{ at, place }] : []
	);
	const first = kept.at(0);
	// Next to each other, and in the album's order.
	const together = kept.every((entry, k) => {
		const before = kept.at(k - 1);
		return (
			k === 0 || (before !== undefined && entry.place === before.place + 1 && entry.at > before.at)
		);
	});
	if (first === undefined || !together) {
		return { album: continues.album };
	}
	return {
		...continues,
		ahead: { ...ahead, at: queue.index + 1 + first.place, count: kept.length }
	};
}

/** Queues an album as it stands, starting at `index`. */
export function inOrder(album: Album, index = 0): Queue {
	return { album, tracks: album.tracks, index, ordered: true };
}

/**
 * Queues one disc of an album, in the album's order: `noDisc` is the songs with no disc number. It
 * is still the album's own order, a part of it, so album gain plays.
 */
export function onDisc(album: Album, disc: number): Queue {
	const tracks = album.tracks.filter(({ discNo }) => discOf(discNo) === disc);
	return { album, tracks, index: 0, ordered: true };
}

/** A small seeded generator (mulberry32), so a shuffle can be repeated. */
function generator(seed: number): () => number {
	let state = seed;
	return () => {
		state = (state + 0x6d2b79f5) | 0;
		let mixed = Math.imul(state ^ (state >>> 15), 1 | state);
		mixed = (mixed + Math.imul(mixed ^ (mixed >>> 7), 61 | mixed)) ^ mixed;
		return ((mixed ^ (mixed >>> 14)) >>> 0) / 2 ** 32;
	};
}

/**
 * Queues an album by Fisher-Yates, striking each track out of what's left. `chosen` plays first:
 * a track the seed picks, unless someone asked for one.
 */
export function shuffled(album: Album, seed: number, chosen?: number): Queue {
	const random = generator(seed);
	const left = [...album.tracks];
	const tracks = left.splice(chosen ?? Math.floor(random() * left.length), 1);
	while (left.length > 0) {
		tracks.push(...left.splice(Math.floor(random() * left.length), 1));
	}
	return { album, tracks, index: 0, ordered: false };
}

/** Returns the track the queue is on. */
export function currentOf({ tracks, index }: Queue): Track | undefined {
	return tracks.at(index);
}

/** Returns the track after this one, without moving. */
export function upcoming({ tracks, index }: Queue): Track | undefined {
	return tracks[index + 1];
}

/** Moves on a track, or returns null at the end of the queue. */
export function next(queue: Queue): Queue | null {
	return upcoming(queue) === undefined ? null : { ...queue, index: queue.index + 1 };
}

/** Moves back a track, or returns null at the start of the queue. */
export function previous(queue: Queue): Queue | null {
	return queue.index === 0 ? null : { ...queue, index: queue.index - 1 };
}

/**
 * Goes back to the first track, keeping the order. It has come to rest: going round again plays
 * what it holds, and nothing past it.
 */
export function rewound(queue: Queue): Queue {
	return { ...queue, index: 0, continues: undefined };
}

/** Takes in an album just edited, keeping the order and the place. Renumbering moves neither. */
export function retagged(queue: Queue, album: Album): Queue {
	const fresh = new Map(album.tracks.map((track) => [track.id, track]));
	const tracks = queue.tracks.map((track) => fresh.get(track.id) ?? track);
	const home = queue.album.id === album.id ? album : queue.album;
	if (queue.albums === undefined) {
		return { ...queue, album: home, tracks };
	}
	const albums = new Map(
		[...queue.albums].map(([id, each]) => [id, each.id === album.id ? album : each])
	);
	return { ...queue, album: home, tracks, albums };
}

/** Whether an album has songs in the queue. */
export function holds(queue: Queue, albumId: string): boolean {
	return (
		queue.album.id === albumId ||
		(queue.albums !== undefined && [...queue.albums.values()].some(({ id }) => id === albumId))
	);
}
