<script lang="ts">
	import { tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import type { TagSession } from './tagEdits.svelte.ts';

	interface Props {
		session: TagSession;
		/** The track open to every tag, if any. */
		open?: string | null;
		/** Drops the draft and leaves. */
		leave: () => void;
		/** Writes the draft. */
		write: () => void;
	}

	let { session, open = null, leave, write }: Props = $props();

	const summary = $derived(session.summary);
	const report = $derived(session.report);
	const plural = (count: number, one: string): string =>
		`${String(count)} ${one}${count === 1 ? '' : 's'}`;
	const changes = $derived(plural(summary.changes, 'change'));
	const where = $derived(
		summary.only !== null && summary.only === open
			? 'on this track'
			: `across ${plural(summary.files, 'file')}`
	);

	let reportBox: HTMLElement | undefined = $state();

	$effect(() => {
		if (report !== null) {
			void tick().then(() => reportBox?.focus());
		}
	});
</script>

<div class="dock" out:fade={{ duration: 120 }}>
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
				{report === null ? 'Reading the files…' : "Can't edit this album"}
			{:else if summary.changes === 0}
				No changes yet
			{:else}
				{changes} <span class="where">{where}</span>
			{/if}
		</span>
		<button type="button" class="leave" disabled={session.saving} onclick={leave}>Leave it</button>
		<button
			type="button"
			class="write"
			disabled={session.saving ||
				report?.retry === false ||
				(summary.changes === 0 && !session.blocked)}
			onclick={write}
		>
			{#if session.saving}Writing…{:else}Write to {summary.files === 1 ? 'file' : 'files'}{/if}
		</button>
	</div>
</div>

<style>
	.dock {
		position: fixed;
		left: 50%;
		bottom: calc(108px + env(safe-area-inset-bottom));
		z-index: 3;
		display: flex;
		flex-direction: column;
		gap: 8px;
		width: min(532px, calc(100% - 32px));
		transform: translateX(-50%);
		animation: arrive var(--morph) var(--curve-morph) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
			translate: 0 16px;
		}
	}

	.bar {
		display: flex;
		align-items: center;
		gap: 12px;
		height: 60px;
		padding: 0 8px 0 24px;
		border-radius: var(--radius-pill);
		background: var(--glass-solid);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--glass-shadow);
		backdrop-filter: var(--glass-blur);
	}

	.count {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		font-size: 14px;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	button {
		flex: none;
		height: var(--target);
		border-radius: var(--radius-pill);
		font-size: 14px;
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

	@media (max-width: 820px) {
		.dock {
			bottom: calc(16px + env(safe-area-inset-bottom));
			width: calc(100% - 32px);
		}

		.bar {
			gap: 8px;
			padding: 0 8px 0 18px;
			background: var(--glass-solid);
		}

		.where {
			display: none;
		}

		.leave {
			padding: 0 14px;
		}

		.write {
			padding: 0 16px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dock {
			animation-name: none;
		}
	}
</style>
