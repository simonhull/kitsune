<script lang="ts">
	import { onMount } from 'svelte';
	import { albumPageSize } from '#lib/library/library.svelte.ts';
	import { whenSettled } from '#lib/morph/settled.ts';
	import AlbumCard from './AlbumCard.svelte';
	import type { GridWindow } from './gridWindow.svelte.ts';

	interface Props {
		grid: GridWindow;
	}

	let { grid }: Props = $props();

	let root: HTMLElement | undefined = $state();
	/**
	 * Whether the page has landed. Before that its scroll isn't final, and reading it now would
	 * swap the page the user came back to for the top of the grid.
	 */
	let landed = $state(false);

	const pages = $derived(Array.from({ length: grid.pageCount }, (_, index) => index));

	function sliceOf(page: number) {
		return grid.entries.slice(page * albumPageSize, (page + 1) * albumPageSize);
	}

	/** Reads the layout from a mounted page. Every card has one height, so one page tells all. */
	function measure(page: HTMLElement): { destroy: () => void } {
		function read(): void {
			const card = page.firstElementChild;
			if (card === null) {
				return;
			}
			const style = getComputedStyle(page);
			grid.measure = {
				columns: style.gridTemplateColumns.split(' ').length,
				rowHeight: card.getBoundingClientRect().height,
				rowGap: Number.parseFloat(style.rowGap)
			};
		}
		const observer = new ResizeObserver(read);
		observer.observe(page);
		return {
			destroy() {
				observer.disconnect();
			}
		};
	}

	function track(): void {
		if (root !== undefined) {
			const { top, bottom } = root.getBoundingClientRect();
			grid.track(top, bottom);
		}
	}

	$effect(() => {
		// A page arriving moves the end of the grid: it may need the next one at once.
		void grid.entries.length;
		void grid.measure;
		if (landed) {
			track();
		}
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

		// The page the user came back to comes first; its neighbors wait for the landing.
		grid.wide = false;
		const cancel = whenSettled(() => {
			frame ||= requestAnimationFrame(() => {
				frame = 0;
				grid.wide = true;
				landed = true;
			});
		});

		return () => {
			cancel();
			cancelAnimationFrame(frame);
			removeEventListener('scroll', onScroll);
			removeEventListener('resize', onScroll);
		};
	});
</script>

<div class="pages" bind:this={root}>
	{#each pages as page (page)}
		{#if grid.mounted(page)}
			<div class="grid" use:measure>
				{#each sliceOf(page) as album (album.id)}
					<AlbumCard {album} />
				{/each}
			</div>
		{:else}
			<div class="box" style:height={`${String(grid.height(page) ?? 0)}px`}></div>
		{/if}
	{/each}
</div>

<style>
	.pages {
		display: flex;
		flex-direction: column;
		gap: 24px;
	}

	.grid {
		display: grid;
		grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
		gap: 24px;
	}

	@media (max-width: 720px) {
		.pages {
			gap: 20px;
		}

		.grid {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 20px 16px;
		}
	}
</style>
