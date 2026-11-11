<!--
	An artist page's songs: Top songs under the headline, which grows in place into all of them
	(`?songs`), flat in an order or by album. The Library's own SongRow, columns and windowing, in an
	artist's context; the shelves below it fall back while the full list shows, and come forward again
	when it folds. Expanding and folding are different motions, and neither is the other reversed.
-->
<script lang="ts">
	import { onMount, tick, untrack, type Snippet } from 'svelte';
	import { clock } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type {
		ArtistDetail,
		ColumnId,
		IndexField,
		LibraryIndex,
		MovableId
	} from '#lib/library/source.ts';
	import {
		artistAlbumOrders,
		artistSongOrders,
		wayOf,
		type ArtistSongs as ArtistSongsPref
	} from '#lib/library/sorts.ts';
	import { matchFromBulk } from '#lib/matching/hooks.ts';
	import { player } from '#lib/player/player.svelte.ts';
	import Icon from '#lib/shell/Icon.svelte';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import { artistSource, selectionSource } from '#lib/shuffle/sources.ts';
	import {
		albumOrder,
		albumSongs,
		albumsLayout,
		coldStart,
		flatOrder,
		headingOf,
		queueFrom,
		rowWindow,
		songsOf,
		topCount,
		type ArtistSong
	} from './artistSongs.ts';
	import BulkEditor from './BulkEditor.svelte';
	import { bulkEdits } from './bulkEdits.svelte.ts';
	import {
		columnsFrom,
		columnsIn,
		columnVars,
		coverFor,
		fit,
		largeFrom,
		monoAdvance,
		movable,
		numbered,
		reorderIn,
		template,
		type ColumnsContext,
		type RowParts
	} from './columns.ts';
	import ColumnsMenu from './ColumnsMenu.svelte';
	import { columnsView } from './columnsView.svelte.ts';
	import { songGrid } from './songGrid.ts';
	import LibraryAlbum from './LibraryAlbum.svelte';
	import { windowAt, type RowWindow } from './libraryRows.ts';
	import { frameFor } from './libraryView.svelte.ts';
	import LinePill from './LinePill.svelte';
	import PickBox from './PickBox.svelte';
	import RowMenu from './RowMenu.svelte';
	import SelectionBar from './SelectionBar.svelte';
	import { onScreen, Selection } from './selection.svelte.ts';
	import {
		actionsFor,
		menuTarget,
		pickedFacts,
		type MenuTarget,
		type SongActions,
		type SongFacts
	} from './songActions.ts';
	import SongHeader from './SongHeader.svelte';
	import SongRow from './SongRow.svelte';
	import { rowEvents, SongList } from './songRow.ts';
	import SortControl, { type SortGroup } from './SortControl.svelte';
	import { sorts } from './sortsView.svelte.ts';
	import TagBar from './TagBar.svelte';

	interface Props {
		id: string;
		name: string;
		/** The artist, once it has come: its own albums, and how many songs it is on. */
		detail: ArtistDetail | undefined;
		/** The full list shows, as `?songs` says. */
		expanded: boolean;
		/** Asks for the full list, or for Top songs again: the page owns the URL. */
		expand: () => void;
		fold: () => void;
		/** The album shelves, which stand below Top songs and give way to the full list. */
		shelves: Snippet;
	}

	let { id, name, detail, expanded, expand, fold, shelves }: Props = $props();

	/** The Library's steps: two lines a song under 700, a cover over its songs to 1024, and 4K's. */
	let windowWidth = $state(typeof innerWidth === 'undefined' ? 1440 : innerWidth);
	let windowHeight = $state(typeof innerHeight === 'undefined' ? 900 : innerHeight);
	const phone = $derived(windowWidth < columnsFrom);
	const stacked = $derived(windowWidth <= 1024);
	const large = $derived(windowWidth >= largeFrom);
	const columnStep = $derived(large ? 'large' : 'columns');
	const cover = $derived(coverFor(windowWidth, stacked));
	const coverGap = $derived(large ? 48 : 32);
	/** A flat row: the Library's heights, two lines on a phone. */
	const flatRow = $derived(phone ? 60 : large ? 56 : 44);
	/** The Appears on divider, between the artist's own albums and the rest. */
	const dividerHeight = $derived(large ? 90 : 72);

	const load = $derived(library.artistSongs(id));
	const index: LibraryIndex | undefined = $derived(load.kind === 'ready' ? load.value : undefined);
	const owned = $derived(new Set((detail?.albums ?? []).map((album) => album.id)));
	const songs = $derived(index === undefined ? [] : songsOf(index, detailOwned()));
	/** The albums it owns: until the artist comes, every album, so nothing is called a guest's. */
	function detailOwned(): ReadonlySet<string> {
		return detail === undefined
			? new Set(index?.artists.flatMap((artist) => artist.albums.map((album) => album.id)) ?? [])
			: owned;
	}

	const pref: ArtistSongsPref = $derived(sorts.artistSongs.value);
	const albumSort = $derived(sorts.library.value.albums);
	/** Top songs is always the first of Top; the full list runs as the person chose. */
	const grouped = $derived(expanded && pref.group === 'album');
	const flatSort = $derived(expanded ? pref.sort : ({ by: 'top', dir: 'desc' } as const));
	/**
	 * The songs in the list's order. The marks are read untracked, as the Library's filters read
	 * them: a star pressed here doesn't move its row out from under the pointer.
	 */
	const list: ArtistSong[] = $derived.by(() => {
		const all = songs;
		const sort = flatSort;
		if (grouped && index !== undefined) {
			const runs = albumOrder(index, detailOwned(), albumSort);
			return albumSongs(runs, all);
		}
		return untrack(() => flatOrder(all, sort, library.marks));
	});
	const cold = $derived(untrack(() => coldStart(songs, library.marks)) && songs.length > 0);
	const top = $derived(topCount(phone));
	/** How many songs there are: the artist says before they come, so the section holds its height. */
	const count = $derived(index === undefined ? (detail?.songCount ?? 0) : songs.length);
	const small = $derived(count <= top);
	const heading = $derived(headingOf(expanded, count, top, cold));
	/** What the list shows: Top's first few, or all of them. */
	const shown = $derived(expanded ? list : list.slice(0, top));
	const ranked = $derived(!grouped && flatSort.by === 'top' && heading !== 'songs' && !cold);

	const parts: RowParts = $derived(
		grouped ? numbered : { handle: false, cover: true, number: ranked }
	);
	const context: ColumnsContext = $derived(grouped ? 'artistAlbums' : 'artist');
	const order = $derived(columnsIn(columnsView.pref.order, context));
	/** What the rows show, what the orders read, and the plays Top ranks by. */
	const fields: IndexField[] = $derived([
		...new Set<IndexField>([...columnsView.fields, 'plays', 'added'])
	]);

	const selection = new Selection();
	let listWidth = $state(0);
	/** By album beside the covers, the songs are what's left of the width. */
	const songsWidth = $derived(grouped && !stacked ? listWidth - cover - coverGap : listWidth);
	const fitted = $derived(fit(order, songsWidth, columnStep, parts, columnsView.pref.widths));
	const grid = $derived(template(fitted.shown, columnStep, parts));
	const lineChars = $derived(
		Math.floor((songsWidth - (grouped ? 96 : 140) - (selection.any ? 26 : 0)) / monoAdvance)
	);
	const now = Date.now();
	const cuedId = $derived(library.playing?.track.id);

	/** By album: the Library's own maths, the artist's albums first, then those it appears on. */
	const metrics = $derived({
		...frameFor(phone ? 'phone' : columnStep, stacked, cover).metrics,
		artistHead: 0,
		artistMin: 0,
		artistGap: 0
	});
	const albums = $derived(
		grouped && index !== undefined && index.artists[0] !== undefined
			? albumsLayout(
					index.artists[0],
					albumOrder(index, detailOwned(), albumSort),
					metrics,
					dividerHeight
				)
			: null
	);
	/** How tall the list stands: before its songs come, as tall as they'll be. */
	const height = $derived(
		albums === null ? (expanded ? count : Math.min(top, count)) * flatRow : albums.height
	);

	// Rows near the screen are mounted, whatever the count: the Library's windowing.
	let listElement: HTMLElement | undefined = $state();
	let mounted = $state<RowWindow>({ first: 0, last: -1 });
	/** The frame line is pinned under the nav: only then does a veil stand over the rows above it. */
	let pinned = $state(false);
	function track(): void {
		pinned = expanded && (lineElement?.getBoundingClientRect().top ?? Infinity) <= pinTop + 1;
		const y = -(listElement?.getBoundingClientRect().top ?? 0);
		const next =
			albums === null
				? rowWindow(shown.length, flatRow, y, windowHeight, mounted)
				: windowAt(albums, y, windowHeight, mounted);
		if (next !== mounted) {
			mounted = next;
		}
	}
	// A new list, or a new layout, finds its window again.
	$effect(() => {
		void shown;
		void albums;
		void flatRow;
		untrack(track);
	});
	const flatRows = $derived(
		albums !== null
			? []
			: shown
					.slice(mounted.first, Math.max(mounted.first, mounted.last + 1))
					.map((song, offset) => ({ song, at: mounted.first + offset }))
	);
	const albumRows = $derived.by(() => {
		if (albums === null) {
			return [];
		}
		const { rows, albums: blocks } = albums;
		const firstAlbum = rows.at(mounted.first)?.albumIndex;
		const lastAlbum = rows.at(mounted.last)?.albumIndex;
		if (firstAlbum === undefined || lastAlbum === undefined || mounted.last < mounted.first) {
			return [];
		}
		return blocks.slice(firstAlbum, lastAlbum + 1).map((block) => ({
			block,
			rows: rows.slice(
				Math.max(mounted.first, block.firstRow),
				Math.min(mounted.last, block.lastRow) + 1
			)
		}));
	});

	// Ask for the songs once the columns are known, and again for a change or more fields.
	$effect(() => {
		const wanted = fields;
		const artist = id;
		if (columnsView.known && !library.holdsArtistSongs(artist, wanted)) {
			untrack(() => void library.loadArtistSongs(artist, wanted));
		}
	});

	/** Every song the list shows, in its order: what a Shift range and the header's box run over. */
	const ids = $derived(shown.map(({ track: song }) => song.id));
	const songOf = $derived(new Map(songs.map((song) => [song.track.id, song])));
	const albumOf = $derived(new Map(songs.map((song) => [song.track.id, song.album.id])));
	const indexNow = (): LibraryIndex => index ?? { artists: [] };

	onMount(() => onScreen.show(selection));
	const songList = new SongList(selection, {
		order: () => ids,
		reveal: (trackId) => {
			reveal(trackId);
		}
	});
	const picked = $derived(selection.inOrder(ids));
	const pickedActions = $derived(actionsFor(picked, albumOf, indexNow));
	const songFacts: SongFacts = (trackId) => {
		const song = songOf.get(trackId);
		return song === undefined
			? undefined
			: { albumId: song.album.id, durationMs: song.track.durationMs };
	};
	const bulk = $derived(bulkEdits.current?.layout === 'selection' ? bulkEdits.current : null);
	let bulkNote = $state('');

	function editPicked(): void {
		bulkNote = '';
		bulkEdits.open(
			'selection',
			picked.flatMap((trackId) => {
				const song = songOf.get(trackId);
				return song === undefined
					? []
					: [
							{
								id: trackId,
								albumId: song.album.id,
								title: song.track.title,
								trackNo: song.track.trackNo
							}
						];
			}),
			library.user?.canEditTags === true
		);
	}

	async function writeBulk(): Promise<void> {
		const written = await bulk?.write();
		if (written?.kind === 'done') {
			bulkEdits.close();
			bulkNote = 'Tags written.';
		} else if (written?.kind === 'problem') {
			await tick();
			document.getElementById(written.field)?.focus();
		}
	}

	/** Puts a song's row on screen, if it isn't, and gives it the focus. */
	function reveal(trackId: string): void {
		const at = ids.indexOf(trackId);
		const row =
			albums === null
				? { top: at * flatRow, height: flatRow }
				: albums.rows.find(
						(each) =>
							each.kind === 'song' &&
							albums.albums.at(each.albumIndex)?.album.tracks.at(each.trackIndex)?.id === trackId
					);
		if (listElement === undefined || row === undefined || at < 0) {
			return;
		}
		const y = listElement.getBoundingClientRect().top + scrollY + row.top;
		if (y < scrollY + lineBottom || y + row.height > scrollY + innerHeight - 120) {
			scrollTo({ top: y - lineBottom - row.height * 2, behavior: 'instant' });
			track();
		}
		void tick().then(() => {
			listElement
				?.querySelector<HTMLElement>(`[data-track="${CSS.escape(trackId)}"] .play`)
				?.focus();
		});
	}

	// Playing: the song, then the list as shown, to its end. Top songs is a window onto the ranked
	// list, so a press there goes on past the fold.
	let latestTap = 0;
	let trouble = $state<string | null>(null);

	/** Plays from a song, through the list in its order: its album is here, or one fetch. */
	function playFrom(trackId: string): void {
		const from = queueFrom(
			expanded ? list : untrack(() => flatOrder(songs, flatSort, library.marks)),
			trackId
		);
		const [first, ...rest] = from;
		if (first === undefined) {
			return;
		}
		trouble = null;
		latestTap += 1;
		const tap = latestTap;
		const later = rest.map(({ track: song, album }) => ({ trackId: song.id, albumId: album.id }));
		const start = (): void => {
			const album = library.album(first.album.id);
			const song = album?.tracks.find((each) => each.id === trackId);
			if (album === undefined || song === undefined) {
				trouble = first.album.id;
				return;
			}
			player.playList({ album, track: song }, later);
		};
		if (library.album(first.album.id) !== undefined) {
			start();
			return;
		}
		void library.loadAlbum(first.album.id).then(() => {
			if (tap === latestTap) {
				start();
			}
		});
	}

	/** Plays one disc of an album, by album. */
	function playDisc(albumId: string, disc: number): void {
		trouble = null;
		void library.loadAlbum(albumId).then(() => {
			const album = library.album(albumId);
			if (album === undefined) {
				trouble = albumId;
			} else {
				player.playDisc(album, disc);
			}
		});
	}

	const source = $derived(artistSource(id, name, indexNow));
	const shuffleSource = $derived(selection.any ? selectionSource(picked, indexNow) : source);
	// The first song is picked ahead, so a press plays at once.
	$effect(() => {
		const ready = shuffleSource;
		if (index !== undefined) {
			untrack(() => void shuffles.prime(ready));
		}
	});
	function shuffle(): void {
		shuffles.start(shuffleSource);
		void shuffles.prime(shuffleSource);
	}

	let rowMenu = $state<{ x: number; y: number; target: MenuTarget; actions: SongActions } | null>(
		null
	);
	let columnsMenu = $state<{ x: number; y: number; hiding: ColumnId | undefined } | null>(null);
	let opener: HTMLElement | null = null;

	function closeMenus(refocus: boolean): void {
		rowMenu = null;
		columnsMenu = null;
		if (refocus) {
			opener?.focus();
		}
	}

	function openColumns(x: number, y: number, hiding?: ColumnId): void {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		columnsMenu = { x, y, hiding };
	}

	const events = rowEvents({
		play: (trackId) => {
			playFrom(trackId);
		},
		pick: (trackId, how) => {
			songList.pick(trackId, how);
		},
		menu: (trackId, x, y) => {
			opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
			const target = menuTarget(
				trackId,
				selection,
				() => picked,
				songFacts,
				(each) => {
					const song = songOf.get(each);
					return song === undefined
						? undefined
						: {
								albumId: song.album.id,
								title: song.track.title,
								line: `${song.album.title.toUpperCase()} · ${clock(song.track.durationMs / 1000)}`,
								artistId: id
							};
				}
			);
			if (target !== undefined) {
				rowMenu = {
					x,
					y,
					target,
					actions:
						target.kind === 'picked' ? pickedActions : actionsFor([trackId], albumOf, indexNow)
				};
			}
		},
		key: (event, trackId) => {
			songList.key(event, trackId);
		},
		rate: (trackId, stars) => {
			library.marks.rate(trackId, stars);
		},
		picked: (trackId) => selection.has(trackId)
	});

	function onWindowKey(event: KeyboardEvent): void {
		songList.windowKey(event, rowMenu !== null || columnsMenu !== null || bulk !== null);
	}

	// The frame line: the list's own controls, while the full list shows.
	const sortGroups: SortGroup[] = $derived(
		grouped
			? [
					{
						label: 'SORT ALBUMS BY',
						options: artistAlbumOrders,
						current: albumSort,
						choose: (next) => {
							relist(() => {
								sorts.library.set({ ...sorts.library.value, albums: next as typeof albumSort });
							});
						}
					}
				]
			: [
					{
						label: 'SORT SONGS BY',
						options: artistSongOrders,
						current: pref.sort,
						choose: (next) => {
							relist(() => {
								sorts.artistSongs.set({ ...pref, sort: next as typeof pref.sort });
							});
						}
					}
				]
	);
	const sortWords = $derived(
		(grouped
			? artistAlbumOrders.find(({ by }) => by === albumSort.by)?.label
			: artistSongOrders.find(({ by }) => by === pref.sort.by)?.label) ?? ''
	);
	const sortWay = $derived(
		grouped ? wayOf(albumSort.by, albumSort.dir) : wayOf(pref.sort.by, pref.sort.dir)
	);

	function regroup(group: ArtistSongsPref['group']): void {
		if (group !== pref.group) {
			relist(() => {
				sorts.artistSongs.set({ ...pref, group });
			});
		}
	}

	onMount(() => {
		void columnsView.load();
		void sorts.artistSongs.load();
		void sorts.library.load();
		let frame = 0;
		function onScroll(): void {
			frame ||= requestAnimationFrame(() => {
				frame = 0;
				track();
			});
		}
		addEventListener('scroll', onScroll, { passive: true });
		addEventListener('resize', onScroll, { passive: true });
		return () => {
			cancelAnimationFrame(frame);
			removeEventListener('scroll', onScroll);
			removeEventListener('resize', onScroll);
		};
	});

	// ---- Motion. Verbs and springs are DESIGN.md's; scroll is only ever set at once, under them. ----

	let root: HTMLElement | undefined = $state();
	let lineElement: HTMLElement | undefined = $state();
	let lineHeight = $state(0);
	let headHeight = $state(0);
	/** Where the frame line pins, under the nav. */
	const pinTop = $derived(phone ? 72 : large ? 100 : 80);
	/** Where a row is clear of what's pinned over it. */
	const lineBottom = $derived(expanded ? pinTop + lineHeight + headHeight : 0);

	/** One frame of a motion: what `Element.animate` takes. */
	type Frame = Record<string, string | number>;
	const reduced = (): boolean => matchMedia('(prefers-reduced-motion: reduce)').matches;
	/** The morph curve: critically damped, as every journey's spring is. */
	const morphCurve = 'cubic-bezier(0.16, 1, 0.3, 1)';
	/** An exit: it starts slow and leaves fast, as the user has already decided. */
	const exitCurve = 'cubic-bezier(0.5, 0, 0.9, 0.6)';
	function token(name: string): number {
		const value = getComputedStyle(document.documentElement).getPropertyValue(name).trim();
		const number = Number.parseFloat(value);
		return Number.isNaN(number) ? 0 : value.endsWith('ms') ? number : number * 1000;
	}

	/** Plays a motion, or under reduced motion the 120 ms blend of its opacity alone. */
	function move(
		element: Element | null | undefined,
		frames: Frame[],
		duration: number,
		delay = 0,
		easing = morphCurve
	): Animation | null {
		if (element === null || element === undefined) {
			return null;
		}
		if (reduced()) {
			const from = frames.at(0)?.opacity ?? 1;
			const to = frames.at(-1)?.opacity ?? 1;
			return from === to
				? null
				: element.animate([{ opacity: from }, { opacity: to }], { duration: 120, fill: 'both' });
		}
		const animation = element.animate(frames, { duration, delay, easing, fill: 'backwards' });
		return animation;
	}

	/** A picture of something as it stands on screen, laid over the page, to move as it leaves. */
	function ghost(
		element: Element | null | undefined,
		strip?: (copy: HTMLElement) => void
	): HTMLElement | null {
		if (!(element instanceof HTMLElement)) {
			return null;
		}
		const box = element.getBoundingClientRect();
		if (box.bottom < 0 || box.top > innerHeight || box.height === 0) {
			return null;
		}
		const copy = element.cloneNode(true) as HTMLElement;
		strip?.(copy);
		copy.inert = true;
		copy.setAttribute('aria-hidden', 'true');
		copy.removeAttribute('id');
		Object.assign(copy.style, {
			position: 'fixed',
			left: `${String(box.left)}px`,
			top: `${String(box.top)}px`,
			width: `${String(box.width)}px`,
			height: `${String(box.height)}px`,
			margin: '0',
			pointerEvents: 'none',
			zIndex: '1'
		});
		copy.classList.add('songs-ghost');
		document.body.append(copy);
		return copy;
	}

	function gone(copy: HTMLElement | null, animation: Animation | null): void {
		if (copy === null) {
			return;
		}
		if (animation === null) {
			copy.remove();
		} else {
			void animation.finished.then(
				() => {
					copy.remove();
				},
				() => {
					copy.remove();
				}
			);
		}
	}

	/** The rows and blocks on screen now, top to bottom. */
	function onScreenRows(): HTMLElement[] {
		return [
			...(listElement?.querySelectorAll<HTMLElement>(
				':scope > .row, :scope > .album, :scope > .divider'
			) ?? [])
		].filter((element) => {
			const box = element.getBoundingClientRect();
			return box.bottom > 0 && box.top < innerHeight;
		});
	}

	/** The song nearest the top of the screen, clear of the frame line, and where it stands. */
	function anchor(): { trackId: string; y: number } | null {
		const rows = listElement?.querySelectorAll<HTMLElement>('[data-track]') ?? [];
		for (const row of rows) {
			const box = row.getBoundingClientRect();
			if (box.bottom > lineBottom + 8 && box.top < innerHeight) {
				const trackId = row.dataset['track'];
				return trackId === undefined ? null : { trackId, y: box.top };
			}
		}
		return null;
	}

	/** Where a song's row stands in the list now. */
	function rowTop(trackId: string): number | undefined {
		if (albums === null) {
			const at = ids.indexOf(trackId);
			return at < 0 ? undefined : at * flatRow;
		}
		return albums.rows.find(
			(each) =>
				each.kind === 'song' &&
				albums.albums.at(each.albumIndex)?.album.tracks.at(each.trackIndex)?.id === trackId
		)?.top;
	}

	/**
	 * Flat ⇄ By album, or a sort: the song at the top of the screen stays under the eye, as scroll is
	 * set at once to keep it there. What was showing fades where it stands, and the new order rises
	 * as one block. Nothing slides sideways, so rows never fly to new places.
	 */
	function relist(change: () => void): void {
		const kept = window.scrollY > 0 ? anchor() : null;
		const old = ghost(listElement);
		const oldHead = ghost(root?.querySelector('.head'));
		change();
		void tick().then(() => {
			if (kept !== null && listElement !== undefined) {
				const top = rowTop(kept.trackId);
				if (top !== undefined) {
					const at = listElement.getBoundingClientRect().top + scrollY + top;
					scrollTo({ top: Math.max(0, at - kept.y), behavior: 'instant' });
				}
			}
			track();
			void tick().then(() => {
				gone(old, move(old, [{ opacity: 1 }, { opacity: 0 }], 120, 0, 'linear'));
				gone(oldHead, move(oldHead, [{ opacity: 1 }, { opacity: 0 }], 120, 0, 'linear'));
				const fast = token('--morph-fast');
				for (const row of onScreenRows()) {
					move(
						row,
						[
							{ opacity: 0, translate: '0 12px' },
							{ opacity: 1, translate: '0 0' }
						],
						fast,
						40
					);
				}
				move(root?.querySelector('.head'), [{ opacity: 0 }, { opacity: 1 }], fast, 40);
			});
		});
	}

	/** Set while a move between Top songs and the full list plays, so the next one waits for none. */
	let shelvesBox: HTMLElement | undefined = $state();
	let was = untrack(() => expanded);
	let captured: {
		shelves: HTMLElement | null;
		rows: HTMLElement | null;
		tools: HTMLElement | null;
		lineY: number;
		keepTop: boolean;
		scrolled: boolean;
	} | null = null;

	// Before the DOM changes: what leaves is pictured where it stands.
	$effect.pre(() => {
		const now = expanded;
		if (now === was) {
			return;
		}
		untrack(() => {
			const keepTop = pref.group === 'flat' && pref.sort.by === 'top' && pref.sort.dir === 'desc';
			const lineY = lineElement?.getBoundingClientRect().top ?? 0;
			if (now) {
				// The rows the full list will show here, mounted with it in one render, not two.
				if (listElement !== undefined && pref.group === 'flat') {
					mounted = rowWindow(
						songs.length,
						flatRow,
						-listElement.getBoundingClientRect().top,
						windowHeight,
						{ first: 0, last: -1 }
					);
				}
				captured = {
					shelves: ghost(shelvesBox),
					rows: keepTop ? null : ghost(listElement),
					tools: null,
					lineY,
					keepTop,
					scrolled: false
				};
			} else {
				const listTop = listElement?.getBoundingClientRect().top ?? 0;
				const scrolled = listTop + top * flatRow < 200;
				// The songs Top shows again, which stay where they are: everything else gathers up.
				const staying = new Set(ids);
				captured = {
					shelves: null,
					rows: ghost(listElement, (copy) => {
						if (keepTop && !scrolled) {
							for (const row of copy.querySelectorAll<HTMLElement>(':scope > .row')) {
								if (staying.has(row.dataset['track'] ?? '')) {
									row.remove();
								}
							}
						}
					}),
					tools: ghost(lineElement?.querySelector('.list-tools')),
					lineY,
					keepTop,
					scrolled
				};
			}
		});
	});

	// After it: the new view moves in.
	$effect(() => {
		const now = expanded;
		if (now === was) {
			return;
		}
		was = now;
		untrack(() => {
			const shot = captured;
			captured = null;
			if (now) {
				arriveExpanded(shot);
			} else {
				arriveFolded(shot);
			}
		});
	});

	/**
	 * All N songs, going deeper: rows 1 to 10 don't move, and those past them rise from where the
	 * button was, as one block. The shelves fall back, and the list's controls rise into the line.
	 */
	function arriveExpanded(shot: typeof captured): void {
		const base = token('--morph');
		const fast = token('--morph-fast');
		if (shot?.shelves !== null && shot?.shelves !== undefined) {
			shot.shelves.style.transformOrigin = '50% 0';
			gone(
				shot.shelves,
				move(
					shot.shelves,
					[
						{ opacity: 1, scale: '1' },
						{ opacity: 0, scale: '0.9' }
					],
					196,
					0,
					exitCurve
				)
			);
		}
		const keepTop = shot?.keepTop ?? true;
		if (shot?.rows !== null && shot?.rows !== undefined) {
			gone(shot.rows, move(shot.rows, [{ opacity: 1 }, { opacity: 0 }], 120, 0, 'linear'));
		}
		void tick().then(() => {
			track();
			void tick().then(() => {
				for (const row of onScreenRows()) {
					const at = ids.indexOf(row.dataset['track'] ?? '');
					if (keepTop && at >= 0 && at < top) {
						continue;
					}
					move(
						row,
						[
							{ opacity: 0, translate: `0 ${keepTop ? '24px' : '12px'}` },
							{ opacity: 1, translate: '0 0' }
						],
						keepTop ? base : fast,
						keepTop ? 60 : 40
					);
				}
				move(
					lineElement?.querySelector('.list-tools'),
					[
						{ opacity: 0, translate: '0 8px' },
						{ opacity: 1, translate: '0 0' }
					],
					base,
					90
				);
				move(
					lineElement?.querySelector('h2'),
					[
						{ opacity: 0, translate: '0 8px' },
						{ opacity: 1, translate: '0 0' }
					],
					fast
				);
				if (!keepTop) {
					move(root?.querySelector('.head'), [{ opacity: 0 }, { opacity: 1 }], 120, 0, 'linear');
				}
			});
		});
	}

	/**
	 * Top 10, or Back: the heading stays where it is on screen, as scroll is set at once. The rows
	 * past the tenth gather up into the button; the button comes forward, and the shelves rise.
	 * It gathers where the expand unspooled: not the expand reversed.
	 */
	function arriveFolded(shot: typeof captured): void {
		const base = token('--morph');
		const fast = token('--morph-fast');
		void tick().then(() => {
			const settle = (): void => {
				if (shot !== null && lineElement !== undefined) {
					const want = lineElement.getBoundingClientRect().top + scrollY - Math.max(0, shot.lineY);
					scrollTo({ top: Math.max(0, want), behavior: 'instant' });
				}
				track();
			};
			settle();
			// A Back restores the scroll the history entry had, after this: the heading wins.
			requestAnimationFrame(settle);
			const button = root?.querySelector('.more-all');
			if (shot?.rows !== null && shot?.rows !== undefined) {
				const origin =
					button === null || button === undefined
						? 0
						: button.getBoundingClientRect().top - shot.rows.getBoundingClientRect().top;
				shot.rows.style.transformOrigin = `50% ${String(Math.max(0, origin))}px`;
				gone(
					shot.rows,
					move(
						shot.rows,
						[
							{ opacity: 1, scale: '1 1' },
							{ opacity: 0, scale: '1 0' }
						],
						196,
						0,
						exitCurve
					)
				);
			}
			if (shot?.tools !== null && shot?.tools !== undefined) {
				gone(shot.tools, move(shot.tools, [{ opacity: 1 }, { opacity: 0 }], 120, 0, 'linear'));
			}
			void tick().then(() => {
				if (!(shot?.keepTop ?? true) || (shot?.scrolled ?? false)) {
					for (const row of onScreenRows()) {
						move(row, [{ opacity: 0 }, { opacity: 1 }], 120, 40, 'linear');
					}
				}
				move(
					button,
					[
						{ opacity: 0, scale: '0.96' },
						{ opacity: 1, scale: '1' }
					],
					fast,
					60
				);
				move(
					shelvesBox,
					[
						{ opacity: 0, translate: '0 20px' },
						{ opacity: 1, translate: '0 0' }
					],
					base,
					90
				);
			});
		});
	}

	// Songs that land after the page fade in where they stand: the section already held their height.
	let landed = $state(untrack(() => load.kind === 'ready'));
	$effect(() => {
		if (load.kind !== 'ready' || landed) {
			return;
		}
		landed = true;
		void tick().then(() => {
			for (const row of onScreenRows()) {
				move(row, [{ opacity: 0 }, { opacity: 1 }], 120, 0, 'linear');
			}
		});
	});

	/** A phone's LINE pill shows the line on the list's first song. */
	const preview = $derived.by(() => {
		const first = shown.at(0);
		return first === undefined ? undefined : { ...first.album, tracks: [first.track] };
	});

	const word = $derived(heading === 'all' ? 'All' : heading === 'top' ? 'Top' : 'Songs');
</script>

<svelte:window
	bind:innerWidth={windowWidth}
	bind:innerHeight={windowHeight}
	onkeydown={onWindowKey}
/>

{#snippet headerBox()}
	<PickBox
		state={selection.boxFor(ids)}
		label={selection.boxFor(ids) === 'all' ? 'Clear every song shown' : 'Select every song shown'}
		ontoggle={() => {
			songList.toggleAll();
		}}
	/>
{/snippet}

<section
	class="songs song-list"
	class:expanded
	class:pinned
	class:grouped
	class:large
	class:stacked
	aria-labelledby="artist-songs"
	bind:this={root}
	use:songGrid={grid}
	style:--cover={`${String(cover)}px`}
	style:--cover-gap={`${String(coverGap)}px`}
	style:--pin-top={`${String(pinTop)}px`}
	style:--line-height={`${String(lineHeight)}px`}
	style:--list-line={`${String(pinTop + lineHeight + headHeight)}px`}
	style={columnVars(fitted.shown, columnsView.pref.widths, columnStep)}
	data-selecting={selection.any ? '' : undefined}
>
	<div class="line" bind:this={lineElement} bind:offsetHeight={lineHeight}>
		{#if heading === 'cold'}
			<span class="eyebrow">NOTHING PLAYED YET · NEWEST FIRST</span>
		{/if}
		<h2 id="artist-songs">
			<span class="word">{word}</span>{#if heading !== 'songs' && heading !== 'cold'}&#32;songs{/if}
			{#if expanded}<span class="count">{count.toLocaleString('en-US')}</span>{/if}
		</h2>
		{#if expanded}
			<div class="tools list-tools">
				<span class="group" role="group" aria-label="Group the songs">
					<button
						type="button"
						aria-pressed={!grouped}
						onclick={() => {
							regroup('flat');
						}}>FLAT</button
					>
					<button
						type="button"
						aria-pressed={grouped}
						onclick={() => {
							regroup('album');
						}}>BY ALBUM</button
					>
				</span>
				<SortControl
					groups={sortGroups}
					words={sortWords.toUpperCase()}
					way={sortWay}
					look="pill"
					label={grouped ? 'Sort the albums' : 'Sort the songs'}
				/>
				{#if phone}
					<LinePill
						album={preview}
						context={grouped ? 'artistAlbums' : 'artist'}
						{lineChars}
						artistWhenDifferent
						{now}
					/>
				{:else}
					<button
						type="button"
						class="pill"
						aria-haspopup="menu"
						onclick={(event) => {
							const box = event.currentTarget.getBoundingClientRect();
							openColumns(Math.min(box.left, innerWidth - 340), box.bottom + 8);
						}}><Icon name="columns" size={14} />COLUMNS</button
					>
				{/if}
				{#if !small}
					<button type="button" class="pill" onclick={fold}
						><Icon name="chevron-up" size={14} />TOP {top}</button
					>
				{/if}
			</div>
		{/if}
		<div class="tools play-tools">
			<button
				type="button"
				class="pill"
				onpointerenter={() => void shuffles.prime(shuffleSource)}
				onclick={shuffle}
				disabled={index === undefined}
				aria-label={selection.any ? 'Shuffle the songs picked' : `Shuffle every song ${name} is on`}
				><Icon name="shuffle" size={14} />SHUFFLE</button
			>
			<button
				type="button"
				class="play-all"
				aria-label={`Play ${name}'s songs`}
				disabled={shown.length === 0}
				onclick={() => {
					const first = shown.at(0);
					if (first !== undefined) {
						playFrom(first.track.id);
					}
				}}><Icon name="play" size={18} /></button
			>
		</div>
	</div>

	{#if !phone && (shown.length > 0 || load.kind !== 'ready')}
		<div class="head" bind:offsetHeight={headHeight}>
			<SongHeader
				shown={fitted.shown}
				hiddenCount={fitted.hiddenCount}
				{parts}
				openMenu={openColumns}
				box={headerBox}
				widths={columnsView.pref.widths}
				step={columnStep}
				onresize={(column, to) => {
					columnsView.resize(column, to);
				}}
				onreorder={(column: MovableId, to: number, keys: boolean) => {
					columnsView.reorder(reorderIn(columnsView.pref, context, fitted.shown, column, to), {
						debounce: keys
					});
				}}
				movable={(column: MovableId) => movable(column, columnsView.pref)}
			/>
		</div>
	{/if}

	{#if load.kind === 'unavailable'}
		<p class="note" role="status">
			These songs couldn't be loaded.
			<button type="button" onclick={() => void library.loadArtistSongs(id, fields)}
				>Try again</button
			>
		</p>
	{/if}

	<div
		class="list"
		bind:this={listElement}
		bind:clientWidth={listWidth}
		style:height={`${String(height)}px`}
		{...events}
	>
		{#each flatRows as { song, at } (song.track.id)}
			<SongRow
				track={song.track}
				album={song.album}
				{phone}
				{parts}
				shown={fitted.shown}
				{order}
				artistWhenDifferent
				pageArtist={name}
				albumOnLine
				{lineChars}
				rank={ranked ? at + 1 : undefined}
				top={at * flatRow}
				height={flatRow}
				{now}
				stars={library.marks.stars(song.track.id)}
				hearted={library.marks.hearted('tracks', song.track.id)}
				cued={song.track.id === cuedId}
				sounding={player.sounding}
				picked={selection.has(song.track.id)}
			/>
		{/each}
		{#if albums !== null}
			{#if albums.divider !== null}
				<div
					class="divider"
					style:top={`${String(albums.divider)}px`}
					style:height={`${String(dividerHeight)}px`}
				>
					<b>Appears on</b><span>THEIR SONGS ON OTHERS' ALBUMS</span>
				</div>
			{/if}
			{#each albumRows as { block, rows } (block.album.id)}
				<LibraryAlbum
					{block}
					top={block.top}
					{rows}
					head={metrics.albumHead}
					{stacked}
					{phone}
					{cover}
					{lineChars}
					shown={fitted.shown}
					pref={columnsView.pref}
					{now}
					{cuedId}
					sounding={player.sounding}
					trouble={trouble === block.album.id}
					playDisc={(disc) => {
						playDisc(block.album.id, disc);
					}}
					{selection}
					pageArtist={name}
				/>
			{/each}
		{/if}
	</div>

	{#if !expanded && !small && count > 0}
		<button type="button" class="more-all" onclick={expand}
			>All songs <span class="n">{count.toLocaleString('en-US')}</span></button
		>
	{/if}
</section>

{#if !expanded}
	<div class="shelves" bind:this={shelvesBox}>
		{@render shelves()}
	</div>
{/if}

{#if rowMenu !== null}
	<RowMenu
		x={rowMenu.x}
		y={rowMenu.y}
		target={rowMenu.target}
		actions={rowMenu.actions}
		onclose={closeMenus}
	/>
{/if}
{#if columnsMenu !== null}
	<ColumnsMenu
		x={columnsMenu.x}
		y={columnsMenu.y}
		hiding={columnsMenu.hiding}
		{context}
		onclose={closeMenus}
	/>
{/if}
{#if bulk !== null}
	<div class="bulk-layer" role="region" aria-label="Editing songs">
		<BulkEditor session={bulk} />
		<TagBar
			session={bulk}
			what="these songs"
			leave={() => {
				bulkEdits.close();
			}}
			write={() => void writeBulk()}
			match={matchFromBulk(bulk)}
		/>
	</div>
{:else if selection.any}
	{@const facts = pickedFacts(picked, songFacts)}
	<SelectionBar
		count={selection.size}
		hidden={selection.size - picked.length}
		albums={facts.albums}
		durationMs={facts.durationMs}
		hearted={picked.every((trackId) => library.marks.hearted('tracks', trackId))}
		actions={pickedActions}
		compact={phone}
		edit={editPicked}
		onclear={() => {
			selection.clear();
		}}
	/>
{/if}
<p class="visually-hidden" role="status">{bulkNote}</p>

<style>
	.songs {
		--song-row: 44px;
		--song-cover: 36px;

		position: relative;
		min-width: 0;
		display: flex;
		flex-direction: column;
	}

	.songs.large {
		--song-row: 56px;
		--song-cover: 46px;
	}

	/* The frame line: the heading, the list's controls while it shows whole, Shuffle and Play. */
	.line {
		z-index: 3;
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: calc(10px * var(--step)) calc(12px * var(--step));
		padding: calc(14px * var(--step)) 0;
		background: var(--ground);
	}

	/* Pinned under the nav in the full list, so its controls stay in reach all the way down. */
	.expanded .line {
		position: sticky;
		top: var(--pin-top);
	}

	/* Rows leave upward behind the nav, under a veil, as the Library's header has them. */
	.pinned .line::before {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: 100%;
		height: var(--pin-top);
		background: var(--ground);
	}

	h2 {
		margin: 0 auto 0 0;
		display: flex;
		align-items: baseline;
		gap: calc(14px * var(--step));
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1;
		white-space: nowrap;
	}

	/* The grungy touch, in the frame: the column stamp's skewed fox scuff, at rest under the word. */
	.word {
		position: relative;
		display: inline-block;
	}

	.word::after {
		content: '';
		position: absolute;
		left: -0.04em;
		right: 0.08em;
		bottom: -0.1em;
		height: calc(4px * var(--step));
		border-radius: 1px;
		background: var(--fox-line);
		opacity: 0.85;
		transform: skewX(-24deg) rotate(-1.2deg);
		/* Grain over a 70% ground, so the scuff prints unevenly, as ink on a stamp. */
		mask-image:
			var(--grain),
			linear-gradient(
				color-mix(in srgb, currentColor 70%, transparent),
				color-mix(in srgb, currentColor 70%, transparent)
			);
		pointer-events: none;
	}

	.count {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 500;
		letter-spacing: 0.04em;
		color: var(--text-meta);
	}

	.eyebrow {
		flex-basis: 100%;
		order: -1;
		margin-bottom: calc(-4px * var(--step));
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.tools {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: calc(8px * var(--step));
	}

	.pill,
	.tools :global(.sort.pill) {
		height: calc(36px * var(--step));
		padding: 0 calc(14px * var(--step));
		display: inline-flex;
		align-items: center;
		gap: calc(8px * var(--step));
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text);
		font-family: var(--font-mono);
		font-size: calc(12px * var(--step));
		font-weight: 600;
		letter-spacing: 0.06em;
		white-space: nowrap;
	}

	.pill:disabled {
		opacity: 0.5;
	}

	@media (hover: hover) {
		.pill:hover:not(:disabled) {
			border-color: var(--edge-hover);
			background: var(--hover);
		}
	}

	.group {
		display: inline-flex;
		gap: 2px;
		padding: calc(3px * var(--step));
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
	}

	.group button {
		height: calc(28px * var(--step));
		padding: 0 calc(11px * var(--step));
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		font-weight: 600;
		letter-spacing: 0.06em;
	}

	.group button[aria-pressed='true'] {
		background: var(--fill);
		color: var(--text);
	}

	.play-all {
		flex: none;
		width: calc(48px * var(--step));
		height: calc(48px * var(--step));
		display: grid;
		place-items: center;
		border: 0;
		border-radius: 50%;
		background: var(--fox);
		color: var(--fox-ink);
	}

	.play-all:disabled {
		opacity: 0.5;
	}

	/* The columns' labels, pinned under the line while the full list scrolls. */
	.head {
		z-index: 2;
		background: var(--ground);
	}

	.expanded .head {
		position: sticky;
		top: calc(var(--pin-top) + var(--line-height));
	}

	.grouped:not(.stacked) .head {
		padding-left: calc(var(--cover) + var(--cover-gap));
	}

	.list {
		position: relative;
	}

	.divider {
		position: absolute;
		left: 0;
		right: 0;
		box-sizing: border-box;
		display: flex;
		align-items: flex-end;
		gap: 12px;
		padding-bottom: calc(10px * var(--step));
		border-bottom: 1px solid var(--edge-raised);
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		font-weight: 600;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.divider b {
		font-family: var(--font-sans);
		font-size: calc(22px * var(--step));
		font-weight: 800;
		letter-spacing: -0.03em;
		color: var(--text);
	}

	/* "All 812 songs": the list's own handle. Pressed, it prints off register, as a lifted chip does. */
	.more-all {
		align-self: flex-start;
		margin-top: calc(14px * var(--step));
		height: calc(44px * var(--step));
		padding: 0 calc(18px * var(--step));
		display: inline-flex;
		align-items: center;
		gap: calc(10px * var(--step));
		border: 1px solid var(--edge-strong);
		border-radius: var(--radius-pill);
		background: var(--card);
		color: var(--text);
		font-size: calc(14px * var(--step));
		font-weight: 650;
		transition:
			box-shadow 120ms var(--curve-morph),
			translate 120ms var(--curve-morph);
	}

	.more-all .n {
		font-family: var(--font-mono);
		font-size: calc(12px * var(--step));
		font-weight: 600;
		letter-spacing: 0.04em;
		color: var(--fox-text);
	}

	@media (hover: hover) {
		.more-all:hover {
			box-shadow: 3px 3px 0 var(--lift-offset);
			translate: -1px -1px;
		}
	}

	.more-all:active {
		box-shadow: 1px 1px 0 var(--lift-offset);
		translate: 0 0;
	}

	.shelves {
		margin-top: calc(56px * var(--step));
		display: flex;
		flex-direction: column;
		gap: calc(56px * var(--step));
	}

	.note {
		min-height: var(--target);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
		color: var(--text-secondary);
	}

	.note button {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox-text);
	}

	.bulk-layer {
		position: fixed;
		inset: 0;
		z-index: 2;
		overflow-y: auto;
		padding: 64px 0 240px;
		background: var(--ground);
		animation: rise var(--morph) var(--curve-morph) both;
	}

	@keyframes rise {
		from {
			opacity: 0;
			translate: 0 16px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.bulk-layer {
			animation: none;
		}

		.more-all {
			transition: none;
		}
	}

	/* A phone: two lines a song, the heading with Shuffle and Play, the list's controls under them. */
	@media (max-width: 699px) {
		.songs {
			--song-row-two: 60px;
			--song-cover: 40px;
		}

		h2 {
			font-size: 30px;
		}

		.list-tools {
			order: 3;
			flex-basis: 100%;
		}

		.more-all {
			align-self: stretch;
			justify-content: center;
		}

		.shelves {
			margin-top: 36px;
			gap: 40px;
		}
	}
</style>
