import { describe, expect, it } from 'vitest';
import { Marks, type MarkServer } from './marks.svelte';
import { ServerError, UnauthorizedError, type Album, type LibraryIndex } from './source';

/** A call the server has yet to answer, which the test settles by hand. */
interface Pending {
	what: string;
	resolve: () => void;
	reject: (error: unknown) => void;
}

/** A server that answers nothing until told, and remembers what it was asked, in order. */
function slowServer() {
	const asked: string[] = [];
	const pending: Pending[] = [];
	function call(what: string): Promise<void> {
		asked.push(what);
		return new Promise((resolve, reject) => {
			pending.push({ what, resolve, reject });
		});
	}
	const server: MarkServer = {
		setHeart: (kind, id, on) => call(`${on ? 'heart' : 'unheart'} ${kind}/${id}`),
		setStars: (id, stars) => call(`stars ${id} ${String(stars)}`)
	};
	/** Answers the oldest call still waiting, and lets the marks act on it. */
	async function answer(error?: unknown): Promise<string | undefined> {
		const next = pending.shift();
		if (error === undefined) {
			next?.resolve();
		} else {
			next?.reject(error);
		}
		// Every await between the answer and the next send has had its turn.
		await new Promise((resolve) => setTimeout(resolve, 0));
		return next?.what;
	}
	return { server, asked, pending, answer };
}

function index(hearted: boolean, stars?: number): LibraryIndex {
	return {
		artists: [
			{
				id: 'ar',
				name: 'Ar',
				sortName: 'Ar',
				hearted,
				albums: [
					{
						id: 'al',
						title: 'Al',
						artist: 'Ar',
						compilation: false,
						hearted,
						tracks: [
							{
								id: 't1',
								title: 'One',
								durationMs: 1000,
								artists: [],
								hearted,
								...(stars === undefined ? {} : { stars })
							}
						]
					}
				]
			}
		]
	};
}

describe('marks', () => {
	it('reads what the index and an album say', () => {
		const marks = new Marks(slowServer().server);
		marks.seedIndex(index(true, 4));
		expect(marks.hearted('artists', 'ar')).toBe(true);
		expect(marks.hearted('albums', 'al')).toBe(true);
		expect(marks.hearted('tracks', 't1')).toBe(true);
		expect(marks.stars('t1')).toBe(4);
		expect(marks.hearted('tracks', 'unknown')).toBe(false);
		expect(marks.stars('unknown')).toBeNull();

		const album = {
			id: 'al',
			hearted: false,
			tracks: [{ id: 't1', hearted: false }]
		} as unknown as Album;
		marks.seedAlbum(album);
		expect(marks.hearted('albums', 'al')).toBe(false);
		expect(marks.hearted('tracks', 't1')).toBe(false);
		expect(marks.stars('t1')).toBeNull();
	});

	it('changes at once, and sends a double press in order, one at a time', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.toggle('tracks', 't1');
		expect(marks.hearted('tracks', 't1')).toBe(true);
		marks.toggle('tracks', 't1');
		expect(marks.hearted('tracks', 't1')).toBe(false);
		expect(asked).toEqual(['heart tracks/t1']);

		await answer();
		expect(asked).toEqual(['heart tracks/t1', 'unheart tracks/t1']);
		await answer();
		expect(marks.hearted('tracks', 't1')).toBe(false);
		expect(marks.failure).toBeNull();
	});

	it('sends only the newest wish once the one in flight lands', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.rate('t1', 2);
		marks.rate('t1', 3);
		marks.rate('t1', 5);
		await answer();
		await answer();
		expect(asked).toEqual(['stars t1 2', 'stars t1 5']);
		expect(marks.stars('t1')).toBe(5);
	});

	it('sends nothing more for a press that went back to what the server has', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.toggle('albums', 'al');
		marks.toggle('albums', 'al');
		marks.toggle('albums', 'al');
		await answer();
		expect(asked).toEqual(['heart albums/al']);
		expect(marks.hearted('albums', 'al')).toBe(true);
	});

	it('puts a mark back when it fails, and says so', async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(false, 3));
		marks.rate('t1', 5);
		expect(marks.stars('t1')).toBe(5);
		await answer(new ServerError(500));
		expect(marks.stars('t1')).toBe(3);
		expect(marks.failure).toEqual({ kind: 'stars', id: 't1' });
	});

	it('keeps a newer press when an older one fails late', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.toggle('tracks', 't1'); // On, sent.
		marks.toggle('tracks', 't1'); // Off, waiting.
		marks.toggle('tracks', 't1'); // On again, waiting in its place.
		await answer(new ServerError(503));
		expect(marks.hearted('tracks', 't1')).toBe(true);
		expect(marks.failure).toBeNull();
		expect(asked).toEqual(['heart tracks/t1', 'heart tracks/t1']);
		await answer();
		expect(marks.hearted('tracks', 't1')).toBe(true);
	});

	it("doesn't let a refetch clobber a press in flight", async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(false));
		marks.toggle('tracks', 't1');
		// An index read before the press landed comes back now.
		marks.seedIndex(index(false));
		expect(marks.hearted('tracks', 't1')).toBe(true);
		await answer();
		expect(marks.hearted('tracks', 't1')).toBe(true);
		// Once it has landed, a fresh read speaks for the server again.
		marks.seedIndex(index(false));
		expect(marks.hearted('tracks', 't1')).toBe(false);
	});

	it('rates in halves, and clears', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(false, 4));
		marks.rate('t1', 3.5);
		expect(marks.stars('t1')).toBe(3.5);
		await answer();
		marks.rate('t1', null);
		expect(marks.stars('t1')).toBeNull();
		await answer();
		marks.heard({ kind: 'tracks', id: 't1', stars: 4.5 });
		expect(marks.stars('t1')).toBe(4.5);
		expect(asked).toEqual(['stars t1 3.5', 'stars t1 null']);
	});

	it('puts a mark back quietly when the session has gone', async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.toggle('artists', 'ar');
		await answer(new UnauthorizedError());
		expect(marks.hearted('artists', 'ar')).toBe(false);
		expect(marks.failure).toBeNull();
	});

	it('forgets everything on a reset, and a late answer changes nothing', async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(true, 5));
		marks.toggle('albums', 'al');
		marks.reset();
		expect(marks.hearted('artists', 'ar')).toBe(false);
		expect(marks.stars('t1')).toBeNull();
		await answer(new ServerError(500));
		expect(marks.hearted('albums', 'al')).toBe(false);
		expect(marks.failure).toBeNull();
	});
});
