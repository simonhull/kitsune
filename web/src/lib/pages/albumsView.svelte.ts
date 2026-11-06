import {
	genreCounts,
	keyed,
	marksOf,
	ordered,
	withGenre,
	type AlbumOrder,
	type AlbumsQuery
} from '#lib/library/albumOrder.ts';
import { library } from '#lib/library/library.svelte.ts';
import { createGridWindow } from './gridWindow.svelte.ts';

/**
 * What the Albums page shows. It outlives the page, as its grid does, so a card opened from a sorted
 * and filtered list comes back to that same list. The URL says what's asked for: see `show`.
 */
class AlbumsView {
	order = $state<AlbumOrder>('name');
	genre = $state<string | null>(null);

	readonly #keyed = $derived(library.albums.map(keyed));
	readonly #ordered = $derived(ordered(this.#keyed, this.order));
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
	show({ order, genre }: AlbumsQuery): void {
		if (order !== this.order || genre !== this.genre) {
			this.order = order;
			this.genre = genre;
			this.grid.rewind();
		}
	}

	reset(): void {
		this.order = 'name';
		this.genre = null;
		this.grid.reset();
	}
}

export const albumsView = new AlbumsView();
