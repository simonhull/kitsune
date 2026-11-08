<script lang="ts">
	import { emWidth } from '#lib/library/fit.ts';
	import {
		lastScan,
		minutesLeft,
		refusalLine,
		scanHelp,
		scanLine,
		scanPercent,
		standing,
		steadyMinutes,
		timeLeft
	} from '#lib/library/health.ts';
	import { library } from '#lib/library/library.svelte.ts';
	import type { LibraryFacts, ScanMode, ScanRefusal } from '#lib/library/source.ts';

	interface Props {
		facts: LibraryFacts;
		admin: boolean;
		/** The server stopped answering, so what's on screen may be old. */
		down: boolean;
		now: number;
	}

	let { facts, admin, down, now }: Props = $props();

	const figure = $derived(facts.issues.toLocaleString('en-US'));
	const words = $derived(standing(facts));
	const scan = $derived(facts.scan);
	const running = $derived(scan.state === 'running');
	const last = $derived(scan.last === undefined ? null : lastScan(scan.last, now));
	const failed = $derived(scan.last?.outcome === 'failed');

	// Not state: it's this derived's own memory of what it last showed.
	let shownMinutes: number | null = null;
	const left = $derived.by(() => {
		shownMinutes = steadyMinutes(shownMinutes, minutesLeft(scan, now));
		return timeLeft(scan, shownMinutes);
	});

	let asking = $state(false);
	let refusal = $state<ScanRefusal | null>(null);

	async function start(mode: ScanMode): Promise<void> {
		asking = true;
		refusal = null;
		refusal = await library.startScan(mode);
		asking = false;
	}
</script>

<section class="panel" aria-labelledby="health-title">
	<div class="standing">
		<h1 id="health-title" class="label">Library health</h1>
		<div class="fit">
			<p class="figure" style:--em={emWidth(figure, 900, -0.07)}>{figure}</p>
		</div>
		<p class="headline">{words.headline}</p>
		<p class="detail">{words.detail}</p>
	</div>

	<div class="scans">
		{#if running}
			<div class="facts">
				<span class="line">{scanLine(scan)}</span>
				{#if down}
					<span>THE SERVER ISN'T ANSWERING</span>
				{:else if left !== null}
					<span>{left}</span>
				{/if}
			</div>
			{#if scan.total > 0}
				<div
					class="track"
					role="progressbar"
					aria-label="Scan progress"
					aria-valuenow={scanPercent(scan)}
					aria-valuemin="0"
					aria-valuemax="100"
					aria-valuetext={scanLine(scan)}
				>
					<div class="fill" style:width="{scanPercent(scan)}%"></div>
				</div>
			{:else}
				<p class="below">COUNTING THE FILES IN YOUR FOLDERS</p>
			{/if}
		{:else if last !== null}
			<div class="facts">
				<span class="line">{last.title}</span>
				{#if last.took !== null}<span>{last.took}</span>{/if}
			</div>
			{#if failed}
				<p class="failure" role="status">{last.detail}</p>
			{:else}
				<p class="below">{last.detail}</p>
			{/if}
		{:else}
			<div class="facts"><span class="line">NO SCAN SINCE THE SERVER STARTED</span></div>
			<p class="below"></p>
		{/if}

		{#if admin}
			<div class="buttons">
				<button
					type="button"
					class="solid"
					disabled={running || asking}
					onclick={() => void start('quick')}>Quick scan</button
				>
				<button
					type="button"
					class="ghost"
					disabled={running || asking}
					onclick={() => void start('full')}>Read every file again</button
				>
			</div>
			{#if refusal !== null}
				<p class="help" role="status">{refusalLine(refusal)}</p>
			{:else}
				<p class="help">{scanHelp(facts)}</p>
			{/if}
		{/if}
	</div>
</section>

<style>
	.panel {
		position: sticky;
		top: 96px;
		min-height: var(--panel-height);
		padding: calc(32px * var(--step));
		border-radius: var(--radius-28);
		background: var(--fox);
		color: var(--fox-ink);
		display: flex;
		flex-direction: column;
		justify-content: space-between;
		gap: 40px;
	}

	.label {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-weight: 400;
		letter-spacing: 0.08em;
		text-transform: uppercase;
	}

	.fit {
		container-type: inline-size;
		margin-top: 20px;
	}

	/* The figure takes the board's size, and only gives ground to a number too wide for the panel. */
	.figure {
		font-size: min(var(--type-hero), calc(100cqi / var(--em)));
		font-weight: 900;
		letter-spacing: var(--tracking-hero);
		line-height: 0.85;
		white-space: nowrap;
	}

	.headline {
		margin-top: 16px;
		font-size: calc(22px * var(--step));
		font-weight: 700;
		letter-spacing: var(--tracking-title);
		text-wrap: balance;
	}

	.detail {
		margin-top: 6px;
		font-size: var(--type-body);
		line-height: 1.4;
	}

	.facts {
		display: flex;
		flex-wrap: wrap;
		align-items: baseline;
		justify-content: space-between;
		gap: 2px 16px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.04em;
	}

	.track {
		margin-top: 10px;
		height: 8px;
		border-radius: 4px;
		background: color-mix(in srgb, var(--fox-ink) 20%, transparent);
		overflow: hidden;
	}

	.fill {
		height: 100%;
		border-radius: 4px;
		background: var(--fox-ink);
	}

	/* As tall as the bar's slot, so nothing moves when a scan starts or ends. */
	.below {
		min-height: 16px;
		margin-top: 2px;
		line-height: 16px;
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		font-variant-numeric: tabular-nums;
		letter-spacing: 0.04em;
	}

	/* Ink on amber turned over: a failure shouldn't read as one more fact. */
	.failure {
		margin-top: 10px;
		padding: 12px 16px;
		border-radius: var(--radius-16);
		background: var(--fox-ink);
		color: var(--on-fox-ink);
		font-size: calc(14px * var(--step));
		font-weight: 600;
		line-height: 1.35;
		overflow-wrap: anywhere;
	}

	.buttons {
		margin-top: 20px;
		display: flex;
		flex-wrap: wrap;
		gap: 10px;
	}

	button {
		height: 48px;
		padding: 0 22px;
		border-radius: var(--radius-pill);
		font-size: calc(14px * var(--step));
		font-weight: 700;
		transition: opacity var(--morph-fast) var(--curve-morph);
	}

	button:focus-visible {
		outline-color: var(--fox-ink);
	}

	button:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.solid {
		border: 0;
		background: var(--fox-ink);
		color: var(--on-fox-ink);
	}

	.solid:hover:enabled {
		background: color-mix(in srgb, var(--fox-ink) 82%, var(--fox));
	}

	.ghost {
		border: 2px solid var(--fox-ink);
		background: transparent;
		color: var(--fox-ink);
	}

	.ghost:hover:enabled {
		background: color-mix(in srgb, var(--fox-ink) 12%, transparent);
	}

	.help {
		margin-top: 10px;
		font-size: calc(13px * var(--step));
		line-height: 1.4;
	}

	@media (max-width: 900px) {
		.panel {
			position: static;
		}
	}

	@media (max-width: 720px) {
		.panel {
			padding: 24px;
		}

		button {
			flex: 1 1 auto;
		}
	}
</style>
