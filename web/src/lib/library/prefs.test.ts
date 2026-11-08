import { afterEach, beforeEach, describe, expect, it, vi } from 'vitest';
import { createPrefs, prefsQuietMs, type PrefsServer } from './prefs.ts';
import { ServerError, type ColumnsPref } from './source.ts';

const columns = (order: ColumnsPref['order']): ColumnsPref => ({
	order,
	artistWhenDifferent: true
});

function server(saved: ColumnsPref | null = null): PrefsServer & { sent: ColumnsPref[] } {
	const sent: ColumnsPref[] = [];
	return {
		sent,
		pref: () => Promise.resolve(saved) as never,
		setPref: (_key, value) => {
			sent.push(value);
			return Promise.resolve();
		}
	};
}

describe('prefs', () => {
	beforeEach(() => {
		vi.useFakeTimers();
	});
	afterEach(() => {
		vi.useRealTimers();
	});

	it('saves once changes stop, and only the last of them', async () => {
		const remote = server();
		const prefs = createPrefs(remote);
		prefs.put('library.columns', columns(['genre']));
		await vi.advanceTimersByTimeAsync(prefsQuietMs - 1);
		prefs.put('library.columns', columns(['genre', 'year']));
		await vi.advanceTimersByTimeAsync(prefsQuietMs - 1);
		expect(remote.sent).toEqual([]);
		await vi.advanceTimersByTimeAsync(1);
		expect(remote.sent).toEqual([columns(['genre', 'year'])]);
	});

	it('sends what waits on a flush, and nothing after', async () => {
		const remote = server();
		const prefs = createPrefs(remote);
		prefs.put('library.columns', columns(['plays']));
		await prefs.flush();
		expect(remote.sent).toEqual([columns(['plays'])]);
		await vi.advanceTimersByTimeAsync(prefsQuietMs * 2);
		expect(remote.sent).toHaveLength(1);
	});

	it('drops what waits on a reset', async () => {
		const remote = server();
		const prefs = createPrefs(remote);
		prefs.put('library.columns', columns(['plays']));
		prefs.reset();
		await vi.advanceTimersByTimeAsync(prefsQuietMs * 2);
		expect(remote.sent).toEqual([]);
	});

	it('reads what was saved, and null when it fails', async () => {
		expect(await createPrefs(server(columns(['year']))).get('library.columns')).toEqual(
			columns(['year'])
		);
		const broken: PrefsServer = {
			pref: () => Promise.reject(new ServerError(500)),
			setPref: () => Promise.reject(new ServerError(500))
		};
		const prefs = createPrefs(broken);
		expect(await prefs.get('library.columns')).toBeNull();
		prefs.put('library.columns', columns([]));
		await expect(prefs.flush()).resolves.toBeUndefined();
	});
});
