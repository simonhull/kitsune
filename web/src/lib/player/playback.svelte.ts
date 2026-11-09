import { library, type Library } from '#lib/library/library.svelte.ts';

/** The gain a song plays at, in dB, while it has no ReplayGain, when none is chosen. */
export const defaultUntaggedGain = -8;

/** How far the untagged gain goes, in dB: it only ever turns a song down. */
export const untaggedGainRange = { min: -20, max: 0 } as const;

const key = 'playback.untagged_gain';

/**
 * The person's playback settings, kept on the server. Playing reads them as they stand and never
 * waits: until they're read, the defaults play.
 */
export class PlaybackSettings {
	/** The gain, in dB, of a song with no ReplayGain, tagged or measured. */
	untaggedGain = $state(defaultUntaggedGain);
	readonly #library: Library;
	#loaded: number | null = null;

	constructor(host: Library) {
		this.#library = host;
		host.watchPref(key, (value) => {
			this.untaggedGain = value;
		});
	}

	/** Reads them once per person signed in. One never set, or not readable, keeps its default. */
	async load(): Promise<void> {
		const user = this.#library.user?.id ?? null;
		if (user === null || this.#loaded === user) {
			return;
		}
		this.#loaded = user;
		this.untaggedGain = (await this.#library.prefs.get(key)) ?? defaultUntaggedGain;
	}

	/** Changes the untagged gain, at once here and quietly on the server. */
	setUntaggedGain(decibels: number): void {
		this.untaggedGain = decibels;
		this.#library.prefs.put(key, decibels);
	}

	/** Forgets the last person's. */
	reset(): void {
		this.#loaded = null;
		this.untaggedGain = defaultUntaggedGain;
	}
}

/** The one store of the person's playback settings. */
export const playbackSettings = new PlaybackSettings(library);
