import {
	genreCounts,
	keyed,
	marksOf,
	ordered,
	withGenre,
	type AlbumsQuery,
	type Standing
} from '#lib/library/albumOrder.ts';
import { albumStars } from '#lib/library/albumStars.ts';
import { albumPlays, sortFields } from '#lib/library/libraryOrder.ts';
import { library } from '#lib/library/library.svelte.ts';
import type { IndexField } from '#lib/library/source.ts';
import type { AlbumOrder, Sort } from '#lib/library/sorts.ts';
import { columnsView } from './columnsView.svelte.ts';
import { createGridWindow } from './gridWindow.svelte.ts';
import { sorts } from './sortsView.svelte.ts';

/** Whether an order reads what only the Library's index knows: plays and ratings are per song. */
export function needsIndex({ by }: Sort<AlbumOrder>): boolean {
	return by === 'plays' || by === 'rating';
}

/**
 * What the Albums page shows. It outlives the page, as its grid does, so a card opened from a sorted
 * and filtered list comes back to that same list. The URL says which genre; the order is the
 * person's (`albums.sort`).
 */
class AlbumsView {
	genre = $state<string | null>(null);

	readonly order = $derived(sorts.albums.value);
	readonly #keyed = $derived(
		(library.trashed.empty
			? library.albums
			: library.albums.filter(({ id }) => !library.trashed.hidesAlbum(id))
		).map(keyed)
	);
	/** Each album's plays and stars, off the index, for the orders that need them. */
	readonly #standing: ReadonlyMap<string, Standing> = $derived.by(() => {
		if (!needsIndex(this.order) || library.index.kind !== 'ready') {
			return new Map<string, Standing>([]);
		}
		return new Map(
			library.index.value.artists.flatMap(({ albums }) =>
				albums.map((album) => {
					const stars = albumStars(album.tracks, library.marks);
					const standing: Standing = {
						plays: albumPlays(album),
						...(stars === null ? {} : { rating: stars.mean })
					};
					return [album.id, standing] as const;
				})
			)
		);
	});
	readonly #ordered = $derived(ordered(this.#keyed, this.order, (id) => this.#standing.get(id)));
	readonly #shown = $derived(
		this.genre === null ? this.#ordered : withGenre(this.#ordered, this.genre)
	);
	/** The albums on screen, in order. */
	readonly albums = $derived(this.#shown.map(({ album }) => album));
	/** Where the rail can jump to in them. */
	readonly marks = $derived(marksOf(this.#shown, this.order));
	/** Every genre in the library, the most used first. A filter narrows the grid, not this. */
	readonly genres = $derived(genreCounts(library.albums));

	readonly grid = createGridWindow(
		() => this.albums,
		() => void library.loadAllAlbums()
	);

	/** Takes up what the URL asks for. A different list starts from its top. */
	show({ genre }: AlbumsQuery): void {
		if (genre !== this.genre) {
			this.genre = genre;
			this.grid.rewind();
		}
	}

	/** Puts the albums in another order, from its top. */
	sortBy(next: Sort<AlbumOrder>): void {
		sorts.albums.set(next);
		this.grid.rewind();
	}

	/**
	 * Reads the index when the order needs it and it isn't held, with what the Library would ask for
	 * besides, so going there next doesn't read it again.
	 */
	async readIndex(): Promise<void> {
		if (!needsIndex(this.order)) {
			return;
		}
		await Promise.all([columnsView.load(), sorts.library.load()]);
		const fields: IndexField[] = [
			...new Set<IndexField>([...columnsView.fields, ...sortFields(sorts.library.value), 'plays'])
		];
		if (library.index.kind !== 'ready' || library.indexStale || !library.holdsFields(fields)) {
			await library.loadIndex(fields);
		}
	}

	reset(): void {
		this.genre = null;
		this.grid.reset();
	}
}

export const albumsView = new AlbumsView();
