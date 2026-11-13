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

function index(...stars: (number | undefined)[]): LibraryIndex {
	return {
		artists: [
			{
				id: 'ar',
				name: 'Ar',
				sortName: 'Ar',
				albums: [
					{
						id: 'al',
						title: 'Al',
						artist: 'Ar',
						compilation: false,
						tracks: stars.map((rated, at) => ({
							id: `t${String(at + 1)}`,
							title: 'One',
							durationMs: 1000,
							artists: [],
							...(rated === undefined ? {} : { stars: rated })
						}))
					}
				]
			}
		]
	};
}

describe('marks', () => {
	it('reads what the index and an album say', () => {
		const marks = new Marks(slowServer().server);
		marks.seedIndex(index(4));
		expect(marks.stars('t1')).toBe(4);
		expect(marks.stars('unknown')).toBeNull();

		const album = {
			id: 'al',
			tracks: [{ id: 't1' }]
		} as unknown as Album;
		marks.seedAlbum(album);
		expect(marks.stars('t1')).toBeNull();
	});

	it('changes at once, and sends presses in order, one at a time', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.rate('t1', 4);
		expect(marks.stars('t1')).toBe(4);
		marks.rate('t1', null);
		expect(marks.stars('t1')).toBeNull();
		expect(asked).toEqual(['stars t1 4']);

		await answer();
		expect(asked).toEqual(['stars t1 4', 'stars t1 null']);
		await answer();
		expect(marks.stars('t1')).toBeNull();
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
		marks.rate('t1', 3);
		marks.rate('t1', null);
		marks.rate('t1', 3);
		await answer();
		expect(asked).toEqual(['stars t1 3']);
		expect(marks.stars('t1')).toBe(3);
	});

	it('puts a rating back when it fails, and says so', async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(3));
		marks.rate('t1', 5);
		expect(marks.stars('t1')).toBe(5);
		await answer(new ServerError(500));
		expect(marks.stars('t1')).toBe(3);
		expect(marks.failure).toEqual({ id: 't1' });
	});

	it('keeps a newer press when an older one fails late', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.rate('t1', 4); // Sent.
		marks.rate('t1', null); // Waiting.
		marks.rate('t1', 4.5); // Waiting in its place.
		await answer(new ServerError(503));
		expect(marks.stars('t1')).toBe(4.5);
		expect(marks.failure).toBeNull();
		expect(asked).toEqual(['stars t1 4', 'stars t1 4.5']);
		await answer();
		expect(marks.stars('t1')).toBe(4.5);
	});

	it("doesn't let a refetch clobber a press in flight", async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(undefined));
		marks.rate('t1', 2);
		// An index read before the press landed comes back now.
		marks.seedIndex(index(undefined));
		expect(marks.stars('t1')).toBe(2);
		await answer();
		expect(marks.stars('t1')).toBe(2);
		// Once it has landed, a fresh read speaks for the server again.
		marks.seedIndex(index(undefined));
		expect(marks.stars('t1')).toBeNull();
	});

	it('rates in halves, clears, and hears what another device or the change from hearts did', async () => {
		const { server, asked, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(4));
		marks.rate('t1', 3.5);
		expect(marks.stars('t1')).toBe(3.5);
		await answer();
		marks.rate('t1', null);
		expect(marks.stars('t1')).toBeNull();
		await answer();
		marks.heard({ kind: 'tracks', id: 't1', stars: 4.5 });
		expect(marks.stars('t1')).toBe(4.5);
		marks.heard({ kind: 'tracks', id: 't1', stars: 0 });
		expect(marks.stars('t1')).toBeNull();
		expect(asked).toEqual(['stars t1 3.5', 'stars t1 null']);
	});

	it('counts the songs at or over a line, as shown', () => {
		const marks = new Marks(slowServer().server);
		marks.seedIndex(index(5, 4, 3.5, undefined, 4.5));
		expect(marks.ratedAtLeast(4)).toBe(3);
		marks.rate('t3', 4);
		expect(marks.ratedAtLeast(4)).toBe(4);
		expect(marks.ratedAtLeast(5)).toBe(1);
	});

	it('puts a rating back quietly when the session has gone', async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.rate('t1', 4);
		await answer(new UnauthorizedError());
		expect(marks.stars('t1')).toBeNull();
		expect(marks.failure).toBeNull();
	});

	it('forgets everything on a reset, and a late answer changes nothing', async () => {
		const { server, answer } = slowServer();
		const marks = new Marks(server);
		marks.seedIndex(index(5));
		marks.rate('t1', 2);
		marks.reset();
		expect(marks.stars('t1')).toBeNull();
		await answer(new ServerError(500));
		expect(marks.stars('t1')).toBeNull();
		expect(marks.failure).toBeNull();
	});
});
