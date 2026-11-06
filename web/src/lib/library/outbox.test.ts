import { describe, expect, it, vi } from 'vitest';
import { createFixtureSource } from './fixtures.ts';
import { Library } from './library.svelte.ts';
import { Outbox } from './outbox.ts';
import {
	ServerError,
	UnauthorizedError,
	type LibrarySource,
	type PlayReport,
	type User
} from './source.ts';

const simon: User = { id: 1, name: 'simon', role: 'admin', createdAt: '2026-01-01T00:00:00Z' };

/** The part of `localStorage` that tabs share. */
class MemoryStorage implements Storage {
	readonly #items = new Map<string, string>();
	/** Past this many items, a write fails as a full disk's does. */
	room = Infinity;

	get length(): number {
		return this.#items.size;
	}

	clear(): void {
		this.#items.clear();
	}

	getItem(key: string): string | null {
		return this.#items.get(key) ?? null;
	}

	key(index: number): string | null {
		return [...this.#items.keys()].at(index) ?? null;
	}

	removeItem(key: string): void {
		this.#items.delete(key);
	}

	setItem(key: string, value: string): void {
		if (this.#items.size >= this.room) {
			throw new DOMException('full', 'QuotaExceededError');
		}
		this.#items.set(key, value);
	}
}

/** A server that takes every play, unless a test says otherwise. */
function server(userId = 1) {
	const received: PlayReport[] = [];
	const reportPlay = vi.fn<LibrarySource['reportPlay']>((play) => {
		received.push(play);
		return Promise.resolve();
	});
	const sessionUser = vi.fn<LibrarySource['sessionUser']>(() =>
		Promise.resolve({ ...simon, id: userId })
	);
	return { received, reportPlay, sessionUser };
}

function outbox(fake = server(), store = new MemoryStorage()) {
	return { fake, store, box: new Outbox(() => store, fake) };
}

describe('Outbox', () => {
	it('sends a play, oldest first, and forgets it once the server has it', async () => {
		const { box, fake, store } = outbox();
		box.add(1, 'late', 2000);
		box.add(1, 'early', 1000);
		await box.flush(1);
		expect(fake.received.map(({ trackId }) => trackId)).toEqual(['early', 'late']);
		expect(fake.received.at(0)).toMatchObject({
			playedAt: '1970-01-01T00:00:01.000Z',
			source: 'web'
		});
		expect(store.length).toBe(0);
	});

	it('gives every play a client ID of its own, the same on every retry', async () => {
		const { box, fake } = outbox();
		fake.reportPlay.mockRejectedValueOnce(new TypeError('offline'));
		box.add(1, 't', 1000);
		await box.flush(1);
		await box.flush(1);
		box.add(1, 't', 1000);
		await box.flush(1);
		const [first, retry] = fake.reportPlay.mock.calls.map(([play]) => play.clientId);
		expect(first).toMatch(/^[0-9a-f]{32}$/);
		expect(retry).toBe(first);
		expect(new Set(fake.received.map(({ clientId }) => clientId)).size).toBe(2);
	});

	it.each([
		['a network failure', new TypeError('offline')],
		['a 5xx', new ServerError(503)],
		['a 401', new UnauthorizedError()]
	])('keeps a play through %s, and stops there', async (_, failure) => {
		const { box, fake, store } = outbox();
		fake.reportPlay.mockRejectedValue(failure);
		box.add(1, 'a', 1000);
		box.add(1, 'b', 2000);
		await box.flush(1);
		expect(fake.reportPlay).toHaveBeenCalledOnce();
		expect(store.length).toBe(2);
	});

	it.each([400, 404])('drops a play the server answers %d to, and goes on', async (status) => {
		const { box, fake, store } = outbox();
		fake.reportPlay.mockRejectedValueOnce(new ServerError(status));
		box.add(1, 'a', 1000);
		box.add(1, 'b', 2000);
		await box.flush(1);
		expect(fake.received.map(({ trackId }) => trackId)).toEqual(['b']);
		expect(store.length).toBe(0);
	});

	it('keeps 1,000 plays a user, dropping the oldest', async () => {
		const { box, fake, store } = outbox();
		for (let at = 0; at < 1005; at += 1) {
			box.add(1, String(at), at);
		}
		box.add(2, 'other', 0);
		expect(store.length).toBe(1001);
		await box.flush(1);
		expect(fake.received).toHaveLength(1000);
		expect(fake.received.at(0)?.trackId).toBe('5');
	});

	it('survives full storage, and storage that is not there', async () => {
		const { box, fake, store } = outbox();
		store.room = 0;
		expect(() => {
			box.add(1, 't', 1000);
		}).not.toThrow();
		const none = new Outbox(() => {
			throw new DOMException('no', 'SecurityError');
		}, fake);
		expect(() => {
			none.add(1, 't', 1000);
		}).not.toThrow();
		await expect(none.flush(1)).resolves.toBeUndefined();
	});

	it('sends only the plays of the user asked for', async () => {
		const { box, fake, store } = outbox();
		box.add(1, 'mine', 1000);
		box.add(12, 'theirs', 1000);
		await box.flush(1);
		expect(fake.received.map(({ trackId }) => trackId)).toEqual(['mine']);
		expect(store.length).toBe(1);
	});

	it('holds on to plays while the cookie is someone else’s', async () => {
		const { box, fake, store } = outbox(server(2));
		box.add(1, 't', 1000);
		await box.flush(1);
		expect(fake.reportPlay).not.toHaveBeenCalled();
		expect(store.length).toBe(1);
	});

	it('stops when the session changes hands part way', async () => {
		const { box, fake, store } = outbox();
		box.add(1, 'a', 1000);
		box.add(1, 'b', 2000);
		fake.sessionUser
			.mockResolvedValueOnce(simon)
			.mockResolvedValueOnce({ ...simon, id: 2, name: 'ada' });
		await box.flush(1);
		expect(fake.received.map(({ trackId }) => trackId)).toEqual(['a']);
		expect(store.length).toBe(1);
	});

	it('holds on to plays when it can’t tell whose session this is', async () => {
		const { box, fake, store } = outbox();
		fake.sessionUser.mockRejectedValue(new ServerError(404));
		box.add(1, 't', 1000);
		await box.flush(1);
		expect(fake.reportPlay).not.toHaveBeenCalled();
		expect(store.length).toBe(1);
	});

	it('picks up a play added while it was sending, without sending anything twice', async () => {
		const { box, fake } = outbox();
		box.add(1, 'a', 1000);
		fake.reportPlay.mockImplementationOnce((play) => {
			fake.received.push(play);
			box.add(1, 'b', 2000);
			void box.flush(1);
			return Promise.resolve();
		});
		await box.flush(1);
		expect(fake.received.map(({ trackId }) => trackId)).toEqual(['a', 'b']);
	});

	it('loses nothing to four tabs, which may each send a play the others sent', async () => {
		const fake = server();
		const store = new MemoryStorage();
		const tabs = Array.from({ length: 4 }, () => new Outbox(() => store, fake));
		tabs.forEach((tab, index) => {
			for (let at = 0; at < 50; at += 1) {
				tab.add(1, `${String(index)}-${String(at)}`, at);
			}
		});
		await Promise.all(tabs.map((tab) => tab.flush(1)));
		expect(new Set(fake.received.map(({ clientId }) => clientId)).size).toBe(200);
		expect(store.length).toBe(0);
	});
});

describe('Library plays', () => {
	function library(overrides: Partial<LibrarySource> = {}, simulated = false) {
		const store = new MemoryStorage();
		const fake = server();
		const source = { ...createFixtureSource(), me: () => Promise.resolve(simon) };
		const subject = new Library(
			{ ...source, ...fake, ...overrides },
			{ outbox: () => store, simulated }
		);
		return { subject, store, fake };
	}

	it('sends a play as soon as it is reported', async () => {
		const { subject, fake, store } = library();
		await subject.checkSession();
		subject.reportPlay('t', 1000);
		await vi.waitFor(() => {
			expect(fake.received).toHaveLength(1);
		});
		await vi.waitFor(() => {
			expect(store.length).toBe(0);
		});
	});

	it('sends what was waiting when the shell loads', async () => {
		const { subject, fake, store } = library();
		new Outbox(() => store, fake).add(1, 'waiting', 1000);
		await subject.loadShell();
		await vi.waitFor(() => {
			expect(fake.received.map(({ trackId }) => trackId)).toEqual(['waiting']);
		});
	});

	it('sends what was waiting when asked, as coming back online does', async () => {
		const { subject, fake, store } = library();
		await subject.checkSession();
		new Outbox(() => store, fake).add(1, 'waiting', 1000);
		subject.flushPlays();
		await vi.waitFor(() => {
			expect(fake.received).toHaveLength(1);
		});
	});

	it('keeps a fixture’s plays to itself', async () => {
		const { subject, fake, store } = library({}, true);
		await subject.loadShell();
		new Outbox(() => store, fake).add(1, 'real', 1000);
		subject.reportPlay('fake', 1000);
		subject.flushPlays();
		await new Promise((done) => setTimeout(done));
		expect(fake.reportPlay).not.toHaveBeenCalled();
		expect(store.length).toBe(1);
	});

	it('has nobody to report for with nobody signed in', () => {
		const { subject, store } = library();
		subject.reportPlay('t', 1000);
		expect(store.length).toBe(0);
	});

	it('leaves the music alone when a report meets a lost session', async () => {
		const reportPlay = vi.fn<LibrarySource['reportPlay']>(() =>
			Promise.reject(new UnauthorizedError())
		);
		const { subject, store } = library({ reportPlay });
		const onExpired = vi.fn(() => Promise.resolve());
		subject.onExpired = onExpired;
		await subject.checkSession();
		subject.reportPlay('t', 1000);
		await vi.waitFor(() => {
			expect(reportPlay).toHaveBeenCalledOnce();
		});
		await new Promise((done) => setTimeout(done));
		expect(onExpired).not.toHaveBeenCalled();
		expect(store.length).toBe(1);
		expect(subject.user).toEqual(simon);
	});
});
