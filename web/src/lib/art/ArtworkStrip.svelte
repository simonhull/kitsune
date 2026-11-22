<!--
	The Artwork strip, after an album's songs: its front, back and each disc, with where each came
	from. A disc keeps its alpha, on the check a picture editor shows. An album with nothing beside
	its cover has no strip.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import type { Album } from '#lib/library/source.ts';
	import { artworkStrip, stripCount } from './art.ts';
	import Picture from './Picture.svelte';

	interface Props {
		album: Album;
	}

	let { album }: Props = $props();

	const pictures = $derived(artworkStrip(album));
</script>

{#if pictures.length > 0}
	<section class="strip" aria-labelledby="artwork" data-morph-part="panel" data-morph-key="artwork">
		<div class="heading">
			<h2 id="artwork">Artwork</h2>
			<span class="count">{stripCount(pictures)}</span>
		</div>
		<ul>
			{#each pictures as picture (picture.key)}
				<li>
					<span class="tile" class:disc={picture.key.startsWith('disc')}>
						{#if picture.image === undefined}
							<Cover albumId={album.id} entry={album} place="card" lazy />
						{:else}
							<Picture
								image={picture.image}
								kind={picture.key === 'back' ? 'back' : 'disc'}
								box={230}
								fit="contain"
								lazy
							/>
						{/if}
					</span>
					<span class="label">{picture.label}</span>
					<span class="from" class:found={picture.from !== 'IN THE FILES'}>{picture.from}</span>
				</li>
			{/each}
		</ul>
	</section>
{/if}

<style>
	.strip {
		padding: calc(56px * var(--step)) max(64px, var(--gutter)) 0;
		display: flex;
		flex-direction: column;
		gap: calc(24px * var(--step));
	}

	.heading {
		display: flex;
		align-items: baseline;
		gap: calc(24px * var(--step));
	}

	h2 {
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.count,
	.from {
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	ul {
		margin: 0;
		padding: 0;
		list-style: none;
		display: grid;
		grid-template-columns: repeat(
			auto-fill,
			minmax(calc(200px * var(--step)), calc(230px * var(--step)))
		);
		gap: calc(24px * var(--step));
	}

	li {
		display: flex;
		flex-direction: column;
		gap: 4px;
	}

	.tile {
		position: relative;
		display: block;
		aspect-ratio: 1;
		margin-bottom: 12px;
		border-radius: var(--radius-20);
		overflow: hidden;
		box-shadow: inset 0 0 0 1px var(--cover-edge);
	}

	/* A disc is drawn with its alpha, on a check. */
	.tile.disc {
		background: repeating-conic-gradient(var(--hover) 0 25%, transparent 0 50%) 0 0 / 16px 16px;
	}

	.label {
		font-size: calc(15px * var(--step));
		font-weight: 800;
	}

	.from.found {
		color: var(--fox-text);
	}

	@media (max-width: 820px) {
		.strip {
			padding: 40px 16px 0;
		}

		ul {
			grid-template-columns: repeat(2, minmax(0, 1fr));
			gap: 16px;
		}
	}
</style>
