<!--
	A file playlist's lines that found no song, as the file wrote them: between the head and the songs,
	in a dashed fox border. Each row is the line's number, the line (with its #EXTINF under it), a
	reason chip turned −1° and a plain hint; on a phone, three short lines. Lines past what Kitsune
	reads are counted in one row.
-->
<script lang="ts">
	import { SurfaceSlot } from '#lib/morph/surface.ts';
	import type { MissingLine } from '#lib/library/source.ts';
	import {
		extinfWords,
		hintFor,
		missingHeading,
		missingLead,
		missingText,
		pastLimitWords,
		reasonWords
	} from './fileWords.ts';

	interface Props {
		lines: readonly MissingLine[];
		pastLimit: number;
		/** The songs that were found, which play in the file's order. */
		found: number;
		phone: boolean;
		onhide: () => void;
	}

	let { lines, pastLimit, found, phone, onhide }: Props = $props();

	const lost = $derived(lines.length + pastLimit);
	const past = $derived(pastLimitWords(pastLimit));

	/** Buds from the not-found pill that opened it, and gives way where it stands. */
	const surface = new SurfaceSlot();
</script>

<section
	class={['missing', phone && 'phone']}
	aria-label="Lines that found no song"
	{@attach surface.attach}
>
	<div class="head">
		<h2>{missingHeading(lost)}</h2>
		<span class="lead">{missingLead(found, phone)}</span>
		{#if !phone}
			<span class="grow"></span>
			<button type="button" class="hide" onclick={onhide}>Hide</button>
		{/if}
	</div>
	{#each lines as line (line.line)}
		{@const inf = extinfWords(line)}
		<div class="row">
			<span class="number">LINE {line.line}</span>
			<span class="said">
				<span class="text">{missingText(line)}</span>
				{#if inf !== '' && !phone}<span class="inf">{inf}</span>{/if}
			</span>
			<span class="why">{reasonWords(line.reason)}</span>
			<span class="hint">{hintFor(line, phone)}</span>
		</div>
	{/each}
	{#if pastLimit > 0}
		<div class="row">
			<span class="number">PAST</span>
			<span class="said"><span class="text">{past.text}</span></span>
			<span class="why">{reasonWords('pastLimit')}</span>
			<span class="hint">{past.hint}</span>
		</div>
	{/if}
</section>

<style>
	.missing {
		padding: 18px 22px 10px;
		border-radius: var(--radius-24);
		background: var(--card);
		border: 1.5px dashed color-mix(in srgb, var(--fox-line) 55%, transparent);
	}

	.head {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		gap: 4px 14px;
		margin-bottom: 8px;
	}

	h2 {
		font-size: calc(18px * var(--step));
		font-weight: 800;
		letter-spacing: -0.02em;
	}

	.lead,
	.number,
	.text,
	.why {
		font-family: var(--font-mono);
	}

	.lead {
		font-size: 11px;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.grow {
		flex: 1;
	}

	.hide {
		height: 32px;
		padding: 0 12px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--hover);
		color: var(--text);
		font: 600 12px var(--font-sans);
	}

	.row {
		display: grid;
		grid-template-columns: 76px minmax(0, 1fr) 190px minmax(0, 0.8fr);
		align-items: center;
		gap: 16px;
		min-height: 58px;
		border-top: 1px solid var(--edge);
	}

	.number {
		font-size: 11px;
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--text-meta);
	}

	.said {
		min-width: 0;
		display: flex;
		flex-direction: column;
		gap: 3px;
	}

	.text {
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		font-size: 13px;
	}

	.inf {
		font-size: 12px;
		color: var(--text-meta);
	}

	.why {
		justify-self: start;
		padding: 5px 9px;
		border-radius: 6px;
		background: var(--hover);
		font-size: 10px;
		font-weight: 700;
		letter-spacing: 0.08em;
		color: var(--text-secondary);
		transform: rotate(-1deg);
	}

	.hint {
		font-size: 13px;
		line-height: 1.4;
		color: var(--text-secondary);
	}

	/* The phone board: each line is three short lines, the chip beside its number. */
	.phone {
		padding: 12px 14px 4px;
		border-radius: 18px;
	}

	.phone .head {
		flex-direction: column;
		gap: 4px;
		margin-bottom: 6px;
	}

	.phone h2 {
		font-size: 15px;
	}

	.phone .lead {
		font-size: 10px;
		letter-spacing: 0.04em;
	}

	.phone .row {
		grid-template-columns: auto minmax(0, 1fr);
		gap: 4px 8px;
		min-height: 0;
		padding: 10px 0;
	}

	.phone .number {
		font-size: 10px;
	}

	.phone .why {
		grid-column: 2;
		grid-row: 1;
		padding: 3px 7px;
		font-size: 9px;
		transform: none;
	}

	.phone .said,
	.phone .hint {
		grid-column: 1 / -1;
	}

	.phone .text {
		font-size: 12px;
	}

	.phone .hint {
		font-size: 12px;
		line-height: 1.35;
		color: var(--text-meta);
	}
</style>
