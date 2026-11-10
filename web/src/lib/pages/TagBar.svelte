<script lang="ts">
	import { tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import type { Report, Summary } from '#lib/library/tags.ts';

	/** What the bar reads of an edit: one album's, or many songs'. */
	interface BarSession {
		/** Null until the files have been read. */
		readonly tags: unknown;
		readonly summary: Summary;
		readonly report: Report | null;
		readonly saving: boolean;
		readonly blocked: boolean;
	}

	interface Props {
		session: BarSession;
		/** The track open to every tag, if any. */
		open?: string | null;
		/** What can't be edited, when the files can't be read. */
		what?: string;
		/** Drops the draft and leaves. */
		leave: () => void;
		/** Writes the draft. */
		write: () => void;
		/** Finds this album's release on MusicBrainz: offered while nothing is unsaved. */
		match?: (() => void) | undefined;
	}

	let { session, open = null, what = 'this album', leave, write, match }: Props = $props();

	const summary = $derived(session.summary);
	const report = $derived(session.report);
	const plural = (count: number, one: string): string =>
		`${String(count)} ${one}${count === 1 ? '' : 's'}`;
	const changes = $derived(plural(summary.changes, 'change'));
	/** Changes that touch no file are shuffle rules: they save, they aren't written. */
	const rulesOnly = $derived(summary.files === 0 && summary.changes > 0);
	const where = $derived(
		rulesOnly
			? 'to your shuffle'
			: summary.only !== null && summary.only === open
				? 'on this track'
				: `across ${plural(summary.files, 'file')}`
	);

	const writeLabel = $derived.by(() => {
		if (session.saving) {
			return rulesOnly ? 'Saving…' : 'Writing…';
		}
		return rulesOnly ? 'Save' : `Write to ${summary.files === 1 ? 'file' : 'files'}`;
	});

	let reportBox: HTMLElement | undefined = $state();

	$effect(() => {
		if (report !== null) {
			void tick().then(() => reportBox?.focus());
		}
	});
</script>

<div class="dock" out:fade={{ duration: 120 }}>
	<div class="stack">
		{#if report !== null}
			<div class="report" role="alert" tabindex="-1" bind:this={reportBox}>
				<p class="message">{report.message}</p>
				{#if report.detail !== undefined}<p class="detail">{report.detail}</p>{/if}
				{#if report.written.length > 0}
					<dl>
						<div>
							<dt>Written</dt>
							{#each report.written as title, index (index)}<dd>{title}</dd>{/each}
						</div>
						<div>
							<dt>Not written</dt>
							{#each report.unwritten as title, index (index)}<dd>{title}</dd>{/each}
						</div>
					</dl>
				{/if}
			</div>
		{/if}
		<div class="bar">
			<span class="count" aria-live="polite">
				{#if session.tags === null}
					{report === null ? 'Reading the files…' : `Can't edit ${what}`}
				{:else if summary.changes === 0}
					No changes yet
				{:else}
					{changes} <span class="where">{where}</span>
				{/if}
			</span>
			<span class="actions">
				{#if match !== undefined && session.tags !== null && open === null}
					<button
						type="button"
						class="leave"
						disabled={session.saving || summary.changes > 0}
						title={summary.changes > 0 ? 'Write or leave your changes first' : undefined}
						onclick={match}>Find matches…</button
					>
				{/if}
				<button type="button" class="leave" disabled={session.saving} onclick={leave}
					>Leave it</button
				>
				<button
					type="button"
					class="write"
					disabled={session.saving ||
						report?.retry === false ||
						(summary.changes === 0 && !session.blocked)}
					onclick={write}
				>
					{writeLabel}
				</button>
			</span>
		</div>
	</div>
</div>

<style>
	/*
	 * The bar acts on the panel above it, so it stands at the foot of that panel's column, centred on
	 * it, and sticks clear of the player island while the column scrolls past. Its column decides
	 * where it is; the window doesn't. Inside a zoomed editor (from 2560) --dock-zoom keeps the
	 * offset in window pixels.
	 */
	.dock {
		position: sticky;
		bottom: calc((108px + env(safe-area-inset-bottom)) / var(--dock-zoom, 1));
		z-index: 3;
		margin-top: auto;
		animation: arrive var(--morph) var(--curve-morph) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
			translate: 0 16px;
		}
	}

	/* As wide as the bar's words, never wider than the column; the report takes the bar's width. */
	.stack {
		display: flex;
		flex-direction: column;
		gap: 8px;
		width: fit-content;
		max-width: 100%;
		margin-inline: auto;
	}

	/*
	 * One row when the column holds it; otherwise the buttons wrap under the count. The radius is half
	 * the one-row height, so one row is the pill it always was and two are a rounded card.
	 */
	.bar {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 8px 12px;
		min-height: 60px;
		padding: 7px;
		border-radius: 30px;
		background: var(--glass-solid);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--glass-shadow);
		backdrop-filter: var(--glass-blur);
	}

	.count {
		flex: 1 0 auto;
		padding-left: 16px;
		font-size: 14px;
		line-height: var(--target);
		font-weight: 600;
		white-space: nowrap;
	}

	.actions {
		display: flex;
		flex-wrap: wrap;
		justify-content: flex-end;
		gap: 8px 12px;
		margin-left: auto;
	}

	button {
		flex: none;
		height: var(--target);
		border-radius: var(--radius-pill);
		font-size: 14px;
		white-space: nowrap;
	}

	.leave {
		padding: 0 18px;
		border: 1px solid var(--edge-strong);
		background: transparent;
		font-weight: 600;
	}

	.write {
		padding: 0 22px;
		border: 0;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	button:disabled {
		opacity: 0.5;
		cursor: default;
	}

	.report {
		display: flex;
		flex-direction: column;
		gap: 10px;
		max-height: 40dvh;
		padding: 16px 20px;
		/* Its words wrap to the bar's width rather than widen it. */
		contain: inline-size;
		overflow-y: auto;
		border-radius: var(--radius-24);
		background: var(--glass-solid);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--glass-shadow);
		backdrop-filter: var(--glass-blur);
		outline: none;
	}

	.message {
		font-size: 14px;
		font-weight: 600;
		line-height: 1.4;
		color: var(--fox-text);
	}

	.detail {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		color: var(--text-meta);
		overflow-wrap: anywhere;
	}

	dl {
		display: grid;
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px 24px;
		margin: 0;
	}

	dt {
		font-family: var(--font-mono);
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
		text-transform: uppercase;
	}

	dd {
		margin: 4px 0 0;
		font-size: 14px;
		overflow-wrap: anywhere;
	}

	/* A phone keeps the bar across the foot of the screen, in two rows when one can't hold it. */
	@media (max-width: 820px) {
		.dock {
			position: fixed;
			right: 16px;
			bottom: calc(16px + env(safe-area-inset-bottom));
			left: 16px;
		}

		.stack {
			width: auto;
		}

		.bar {
			gap: 8px;
		}

		.count {
			padding-left: 10px;
		}

		/* Wrapped, the buttons share the second row between them. */
		.actions {
			flex: 1 1 auto;
			flex-wrap: nowrap;
			gap: 6px;
		}

		.actions button {
			flex: 1 1 auto;
		}

		.leave,
		.write {
			padding: 0 12px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dock {
			animation-name: none;
		}
	}
</style>
