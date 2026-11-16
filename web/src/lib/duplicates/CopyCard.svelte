<!--
	One copy of a recording, as a group's page compares it (board H): its album, path, facts, and
	Play. The keeper wears the fox ring and KEEP THIS ONE, with why; any other offers to be kept
	instead. On a phone its facts are two mono lines.
-->
<script lang="ts">
	import Cover from '#lib/cover/Cover.svelte';
	import Icon from '#lib/shell/Icon.svelte';
	import type { DuplicateCopy } from './types.ts';
	import { albumLine, copyFacts, copyLines } from './words.ts';

	interface Props {
		copy: DuplicateCopy;
		keeper: boolean;
		/** Why it's kept, under its facts, when it is. */
		why: string;
		/** Whether the others offer to be kept instead: not while the confirm is open. */
		choosing: boolean;
		phone: boolean;
		onkeep: () => void;
		onplay: () => void;
	}

	let { copy, keeper, why, choosing, phone, onkeep, onplay }: Props = $props();

	const lines = $derived(copyLines(copy));
	const initials = $derived(
		copy.album.title
			.split(/\s+/)
			.filter((word) => /^\p{L}/u.test(word))
			.slice(0, 3)
			.map((word) => word[0]?.toUpperCase() ?? '')
			.join('')
	);
</script>

<section class="copy" class:keeper class:phone aria-label={copy.album.title} data-testid="copy">
	{#if keeper}
		<span class="tag mono">KEEP THIS ONE</span>
	{/if}
	<div class="head">
		{#if !phone}
			<span class="cover">
				{#if copy.album.artwork !== undefined}
					<Cover
						albumId={copy.album.id}
						entry={{ artwork: copy.album.artwork }}
						place="small"
						box={64}
					/>
				{:else}
					<span class="initials mono" aria-hidden="true">{initials}</span>
				{/if}
			</span>
		{/if}
		<span class="names">
			<span class="album">{copy.album.title}</span>
			{#if !phone}
				<span class="year mono">{albumLine(copy)}</span>
			{/if}
		</span>
		<button
			type="button"
			class="play"
			aria-label={`Play the copy on ${copy.album.title}`}
			onclick={onplay}
		>
			<Icon name="play" size={14} />
		</button>
	</div>
	{#if phone}
		<span class="line mono">{lines[0]}</span>
		<span class="line path mono">{lines[1]}</span>
	{:else}
		<span class="path mono">{copy.path}</span>
		<dl class="facts">
			{#each copyFacts(copy) as fact (fact.label)}
				<dt class="mono">{fact.label}</dt>
				<dd class:bold={fact.bold === true} class:quiet={fact.quiet === true}>{fact.value}</dd>
			{/each}
		</dl>
	{/if}
	{#if keeper}
		<span class="why mono" data-testid="why">{why}</span>
	{:else if choosing}
		<button type="button" class="instead" onclick={onkeep}>Keep this one instead</button>
	{/if}
</section>

<style>
	.mono {
		font-family: var(--font-mono);
	}

	.copy {
		position: relative;
		display: flex;
		flex-direction: column;
		gap: calc(14px * var(--step));
		min-width: 0;
		padding: calc(22px * var(--step)) calc(24px * var(--step));
		border-radius: calc(22px * var(--step));
		background: var(--card);
		border: 1px solid var(--edge);
	}

	.copy.keeper {
		border: 2px solid var(--fox-line);
		box-shadow: 0 0 0 6px color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.tag {
		position: absolute;
		right: calc(22px * var(--step));
		top: calc(-14px * var(--step));
		padding: 5px 10px;
		border-radius: 6px;
		background: var(--fox);
		color: var(--fox-ink);
		font-size: calc(11px * var(--step));
		font-weight: 800;
		letter-spacing: 0.08em;
		transform: rotate(-3deg);
		box-shadow: 3px 3px 0 color-mix(in srgb, var(--misprint) 55%, transparent);
	}

	.head {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.cover {
		flex: none;
		width: calc(64px * var(--step));
		height: calc(64px * var(--step));
		overflow: hidden;
		border-radius: var(--radius-12);
		background: var(--card-raised);
	}

	.cover :global(img) {
		width: 100%;
		height: 100%;
		object-fit: cover;
	}

	.initials {
		display: grid;
		place-items: center;
		width: 100%;
		height: 100%;
		font-size: calc(15px * var(--step));
		font-weight: 700;
		color: var(--text-meta);
	}

	.names {
		display: flex;
		flex-direction: column;
		gap: 3px;
		min-width: 0;
	}

	.album {
		font-size: calc(19px * var(--step));
		font-weight: 800;
		letter-spacing: -0.01em;
	}

	.year {
		font-size: calc(11px * var(--step));
		color: var(--text-meta);
	}

	.play {
		display: grid;
		place-items: center;
		flex: none;
		width: calc(44px * var(--step));
		height: calc(44px * var(--step));
		margin-left: auto;
		border-radius: 50%;
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		cursor: pointer;
	}

	.play:hover {
		background: var(--hover);
	}

	.path {
		font-size: calc(11.5px * var(--step));
		line-height: 1.5;
		color: var(--text-meta);
		word-break: break-all;
	}

	.facts {
		display: grid;
		grid-template-columns: calc(120px * var(--step)) minmax(0, 1fr);
		row-gap: calc(9px * var(--step));
		margin: 0;
	}

	dt {
		font-size: calc(10.5px * var(--step));
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	dd {
		margin: 0;
		font-size: calc(14px * var(--step));
		font-weight: 500;
	}

	dd.bold {
		font-weight: 700;
	}

	dd.quiet {
		color: var(--text-meta);
	}

	.why {
		font-size: calc(11px * var(--step));
		font-weight: 700;
		letter-spacing: 0.06em;
		color: var(--fox-text);
	}

	.instead {
		align-self: flex-start;
		height: calc(38px * var(--step));
		padding: 0 16px;
		border-radius: var(--radius-pill);
		border: 1px solid var(--edge-strong);
		background: none;
		color: var(--text);
		font: 600 calc(14px * var(--step)) var(--font-sans);
		cursor: pointer;
	}

	.instead:hover {
		background: var(--hover);
	}

	.copy.phone {
		gap: 6px;
		padding: 14px 16px;
		border-radius: 18px;
	}

	.phone .tag {
		top: -11px;
		right: 14px;
		padding: 3px 8px;
		font-size: 10px;
	}

	.phone .album {
		font-size: 16px;
	}

	.phone .play {
		width: 40px;
		height: 40px;
	}

	.line {
		font-size: 10.5px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.line.path {
		color: var(--text-meta);
		word-break: break-all;
	}

	.phone .instead {
		align-self: stretch;
	}
</style>
