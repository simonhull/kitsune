import type { Album, Track } from '#lib/library/source.ts';

/** What the player works through: one album, in some order, and how far it has got. */
export interface Queue {
	album: Album;
	tracks: readonly Track[];
	index: number;
	/** Whether this is the album's own order, which is what album gain is measured for. */
	ordered: boolean;
}

/** Queues an album as it stands, starting at `index`. */
export function inOrder(album: Album, index = 0): Queue {
	return { album, tracks: album.tracks, index, ordered: true };
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
	return { ...queue, album, tracks: queue.tracks.map((track) => fresh.get(track.id) ?? track) };
}
