<script lang="ts">
	import { resolve } from '$app/paths';
	import Cover from '#lib/cover/Cover.svelte';
	import { albumHoldings, clock, credit, trackLabel } from '#lib/library/format.ts';
	import { morph } from '#lib/morph/registry.ts';
	import Icon from '#lib/shell/Icon.svelte';
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
				<a
					class="title"
					href={resolve('/album/[id]', { id: album.id })}
					title={album.title}
					use:morph={{ id: `title:${album.id}`, slot: 'card', kind: 'title' }}>{album.title}</a
				>
				{#if trouble}
					<p class="facts trouble" role="status">Couldn't load this album</p>
				{:else}
					<p class="facts">{albumHoldings(album.year, album.tracks.length)}</p>
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
					<button
						type="button"
						class="row"
						class:cued
						style:top={at}
						style:height={`${String(row.height)}px`}
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
						<span class="mono length">{clock(track.durationMs / 1000)}</span>
					</button>
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

	.row {
		position: absolute;
		left: 0;
		right: 0;
		display: flex;
		align-items: center;
		gap: 16px;
		padding: 0 14px;
		border: 0;
		border-bottom: 1px solid var(--edge-card);
		background: transparent;
		font-size: var(--type-body);
		font-weight: 600;
		text-align: left;
	}

	@media (hover: hover) {
		.row:hover {
			border-radius: var(--radius-12);
			background: rgb(255 255 255 / 8%);
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
		margin-left: auto;
	}

	.row.cued,
	.row.cued:hover {
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
			gap: 12px;
			padding: 0 10px;
		}

		.number {
			width: 22px;
		}
	}
</style>
