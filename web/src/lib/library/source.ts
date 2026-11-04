/** An album's list entry: everything a page needs for its first frame. */
export interface AlbumEntry {
	/** The ID, which is also the cover seed. */
	id: string;
	title: string;
	artist: string;
}

export interface Track {
	number: number;
	title: string;
	featuring?: string;
	seconds: number;
	/** ReplayGain in dB, or null when the file has none. */
	replayGain: number | null;
}

export interface Album extends AlbumEntry {
	year: number;
	minutes: number;
	tracks: Track[];
	format: string;
	path: string;
	added: string;
}

export interface Home {
	greeting: string;
	summary: string;
	continueListening: {
		album: AlbumEntry;
		trackNumber: number;
		trackCount: number;
		positionSeconds: number;
	};
	health: { issues: number; detail: string };
	week: { listened: string; detail: string };
	recentlyAdded: AlbumEntry[];
	onRepeat: AlbumEntry[];
}

export interface NowPlaying {
	album: AlbumEntry;
	track: Track;
	trackCount: number;
	positionSeconds: number;
	format: string;
	upNext: { title: string; albumId: string }[];
	moreCount: number;
}

/** Where every page gets its data. The real API replaces the fixtures behind this. */
export interface LibrarySource {
	home(): Promise<Home>;
	/** Rejects with {@link AlbumNotFoundError} for an unknown ID. */
	album(id: string): Promise<Album>;
	nowPlaying(): Promise<NowPlaying>;
}

export class AlbumNotFoundError extends Error {
	override name = 'AlbumNotFoundError';

	constructor(id: string) {
		super(`no album with ID ${id}`);
	}
}
