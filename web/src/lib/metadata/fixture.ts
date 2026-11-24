/**
 * Health's Metadata card on the boards' library, worked out in the page so e2e needs no server. How
 * the sources stand follows the fixture settings, as the server's report follows its own: turning
 * lookups on starts a pass, a key added takes fanart.tv out of needing one, and a key changed lifts
 * a refusal. A pass goes artist by artist on a clock. The counts are the boards', made up.
 */

import type {
	ArtistLink,
	MetadataSettings,
	SecretSetting,
	Setting,
	SettingsChange
} from '#lib/library/source.ts';
import type {
	MetadataArtists,
	MetadataSource,
	MetadataStatus,
	MetadataStatusSource,
	SourceState
} from './types.ts';

/** Where the fixtures start. */
export type MetadataFixture =
	/** Lookups off, as on a new server: the card offers them. */
	| 'off'
	/** A pass going, fanart.tv waiting for its project key. */
	| 'running'
	/** fanart.tv refused its key; nothing else to do. */
	| 'refused'
	/** A pass going while Wikipedia rests after a 503. */
	| 'resting'
	/** Every key set, every artist asked about: up to date. */
	| 'done';

/** The `?metadata=` a fixtures page opens with. */
export function metadataFixture(value: string | null): MetadataFixture {
	return value === 'running' || value === 'refused' || value === 'resting' || value === 'done'
		? value
		: 'off';
}

const setting = <T>(value: T, source: Setting<T>['source'] = 'default'): Setting<T> => ({
	value,
	source
});

const secret = (set: boolean): SecretSetting =>
	set
		? { set: true, source: 'env', from: 'KITSUNE_FANARTTV_API_KEY' }
		: { set: false, source: 'default' };

/** The settings a fixture starts with. */
export function metadataSettings(start: MetadataFixture): MetadataSettings {
	const keyed = start !== 'off' && start !== 'running';
	return {
		'metadata.remote': start === 'off' ? setting(false) : setting(true, 'web'),
		'metadata.fanarttv': setting(true),
		'metadata.wikipedia': setting(true),
		'metadata.language': setting('en'),
		'fanarttv.api_key': secret(keyed),
		'fanarttv.client_key':
			start === 'done'
				? { set: true, source: 'env', from: 'KITSUNE_FANARTTV_CLIENT_KEY' }
				: { set: false, source: 'default' }
	};
}

/** The boards' album artists: those with an MBID the pass asks about, and those Matching must fix. */
const albumArtists = 418;
const withoutMbid = 67;
const withMbid = albumArtists - withoutMbid;

const minuteMs = 60_000;
const dayMs = 86_400_000;

export interface MetadataFixtureOptions {
	start: MetadataFixture;
	/** The settings as they are now. */
	settings: () => MetadataSettings;
	/** Artists a pass names as the one in hand, in turn. */
	artists: () => readonly ArtistLink[];
	/** How long each artist of a pass takes, in milliseconds. */
	artistMs?: number;
	now?: () => number;
	respond: <T>(answer: () => T) => Promise<T>;
}

/** A pass on the clock: it began at `at`, `from` artists in, and goes to `total`. */
interface Pass {
	at: number;
	from: number;
	total: number;
}

/** The card's route, and what tells it a setting changed. */
export interface MetadataFixtureSource extends MetadataStatusSource {
	changed(change: SettingsChange): void;
}

export function createMetadataFixture(options: MetadataFixtureOptions): MetadataFixtureSource {
	const now = options.now ?? Date.now;
	const artistMs = options.artistMs ?? 900;
	const started = now();
	let pass: Pass | null =
		options.start === 'running'
			? { at: started, from: 66, total: 140 }
			: options.start === 'resting'
				? { at: started, from: 31, total: 140 }
				: null;
	let refused = options.start === 'refused';
	/** Asked about every artist: the counts are the finished ones. */
	let finished = options.start === 'refused' || options.start === 'done';

	function done(at: number): number {
		return pass === null
			? 0
			: Math.min(pass.total, pass.from + Math.floor((at - pass.at) / artistMs));
	}

	function settle(at: number): void {
		if (pass !== null && done(at) >= pass.total) {
			pass = null;
			finished = true;
		}
	}

	function artists(at: number): MetadataArtists {
		if (finished) {
			return {
				albumArtists,
				withMbid,
				lookedUp: withMbid,
				withArt: 312,
				foundNothing: 39,
				withoutMbid
			};
		}
		const asked = done(at);
		const before = pass === null ? 0 : withMbid - pass.total;
		return {
			albumArtists,
			withMbid,
			lookedUp: before + asked,
			withArt: Math.round((before + asked) * 0.8),
			foundNothing: Math.round((before + asked) * 0.1),
			withoutMbid
		};
	}

	function sources(s: MetadataSettings): MetadataSource[] {
		const remote = s['metadata.remote'].value;
		const wiki = s['metadata.wikipedia'].value;
		const fanart = s['metadata.fanarttv'].value;
		const keyed = s['fanarttv.api_key'].set;
		const resting = options.start === 'resting' && pass !== null;
		const fanartState: SourceState =
			!remote || !fanart ? 'off' : !keyed ? 'needsKey' : refused ? 'refused' : 'on';
		return [
			{
				name: 'musicbrainz',
				enabled: wiki,
				state: remote && wiki ? 'on' : 'off',
				key: 'none',
				clientKey: 'none'
			},
			{
				name: 'wikipedia',
				enabled: wiki,
				state: !remote || !wiki ? 'off' : resting ? 'resting' : 'on',
				key: 'none',
				clientKey: 'none',
				...(resting
					? {
							until: new Date(started + 40 * minuteMs).toISOString(),
							failure: {
								words: 'Wikipedia answered 503',
								status: 503,
								at: new Date(started - 2 * minuteMs).toISOString()
							}
						}
					: {})
			},
			{
				name: 'fanarttv',
				enabled: fanart,
				state: fanartState,
				key: keyed ? 'set' : 'missing',
				clientKey: s['fanarttv.client_key'].set ? 'set' : 'missing',
				...(refused
					? {
							failure: {
								words: 'fanart.tv refused the key',
								status: 401,
								at: new Date(started - 9 * minuteMs).toISOString()
							}
						}
					: {})
			}
		];
	}

	return {
		status: () =>
			options.respond((): MetadataStatus => {
				const at = now();
				settle(at);
				const s = options.settings();
				const remote = s['metadata.remote'].value;
				const asked = done(at);
				const named = options.artists();
				const artist = pass === null ? undefined : named.at(asked % Math.max(1, named.length));
				return {
					remote,
					language: s['metadata.language'].value,
					pass:
						pass === null || !remote
							? { running: false, done: 0, total: 0 }
							: {
									running: true,
									done: asked,
									total: pass.total,
									...(artist === undefined ? {} : { artist })
								},
					sources: sources(s),
					artists: artists(at),
					...(finished && remote
						? { nextRefresh: new Date(started + 9 * dayMs).toISOString() }
						: {})
				};
			}),
		changed(change) {
			if ('fanarttv.api_key' in change || 'fanarttv.client_key' in change) {
				refused = false;
			}
			// Turned on, or a source switched on, the pass goes over everyone not asked about yet.
			if (change['metadata.remote'] === true && !finished && pass === null) {
				pass = { at: now(), from: 0, total: withMbid };
			}
		}
	};
}
