import { library } from '#lib/library/library.svelte.ts';
import type { LibraryIndex } from '#lib/library/source.ts';
import { player } from '#lib/player/player.svelte.ts';
import {
	artistAt,
	layOut,
	letterTarget,
	rowAt,
	rowKey,
	windowAt,
	type AlbumBlock,
	type ArtistBlock,
	type Layout,
	type Row,
	type RowMetrics
} from './libraryRows.ts';
import { createTapper } from './libraryTap.ts';

/** Where the columns stack. The page's styles say the same. */
const stackedQuery = '(max-width: 1024px)';

/** How a width lays out: its row heights, and two lines on the screen. */
interface Frame {
	metrics: RowMetrics;
	/** How far down the page the list starts. */
	top: number;
	/** How far down the screen a row is clear of what's pinned over it. */
	line: number;
}

const wide: Frame = {
	metrics: {
		song: 44,
		disc: 32,
		albumHead: 0,
		// The cover, its title and its facts.
		albumMin: 264,
		artistHead: 0,
		// Where the name pins, 38 down, then its three lines and its facts, with the artist's heart.
		artistMin: 330,
		albumGap: 40,
		artistGap: 72
	},
	top: 96,
	line: 96
};

const stacked: Frame = {
	metrics: {
		song: 48,
		disc: 32,
		albumHead: 72,
		albumMin: 0,
		artistHead: 80,
		artistMin: 0,
		albumGap: 20,
		artistGap: 28
	},
	top: 76,
	line: 156
};

/** How far up from the bottom of the screen the NEXT panel reaches, at its two lines. */
const nextReach = 340;

const noLibrary: LibraryIndex = { artists: [] };

/** An album on screen, with the rows of it that are mounted. */
export interface MountedAlbum {
	block: AlbumBlock;
	rows: readonly Row[];
}

/** An artist on screen, with the albums of it that are. */
export interface MountedArtist {
	index: number;
	block: ArtistBlock;
	albums: MountedAlbum[];
}

/** Where the screen stands: a row and how far into it, never a pixel, which a new layout would move. */
interface Anchor {
	row: number;
	offset: number;
	/** The song at that row, to find it again in a list that has changed. */
	song: string | undefined;
}

/**
 * What the Library page shows. It outlives the page, because coming back must land on the same
 * song, with the same rows mounted, on the first frame.
 */
class LibraryView {
	stacked = $state(typeof matchMedia === 'undefined' ? false : matchMedia(stackedQuery).matches);
	first = $state(0);
	last = $state(-1);
	/** The artist pinned on screen, by index. */
	here = $state(0);
	/** Whether the next artist is still below the NEXT panel: past that, it speaks for itself. */
	nextAhead = $state(false);
	/** The album a tap couldn't play. */
	trouble = $state<string | null>(null);

	readonly frame = $derived(this.stacked ? stacked : wide);
	readonly #index = $derived(library.index.kind === 'ready' ? library.index.value : noLibrary);
	readonly layout: Layout = $derived(layOut(this.#index, this.frame.metrics));
	readonly mounted: MountedArtist[] = $derived(this.#mount());

	/** Null at the top of a list nobody has scrolled. */
	#anchor: Anchor | null = null;

	readonly #tap = createTapper({
		album: (id) => library.album(id),
		load: (id) => library.loadAlbum(id),
		play: (album, index) => {
			player.play(album, index);
		},
		failed: (id) => {
			this.trouble = id;
		}
	});

	#mount(): MountedArtist[] {
		const { rows, albums, artists } = this.layout;
		const { first, last } = this;
		const firstAlbum = rows.at(first)?.albumIndex;
		const lastAlbum = rows.at(last)?.albumIndex;
		if (firstAlbum === undefined || lastAlbum === undefined || last < first) {
			return [];
		}
		const mounted: MountedArtist[] = [];
		for (const block of albums.slice(firstAlbum, lastAlbum + 1)) {
			const shown = rows.slice(Math.max(first, block.firstRow), Math.min(last, block.lastRow) + 1);
			const artist = artists.at(block.artistIndex);
			if (artist !== undefined) {
				if (mounted.at(-1)?.index !== block.artistIndex) {
					mounted.push({ index: block.artistIndex, block: artist, albums: [] });
				}
				mounted.at(-1)?.albums.push({ block, rows: shown });
			}
		}
		return mounted;
	}

	/** Follows the width. Returns the cleanup. */
	watch(): () => void {
		const query = matchMedia(stackedQuery);
		const read = (): void => {
			this.stacked = query.matches;
		};
		read();
		query.addEventListener('change', read);
		return () => {
			query.removeEventListener('change', read);
		};
	}

	/** Reads the screen. A scroll settling a landing mustn't move the place: only `keep` does that. */
	track(scroll: number, height: number, keep: boolean): void {
		const { layout, frame } = this;
		if (layout.rows.length === 0) {
			return;
		}
		const y = scroll - frame.top;
		const mounted = { first: this.first, last: this.last };
		const next = windowAt(layout, y, height, mounted);
		if (next !== mounted) {
			this.first = next.first;
			this.last = next.last;
		}
		const line = y + frame.line;
		this.here = artistAt(layout, line);
		const following = layout.artists.at(this.here + 1);
		this.nextAhead = following !== undefined && following.top - y > height - nextReach;
		if (keep) {
			const row = rowAt(layout, line);
			const offset = line - (layout.rows.at(row)?.top ?? 0);
			this.#anchor = { row, offset, song: rowKey(layout, row) };
		}
	}

	/** Finds the scroll that puts a place in the list on the line. */
	#scrollTo(y: number): number {
		return y - this.frame.line + this.frame.top;
	}

	/** Finds the scroll that puts the screen back on its song, in whatever the layout is now. */
	landing(): number {
		const { layout } = this;
		if (this.#anchor === null) {
			return 0;
		}
		const { row, offset, song } = this.#anchor;
		const held = song === undefined || rowKey(layout, row) === song;
		const found = held ? row : layout.rows.findIndex((_, index) => rowKey(layout, index) === song);
		return this.#scrollTo((layout.rows.at(Math.max(found, 0))?.top ?? 0) + offset);
	}

	/** Finds the scroll that puts a letter's first artist on the line. */
	jump(letter: string): number {
		const { layout, frame } = this;
		const artist = layout.artists.at(letterTarget(layout, letter));
		return this.#scrollTo((artist?.top ?? 0) + frame.metrics.artistHead);
	}

	/** Plays a song and carries on through its album. */
	play(albumId: string, trackId: string): void {
		this.trouble = null;
		this.#tap(albumId, trackId);
	}

	reset(): void {
		this.first = 0;
		this.last = -1;
		this.here = 0;
		this.trouble = null;
		this.#anchor = null;
	}
}

export const libraryView = new LibraryView();
