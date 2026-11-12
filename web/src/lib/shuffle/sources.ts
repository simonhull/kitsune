import { library } from '#lib/library/library.svelte.ts';
import type { Album, LibraryIndex, ShuffleMode, ShuffleScope } from '#lib/library/source.ts';
import { keepFor, type LibraryFilter } from '#lib/pages/libraryRows.ts';
import { candidatesOf, unitsOf, type Candidate, type Unit } from './pick.ts';
import type { ShuffleSource } from './session.svelte.ts';
import { libraryLabels } from './words.ts';

/**
 * A song's marks as they stand now: what a press of a heart or a star has said since the index came.
 * Its stars are the counted ones, read from memory as the server's shuffle reads its pref, so the
 * first pick weighs songs as the server will, and waits on nothing.
 */
function marksOf(trackId: string): { stars?: number; hearted: boolean } {
	const rating = library.marks.counted(trackId);
	return {
		hearted: library.marks.hearted('tracks', trackId),
		...(rating === null ? {} : { stars: rating.stars })
	};
}

/** The index, when the Library page has it. */
function indexNow(): LibraryIndex {
	return library.index.kind === 'ready' ? library.index.value : { artists: [] };
}

/** What a Library filter shuffles: the whole library, or what the filter shows, endlessly. */
export function librarySource(filter: LibraryFilter): ShuffleSource {
	return {
		key: `library:${filter}`,
		scope: filter === 'everything' ? { kind: 'library' } : { kind: filter },
		mode: 'endless',
		label: libraryLabels[filter],
		units: () => {
			const keep = keepFor(filter, library.marks);
			const { inScope, everyone } = candidatesOf(indexNow(), marksOf, keep);
			return unitsOf(inScope, everyone);
		}
	};
}

/**
 * What a selection shuffles: those songs, each once. Their first comes from `index`, the list they
 * were picked in: the Library's, unless a page holds its own.
 */
export function selectionSource(
	trackIds: readonly string[],
	index: () => LibraryIndex = indexNow
): ShuffleSource {
	const picked = new Set(trackIds);
	return {
		key: `tracks:${trackIds.join(',')}`,
		scope: { kind: 'tracks', tracks: [...trackIds] },
		mode: 'finite',
		label: `${trackIds.length.toLocaleString('en-US')} ${trackIds.length === 1 ? 'SONG' : 'SONGS'}`,
		units: () => {
			const { inScope, everyone } = candidatesOf(index(), marksOf, (_a, _b, track) =>
				picked.has(track.id)
			);
			return unitsOf(inScope, everyone);
		}
	};
}

/**
 * What an artist page's Shuffle shuffles: every song the artist is on, as an artist or an album
 * artist, each once. The first is picked from the page's own list of them, `index`.
 */
export function artistSource(id: string, name: string, index: () => LibraryIndex): ShuffleSource {
	return {
		key: `artist:${id}`,
		scope: { kind: 'artist', id },
		mode: 'finite',
		label: name.toUpperCase(),
		units: () => {
			const { inScope, everyone } = candidatesOf(index(), marksOf);
			return unitsOf(inScope, everyone);
		}
	};
}

/**
 * What an album's Shuffle shuffles: its songs, each once. Its rules come from the server, since the
 * album page has no index; a run that reaches another album finds that album there too.
 */
export function albumSource(album: Album): ShuffleSource {
	return {
		key: `album:${album.id}`,
		scope: { kind: 'album', id: album.id },
		mode: 'finite',
		label: album.title.toUpperCase(),
		units: () => albumUnits(album)
	};
}

async function albumUnits(album: Album): Promise<Unit[]> {
	const own = album.tracks.map(({ id }) => id);
	const rules = await library.shuffleRules(own).catch(() => ({ runs: [], excluded: [] }));
	const excluded = new Set(rules.excluded);
	const runOf = new Map(
		rules.runs.flatMap(({ id, tracks }) =>
			tracks.map(
				(track, position) => [track, { run: id, position, length: tracks.length }] as const
			)
		)
	);
	const candidate = (id: string, albumId: string): Candidate => ({
		id,
		albumId,
		...marksOf(id),
		mark: { ...runOf.get(id), ...(excluded.has(id) ? { excluded: true } : {}) }
	});
	const inScope = own.map((id) => candidate(id, album.id));
	const elsewhere = rules.runs.flatMap(({ tracks }) => tracks).filter((id) => !own.includes(id));
	const found = await Promise.all(
		elsewhere.map(async (id) => {
			try {
				return candidate(id, await library.trackAlbum(id));
			} catch {
				return null;
			}
		})
	);
	const everyone = [...inScope, ...found.filter((each) => each !== null)];
	return unitsOf(inScope, everyone);
}

/**
 * A remembered shuffle's source, rebuilt from what the server says it draws from, so Now playing
 * says what it shuffles. An album's or an artist's name shows once it's known. A resumed shuffle
 * never picks a first song, so its units are none.
 */
export function sourceOf(scope: ShuffleScope, mode: ShuffleMode): ShuffleSource {
	const none = (): Unit[] => [];
	switch (scope.kind) {
		case 'library':
		case 'hearted':
		case 'rated':
			return { ...librarySource(scope.kind === 'library' ? 'everything' : scope.kind), mode };
		case 'tracks':
			return { ...selectionSource(scope.tracks), mode };
		case 'album':
			return {
				key: `album:${scope.id}`,
				scope,
				mode,
				get label() {
					return library.album(scope.id)?.title.toUpperCase() ?? 'THIS ALBUM';
				},
				units: none
			};
		case 'artist':
			return {
				key: `artist:${scope.id}`,
				scope,
				mode,
				get label() {
					return library.artistName(scope.id)?.toUpperCase() ?? 'THIS ARTIST';
				},
				units: none
			};
	}
}
