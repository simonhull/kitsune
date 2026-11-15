<!--
	What the rules choose, as they're typed: the count in the fox with its teal misprint, how long
	they run and on how many albums (and what the limit cut, and how it stands against the saved
	rules), then the first songs, each a 46px row, a song the edit brings in marked NEW. It keeps the
	last answer until the next comes, so it never blanks.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import { library } from '#lib/library/library.svelte.ts';
	import type { PlaylistAlbum, SmartPreview } from '#lib/library/source.ts';
	import { starsText } from '#lib/library/smartlist.ts';
	import { liveFoot, liveLine, liveRowLine, matchWord } from './playlistWords.ts';

	interface Props {
		answer: SmartPreview | null;
		/** How many the saved rules choose; undefined for a new draft or while nothing changed. */
		saved: number | undefined;
		/** The songs the saved rules choose, which a NEW mark is against; null marks none. */
		savedIds: ReadonlySet<string> | null;
		/** How many rows it shows. */
		rows: number;
		/** Under the rules on a phone: no card, and no count, which rides in the header. */
		phone?: boolean;
	}

	let { answer, saved, savedIds, rows, phone = false }: Props = $props();

	const now = Date.now();
	const shown = $derived(answer?.songs.slice(0, rows) ?? []);

	function albumOf(id: string): Pick<PlaylistAlbum, 'title' | 'artwork'> | undefined {
		return library.entry(id) ?? answer?.mosaic.find((album) => album.id === id);
	}
</script>

<section
	class="live"
	class:phone
	id={phone ? 'songs' : undefined}
	aria-label="Songs the rules choose"
	aria-live="polite"
>
	{#if !phone}
		<div class="head">
			<div class="count-line">
				<span class="count" data-testid="smart-count" data-count={answer?.songCount}
					>{answer?.songCount.toLocaleString('en-US') ?? '…'}</span
				>
				<span class="word">{matchWord(answer?.songCount ?? 0)}</span>
			</div>
			<div class="line">{answer === null ? 'COUNTING' : liveLine(answer, saved)}</div>
		</div>
	{/if}
	<div class="rows">
		{#each shown as song (song.id)}
			{@const album = albumOf(song.albumId)}
			{@const fresh = savedIds !== null && !savedIds.has(song.id)}
			<div class="row" class:fresh>
				<span class="cover"
					><Cover albumId={song.albumId} entry={album} place="small" lazy box={30} /></span
				>
				<span class="words">
					<span class="title">{song.title}</span>
					<span class="sub"
						>{liveRowLine(album?.title ?? '', song.lastPlayedAt, song.plays ?? 0, now)}</span
					>
				</span>
				{#if fresh}<span class="new">NEW</span>{/if}
				<span class="stars">{song.stars === undefined ? '' : starsText(song.stars * 2)}</span>
			</div>
		{/each}
	</div>
	{#if answer !== null}
		<div class="foot">{liveFoot(answer.songCount, shown.length)}</div>
	{/if}
</section>

<style>
	.live {
		min-width: 0;
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-24);
		background: var(--card);
		box-shadow: 0 0 0 1px var(--edge);
		overflow: hidden;
	}

	.head {
		padding: calc(20px * var(--step)) calc(22px * var(--step)) 14px;
		border-bottom: 1px solid var(--edge);
	}

	.count-line {
		display: flex;
		align-items: baseline;
		gap: 10px;
	}

	.count {
		font-family: var(--font-mono);
		font-size: calc(56px * var(--step));
		font-weight: 700;
		letter-spacing: -0.04em;
		line-height: 1;
		color: var(--fox-text);
		text-shadow: 2px 1px 0 color-mix(in srgb, var(--misprint) 55%, transparent);
	}

	.word {
		font-family: var(--font-mono);
		font-size: 13px;
		font-weight: 700;
		letter-spacing: 0.1em;
	}

	.line,
	.foot,
	.sub {
		font-family: var(--font-mono);
		font-size: 11px;
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.line {
		margin-top: 8px;
	}

	.rows {
		flex: 1;
		min-height: 0;
		overflow: hidden;
		padding: 6px 10px;
	}

	.row {
		display: flex;
		align-items: center;
		gap: 10px;
		height: calc(46px * var(--step));
		padding: 0 8px;
		border-radius: var(--radius-12);
	}

	.row.fresh {
		background: color-mix(in srgb, var(--fox) 8%, transparent);
	}

	.cover {
		position: relative;
		flex: none;
		width: calc(30px * var(--step));
		height: calc(30px * var(--step));
		border-radius: 7px;
		overflow: hidden;
		background: var(--card-raised);
	}

	.words {
		flex: 1;
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 1px;
	}

	.title,
	.sub {
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
	}

	.title {
		font-size: 14px;
		font-weight: 600;
	}

	.sub {
		letter-spacing: 0;
	}

	.new {
		flex: none;
		font-family: var(--font-mono);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.1em;
		color: var(--fox-text);
	}

	.stars {
		flex: none;
		font-size: 12px;
		letter-spacing: 1px;
		color: var(--fox-text);
	}

	.foot {
		flex: none;
		padding: 10px calc(22px * var(--step)) 14px;
	}

	/* A phone lists them under the rules, as a plain list. */
	.phone {
		border-radius: 0;
		background: none;
		box-shadow: none;
		overflow: visible;
	}

	.phone .rows {
		padding: 6px 0;
		overflow: visible;
	}

	.phone .row {
		height: 52px;
		padding: 0;
		border-bottom: 1px solid var(--edge);
		border-radius: 0;
	}

	.phone .cover {
		width: 36px;
		height: 36px;
		border-radius: 8px;
	}

	.phone .foot {
		padding: 10px 0;
	}
</style>
