<script lang="ts">
	import type { Snippet } from 'svelte';
	import Cover from '#lib/cover/Cover.svelte';
	import type { ShuffleMode } from '#lib/library/source.ts';
	import type { UpNextItem } from './session.svelte.ts';

	interface Props {
		items: readonly UpNextItem[];
		mode: ShuffleMode;
		/** How many songs the shuffle picks from, once the server has said. */
		songs: number | null;
		/** What a row says of why it came up, if anything does. */
		why?: Snippet<[UpNextItem]>;
	}

	let { items, mode, songs, why }: Props = $props();

	const facts = $derived(
		[
			mode === 'endless' ? 'ENDLESS' : 'EACH ONCE',
			songs === null ? null : `${songs.toLocaleString('en-US')} ${songs === 1 ? 'SONG' : 'SONGS'}`
		]
			.filter((each) => each !== null)
			.join(' · ')
	);
</script>

<section class="up-next" aria-label="Up next">
	<div class="head">
		<h2>Up next</h2>
		<span class="facts">{facts}</span>
	</div>
	{#if items.length === 0}
		<p class="empty">Picking what comes next…</p>
	{:else}
		<ol>
			{#each items as item (item.key)}
				<li class:has-why={why !== undefined}>
					<span class="cover"
						><Cover albumId={item.album.id} entry={item.album} place="small" /></span
					>
					<span class="names">
						<span class="title">{item.titles.join(' + ')}</span>
						<span class="artist">{item.artist}</span>
					</span>
					{#if why !== undefined}{@render why(item)}{/if}
				</li>
			{/each}
		</ol>
	{/if}
</section>

<style>
	.up-next {
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.head {
		display: flex;
		align-items: baseline;
		justify-content: space-between;
		gap: 16px;
		padding: 0 14px;
	}

	h2 {
		font-size: 40px;
		font-weight: 800;
		letter-spacing: -0.04em;
	}

	.facts,
	.empty {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.06em;
		color: var(--text-meta);
		white-space: nowrap;
	}

	.empty {
		padding: 20px 14px;
	}

	ol {
		margin: 20px 0 0;
		padding: 0;
		list-style: none;
		display: flex;
		flex-direction: column;
		gap: 6px;
	}

	li {
		position: relative;
		min-height: 64px;
		padding: 10px 14px;
		display: flex;
		align-items: center;
		gap: 14px;
		border-radius: 16px;
	}

	.cover {
		position: relative;
		flex: none;
		width: 44px;
		height: 44px;
		border-radius: 10px;
		overflow: hidden;
	}

	.names {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 2px;
	}

	.title,
	.artist {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.title {
		font-size: 15px;
		font-weight: 600;
	}

	.artist {
		font-size: 13px;
		color: var(--text-meta);
	}

	/* A phone: a row is a line of the list, and what it says goes under the title. */
	@media (max-width: 720px) {
		h2 {
			font-size: 28px;
		}

		.head {
			padding: 0 8px;
		}

		ol {
			margin-top: 12px;
			gap: 0;
		}

		li {
			padding: 8px;
			gap: 12px;
			border-radius: 0;
			border-bottom: 1px solid var(--edge);
		}

		/* What a row says of itself takes the artist's line, with the artist in it, and a ⋯ at the end. */
		li.has-why {
			display: grid;
			grid-template-columns: 44px minmax(0, 1fr) auto;
			grid-template-rows: auto auto;
			column-gap: 12px;
			row-gap: 3px;
		}

		.has-why .cover {
			grid-row: 1 / span 2;
		}

		.has-why .names {
			grid-column: 2;
			grid-row: 1;
			align-self: end;
		}

		.has-why .artist {
			display: none;
		}
	}

	/* A 4K screen: the list steps up with the page, so it reads from across a room. */
	@media (min-width: 2560px) {
		h2 {
			font-size: 56px;
		}

		li {
			min-height: 88px;
			gap: 20px;
		}

		.cover {
			width: 64px;
			height: 64px;
			border-radius: 14px;
		}

		.title {
			font-size: 21px;
		}

		.artist {
			font-size: 18px;
		}
	}
</style>
