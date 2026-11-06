import { afterEach, describe, expect, it, vi } from 'vitest';
import { letterOf } from '#lib/pages/libraryRows.ts';
import { burstSize, burstTimes, createFixtureSource, createScanFixtureSource } from './fixtures';
import { AlbumNotFoundError, ArtistNotFoundError, LoginError, type LibrarySource } from './source';

const source: LibrarySource = createFixtureSource();

afterEach(() => {
	vi.useRealTimers();
});

describe('fixtures', () => {
	it('serves an album for every list entry, with the same first-frame fields', async () => {
		const pages = await Promise.all([source.albumsPage('added'), source.albumsPage('name')]);
		for (const entry of pages.flatMap((page) => page.albums)) {
			const album = await source.album(entry.id);
			expect(album).toMatchObject(entry);
			expect(album.tracks.length).toBe(entry.trackCount);
		}
	});

	it('lists the newest album first and honors a limit', async () => {
		const { albums, nextCursor } = await source.albumsPage('added', undefined, 3);
		expect(albums.map((album) => album.title)).toEqual(['Glasshouse', 'Paper Suns', 'Red Signal']);
		expect(nextCursor).toBeUndefined();
	});

	it('sorts by name', async () => {
		const titles = (await source.albumsPage('name')).albums.map((album) => album.title);
		expect(titles).toEqual([...titles].sort((a, b) => a.localeCompare(b)));
	});

	it('keeps one album off Home for the no-card path', async () => {
		const listed = (await source.albumsPage('name')).albums.map((entry) => entry.id);
		expect(listed).not.toContain('tin-lantern-78');
		await expect(source.album('tin-lantern-78')).resolves.toBeDefined();
	});

	it('finds artists, albums and tracks, whatever the case', async () => {
		const { artists, albums, tracks } = await source.search('GLASSHOUSE');
		expect(albums.map(({ title }) => title)).toEqual(['Glasshouse']);
		expect(artists).toEqual([]);
		expect((await source.search('duarte')).artists.map(({ name }) => name)).toEqual([
			'Ines Duarte'
		]);
		expect(tracks).toEqual([]);
		expect((await source.search('salt in')).tracks.at(0)).toMatchObject({
			title: 'Salt in the Lungs',
			albumId: 'glasshouse-66',
			album: 'Glasshouse'
		});
	});

	it('finds nothing for nothing, or for what it never heard of', async () => {
		const empty = { artists: [], albums: [], tracks: [] };
		await expect(source.search('  ')).resolves.toEqual(empty);
		await expect(source.search('zzzz')).resolves.toEqual(empty);
	});

	it('rejects a search taken back while it was on its way', async () => {
		const question = new AbortController();
		const asked = createFixtureSource({ latencyMs: 5 }).search('fox', question.signal);
		question.abort();
		await expect(asked).rejects.toMatchObject({ name: 'AbortError' });
	});

	it('lists the artists who own an album, and no one else', async () => {
		const names = (await source.artists()).map(({ name }) => name);
		expect(names).toContain('Low Tide Choir');
		expect(names).not.toContain('Ines Duarte');
		expect(names).not.toContain('Various Artists');
		expect(names).toEqual(names.toSorted((a, b) => a.localeCompare(b)));
	});

	it('splits what an artist owns from where it appears', async () => {
		const ilse = await source.artist('artist-ilse-varga');
		expect(ilse.albums.map(({ title }) => title)).toEqual(['Quiet Machines']);
		expect(ilse.appearsOn.map(({ title }) => title)).toEqual(['Sea Sampler']);
	});

	it('gives a guest a page with nothing of its own', async () => {
		const guest = await source.artist('artist-ines-duarte');
		expect(guest.albumCount).toBe(0);
		expect(guest.appearsOn.map(({ title }) => title)).toEqual(['Glasshouse', 'Sea Sampler']);
	});

	it('links an album to its artist, and a compilation to nobody', async () => {
		expect((await source.album('glasshouse-66')).artists).toEqual([
			{ id: 'artist-low-tide-choir', name: 'Low Tide Choir' }
		]);
		expect((await source.album('sea-sampler-31')).artists).toEqual([]);
	});

	it('finds no one for an unknown artist', async () => {
		await expect(source.artist('nobody')).rejects.toBeInstanceOf(ArtistNotFoundError);
	});

	it('has a user, and turns away a login with no name', async () => {
		expect((await source.me()).name).toBe('Simon');
		await expect(source.login('', 'x')).rejects.toBeInstanceOf(LoginError);
	});

	it('rejects an unknown album', async () => {
		await expect(source.album('nope')).rejects.toBeInstanceOf(AlbumNotFoundError);
	});

	it('honors latency', async () => {
		vi.useFakeTimers();
		const slow = createFixtureSource({ latencyMs: 300 });
		let settled = false;
		const pending = slow.facts().then(() => {
			settled = true;
		});
		await vi.advanceTimersByTimeAsync(299);
		expect(settled).toBe(false);
		await vi.advanceTimersByTimeAsync(1);
		await pending;
		expect(settled).toBe(true);
	});
});

describe('edits on the fixtures', () => {
	it('lets the signed-in user edit tags', async () => {
		await expect(createFixtureSource().me()).resolves.toMatchObject({ canEditTags: true });
	});

	it('keeps tags from anyone but an administrator', async () => {
		const source = createFixtureSource({ role: 'user' });
		await expect(source.me()).resolves.toMatchObject({ canEditTags: false });
		await expect(source.albumTags('glasshouse-66')).rejects.toMatchObject({
			failure: { kind: 'forbidden' }
		});
	});

	it('moves ETags on, as a rewrite would, and refuses an old one', async () => {
		const source = createFixtureSource();
		const before = await source.album('glasshouse-66');
		const after = await source.editAlbum('glasshouse-66', before.etag, {
			all: { set: { date: ['1999'], genres: ['Noise'] } }
		});
		expect(after.year).toBe(1999);
		expect(after.genres).toEqual(['Noise']);
		expect(
			after.tracks.every(({ year, genres }) => year === 1999 && genres.at(0) === 'Noise')
		).toBe(true);
		expect(after.etag).not.toBe(before.etag);
		await expect(
			source.editAlbum('glasshouse-66', before.etag, { all: { set: { date: ['2000'] } } })
		).rejects.toMatchObject({ failure: { kind: 'stale' } });
	});

	it('reads back what it wrote, track on top of album, and reorders by number', async () => {
		const source = createFixtureSource();
		const { etag } = await source.album('glasshouse-66');
		const after = await source.editAlbum('glasshouse-66', etag, {
			all: { set: { date: ['1999'] } },
			tracks: [
				{ id: 'glasshouse-66-1', set: { title: ['First'], track: ['99'] }, clear: ['date'] },
				{ id: 'glasshouse-66-2', tags: [{ key: 'MOOD', values: [] }] }
			]
		});
		expect(after.tracks.at(-1)).toMatchObject({ id: 'glasshouse-66-1', title: 'First' });
		const tags = await source.albumTags('glasshouse-66');
		expect(tags.etag).toBe(after.etag);
		const [first, second] = tags.tracks;
		expect(first?.fields.date).toEqual([]);
		expect(second?.fields.date).toEqual(['1999']);
		expect(second?.tags.map(({ key }) => key)).not.toContain('MOOD');
	});

	it('shows a tag editor the boards’ tags, read-only ones included', async () => {
		const { tracks } = await createFixtureSource().albumTags('glasshouse-66');
		const [, second, third] = tracks;
		expect(second?.fields.composers).toEqual(['Mara Quill']);
		expect(third?.readOnly).toEqual(['lyrics']);
		expect(second?.tags.filter(({ readOnly }) => readOnly === true).length).toBeGreaterThan(0);
	});

	it('keeps an edit to the source it was made on', async () => {
		const { etag } = await createFixtureSource().album('night-ferry-94');
		await createFixtureSource().editAlbum('night-ferry-94', etag, {
			all: { set: { date: ['1999'] } }
		});
		const fresh = await createFixtureSource().album('night-ferry-94');
		expect(fresh.year).not.toBe(1999);
	});

	it('answers for an album it has never heard of', async () => {
		const source = createFixtureSource();
		await expect(source.editAlbum('nope', 'x', {})).rejects.toMatchObject({
			failure: { kind: 'gone' }
		});
		await expect(source.albumTags('nope')).rejects.toMatchObject({ failure: { kind: 'gone' } });
	});

	describe('faults', () => {
		const edit = { all: { set: { date: ['1999'] } } };

		it('fails once, then takes the same edit', async () => {
			const source = createFixtureSource({ editFault: 'busy' });
			const { etag } = await source.album('glasshouse-66');
			await expect(source.editAlbum('glasshouse-66', etag, edit)).rejects.toMatchObject({
				failure: { kind: 'busy' }
			});
			await expect(source.editAlbum('glasshouse-66', etag, edit)).resolves.toMatchObject({
				year: 1999
			});
		});

		it('moves the album on under a stale edit, and not under a drift', async () => {
			const stale = createFixtureSource({ editFault: 'stale' });
			const { etag } = await stale.album('glasshouse-66');
			await expect(stale.editAlbum('glasshouse-66', etag, edit)).rejects.toMatchObject({
				failure: { kind: 'stale' }
			});
			expect((await stale.albumTags('glasshouse-66')).etag).not.toBe(etag);
			const drift = createFixtureSource({ editFault: 'drift' });
			await expect(drift.editAlbum('glasshouse-66', etag, edit)).rejects.toMatchObject({
				failure: { kind: 'stale' }
			});
			expect((await drift.albumTags('glasshouse-66')).etag).toBe(etag);
		});

		it('stops partway, holding what it reached', async () => {
			const source = createFixtureSource({ editFault: 'partial' });
			const { etag } = await source.album('glasshouse-66');
			const error: unknown = await source.editAlbum('glasshouse-66', etag, edit).catch((e) => e);
			expect(error).toMatchObject({ failure: { kind: 'failed' } });
			const { tracks } = await source.albumTags('glasshouse-66');
			expect(tracks.filter(({ fields }) => fields.date?.at(0) === '1999').length).toBe(5);
		});
	});

	it('holds the library during a scan, as a server does', async () => {
		const source = createScanFixtureSource({
			scanMs: 1000,
			gapMs: 100,
			startedAt: 0,
			now: () => 0
		});
		await expect(source.editAlbum('scan-0', 'x', {})).rejects.toMatchObject({
			failure: { kind: 'busy' }
		});
	});
});

describe('a scan', () => {
	let clock = 0;
	const scan = createScanFixtureSource({
		scanMs: 400_000,
		gapMs: 27_000,
		startedAt: 0,
		now: () => clock
	});
	const times = burstTimes(400_000, 27_000);

	function at(ms: number): void {
		clock = ms;
	}

	it('finds nothing at first, and is not ready until the end', async () => {
		at(0);
		await expect(scan.facts()).resolves.toMatchObject({ albums: 0, tracks: 0, roots: 1 });
		await expect(scan.ready()).resolves.toBe(false);
		at(399_999);
		await expect(scan.ready()).resolves.toBe(false);
		at(400_000);
		await expect(scan.ready()).resolves.toBe(true);
	});

	it('lands bursts one at a time, at uneven gaps', async () => {
		at(times.at(0) ?? 0);
		const first = burstSize(0, 400_000, 27_000);
		await expect(scan.facts()).resolves.toMatchObject({ albums: first });
		at(times.at(1) ?? 0);
		await expect(scan.facts()).resolves.toMatchObject({
			albums: first + burstSize(1, 400_000, 27_000)
		});
		const gaps = times.slice(1).map((time, index) => time - (times.at(index) ?? 0));
		expect(new Set(gaps).size).toBeGreaterThan(1);
	});

	it('finds 5 to 7 albums at a time at real tempo, every 2.7 s or so', () => {
		const sizes = Array.from({ length: 150 }, (_, index) => burstSize(index, 412_000, 2700));
		expect(Math.min(...sizes)).toBeGreaterThanOrEqual(5);
		expect(Math.max(...sizes)).toBeLessThanOrEqual(7);
		const real = burstTimes(412_000, 2700);
		expect(412_000 / real.length).toBeGreaterThan(2400);
		expect(412_000 / real.length).toBeLessThan(3000);
	});

	it('finds about the same library whatever the tempo, so a short scan is a dense one', () => {
		for (const [scanMs, gapMs] of [
			[412_000, 2700],
			[30_000, 2700],
			[1_700_000, 11_000]
		] as const) {
			const found = burstTimes(scanMs, gapMs).reduce(
				(sum, _, index) => sum + burstSize(index, scanMs, gapMs),
				0
			);
			expect(found).toBeGreaterThan(750);
			expect(found).toBeLessThan(1100);
		}
		expect(burstSize(0, 30_000, 2700)).toBeGreaterThan(60);
	});

	it('never lands a burst after the end', () => {
		expect(Math.max(...burstTimes(30_000, 7000))).toBeLessThan(30_000);
		expect(burstTimes(1000, 27_000).length).toBeGreaterThan(0);
	});

	it('lists the newest first, and pages by name on cursors with nothing lost', async () => {
		at(400_000);
		const { albums } = await scan.facts();
		const newest = await scan.albumsPage('added', undefined, 3);
		expect(newest.albums.at(0)?.id).toBe(`scan-${String(albums - 1)}`);
		const seen = new Set<string>();
		let cursor: string | undefined;
		do {
			const page = await scan.albumsPage('name', cursor, 200);
			for (const album of page.albums) {
				seen.add(album.id);
			}
			cursor = page.nextCursor;
		} while (cursor !== undefined);
		expect(seen.size).toBe(albums);
	});

	it('serves every album it has found, and none it has not', async () => {
		at(times.at(0) ?? 0);
		const { albums } = await scan.albumsPage('added', undefined, 1);
		const [entry] = albums;
		expect(await scan.album(entry?.id ?? '')).toMatchObject(entry ?? {});
		await expect(scan.album('scan-99999')).rejects.toBeInstanceOf(AlbumNotFoundError);
		await expect(scan.album('glasshouse-66')).rejects.toBeInstanceOf(AlbumNotFoundError);
	});
});

describe('the library index', () => {
	it('has what the page must render', async () => {
		const { artists } = await createFixtureSource().libraryIndex();
		const albums = artists.flatMap((artist) => artist.albums);
		expect(artists.find(({ name }) => name === 'Various Artists')?.id).toBeUndefined();
		expect(albums.filter(({ compilation }) => compilation).length).toBeGreaterThan(0);
		expect(
			albums.some(({ tracks }) => new Set(tracks.map(({ discNo }) => discNo ?? 1)).size > 1)
		).toBe(true);
		expect(albums.some(({ artwork }) => artwork === undefined)).toBe(true);
		expect(artists.some(({ name }) => name.length > 60)).toBe(true);
		expect(artists.some(({ sortName }) => letterOf(sortName) === '#')).toBe(true);
		expect(new Set(artists.map(({ sortName }) => letterOf(sortName))).size).toBeGreaterThan(5);
	});

	it('files # first and every letter in order, as the server does', async () => {
		const { artists } = await createFixtureSource().libraryIndex();
		const letters = artists.map(({ sortName }) => letterOf(sortName));
		expect(letters).toEqual(letters.toSorted());
		expect(letters.at(0)).toBe('#');
	});

	it('plays what it lists', async () => {
		const fixture = createFixtureSource();
		const { artists } = await fixture.libraryIndex();
		for (const { id, tracks } of artists.flatMap((artist) => artist.albums)) {
			expect((await fixture.album(id)).tracks.map((track) => track.id)).toEqual(
				tracks.map((track) => track.id)
			);
		}
	});
});
