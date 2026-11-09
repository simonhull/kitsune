<script lang="ts">
	import { onMount, tick, untrack } from 'svelte';
	import { afterNavigate } from '$app/navigation';
	import { resolve } from '$app/paths';
	import { emWidth } from '#lib/library/fit.ts';
	import { artistHoldings, clock, libraryLine } from '#lib/library/format.ts';
	import { emptyLine } from '#lib/library/home.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { ColumnId } from '#lib/library/source.ts';
	import { fit, monoAdvance, template } from '#lib/pages/columns.ts';
	import ColumnsMenu from '#lib/pages/ColumnsMenu.svelte';
	import { columnsView } from '#lib/pages/columnsView.svelte.ts';
	import LibraryAlbum from '#lib/pages/LibraryAlbum.svelte';
	import LibraryFilters from '#lib/pages/LibraryFilters.svelte';
	import LibraryRail from '#lib/pages/LibraryRail.svelte';
	import RowMenu from '#lib/pages/RowMenu.svelte';
	import BulkEditor from '#lib/pages/BulkEditor.svelte';
	import { bulkEdits } from '#lib/pages/bulkEdits.svelte.ts';
	import SelectionBar from '#lib/pages/SelectionBar.svelte';
	import { matchFromBulk } from '#lib/matching/hooks.ts';
	import TagBar from '#lib/pages/TagBar.svelte';
	import { selection } from '#lib/pages/selection.svelte.ts';
	import {
		actionsFor,
		albumsByTrack,
		type MenuTarget,
		type SongActions
	} from '#lib/pages/songActions.ts';
	import { letterOf, railLetters } from '#lib/pages/libraryRows.ts';
	import { hasLetters, railFor, railLit, sortFields } from '#lib/library/libraryOrder.ts';
	import { artistAlbumOrders, libraryArtistOrders, wayOf } from '#lib/library/sorts.ts';
	import SortControl, { type SortGroup } from '#lib/pages/SortControl.svelte';
	import { sorts } from '#lib/pages/sortsView.svelte.ts';
	import { libraryView as view } from '#lib/pages/libraryView.svelte.ts';
	import SongHeader from '#lib/pages/SongHeader.svelte';
	import { player } from '#lib/player/player.svelte.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import ScanCard from '#lib/pages/ScanCard.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import { shuffles } from '#lib/shuffle/session.svelte.ts';
	import { shuffleSettings } from '#lib/shuffle/settings.svelte.ts';
	import { librarySource } from '#lib/shuffle/sources.ts';
	import { settingsLine } from '#lib/shuffle/words.ts';
	import { cubicOut } from 'svelte/easing';
	import type { TransitionConfig } from 'svelte/transition';

	/** The headline's weight and tracking: the fit is worked out for exactly these. */
	const nameWeight = 900;
	const nameTracking = -0.06;

	const facts = $derived(library.facts);
	/** The clock a running scan's estimate reads, ticking only while one runs. */
	let scanNow = $state(Date.now());
	$effect(() => {
		if (facts?.scan.state !== 'running') {
			return;
		}
		const clock = setInterval(() => {
			scanNow = Date.now();
		}, 1000);
		return () => {
			clearInterval(clock);
		};
	});

	/** A finished scan's card folds away: its height closes as it fades, so the list rises to meet it. */
	function fold(node: HTMLElement): TransitionConfig {
		const height = node.offsetHeight;
		return {
			duration: matchMedia('(prefers-reduced-motion: reduce)').matches ? 0 : 420,
			easing: cubicOut,
			css: (t) => `height: ${String(t * height)}px; opacity: ${String(t)}; overflow: hidden;`
		};
	}
	const load = $derived(library.index);
	const layout = $derived(view.layout);
	const artist = $derived(layout.artists.at(view.here)?.artist);
	const next = $derived(layout.artists.at(view.here + 1)?.artist);
	const sort = $derived(sorts.library.value);
	/** Only a name order has letters to jump to: under any other the rail goes. */
	const lettered = $derived(hasLetters(sort));
	const letter = $derived(artist === undefined ? undefined : letterOf(artist.sortName));
	const letters = $derived(railFor(railLetters(layout), sort));
	const railAt = $derived(letter === undefined ? undefined : railLit(letters, letter));
	/** The Sort menu: the artists' order, and their albums'. */
	const sortGroups: SortGroup[] = $derived([
		{
			label: 'ARTISTS',
			options: libraryArtistOrders,
			current: sort.artists,
			choose: (next) => {
				view.toTop();
				sorts.library.set({ ...sort, artists: next as typeof sort.artists });
			}
		},
		{
			label: 'THEIR ALBUMS',
			options: artistAlbumOrders,
			current: sort.albums,
			choose: (next) => {
				view.toTop();
				sorts.library.set({ ...sort, albums: next as typeof sort.albums });
			}
		}
	]);
	/** What the button says: the artists' order, short enough for the frame line. */
	const sortWords = $derived(
		sort.artists.by === 'name'
			? wayOf('name', sort.artists.dir)
			: (libraryArtistOrders.find(({ by }) => by === sort.artists.by)?.label ?? '').toUpperCase()
	);
	const filtered = $derived(view.filter !== 'everything');
	/** Under a filter, the letters it leaves empty: grey, and a jump to one goes on to the next. */
	const quiet = $derived(
		new Set(filtered ? letters.filter((each) => !layout.letters.has(each)) : [])
	);
	const cuedId = $derived(library.playing?.track.id);
	/** What Shuffle shuffles here: the library, or what the filter shows. */
	const shuffleSource = $derived(librarySource(view.filter));
	/**
	 * Picks the first song ahead, so a press plays at once: when the list is ready, again for each
	 * filter, and again after every press.
	 */
	$effect(() => {
		const source = shuffleSource;
		if (load.kind === 'ready') {
			untrack(() => void shuffles.prime(source));
		}
	});
	function shuffleList(): void {
		shuffles.start(shuffleSource);
		void shuffles.prime(shuffleSource);
	}

	/** How wide a song row is, measured, so the columns that fit are known before a row mounts. */
	let songsWidth = $state(0);
	/** The columns' step: the phone has none, so it fits as the step above it would. */
	const columnStep = $derived(view.step === 'large' ? 'large' : 'columns');
	const fitted = $derived(fit(columnsView.pref.order, songsWidth, columnStep));
	const grid = $derived(template(fitted.shown, columnStep));
	/**
	 * Set for a moment when the width crosses a step, so what changes size moves to it. Nothing
	 * else animates a resize: between the steps everything follows the width as it goes.
	 */
	let stepping = $state(false);
	let lastStep = view.step;
	$effect(() => {
		const step = view.step;
		if (step === lastStep) {
			return;
		}
		lastStep = step;
		stepping = true;
		const done = setTimeout(() => {
			stepping = false;
		}, 450);
		return () => {
			clearTimeout(done);
		};
	});
	/** What the columns show, and what the order reads. */
	const fields = $derived([...new Set([...columnsView.fields, ...sortFields(sort)])]);
	/** When "Yesterday" is, for Last played: read once a visit. */
	const now = Date.now();

	/** Every song the list shows, in its order: what a Shift range and the header's box run over. */
	const order = $derived(
		layout.rows.flatMap((row) => {
			const id =
				row.kind === 'song'
					? layout.albums.at(row.albumIndex)?.album.tracks.at(row.trackIndex)?.id
					: undefined;
			return id === undefined ? [] : [id];
		})
	);
	/** Where each of them is in the list's rows, to scroll to. */
	const rowOf = $derived(
		new Map(
			layout.rows.flatMap((row, index) => {
				const id =
					row.kind === 'song'
						? layout.albums.at(row.albumIndex)?.album.tracks.at(row.trackIndex)?.id
						: undefined;
				return id === undefined ? [] : [[id, index] as const];
			})
		)
	);
	/** Each song of the whole index, filter or not, with its album. */
	const songs = $derived(
		new Map(
			(load.kind === 'ready' ? load.value.artists : []).flatMap((artist) =>
				artist.albums.flatMap((album) =>
					album.tracks.map((track) => [track.id, { track, album, artist }] as const)
				)
			)
		)
	);
	const albumOf = $derived(albumsByTrack(load.kind === 'ready' ? load.value : { artists: [] }));
	const picked = $derived(selection.inOrder(order));
	const pickedActions = $derived(actionsFor(picked, albumOf));
	const pickedFacts = $derived({
		albums: new Set(picked.map((id) => songs.get(id)?.album.id).filter((id) => id !== undefined))
			.size,
		durationMs: picked.reduce((sum, id) => sum + (songs.get(id)?.track.durationMs ?? 0), 0)
	});
	const box = $derived(selection.boxFor(order));
	/** Bulk editing a selection takes the page, over the list, which keeps its place beneath. */
	const bulk = $derived(bulkEdits.current?.layout === 'selection' ? bulkEdits.current : null);
	const canEditTags = $derived(library.user?.canEditTags === true);
	let bulkNote = $state('');

	function editPicked(): void {
		bulkNote = '';
		bulkEdits.open(
			'selection',
			picked.flatMap((id) => {
				const song = songs.get(id);
				return song === undefined
					? []
					: [{ id, albumId: song.album.id, title: song.track.title, trackNo: song.track.trackNo }];
			}),
			canEditTags
		);
	}

	function leaveBulk(words = ''): void {
		bulkEdits.close();
		bulkNote = words;
	}

	async function writeBulk(): Promise<void> {
		const written = await bulk?.write();
		if (written?.kind === 'done') {
			leaveBulk('Tags written.');
		} else if (written?.kind === 'problem') {
			await tick();
			document.getElementById(written.field)?.focus();
		}
	}

	/** The ⋯ menu, where it was opened, and on what. */
	let rowMenu = $state<{ x: number; y: number; target: MenuTarget; actions: SongActions } | null>(
		null
	);

	function pick(trackId: string, how: 'toggle' | 'range'): void {
		if (how === 'range') {
			selection.extend(trackId, order);
		} else {
			selection.toggle(trackId);
		}
	}

	/** Opens the ⋯ menu: on the picked songs, when it's opened on one of them. */
	function openRowMenu(trackId: string, x: number, y: number): void {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		if (selection.has(trackId) && selection.size > 1) {
			const facts = `${String(pickedFacts.albums)} ${pickedFacts.albums === 1 ? 'ALBUM' : 'ALBUMS'} · ${clock(pickedFacts.durationMs / 1000)}`;
			rowMenu = {
				x,
				y,
				target: { kind: 'picked', count: picked.length, line: facts, trackIds: picked },
				actions: pickedActions
			};
			return;
		}
		const song = songs.get(trackId);
		if (song === undefined) {
			return;
		}
		rowMenu = {
			x,
			y,
			target: {
				kind: 'song',
				trackId,
				albumId: song.album.id,
				title: song.track.title,
				line: `${song.album.title.toUpperCase()} · ${clock(song.track.durationMs / 1000)}`,
				artistId: song.artist.id
			},
			actions: actionsFor([trackId], albumOf)
		};
	}

	function closeRowMenu(refocus: boolean): void {
		rowMenu = null;
		if (refocus) {
			opener?.focus();
		}
	}

	/** Puts a song's row on screen, if it isn't, and gives it the focus. */
	async function focusRow(trackId: string): Promise<void> {
		const row = layout.rows.at(rowOf.get(trackId) ?? -1);
		if (row === undefined) {
			return;
		}
		const y = row.top + view.frame.top;
		const clear = view.frame.line;
		if (y < scrollY + clear || y + row.height > scrollY + innerHeight - 120) {
			scrollTo({ top: y - clear - row.height * 2, behavior: 'instant' });
			track();
		}
		await tick();
		document.querySelector<HTMLElement>(`[data-track="${CSS.escape(trackId)}"] .play`)?.focus();
	}

	/** Space picks the focused song; the arrows move, and with Shift take the songs passed. */
	function rowKey(event: KeyboardEvent, trackId: string): void {
		if (event.key === ' ' && !event.ctrlKey && !event.metaKey && !event.altKey) {
			event.preventDefault();
			selection.toggle(trackId);
			return;
		}
		if (event.key !== 'ArrowDown' && event.key !== 'ArrowUp') {
			return;
		}
		event.preventDefault();
		const at = order.indexOf(trackId);
		const next = order.at(at + (event.key === 'ArrowDown' ? 1 : -1));
		if (at < 0 || next === undefined || (at === 0 && event.key === 'ArrowUp')) {
			return;
		}
		if (event.shiftKey) {
			selection.add(trackId);
			selection.add(next);
		}
		void focusRow(next);
	}

	function onWindowKey(event: KeyboardEvent): void {
		if (
			event.key === 'Escape' &&
			selection.any &&
			menu === null &&
			rowMenu === null &&
			bulk === null
		) {
			selection.clear();
		}
	}

	/** The columns menu, where it was opened, and what opened it, to go back to. */
	let menu = $state<{ x: number; y: number; hiding: ColumnId | undefined } | null>(null);
	let opener: HTMLElement | null = null;

	function openMenu(x: number, y: number, hiding?: ColumnId): void {
		opener = document.activeElement instanceof HTMLElement ? document.activeElement : null;
		menu = { x, y, hiding };
	}

	function closeMenu(refocus: boolean): void {
		menu = null;
		if (refocus) {
			opener?.focus();
		}
	}

	// Before the first frame: marks pressed elsewhere since the filter last looked show on arrival.
	if (view.filter !== 'everything') {
		view.refilter();
	}

	/**
	 * Whether the page has landed. Before that its scroll isn't final, and reading it now would swap
	 * the song the user came back to for the top of the list.
	 */
	let landed = false;

	function track(): void {
		view.track(scrollY, innerHeight, landed);
	}

	function land(): void {
		scrollTo({ top: view.landing(), behavior: 'instant' });
		view.track(scrollY, innerHeight, false);
	}

	/** Puts a letter's first artist at the top, at once: a jump never animates a scroll. */
	function jump(to: string): void {
		scrollTo({ top: view.jump(to, letters), behavior: 'instant' });
		track();
	}

	function widest(name: string): number {
		return Math.max(...name.split(' ').map((word) => emWidth(word, nameWeight, nameTracking)), 0.1);
	}

	// A scan or an edit ending asks again, and so do columns that need more. Untracked: a failure
	// must wait for Try again. The columns come first, so the index is asked for once.
	$effect(() => {
		const wanted = fields;
		if (
			facts !== null &&
			columnsView.known &&
			sorts.library.known &&
			(library.indexStale || library.index.kind === 'loading' || !library.holdsFields(wanted))
		) {
			untrack(() => void library.loadIndex(fields));
		}
	});

	// A new layout has moved every pixel: a new width, or a new list, finds the same song again.
	$effect(() => {
		void view.layout;
		if (landed) {
			untrack(land);
		}
	});

	// Here and not at mount: Kit scrolls, for a link or from history, after the page has mounted.
	afterNavigate(() => {
		land();
		landed = true;
	});

	onMount(() => {
		let frame = 0;
		function onScroll(): void {
			frame ||= requestAnimationFrame(() => {
				frame = 0;
				track();
			});
		}
		addEventListener('scroll', onScroll, { passive: true });
		addEventListener('resize', onScroll, { passive: true });
		const stopWatching = view.watch();
		void columnsView.load();
		void sorts.library.load();
		return () => {
			cancelAnimationFrame(frame);
			removeEventListener('scroll', onScroll);
			removeEventListener('resize', onScroll);
			stopWatching();
		};
	});
</script>

<svelte:head>
	<title>Library · Kitsune</title>
</svelte:head>

<svelte:window onkeydown={onWindowKey} />

{#snippet headerBox()}
	<input
		type="checkbox"
		class="all"
		aria-label={box === 'all' ? 'Clear every song in view' : 'Select every song in view'}
		checked={box === 'all'}
		indeterminate={box === 'some'}
		onclick={(event) => {
			selection.toggleAll(order);
			event.currentTarget.checked = selection.boxFor(order) === 'all';
			event.currentTarget.indeterminate = selection.boxFor(order) === 'some';
		}}
	/>
{/snippet}

<div
	class="library"
	class:large={view.step === 'large'}
	class:stepping
	style:--cover={`${String(view.cover)}px`}
	style:--list-line={`${String(view.frame.line)}px`}
>
	<h1>Library</h1>

	{#if facts !== null && facts.albums === 0}
		<p class="note" role="status">{emptyLine(facts, library.deck)}</p>
		{#if facts.scan.state === 'running'}
			<div class="scanning alone"><ScanCard kind="scan" scan={facts.scan} now={scanNow} /></div>
		{/if}
	{:else if load.kind === 'unavailable'}
		<p class="note" role="status">
			The library couldn't be loaded.
			<button type="button" onclick={() => void library.loadIndex(fields)}>Try again</button>
		</p>
	{:else if load.kind === 'ready'}
		<div class="veil" aria-hidden="true"></div>
		<div class="top">
			<div class="shuffle" data-morph-part="intro" data-morph-key="shuffle">
				<button
					type="button"
					class="shuffle-button"
					onpointerenter={() => void shuffles.prime(shuffleSource)}
					onclick={shuffleList}
					aria-label={filtered ? 'Shuffle these' : 'Shuffle the library'}
					><Icon name="shuffle" size={18} /><span class="shuffle-words"
						>{filtered ? 'Shuffle these' : 'Shuffle the library'}</span
					></button
				>
				<span class="shuffle-line"
					>{#each settingsLine(shuffleSettings.current).split(' · ') as part, index (part)}{index >
						0
							? ' · '
							: ''}<span class="part">{part}</span>{/each}</span
				>
			</div>
			<div class="filters" data-morph-part="intro" data-morph-key="filters">
				<LibraryFilters
					filter={view.filter}
					choose={(filter) => {
						view.choose(filter);
					}}
				/>
			</div>
			{#if view.stacked}
				<div class="sort-round">
					<SortControl
						groups={sortGroups}
						words={sortWords}
						look="round"
						label="Sort the library"
					/>
				</div>
			{/if}
			{#if letter !== undefined}
				<div class="frame" data-morph-part="intro" data-morph-key="frame">
					{#if !view.stacked}
						<SortControl
							groups={sortGroups}
							words={sortWords}
							look="line"
							label="Sort the library"
						/>
					{/if}
					<p class="frame-words">
						{libraryLine(
							lettered ? letter : undefined,
							layout.artists.length,
							layout.songs,
							layout.totalSongs
						)}
					</p>
				</div>
			{:else if filtered}
				<p class="frame none" role="status">
					{view.filter === 'hearted' ? 'Nothing hearted yet.' : 'No song rated 4 or 5 yet.'}
				</p>
			{/if}
		</div>
		{#if facts !== null && facts.scan.state === 'running'}
			<div class="scanning dock" class:stacked={view.stacked} out:fold>
				<ScanCard kind="scan" scan={facts.scan} now={scanNow} />
			</div>
		{/if}
		{#if view.columns && !view.stacked}
			<div class="columns-head" data-morph-part="intro" data-morph-key="columns-head">
				<SongHeader
					shown={fitted.shown}
					hiddenCount={fitted.hiddenCount}
					{grid}
					{openMenu}
					box={headerBox}
				/>
			</div>
		{/if}
		{#if next !== undefined}
			<div class="next" class:ahead={view.nextAhead} data-morph-part="intro" data-morph-key="next">
				<span class="next-label">NEXT</span>
				<span class="next-name" title={next.name}>{next.name}</span>
			</div>
		{/if}

		<div class="list" style:height={`${String(layout.height)}px`}>
			<div class="measure" aria-hidden="true" bind:clientWidth={songsWidth}></div>
			{#each view.mounted as { index, block, albums } (index)}
				{@const { name, id } = block.artist}
				<section
					class="artist"
					aria-label={name}
					style:top={`${String(block.top)}px`}
					style:height={`${String(block.height)}px`}
				>
					<div class="head-column">
						<div
							class="head"
							class:with-header={view.stacked && view.columns}
							data-morph-part="intro"
							data-morph-key={`head:${name}`}
							data-morph-pinned
						>
							<!-- The fit divides by the words' width, known before layout: the first frame is final. -->
							<h2
								title={name}
								style:--all={emWidth(name, nameWeight, nameTracking)}
								style:--word={widest(name)}
							>
								{#if id === undefined}
									{name}
								{:else}
									<a href={resolve('/artists/[id]', { id })}>{name}</a>
								{/if}
							</h2>
							<div class="facts">
								{#if id !== undefined}
									<HeartButton kind="artists" {id} {name} size={18} />
								{/if}
								<p class="holdings">{artistHoldings(block.artist.albums, block)}</p>
							</div>
							{#if view.stacked && view.columns}
								<div class="stacked-header">
									<SongHeader
										shown={fitted.shown}
										hiddenCount={fitted.hiddenCount}
										{grid}
										{openMenu}
										box={headerBox}
									/>
								</div>
							{/if}
						</div>
					</div>
					<div class="albums">
						{#each albums as { block: album, rows } (album.album.id)}
							<LibraryAlbum
								block={album}
								top={album.top - block.top}
								{rows}
								head={view.frame.metrics.albumHead}
								stacked={view.stacked}
								phone={!view.columns}
								cover={view.cover}
								lineChars={Math.floor((songsWidth - 96 - (selection.any ? 26 : 0)) / monoAdvance)}
								shown={fitted.shown}
								{grid}
								pref={columnsView.pref}
								{now}
								{cuedId}
								sounding={player.sounding}
								trouble={view.trouble === album.album.id}
								play={(trackId) => {
									view.play(album.album.id, trackId);
								}}
								playDisc={(disc) => {
									view.playDisc(album.album.id, disc);
								}}
								{pick}
								openMenu={openRowMenu}
								{rowKey}
							/>
						{/each}
					</div>
				</section>
			{/each}
		</div>

		{#if lettered}
			<LibraryRail {letters} {quiet} here={railAt} onjump={jump} />
		{/if}
		{#if menu !== null}
			<ColumnsMenu x={menu.x} y={menu.y} hiding={menu.hiding} onclose={closeMenu} />
		{/if}
		{#if rowMenu !== null}
			<RowMenu
				x={rowMenu.x}
				y={rowMenu.y}
				target={rowMenu.target}
				actions={rowMenu.actions}
				onclose={closeRowMenu}
			/>
		{/if}
		{#if bulk !== null}
			<div class="bulk-layer" role="region" aria-label="Editing songs">
				<BulkEditor session={bulk} />
				<TagBar
					session={bulk}
					what="these songs"
					leave={() => {
						leaveBulk();
					}}
					write={() => void writeBulk()}
					match={matchFromBulk(bulk)}
				/>
			</div>
		{:else if selection.any}
			<SelectionBar
				count={selection.size}
				hidden={selection.size - picked.filter((id) => rowOf.has(id)).length}
				albums={pickedFacts.albums}
				durationMs={pickedFacts.durationMs}
				hearted={picked.every((id) => library.marks.hearted('tracks', id))}
				actions={pickedActions}
				compact={!view.columns}
				edit={editPicked}
				onclear={() => {
					selection.clear();
				}}
			/>
		{/if}
	{/if}
</div>

<p class="visually-hidden" role="status">{bulkNote}</p>

<style>
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
	}

	/*
	 * One layout from a phone to 4K and beyond, never a centred column. These are its sizes; from
	 * 2560 they take one step up together, so a 4K screen at 100% reads like 1920. --cover comes from
	 * the view, which needs it for the rows' maths.
	 */
	.library {
		--name-column: clamp(220px, 23.6vw, 340px);
		--page-edge: 64px;
		--name-gap: 44px;
		--cover-gap: 32px;
		--songs-right: 64px;
		--head-top: 96px;
		--head-row: 40px;
		--head-type: 11px;
		--col-gap: 12px;
		--row-pad: 14px;
		--cell-type: 14px;
		--star-type: 13px;
		--box-size: 16px;
		--more-size: 36px;
		--songs-left: calc(var(--page-edge) + var(--name-column) + var(--name-gap));
		/* Shuffle and its line, above the filters: libraryView's shuffleRow. */
		--shuffle-row: 112px;

		position: relative;
		padding: var(--head-top) 0 140px;
	}

	.library.large {
		--name-column: clamp(340px, 13.6vw, 520px);
		--page-edge: 96px;
		--name-gap: 64px;
		--cover-gap: 48px;
		--songs-right: 120px;
		--head-top: 128px;
		--head-row: 50px;
		--head-type: 13px;
		--col-gap: 16px;
		--row-pad: 18px;
		--cell-type: 17px;
		--star-type: 16px;
		--box-size: 20px;
		--more-size: 44px;
		--shuffle-row: 140px;
		--type-display: 132px;
		--type-title: 22px;
		--type-body: 18px;
		--type-mono: 15px;
		--type-section: 52px;
	}

	/* Crossing a step, what changes size moves to it; nothing else animates a resize. */
	.stepping :global(:is(.artist, .album, .side, .square, .songs, .row, .disc, .header, h2)) {
		transition:
			top var(--morph) var(--curve-morph),
			height var(--morph) var(--curve-morph),
			width var(--morph) var(--curve-morph),
			left var(--morph) var(--curve-morph),
			font-size var(--morph) var(--curve-morph),
			padding var(--morph) var(--curve-morph);
	}

	@media (prefers-reduced-motion: reduce) {
		.stepping :global(*) {
			transition: none;
		}
	}

	/* The boards have no page title: the pinned artist is the headline. Screen readers still get one. */
	h1 {
		position: absolute;
		width: 1px;
		height: 1px;
		overflow: hidden;
		clip-path: inset(50%);
		white-space: nowrap;
	}

	.note {
		min-height: var(--target);
		margin-left: var(--page-edge);
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 0 8px;
		max-width: 44ch;
		font-size: var(--type-title);
		line-height: 1.4;
		color: var(--text-secondary);
	}

	.note button {
		min-height: var(--target);
		padding: 0;
		border: 0;
		background: none;
		color: var(--fox-text);
	}

	.veil {
		display: none;
	}

	/* Solid, because the name it stands under leaves upward, past it. */
	.top {
		position: fixed;
		z-index: 2;
		top: var(--head-top);
		left: var(--page-edge);
		width: var(--name-column);
		padding-bottom: 16px;
		background: var(--ground);
	}

	.scanning {
		padding-top: 16px;
	}

	/* The Shuffle board's: the fox's own button, and the settings it shuffles by under it. */
	.shuffle {
		height: calc(var(--shuffle-row) - 20px);
		margin-bottom: 20px;
		display: flex;
		flex-direction: column;
		align-items: flex-start;
		gap: 10px;
	}

	.large .shuffle-button {
		height: 64px;
		padding: 0 28px;
		gap: 12px;
		font-size: 18px;
	}

	.shuffle-button {
		height: 52px;
		padding: 0 22px;
		display: flex;
		align-items: center;
		gap: 10px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fox);
		color: var(--fox-ink);
		font-size: 15px;
		font-weight: 700;
		white-space: nowrap;
	}

	.shuffle-line {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.06em;
		line-height: 1.5;
		color: var(--text-meta);
	}

	/* The line breaks between settings, never inside one. */
	.part {
		white-space: nowrap;
	}

	/* In the name column, under the artist and above what's next, so neither is covered. */
	.dock {
		position: fixed;
		z-index: 2;
		left: var(--page-edge);
		bottom: 320px;
		width: var(--name-column);
		padding-top: 0;
	}

	/* No name column on a phone: just above the player island, the card's note left out. */
	.dock.stacked {
		left: 16px;
		right: 16px;
		border-radius: var(--radius-20);
		box-shadow: var(--glass-shadow);
		bottom: 104px;
		width: auto;
		--scan-pad: 14px 18px;
	}

	.dock.stacked :global(.note) {
		display: none;
	}

	.scanning.alone {
		max-width: 560px;
	}

	/*
	 * The Sort button stands first, so a narrow column cuts the counts, never the button. The line
	 * is as tall as the words were with their margin, so the name pins where it always did.
	 */
	.frame {
		margin-top: 2px;
		height: 28px;
		display: flex;
		align-items: center;
		gap: 10px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.frame :global(.sort) {
		flex: none;
		margin-left: -10px;
	}

	.frame-words {
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.sort-round {
		order: 3;
	}

	.next {
		position: fixed;
		bottom: 200px;
		left: var(--page-edge);
		width: var(--name-column);
		padding-top: 16px;
		border-top: 1px solid var(--edge);
		display: flex;
		flex-direction: column;
		gap: 6px;
		opacity: 0;
		visibility: hidden;
		transition:
			opacity var(--morph-fast) var(--curve-morph),
			visibility 0s var(--morph-fast);
	}

	.next.ahead {
		opacity: 1;
		visibility: visible;
		transition: opacity var(--morph-fast) var(--curve-morph);
	}

	.next-label {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.next-name {
		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 2;
		line-clamp: 2;
		overflow: hidden;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
		line-height: 1.1;
		overflow-wrap: anywhere;
		color: var(--text-label);
	}

	/* Over the songs, under the nav. Rows leave upward behind it, and behind the veil above it. */
	.columns-head {
		position: sticky;
		z-index: 2;
		top: var(--head-top);
		height: var(--head-row);
		margin: 0 var(--songs-right) calc(var(--list-line) - var(--head-top) - var(--head-row))
			var(--songs-left);
		padding-left: calc(var(--cover) + var(--cover-gap));
		background: var(--ground);
	}

	.columns-head::before {
		content: '';
		position: absolute;
		left: 0;
		right: 0;
		bottom: 100%;
		height: var(--head-top);
		background: var(--ground);
	}

	.list {
		position: relative;
	}

	.all {
		width: var(--box-size, 16px);
		height: var(--box-size, 16px);
		margin: 0;
		accent-color: var(--fox);
		opacity: 0.5;
	}

	.all:checked,
	.all:indeterminate,
	.all:hover,
	.all:focus-visible {
		opacity: 1;
	}

	/* As wide as a song row: what the columns are fitted to. */
	.measure {
		position: absolute;
		top: 0;
		left: calc(var(--songs-left) + var(--cover) + var(--cover-gap));
		right: var(--songs-right);
		height: 0;
		visibility: hidden;
	}

	.artist {
		position: absolute;
		left: 0;
		right: 0;
	}

	.head-column {
		position: absolute;
		z-index: 1;
		top: 0;
		bottom: 0;
		left: var(--page-edge);
		width: var(--name-column);
		pointer-events: none;
	}

	.none {
		font-family: var(--font-sans);
		font-size: var(--type-title);
		letter-spacing: 0;
		color: var(--text-secondary);
	}

	/* The name stays while its albums scroll; the next one pushes it out. */
	.head {
		position: sticky;
		top: calc(var(--head-top) + var(--shuffle-row) + 90px);
		pointer-events: auto;
	}

	/* Three lines at the most, and no word cut: the board's size for any name that fits it. */
	h2 {
		--fit: min(calc(var(--name-column) / var(--word)), calc(var(--name-column) * 2.5 / var(--all)));

		display: -webkit-box;
		-webkit-box-orient: vertical;
		-webkit-line-clamp: 3;
		line-clamp: 3;
		overflow: hidden;
		padding-bottom: 0.09em;
		margin-bottom: -0.09em;
		font-size: clamp(24px, var(--fit), var(--type-display));
		font-weight: 900;
		letter-spacing: -0.06em;
		line-height: 0.88;
		overflow-wrap: anywhere;
	}

	h2 a:hover {
		color: var(--fox-text);
	}

	/* Amber: the artist is the frame's, not a cover's. */
	.facts {
		--heart-on: var(--star);
		--heart-off: var(--text-secondary);

		margin-top: 14px;
		display: flex;
		align-items: center;
		gap: 8px;
	}

	.facts :global(.heart) {
		margin-left: -12px;
	}

	.holdings {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-secondary);
	}

	.albums {
		position: absolute;
		top: 0;
		bottom: 0;
		left: var(--songs-left);
		right: var(--songs-right);
	}

	@media (max-height: 720px) {
		.next {
			display: none;
		}
	}

	/* The columns stack. The list starts under the nav, and a veil keeps songs from passing behind it. */
	@media (max-width: 1024px) {
		.library {
			padding: 76px 0 128px;
		}

		/* The filters stand under the nav, and the veil goes down past them. */
		.veil {
			display: block;
			position: fixed;
			z-index: 2;
			inset: 0 0 auto;
			height: 128px;
			background: var(--ground);
		}

		.top {
			top: 78px;
			left: 16px;
			width: auto;
			padding: 0;
			display: flex;
			align-items: center;
			gap: 6px;
			background: none;
		}

		/* Stacked, Shuffle stands beside the filters, without its line. */
		.shuffle {
			order: 2;
			height: auto;
			margin: 0;
		}

		.shuffle-button {
			height: 46px;
			padding: 0 18px;
		}

		.shuffle-line {
			display: none;
		}

		.frame,
		.next {
			display: none;
		}

		.none {
			display: block;
			position: fixed;
			top: 140px;
		}

		.head-column {
			left: 0;
			right: 0;
			width: auto;
		}

		.head {
			top: 128px;
			height: 80px;
			padding: 8px 48px 12px 16px;
			background: var(--ground);
			border-bottom: 1px solid var(--edge);
		}

		/* The columns' header stands under the name, and pins with it. */
		.head.with-header {
			height: 120px;
			padding-bottom: 0;
			border-bottom: 0;
		}

		.stacked-header {
			margin-top: 6px;
		}

		.measure {
			left: 16px;
			right: 48px;
		}

		h2 {
			display: block;
			padding: 0;
			margin: 0;
			overflow: hidden;
			text-overflow: ellipsis;
			white-space: nowrap;
			font-size: clamp(22px, calc((100vw - 64px) / var(--all)), var(--type-section));
			letter-spacing: -0.05em;
			line-height: 0.95;
		}

		/* The head is 80 tall: the heart keeps its target, and lets it overhang the line. */
		.facts {
			height: 22px;
			margin-top: 2px;
		}

		.albums {
			left: 16px;
			right: 48px;
		}

		.note {
			margin-left: 16px;
		}
	}

	/* A phone: Shuffle is the fox's round button, its words for the screen reader. */
	@media (max-width: 600px) {
		.shuffle-button {
			width: 44px;
			height: 44px;
			padding: 0;
			justify-content: center;
		}

		.shuffle-words {
			display: none;
		}
	}
</style>
