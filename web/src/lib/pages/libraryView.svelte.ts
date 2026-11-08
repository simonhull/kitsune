import { untrack } from 'svelte';
import { library } from '#lib/library/library.svelte.ts';
import type { LibraryIndex } from '#lib/library/source.ts';
import { player } from '#lib/player/player.svelte.ts';
import {
	artistAt,
	keepFor,
	layOut,
	letterTarget,
	rowAt,
	rowKey,
	windowAt,
	type AlbumBlock,
	type ArtistBlock,
	type Layout,
	type LibraryFilter,
	type Row,
	type RowMetrics
} from './libraryRows.ts';
import { columnsFrom, coverFor, largeFrom, stepFor, type Step } from './columns.ts';
import { createTapper } from './libraryTap.ts';

/** Where the columns stack. The page's styles say the same. */
const stackedQuery = '(max-width: 1024px)';
/** Where song rows have columns, and a header over them. Narrower, they're two lines. */
const columnsQuery = `(min-width: ${String(columnsFrom)}px)`;
/** Where everything takes one step up. */
const largeQuery = `(min-width: ${String(largeFrom)}px)`;

/** How a width lays out: its row heights, and two lines on the screen. */
export interface Frame {
	metrics: RowMetrics;
	/** How far down the page the list starts. */
	top: number;
	/** How far down the screen a row is clear of what's pinned over it. */
	line: number;
}

/** What stands under a cover beside its songs: its title and its facts, at each step. */
const coverNames = { columns: 56, large: 72 };
/** A stacked album's head: its cover, and the room under it. */
const stackedHead = 112 + 20;
/** The columns' header, in a stacked layout part of the artist's head. */
const headerHeight = 40;

/**
 * The frame for a width. Beside the names, the songs start under the columns' header: at 152 under
 * one standing at 96, 40 tall, and at 200 a step up, under one at 128, 50 tall. Stacked, an album
 * is headed by its cover, and its artist's head carries the header when there are columns.
 */
export function frameFor(step: Step, stacked: boolean, cover: number): Frame {
	if (stacked) {
		const columns = step !== 'phone';
		return {
			metrics: {
				song: columns ? 44 : 58,
				disc: 32,
				albumHead: stackedHead,
				albumMin: 0,
				artistHead: columns ? 80 + headerHeight : 80,
				artistMin: 0,
				albumGap: 24,
				artistGap: 28
			},
			top: 128,
			line: columns ? 208 + headerHeight : 208
		};
	}
	const large = step === 'large';
	return {
		metrics: {
			song: large ? 56 : 44,
			disc: large ? 40 : 32,
			albumHead: 0,
			albumMin: cover + (large ? coverNames.large : coverNames.columns),
			artistHead: 0,
			// Where the name pins, under the filters, then its three lines and its facts.
			artistMin: large ? 460 : 382,
			albumGap: large ? 56 : 40,
			artistGap: large ? 96 : 72
		},
		top: large ? 200 : 152,
		line: large ? 200 : 152
	};
}

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
	/** Where its artist stands in the whole index: a filter that hides the song may keep the artist. */
	artist: number | undefined;
}

/**
 * What the Library page shows. It outlives the page, because coming back must land on the same
 * song, with the same rows mounted, on the first frame.
 */
class LibraryView {
	stacked = $state(typeof matchMedia === 'undefined' ? false : matchMedia(stackedQuery).matches);
	/** How wide the window is, to the pixel: the covers grow with it, between the steps. */
	width = $state(typeof innerWidth === 'undefined' ? 1920 : innerWidth);
	readonly step: Step = $derived(stepFor(this.width));
	/** Whether rows have columns: from 700 wide. */
	readonly columns = $derived(this.step !== 'phone');
	/** How big a cover is, in pixels. */
	readonly cover = $derived(coverFor(this.width, this.stacked));
	first = $state(0);
	last = $state(-1);
	/** The artist pinned on screen, by index. */
	here = $state(0);
	/** Whether the next artist is still below the NEXT panel: past that, it speaks for itself. */
	nextAhead = $state(false);
	/** The album a tap couldn't play. */
	trouble = $state<string | null>(null);
	/** What the list shows. It's kept here so it's still chosen on coming back. */
	filter = $state<LibraryFilter>('everything');

	readonly frame: Frame = $derived(frameFor(this.step, this.stacked, this.cover));
	readonly #index = $derived(library.index.kind === 'ready' ? library.index.value : noLibrary);
	/** Moves on to look at the marks again: a filter reads them once, and a press doesn't move rows. */
	#looked = $state(0);
	/**
	 * The marks are read untracked, so a heart taken off under Hearted leaves its row where the press
	 * was, to put back. Choosing a filter again, or coming back to the page, reads them afresh.
	 */
	readonly layout: Layout = $derived.by(() => {
		const index = this.#index;
		const { metrics } = this.frame;
		const filter = this.filter;
		void this.#looked;
		return untrack(() => layOut(index, metrics, keepFor(filter, library.marks)));
	});
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
		const queries = [matchMedia(stackedQuery), matchMedia(columnsQuery), matchMedia(largeQuery)];
		const read = (): void => {
			this.stacked = queries[0]?.matches ?? false;
			this.width = innerWidth;
		};
		read();
		for (const query of queries) {
			query.addEventListener('change', read);
		}
		return () => {
			for (const query of queries) {
				query.removeEventListener('change', read);
			}
		};
	}

	/** Reads the screen. A scroll settling a landing mustn't move the place: only `keep` does that. */
	track(scroll: number, height: number, keep: boolean): void {
		// A resize comes through here: the covers follow the width, a step only at its edge.
		if (typeof innerWidth !== 'undefined' && innerWidth !== this.width) {
			this.width = innerWidth;
		}
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
			const artist = layout.artists.at(this.here)?.source;
			this.#anchor = { row, offset, song: rowKey(layout, row), artist };
		}
	}

	/** Finds the scroll that puts a place in the list on the line. */
	#scrollTo(y: number): number {
		return y - this.frame.line + this.frame.top;
	}

	/**
	 * Finds the scroll that puts the screen back on its song, in whatever the layout is now. A song
	 * a filter hides lands on its artist, or the next artist shown after it.
	 */
	landing(): number {
		const { layout } = this;
		if (this.#anchor === null) {
			return 0;
		}
		const { row, offset, song, artist } = this.#anchor;
		const held = song === undefined || rowKey(layout, row) === song;
		const found = held ? row : layout.rows.findIndex((_, index) => rowKey(layout, index) === song);
		if (found >= 0 || artist === undefined) {
			return this.#scrollTo((layout.rows.at(Math.max(found, 0))?.top ?? 0) + offset);
		}
		const next = layout.artists.find(({ source }) => source >= artist) ?? layout.artists.at(-1);
		return this.#scrollTo((next?.top ?? 0) + this.frame.metrics.artistHead);
	}

	/** Shows what a filter keeps. Choosing the one already shown looks at the marks again. */
	choose(filter: LibraryFilter): void {
		this.filter = filter;
		this.refilter();
	}

	/** Looks at the marks again, for a list that may be behind them. */
	refilter(): void {
		this.#looked += 1;
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
		this.filter = 'everything';
		this.#anchor = null;
	}
}

export const libraryView = new LibraryView();
