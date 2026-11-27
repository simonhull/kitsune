/** `GET /admin/metadata`'s types, by the contract's names: see `api/openapi.yaml`. */

import type { ArtistLink } from '#lib/library/source.ts';

/** The sources the pass asks, in the order the server lists them. */
export type MetadataSourceName =
	'musicbrainz' | 'wikipedia' | 'fanarttv' | 'lastfm' | 'coverartarchive';

/**
 * `on` (asked as artists and albums fall due), `off` (its switch or the master switch), `needsKey`
 * (fanart.tv without its project key, Last.fm without its API key), `refused` (it refused the key)
 * or `resting` (down or busy, until `until`).
 */
export type SourceState = 'on' | 'off' | 'needsKey' | 'refused' | 'resting';

/** Whether a key is set, never what it is; `none` for a source that takes none. */
export type KeyState = 'none' | 'set' | 'missing';

/** The last thing that went wrong with a source, in the server's words. */
export interface MetadataFailure {
	words: string;
	status?: number;
	at: string;
}

export interface MetadataSource {
	name: MetadataSourceName;
	enabled: boolean;
	state: SourceState;
	key: KeyState;
	clientKey: KeyState;
	until?: string;
	failure?: MetadataFailure;
}

/** An album the pass has in hand. */
export interface AlbumLink {
	id: string;
	name: string;
}

/**
 * The pass: `done` of `total` artists, then albums (`phase`), and the one in hand. All zero while
 * none runs.
 */
export interface MetadataPass {
	running: boolean;
	phase: 'artists' | 'albums';
	done: number;
	total: number;
	artist?: ArtistLink;
	album?: AlbumLink;
}

/** The library's albums by their covers. */
export interface MetadataAlbums {
	albums: number;
	withMbid: number;
	/** Whose cover is the Cover Art Archive's, as their files have none. */
	fromArchive: number;
	withoutCover: number;
}

/** The library's album artists, by what the pass has of them. */
export interface MetadataArtists {
	albumArtists: number;
	withMbid: number;
	lookedUp: number;
	withArt: number;
	foundNothing: number;
	withoutMbid: number;
}

/** `GET /admin/metadata`: Health's Metadata card. */
export interface MetadataStatus {
	remote: boolean;
	language: string;
	pass: MetadataPass;
	sources: MetadataSource[];
	artists: MetadataArtists;
	albums: MetadataAlbums;
	nextRefresh?: string;
}

/** The route, for administrators. It rejects with `ServerError`; a 503 means no pass is set up. */
export interface MetadataStatusSource {
	status(): Promise<MetadataStatus>;
}
