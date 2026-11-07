<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { albumStars } from '#lib/library/albumStars.ts';
	import { albumHoldings, clock, credit, trackLabel } from '#lib/library/format.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import { morph } from '#lib/morph/registry.ts';
	import HeartButton from '#lib/shell/HeartButton.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import StarsShown from '#lib/shell/StarsShown.svelte';
	import type { AlbumBlock, Row } from './libraryRows.ts';

	interface Props {
		block: AlbumBlock;
		/** Where it starts in its artist. */
		top: number;
		/** The rows of it that are mounted. */
		rows: readonly Row[];
		/** How far down the page's rows start, past what stands above them. */
		head: number;
		/** The cover here is a small one, above the songs. */
		stacked: boolean;
		/** The song cued, if any. */
		cuedId: string | undefined;
		/** Whether that song is sounding now. */
		sounding: boolean;
		/** A tap on this album couldn't be played. */
		trouble: boolean;
		play: (trackId: string) => void;
	}

	let { block, top, rows, head, stacked, cuedId, sounding, trouble, play }: Props = $props();

	const album = $derived(block.album);
	const hearted = $derived(library.marks.hearted('albums', album.id));
	const stars = $derived(albumStars(album.tracks, library.marks));
</script>

<section
	class="album"
	aria-label={album.title}
	style:top={`${String(top)}px`}
	style:height={`${String(block.height)}px`}
>
	<div class="side">
		<div
			class="pin"
			data-morph-part="card"
			data-morph-key={`cover:${album.id}`}
			data-morph-pinned={stacked ? undefined : ''}
		>
			<a class="art" href={resolve('/album/[id]', { id: album.id })} aria-label={album.title}>
				<span class="square" use:morph={{ id: `cover:${album.id}`, slot: 'card', kind: 'cover' }}>
					<Cover albumId={album.id} entry={album} place={stacked ? 'small' : 'card'} lazy />
				</span>
			</a>
			<div class="names">
				<div class="title-line">
					<a
						class="title"
						href={resolve('/album/[id]', { id: album.id })}
						title={album.title}
						use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{album.title}</a
					>
					{#if hearted}
						<span class="hearted" role="img" aria-label="Hearted"
							><Icon name="heart" size={16} /></span
						>
					{/if}
				</div>
				{#if trouble}
					<p class="facts trouble" role="status">Couldn't load this album</p>
				{:else}
					<p class="facts">
						{albumHoldings(album.year, album.tracks.length)} ·
						<span class="stars"><StarsShown {stars} /></span>
					</p>
				{/if}
			</div>
		</div>
	</div>
	<div
		class="songs"
		data-morph-part="card"
		data-morph-key={`songs:${album.id}`}
		style:top={`${String(head)}px`}
	>
		{#each rows as row (row.kind === 'song' ? row.trackIndex : `disc-${String(row.disc)}`)}
			{@const at = `${String(row.top - block.top - head)}px`}
			{#if row.kind === 'disc'}
				<p class="disc" style:top={at} style:height={`${String(row.height)}px`}>
					DISC {row.disc}
				</p>
			{:else}
				{@const track = album.tracks.at(row.trackIndex)}
				{#if track !== undefined}
					{@const cued = track.id === cuedId}
					{@const others = credit(track, album.artist)}
					<div class="row" class:cued style:top={at} style:height={`${String(row.height)}px`}>
						<button
							type="button"
							class="play"
							aria-label={`Play ${track.title}`}
							aria-current={cued ? 'true' : undefined}
							onclick={() => {
								play(track.id);
							}}
						>
							<span class="mono number">
								{#if cued && sounding && !stacked}
									<Icon name="play" size={12} />
								{:else}
									{trackLabel(track, false)}
								{/if}
							</span>
							<span class="name">{track.title}</span>
							{#if others !== undefined}
								<span class="others">{others}</span>
							{/if}
						</button>
						<HeartButton kind="tracks" id={track.id} name={track.title} />
						<span class="mono length">{clock(track.durationMs / 1000)}</span>
					</div>
				{/if}
			{/if}
		{/each}
	</div>
</section>

<style>
	.album {
		position: absolute;
		left: 0;
		right: 0;
	}

	.side {
		position: absolute;
		top: 0;
		bottom: 0;
		left: 0;
		width: 208px;
	}

	/* Stuck under the nav while the songs beside it scroll, and gone with the last of them. */
	.pin {
		position: sticky;
		top: 96px;
	}

	.art {
		display: block;
	}

	.square {
		position: relative;
		display: block;
		width: 208px;
		height: 208px;
		border-radius: var(--radius-20);
		overflow: hidden;
	}

	.names {
		margin-top: 12px;
		min-width: 0;
	}

	.title-line {
		display: flex;
		align-items: center;
		justify-content: space-between;
		gap: 8px;
	}

	.hearted {
		flex: none;
		display: flex;
		color: var(--text);
	}

	/* As wide as the words: a flight measures the element, and an empty right side would stretch. */
	.title {
		display: block;
		width: fit-content;
		max-width: 100%;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: var(--type-title);
		font-weight: 700;
		letter-spacing: var(--tracking-title);
	}

	.title:hover {
		color: var(--fox);
	}

	.facts {
		margin-top: 2px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
	}

	.stars {
		color: var(--text-secondary);
	}

	.trouble {
		color: var(--fox);
	}

	.songs {
		position: absolute;
		bottom: 0;
		left: 240px;
		right: 0;
	}

	.disc {
		position: absolute;
		left: 0;
		right: 0;
		display: flex;
		align-items: flex-end;
		padding: 0 14px 6px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	/* The song is one button, and its heart another beside it: a button can't hold a button. */
	.row {
		--heart-on: var(--text);
		--heart-off: var(--text-secondary);

		position: absolute;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		padding: 0 4px 0 14px;
		border-bottom: 1px solid var(--edge-card);
		font-size: var(--type-body);
		font-weight: 600;
	}

	.play {
		flex: 1;
		min-width: 0;
		height: 100%;
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 0;
		border: 0;
		background: transparent;
		color: inherit;
		font: inherit;
		text-align: left;
	}

	/* Its own column before the time: there when hearted, on hover and on the cued row. */
	.row :global(.heart) {
		opacity: 0;
	}

	.row :global(.heart.on),
	.row :global(.heart:focus-visible),
	.row.cued :global(.heart) {
		opacity: 1;
	}

	@media (hover: hover) {
		.row:hover {
			border-radius: var(--radius-12);
			background: rgb(255 255 255 / 8%);
		}

		.row:hover :global(.heart) {
			opacity: 1;
		}
	}

	.mono {
		flex: none;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		font-variant-numeric: tabular-nums;
		color: var(--text-meta);
	}

	.number {
		width: 24px;
		display: flex;
		align-items: center;
	}

	.name {
		flex: 0 1 auto;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.others {
		flex: 0 1 40%;
		min-width: 0;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 14px;
		font-weight: 400;
		color: var(--text-meta);
	}

	.length {
		width: 40px;
		text-align: right;
	}

	.row.cued,
	.row.cued:hover {
		--heart-on: var(--fox-ink);
		--heart-off: var(--fox-ink);

		border-radius: var(--radius-12);
		background: var(--fox);
		color: var(--fox-ink);
	}

	.cued .mono,
	.cued .others {
		color: var(--fox-ink);
	}

	@media (max-width: 1024px) {
		.side {
			bottom: auto;
			width: auto;
			right: 0;
			height: 72px;
		}

		.pin {
			position: static;
			height: 100%;
			display: flex;
			align-items: center;
			gap: 12px;
		}

		.square {
			width: 64px;
			height: 64px;
			flex: none;
			border-radius: 14px;
		}

		.names {
			margin-top: 0;
		}

		.songs {
			left: 0;
		}

		.row {
			padding: 0 4px 0 10px;
		}

		.play {
			gap: 12px;
		}

		.number {
			width: 22px;
		}
	}
</style>
