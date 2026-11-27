/** Choose artwork's types, by the contract's names: see `api/openapi.yaml`. */

import type { Credit, ExtraImage, Picture } from '#lib/library/source.ts';

/** The kinds of an artist's image, in the order the picker's strip shows them. */
export const artistImageKinds = ['thumb', 'background', 'logo', 'banner'] as const;

export type ArtistImageKind = (typeof artistImageKinds)[number];

/** One image an artist could show. `id` is opaque: it goes back verbatim to pin it. */
export interface ImageCandidate {
	id: string;
	/** `folder`: a file in the artist's folder; `upload`: one an administrator uploaded. */
	source: 'folder' | 'fanart' | 'wikimedia' | 'upload';
	/** The source image's, 0 when its source doesn't say. */
	width: number;
	height: number;
	/** Its language; absent when it has no words. */
	lang?: string;
	credit?: Credit;
	/** The server's own rendering, at most 400 px; absent when it couldn't be fetched. */
	preview?: Picture;
	/** The one shown now. */
	chosen: boolean;
	/** A pin chose it. */
	pinned: boolean;
	pinnedBy?: string;
	pinnedAt?: string;
}

/** How one source stood for these candidates. */
export interface CandidateSource {
	source: 'fanart' | 'wikimedia';
	state: 'answered' | 'off' | 'held' | 'unavailable' | 'failed';
	/** Why it failed, in the server's words. */
	words?: string;
}

/** `GET /admin/artists/{id}/images?kind=`. */
export interface ArtistImageCandidates {
	kind: ArtistImageKind;
	mbid?: string;
	/** The kind holds a pin. */
	pinned: boolean;
	/** Not this artist holds the artist back: no source is asked about it. */
	held: boolean;
	candidates: ImageCandidate[];
	sources: CandidateSource[];
}

/** A refusal from one of Choose artwork's routes, with the server's words when it gave any. */
export class ArtworkError extends Error {
	override name = 'ArtworkError';
	readonly status: number;
	readonly detail: string;

	constructor(status: number, detail: string) {
		super(detail === '' ? `the server answered ${String(status)}` : detail);
		this.status = status;
		this.detail = detail;
	}
}

/** Choose artwork's routes, for administrators. Each rejects with an {@link ArtworkError}. */
export interface ArtworkSource {
	candidates(artistId: string, kind: ArtistImageKind): Promise<ArtistImageCandidates>;
	/** Pins a candidate; resolves with the image now shown. */
	pin(artistId: string, kind: ArtistImageKind, candidate: string): Promise<ExtraImage>;
	/** Automatic: the kind goes back to what the folders and the sources give. */
	unpin(artistId: string, kind: ArtistImageKind): Promise<void>;
	/** Pins an image from this computer, sent as it is with its own media type. */
	upload(artistId: string, kind: ArtistImageKind, file: Blob): Promise<ExtraImage>;
	/** Look again now: the sources are asked about the artist again. */
	refresh(artistId: string): Promise<void>;
	/** Not this artist: what the sources gave goes, and the artist is held back. */
	reject(artistId: string): Promise<void>;
}
