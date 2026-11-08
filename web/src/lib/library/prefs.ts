import type { LibrarySource, PrefKey, Prefs } from './source.ts';

/** How long a change waits for the next: dragging a column through a list saves once, at the end. */
export const prefsQuietMs = 600;

export type PrefsServer = Pick<LibrarySource, 'pref' | 'setPref'>;

/** What a page needs of the person's preferences. */
export interface PrefsStore {
	/** The saved value, or null when there's none, or when it can't be read: a page uses its defaults. */
	get<K extends PrefKey>(key: K): Promise<Prefs[K] | null>;
	/** Saves quietly, once changes stop coming. Only the latest value of a key is sent. */
	put<K extends PrefKey>(key: K, value: Prefs[K]): void;
	/** Sends whatever is waiting, now. */
	flush(): Promise<void>;
	/** Whether a change to `key` waits to be saved, or is on its way: it's newer than the server's. */
	pending(key: PrefKey): boolean;
	/** Drops whatever is waiting: the next person mustn't save the last one's columns. */
	reset(): void;
}

/** Keeps preferences on the server, saving after a quiet moment. A failed save is let go. */
export function createPrefs(server: PrefsServer, quietMs = prefsQuietMs): PrefsStore {
	const waiting = new Map<PrefKey, Prefs[PrefKey]>();
	/** Saves on their way, by key. */
	const sending = new Map<PrefKey, number>();
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function save(key: PrefKey, value: Prefs[PrefKey]): Promise<void> {
		sending.set(key, (sending.get(key) ?? 0) + 1);
		try {
			await server.setPref(key, value);
		} catch {
			// A save that fails is let go: the page already shows the change, and the next one says it all.
		} finally {
			const left = (sending.get(key) ?? 1) - 1;
			if (left === 0) {
				sending.delete(key);
			} else {
				sending.set(key, left);
			}
		}
	}

	async function send(): Promise<void> {
		clearTimeout(timer);
		timer = undefined;
		const batch = [...waiting];
		waiting.clear();
		await Promise.all(batch.map(([key, value]) => save(key, value)));
	}

	return {
		async get(key) {
			try {
				return await server.pref(key);
			} catch {
				return null;
			}
		},
		put(key, value) {
			waiting.set(key, value);
			clearTimeout(timer);
			timer = setTimeout(() => void send(), quietMs);
		},
		flush: send,
		pending: (key) => waiting.has(key) || sending.has(key),
		reset() {
			clearTimeout(timer);
			timer = undefined;
			waiting.clear();
		}
	};
}
