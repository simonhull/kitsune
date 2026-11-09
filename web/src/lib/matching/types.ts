import type { FieldValues } from '#lib/library/source.ts';

/** What `POST /albums/{id}/matches` asks for besides the album's own tags. */
export interface MatchTerms {
	artist?: string;
	album?: string;
	/** A catalog number, or a barcode when it's all digits. */
	code?: string;
}

/** One thing a confidence rests on, as facts for the client to word. */
export interface MatchEvidence {
	kind:
		'lengths' | 'titleArtist' | 'title' | 'artist' | 'tagged' | 'discs' | 'songs' | 'fingerprint';
	verdict: 'agrees' | 'neutral' | 'differs';
	count?: number;
	of?: number;
	format?: string;
}

export interface MatchCredit {
	name: string;
	id?: string;
	join: string;
}

export interface MatchMedium {
	position: number;
	format: string;
	title: string;
	trackCount: number;
}

export interface MatchRelease {
	id: string;
	groupId?: string;
	title: string;
	/** The credit as it reads. */
	artist: string;
	artists: MatchCredit[];
	date?: string;
	originalDate?: string;
	country?: string;
	barcode?: string;
	status?: string;
	labels: { name: string; catalogNumber: string }[];
	media: MatchMedium[];
	trackCount: number;
}

export interface MatchCandidate {
	/** Opaque: for `GET /matches/{id}`. */
	id: string;
	/** Confidence, 0 to 100. */
	score: number;
	evidence: MatchEvidence[];
	release: MatchRelease;
	cover?: string;
	page: string;
}

export interface MatchList {
	source: 'musicbrainz';
	asked: { artist: string; album: string; catalogNumber: string; barcode: string };
	candidates: MatchCandidate[];
}

/** A release track, and what it would write to the file paired with it. */
export interface MatchTrack {
	disc: number;
	position: number;
	title: string;
	artist: string;
	lengthMs?: number;
	proposed: FieldValues;
	/** MusicBrainz IDs: always written with the song. */
	ids: FieldValues;
}

export interface MatchReview {
	candidate: MatchCandidate;
	album: FieldValues;
	albumIds: FieldValues;
	tracks: MatchTrack[];
	/** One for each album track, in album order. */
	pairs: { track: string; release?: number }[];
}

export type MatchState = 'todo' | 'skipped' | 'matched';

export interface QueuedAlbum {
	id: string;
	title: string;
	artist: string;
	songs: number;
	state: MatchState;
}

export interface MatchQueue {
	counts: { todo: number; skipped: number; matched: number };
	albums: QueuedAlbum[];
}
