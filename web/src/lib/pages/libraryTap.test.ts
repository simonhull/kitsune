import { describe, expect, it, vi } from 'vitest';
import type { Album } from '#lib/library/source.ts';
import { createTapper, type TapHost } from './libraryTap';

function album(id: string, ...tracks: string[]): Album {
	return {
		id,
		title: id,
		artist: 'x',
		trackCount: tracks.length,
		compilation: false,
		artists: [],
		tracks: tracks.map((track) => ({
			id: track,
			title: track,
			artists: [],
			durationMs: 1000,
			format: 'FLAC',
			genres: [],
			etag: track
		})),
		discs: [],
		durationMs: 1000,
		formats: [],
		addedAt: '2026-01-01T00:00:00Z',
		genres: [],
		etag: id
	};
}

/** A host whose fetches the test finishes by hand. */
function host(cached: Album[] = []) {
	const albums = new Map(cached.map((each) => [each.id, each]));
	const pending = new Map<string, () => void>();
	const play = vi.fn<TapHost['play']>();
	const playDisc = vi.fn<TapHost['playDisc']>();
	const failed = vi.fn<TapHost['failed']>();
	const tapper: TapHost = {
		album: (id) => albums.get(id),
		load: (id) =>
			new Promise((resolve) => {
				pending.set(id, resolve);
			}),
		play,
		playDisc,
		failed
	};
	function arrive(loaded: Album | string): Promise<void> {
		const id = typeof loaded === 'string' ? loaded : loaded.id;
		if (typeof loaded !== 'string') {
			albums.set(id, loaded);
		}
		pending.get(id)?.();
		return Promise.resolve();
	}
	return { tapper, play, playDisc, failed, arrive };
}

describe('tapping a song', () => {
	it('plays at once from an album already here: no await stands between a tap and the sound', () => {
		const { tapper, play } = host([album('a', 't1', 't2')]);
		createTapper(tapper)('a', { track: 't2' });
		expect(play).toHaveBeenCalledWith(expect.objectContaining({ id: 'a' }), 1);
	});

	it('fetches the album first, then plays on through it', async () => {
		const { tapper, play, arrive } = host();
		createTapper(tapper)('a', { track: 't2' });
		expect(play).not.toHaveBeenCalled();
		await arrive(album('a', 't1', 't2'));
		await vi.waitFor(() => {
			expect(play).toHaveBeenCalledWith(expect.objectContaining({ id: 'a' }), 1);
		});
	});

	it('says so when the album can’t be fetched, and leaves the queue alone', async () => {
		const { tapper, play, failed, arrive } = host();
		createTapper(tapper)('a', { track: 't1' });
		await arrive('a');
		await vi.waitFor(() => {
			expect(failed).toHaveBeenCalledWith('a');
		});
		expect(play).not.toHaveBeenCalled();
	});

	it('says so for a song its album no longer has', () => {
		const { tapper, play, failed } = host([album('a', 't1')]);
		createTapper(tapper)('a', { track: 'gone' });
		expect(failed).toHaveBeenCalledWith('a');
		expect(play).not.toHaveBeenCalled();
	});

	it('lets a second tap win: the older fetch, landing late, does nothing', async () => {
		const { tapper, play, failed, arrive } = host();
		const tap = createTapper(tapper);
		tap('a', { track: 't1' });
		tap('b', { track: 't9' });
		await arrive(album('b', 't9'));
		await vi.waitFor(() => {
			expect(play).toHaveBeenCalledTimes(1);
		});
		await arrive(album('a', 't1'));
		await new Promise((resolve) => setTimeout(resolve));
		expect(play).toHaveBeenCalledTimes(1);
		expect(play).toHaveBeenCalledWith(expect.objectContaining({ id: 'b' }), 0);
		expect(failed).not.toHaveBeenCalled();
	});
});

describe('tapping a disc', () => {
	function discs(): Album {
		const two = album('a', 't1', 't2', 't3');
		return {
			...two,
			tracks: two.tracks.map((track, index) => ({ ...track, discNo: index === 0 ? 1 : 2 }))
		};
	}

	it('plays that disc of an album already here, at once', () => {
		const { tapper, playDisc, play } = host([discs()]);
		createTapper(tapper)('a', { disc: 2 });
		expect(playDisc).toHaveBeenCalledWith(expect.objectContaining({ id: 'a' }), 2);
		expect(play).not.toHaveBeenCalled();
	});

	it('fetches the album first, and says so for a disc it no longer has', async () => {
		const { tapper, playDisc, failed, arrive } = host();
		createTapper(tapper)('a', { disc: 3 });
		await arrive(discs());
		await vi.waitFor(() => {
			expect(failed).toHaveBeenCalledWith('a');
		});
		expect(playDisc).not.toHaveBeenCalled();
	});
});
