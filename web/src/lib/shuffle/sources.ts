import { library } from '#lib/library/library.svelte.ts';
import type { Album, LibraryIndex } from '#lib/library/source.ts';
import { keepFor, type LibraryFilter } from '#lib/pages/libraryRows.ts';
import { candidatesOf, unitsOf, type Candidate, type Unit } from './pick.ts';
import type { ShuffleSource } from './session.svelte.ts';

/** A song's marks as they stand now: what a press of a heart or a star has said since the index came. */
function marksOf(trackId: string): { stars?: number; hearted: boolean } {
	const stars = library.marks.stars(trackId);
	return {
		hearted: library.marks.hearted('tracks', trackId),
		...(stars === null ? {} : { stars })
	};
}

/** The index, when the Library page has it. */
function indexNow(): LibraryIndex {
	return library.index.kind === 'ready' ? library.index.value : { artists: [] };
}

/** What a Library filter shuffles: the whole library, or what the filter shows, endlessly. */
export function librarySource(filter: LibraryFilter): ShuffleSource {
	const labels = { everything: 'THE LIBRARY', hearted: 'WHAT YOU HEARTED', rated: '4★ AND UP' };
	return {
		key: `library:${filter}`,
		scope: filter === 'everything' ? { kind: 'library' } : { kind: filter },
		mode: 'endless',
		label: labels[filter],
		units: () => {
			const keep = keepFor(filter, library.marks);
			const { inScope, everyone } = candidatesOf(indexNow(), marksOf, keep);
			return unitsOf(inScope, everyone);
		}
	};
}

/** What a selection on the Library list shuffles: those songs, each once. */
export function selectionSource(trackIds: readonly string[]): ShuffleSource {
	const picked = new Set(trackIds);
	return {
		key: `tracks:${trackIds.join(',')}`,
		scope: { kind: 'tracks', tracks: [...trackIds] },
		mode: 'finite',
		label: `${trackIds.length.toLocaleString('en-US')} ${trackIds.length === 1 ? 'SONG' : 'SONGS'}`,
		units: () => {
			const { inScope, everyone } = candidatesOf(indexNow(), marksOf, (_a, _b, track) =>
				picked.has(track.id)
			);
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
