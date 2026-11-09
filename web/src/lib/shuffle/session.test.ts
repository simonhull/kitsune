import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from '#lib/library/fixtures.ts';
import { Library } from '#lib/library/library.svelte.ts';
import {
	AlbumNotFoundError,
	ServerError,
	type Album,
	type LibrarySource,
	type ShufflePage,
	type ShufflePick,
	type ShuffleStart,
	type Track
} from '#lib/library/source.ts';
import { SilentOutput } from '#lib/player/output.ts';
import { Player } from '#lib/player/player.svelte.ts';
import type { Unit } from './pick.ts';
import { Shuffles, type ShuffleSource } from './session.svelte.ts';

function track(album: string, index: number): Track {
	return {
		id: `${album}-${String(index)}`,
		title: `${album} ${String(index)}`,
		artists: ['A'],
		durationMs: 100_000,
		format: 'FLAC',
		genres: [],
		etag: '',
		hearted: false
	};
}

/** Albums a to t, three songs each. */
const albums = new Map<string, Album>(
	Array.from({ length: 20 }, (_, n) => {
		const id = String.fromCharCode(97 + n);
		const album: Album = {
			id,
			title: id.toUpperCase(),
			artist: 'A',
			artists: [],
			trackCount: 3,
			compilation: false,
			tracks: [0, 1, 2].map((index) => track(id, index)),
			discs: [],
			durationMs: 300_000,
			formats: ['FLAC'],
			addedAt: '2026-10-12T18:00:00Z',
			genres: [],
			etag: '',
			hearted: false
		};
		return [id, album] as const;
	})
);

function pick(id: string): ShufflePick {
	const albumId = id.split('-')[0] ?? '';
	return {
		songs: [
			{ id, title: id, artists: ['A'], albumId, album: albumId.toUpperCase(), durationMs: 1 }
		],
		why: {
			stars: 0,
			rating: 1,
			hearted: false,
			heart: 1,
			played: false,
			sinceDays: 0,
			recency: 1.2,
			memory: true,
			earlySkips: 0,
			skipMemory: 1,
			starts: 0,
			skips: 0,
			skipRate: 1,
			listens: 0,
			heard: 0,
			listen: 1,
			flow: 1
		}
	};
}

/** Every song but the first, in album order: the server's order, for the test. */
const order = [...albums.values()].flatMap(({ tracks }) => tracks.map(({ id }) => id));

function page(from: number, first?: string, size = 12): ShufflePage {
	const ids = first === undefined ? order : [first, ...order.filter((id) => id !== first)];
	const picks = ids.slice(from, from + size).map(pick);
	return {
		id: 'S1',
		mode: 'endless',
		from,
		to: from + picks.length,
		songs: ids.length,
		done: false,
		picks
	};
}

function setup(overrides: Partial<LibrarySource> = {}) {
	const starts: ShuffleStart[] = [];
	const nexts: number[] = [];
	const source: LibrarySource = {
		...createFixtureSource(),
		streamUrl: (id: string) => id,
		album: (id) => {
			const album = albums.get(id);
			return album === undefined
				? Promise.reject(new AlbumNotFoundError(id))
				: Promise.resolve(album);
		},
		startShuffle: (start) => {
			starts.push(start);
			return Promise.resolve(page(0, start.first));
		},
		nextShuffle: (_id, after) => {
			nexts.push(after);
			return Promise.resolve(page(after));
		},
		...overrides
	};
	const library = new Library(source, { simulated: true });
	vi.spyOn(library, 'reportPlay').mockImplementation(() => undefined);
	const player = new Player(library, () => new SilentOutput({ ticking: false }));
	return { library, player, shuffles: new Shuffles(library, player), starts, nexts };
}

function sourceOf(units: Unit[]): ShuffleSource {
	return {
		key: 'library:everything',
		scope: { kind: 'library' },
		mode: 'endless',
		label: 'THE LIBRARY',
		units: () => units
	};
}

/** Lets everything waiting on a resolved promise, and a timer, run. */
async function settle(times = 6): Promise<void> {
	for (let i = 0; i < times; i++) {
		await new Promise((done) => setTimeout(done));
	}
}

const unitOf = (id: string): Unit => ({
	songs: [{ id, albumId: id.split('-')[0] ?? '', hearted: false }]
});

describe('Shuffles', () => {
	it('plays the first song inside the press when its album is here, and asks the server with it', async () => {
		const { library, player, shuffles, starts } = setup();
		await library.loadAlbum('c');
		shuffles.start(sourceOf([unitOf('c-1')]));
		expect(player.current?.id).toBe('c-1');
		expect(player.sounding).toBe(true);
		expect(starts).toEqual([{ scope: { kind: 'library' }, mode: 'endless', first: 'c-1' }]);
		await settle();
		expect(shuffles.current?.ref.id).toBe('S1');
		const queued = player.queue?.tracks.map(({ id }) => id) ?? [];
		expect(queued.slice(0, 4)).toEqual(['c-1', 'a-0', 'a-1', 'a-2']);
	});

	it('fetches a first song’s album, then plays it', async () => {
		const { player, shuffles } = setup();
		shuffles.start(sourceOf([unitOf('d-2')]));
		expect(player.current).toBeNull();
		await settle();
		expect(player.current?.id).toBe('d-2');
	});

	it('plays a whole run from its first song', async () => {
		const { library, player, shuffles } = setup();
		await Promise.all([library.loadAlbum('a'), library.loadAlbum('b')]);
		const run: Unit = {
			songs: [
				{ id: 'b-0', albumId: 'b', hearted: false },
				{ id: 'a-2', albumId: 'a', hearted: false }
			]
		};
		shuffles.start(sourceOf([run]));
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['b-0', 'a-2']);
	});

	it('keeps three songs queued ahead, and fetches the next page before the picks run out', async () => {
		const { library, player, shuffles, nexts } = setup();
		await library.loadAlbum('a');
		shuffles.start(sourceOf([unitOf('a-0')]));
		await settle();
		expect(nexts).toEqual([12]);
		for (let i = 0; i < 30; i++) {
			player.next();
			await settle();
			const queue = player.queue;
			expect((queue?.tracks.length ?? 0) - (queue?.index ?? 0) - 1).toBeGreaterThanOrEqual(3);
		}
		expect(nexts.length).toBeGreaterThan(1);
		expect(new Set(nexts).size).toBe(nexts.length);
	});

	it('plays on when the server fails, and asks again', async () => {
		vi.useFakeTimers({ toFake: ['setTimeout'] });
		let fail = true;
		const startShuffle = vi.fn((start: ShuffleStart) =>
			fail ? Promise.reject(new ServerError(503)) : Promise.resolve(page(0, start.first))
		);
		const { library, player, shuffles } = setup({ startShuffle });
		const loaded = library.loadAlbum('a');
		await vi.runAllTimersAsync();
		await loaded;
		shuffles.start(sourceOf([unitOf('a-0')]));
		expect(player.current?.id).toBe('a-0');
		await vi.advanceTimersByTimeAsync(10);
		expect(startShuffle).toHaveBeenCalledOnce();
		fail = false;
		await vi.advanceTimersByTimeAsync(1100);
		vi.useRealTimers();
		await settle();
		expect(startShuffle).toHaveBeenCalledTimes(2);
		expect(player.current?.id).toBe('a-0');
		expect(player.queue?.tracks.length).toBeGreaterThan(3);
	});

	it('stops feeding a queue someone replaced', async () => {
		const { library, player, shuffles } = setup();
		await library.loadAlbum('a');
		shuffles.start(sourceOf([unitOf('a-0')]));
		await settle();
		const album = albums.get('e');
		if (album === undefined) {
			throw new Error('no album e');
		}
		player.play(album);
		await settle();
		expect(shuffles.current?.active).toBe(false);
		expect(shuffles.current?.upNext).toEqual([]);
		expect(player.queue?.tracks.map(({ id }) => id)).toEqual(['e-0', 'e-1', 'e-2']);
	});

	it('names what comes up next, queued or not, with why', async () => {
		const { library, shuffles } = setup();
		await library.loadAlbum('a');
		shuffles.start(sourceOf([unitOf('a-0')]));
		await settle();
		const next = shuffles.current?.upNext ?? [];
		expect(next.at(0)).toMatchObject({ titles: ['a 1'], album: { id: 'a' } });
		expect(next.at(0)?.why?.recency).toBe(1.2);
		// The rest of the first page, and the second, fetched as the first ran low.
		expect(next.length).toBe(23);
	});

	it('uses a pick made ahead once', async () => {
		const { player, shuffles } = setup();
		const units = vi.fn(() => [unitOf('f-1')]);
		const source = { ...sourceOf([]), units };
		await shuffles.prime(source);
		shuffles.start(source);
		expect(player.current?.id).toBe('f-1');
		expect(units).toHaveBeenCalledOnce();
		shuffles.start(source);
		expect(units).toHaveBeenCalledTimes(2);
	});
});

describe('Up next', () => {
	async function shuffling(): Promise<ReturnType<typeof setup>> {
		const context = setup();
		await context.library.loadAlbum('c');
		context.shuffles.start(sourceOf([unitOf('c-1')]));
		await settle();
		return context;
	}

	const keys = (shuffles: Shuffles): string[] =>
		(shuffles.current?.upNext ?? []).slice(0, 5).map(({ key }) => key);

	it('plays next straight after the song playing, and the shuffle carries on after it', async () => {
		const { library, player, shuffles } = await shuffling();
		await library.loadAlbum('t');
		const album = library.album('t');
		const song = album?.tracks.at(0);
		if (album === undefined || song === undefined) {
			throw new Error('no album t');
		}
		player.playNext([{ album, track: song }]);
		expect(keys(shuffles)).toEqual(['t-0', 'a-0', 'a-1', 'a-2', 'b-0']);
		expect(shuffles.current?.active).toBe(true);
		player.next();
		await settle();
		expect(player.current?.id).toBe('t-0');
		player.next();
		await settle();
		expect(player.current?.id).toBe('a-0');
		expect(shuffles.current?.active).toBe(true);
	});

	it('takes songs out, queued or still to queue, and tops the queue up again', async () => {
		const { player, shuffles } = await shuffling();
		shuffles.current?.remove(['a-1', 'b-1']);
		await settle();
		expect(keys(shuffles)).toEqual(['a-0', 'a-2', 'b-0', 'b-2', 'c-0']);
		const queue = player.queue;
		expect((queue?.tracks.length ?? 0) - (queue?.index ?? 0) - 1).toBeGreaterThanOrEqual(3);
	});

	it('moves a song up or down, across what is queued and what is still to queue', async () => {
		const { shuffles } = await shuffling();
		const shuffle = shuffles.current;
		await shuffle?.move('a-0', 1);
		expect(keys(shuffles)).toEqual(['a-1', 'a-0', 'a-2', 'b-0', 'b-1']);
		// a-2 is the last queued, b-0 the first pick: they trade places.
		await shuffle?.move('a-2', 1);
		expect(keys(shuffles)).toEqual(['a-1', 'a-0', 'b-0', 'a-2', 'b-1']);
		expect(shuffle?.upNext.at(2)?.queued).toBe(true);
		expect(shuffle?.upNext.at(3)?.queued).toBe(false);
		await shuffle?.move('b-1', -1);
		expect(keys(shuffles)).toEqual(['a-1', 'a-0', 'b-0', 'b-1', 'a-2']);
	});
});
