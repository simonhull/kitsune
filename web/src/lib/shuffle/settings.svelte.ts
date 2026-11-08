import { library, type Library } from '#lib/library/library.svelte.ts';
import { defaultShuffleSettings, type ShuffleSettings } from './words.ts';

/** The preference each setting is kept under. */
const keys = {
	ratings: 'shuffle.ratings',
	flow: 'shuffle.flow',
	memory: 'shuffle.memory'
} as const;

/**
 * The person's shuffle settings, kept on the server. The server reads them for every page; here
 * they weigh the first pick, which never waits, and word the settings where they show.
 */
export class ShuffleSettingsStore {
	current = $state<ShuffleSettings>({ ...defaultShuffleSettings });
	readonly #library: Library;
	#loaded: number | null = null;

	constructor(host: Library) {
		this.#library = host;
		host.watchPref(keys.ratings, (value) => {
			this.current.ratings = value;
		});
		host.watchPref(keys.flow, (value) => {
			this.current.flow = value;
		});
		host.watchPref(keys.memory, (value) => {
			this.current.memory = value;
		});
	}

	/** Reads them once per person signed in. One never set, or not readable, keeps its default. */
	async load(): Promise<void> {
		const user = this.#library.user?.id ?? null;
		if (user === null || this.#loaded === user) {
			return;
		}
		this.#loaded = user;
		const [ratings, flow, memory] = await Promise.all([
			this.#library.prefs.get(keys.ratings),
			this.#library.prefs.get(keys.flow),
			this.#library.prefs.get(keys.memory)
		]);
		this.current = {
			ratings: ratings ?? defaultShuffleSettings.ratings,
			flow: flow ?? defaultShuffleSettings.flow,
			memory: memory ?? defaultShuffleSettings.memory
		};
	}

	/** Changes one, at once here and quietly on the server. */
	set<K extends keyof ShuffleSettings>(key: K, value: ShuffleSettings[K]): void {
		this.current[key] = value;
		this.#library.prefs.put(keys[key], value as never);
	}

	/** Forgets the last person's. */
	reset(): void {
		this.#loaded = null;
		this.current = { ...defaultShuffleSettings };
	}
}

/** The one store of the person's shuffle settings. */
export const shuffleSettings = new ShuffleSettingsStore(library);
