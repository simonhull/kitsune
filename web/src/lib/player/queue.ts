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

/** Puts songs at the end of the queue. */
export function appended(queue: Queue, songs: readonly Queued[]): Queue {
	return spliced(queue, queue.tracks.length, songs);
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
	return { ...queue, tracks, ordered: false, albums };
}

/** Takes songs out of what's still to play. The one playing, and those before it, stay. */
export function without(queue: Queue, ids: readonly string[]): Queue {
	const ahead = queue.tracks.slice(queue.index + 1).filter(({ id }) => !ids.includes(id));
	if (ahead.length === queue.tracks.length - queue.index - 1) {
		return queue;
	}
	return {
		...queue,
		tracks: [...queue.tracks.slice(0, queue.index + 1), ...ahead],
		ordered: false,
		albums: albumsOf(queue)
	};
}

/**
 * Puts the songs still to play in a new order, `ahead` naming each of them once. Anything it
 * doesn't name keeps its place after them; a name it doesn't hold is passed over.
 */
export function reordered(queue: Queue, ahead: readonly string[]): Queue {
	const rest = queue.tracks.slice(queue.index + 1);
	const byId = new Map(rest.map((track) => [track.id, track]));
	const named = ahead.flatMap((id) => byId.get(id) ?? []);
	const left = rest.filter(({ id }) => !ahead.includes(id));
	return {
		...queue,
		tracks: [...queue.tracks.slice(0, queue.index + 1), ...named, ...left],
		ordered: false,
		albums: albumsOf(queue)
	};
}

/** Queues an album as it stands, starting at `index`. */
export function inOrder(album: Album, index = 0): Queue {
	return { album, tracks: album.tracks, index, ordered: true };
}

/**
 * Queues one disc of an album, in the album's order. A song with no disc number is on disc 1. It
 * is still the album's own order, a part of it, so album gain plays.
 */
export function onDisc(album: Album, disc: number): Queue {
	const tracks = album.tracks.filter(({ discNo }) => (discNo ?? 1) === disc);
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

/** Goes back to the first track, keeping the order. */
export function rewound(queue: Queue): Queue {
	return { ...queue, index: 0 };
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
