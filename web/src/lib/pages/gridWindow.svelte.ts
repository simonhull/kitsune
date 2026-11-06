import { albumPageSize, library } from '#lib/library/library.svelte.ts';
import type { AlbumEntry } from '#lib/library/source.ts';

/** How far past the viewport a page still counts as in view, in viewports. */
const viewportMargin = 0.5;
/** How close to the end the next page is asked for, in viewports. */
const fetchAhead = 1.5;

/** How the grid lays out, measured from a page that's mounted. */
interface Measure {
	columns: number;
	/** One row of cards, and the gap under it. */
	rowHeight: number;
	rowGap: number;
}

/**
 * Which pages of an album grid are real. Entries stay where they are, but hundreds of mounted
 * cards make every morph clone, measure and tear down a page nobody can see. Everything else is a
 * box of the height it would be, so the scrollbar and a restored scroll are always right. This
 * outlives its page, because coming back must mount the page it left, and nothing else, first.
 */
export class GridWindow {
	first = $state(0);
	last = $state(0);
	/** Whether the neighbors are mounted too. Off until a landing has finished. */
	wide = $state(false);
	measure = $state.raw<Measure | null>(null);

	readonly #entries: () => readonly AlbumEntry[];
	readonly #nearEnd: () => void;

	/** Asks `nearEnd`, if anything, when the end of the grid comes near. */
	constructor(entries: () => readonly AlbumEntry[], nearEnd: () => void = () => undefined) {
		this.#entries = entries;
		this.#nearEnd = nearEnd;
	}

	get entries(): readonly AlbumEntry[] {
		return this.#entries();
	}

	get pageCount(): number {
		return Math.ceil(this.entries.length / albumPageSize);
	}

	mounted(page: number): boolean {
		const spread = this.wide ? 1 : 0;
		return page >= this.first - spread && page <= this.last + spread;
	}

	/** The height of a page that isn't mounted. */
	height(page: number): number | undefined {
		if (this.measure === null) {
			return undefined;
		}
		const { columns, rowHeight, rowGap } = this.measure;
		const cards = Math.min(albumPageSize, this.entries.length - page * albumPageSize);
		const rows = Math.ceil(cards / columns);
		return rows * rowHeight + (rows - 1) * rowGap;
	}

	/** Works out from the grid's place on screen which pages the viewport can reach. */
	track(top: number, bottom: number): void {
		const full = this.height(0);
		if (full !== undefined && this.measure !== null) {
			const stride = full + this.measure.rowGap;
			const margin = innerHeight * viewportMargin;
			const end = Math.max(0, this.pageCount - 1);
			this.first = Math.min(end, Math.max(0, Math.floor((-top - margin) / stride)));
			this.last = Math.min(end, Math.max(0, Math.floor((innerHeight - top + margin) / stride)));
		}
		if (bottom < innerHeight * (1 + fetchAhead)) {
			this.#nearEnd();
		}
	}

	/** Mounts the page holding an album: for landing on a card nobody left from. */
	aim(albumId: string): void {
		const index = this.entries.findIndex((album) => album.id === albumId);
		if (index >= 0) {
			this.first = this.last = Math.floor(index / albumPageSize);
		}
	}

	reset(): void {
		this.first = 0;
		this.last = 0;
		this.wide = false;
		this.measure = null;
	}
}

/** Makes the window of one grid over `entries`, which may change. */
export function createGridWindow(
	entries: () => readonly AlbumEntry[],
	nearEnd?: () => void
): GridWindow {
	return new GridWindow(entries, nearEnd);
}

/** Home's grid, which fetches its albums a page at a time as they come near. */
export const homeGrid = createGridWindow(
	() => library.albums,
	() => {
		if (library.hasMoreAlbums) {
			void library.loadMoreAlbums();
		}
	}
);
