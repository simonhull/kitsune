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
	/** Drops whatever is waiting: the next person mustn't save the last one's columns. */
	reset(): void;
}

/** Keeps preferences on the server, saving after a quiet moment. A failed save is let go. */
export function createPrefs(server: PrefsServer, quietMs = prefsQuietMs): PrefsStore {
	const waiting = new Map<PrefKey, Prefs[PrefKey]>();
	let timer: ReturnType<typeof setTimeout> | undefined;

	async function send(): Promise<void> {
		clearTimeout(timer);
		timer = undefined;
		const batch = [...waiting];
		waiting.clear();
		// A save that fails is let go: the page already shows the change, and the next one says it all.
		await Promise.all(
			batch.map(([key, value]) => server.setPref(key, value).catch(() => undefined))
		);
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
		reset() {
			clearTimeout(timer);
			timer = undefined;
			waiting.clear();
		}
	};
}
