<!--
	The tag editor's dock: what a write would do, Leave it and Write to files, and over them the
	report of a write that stopped. Back or Esc is Leave it.
-->
<script lang="ts">
	import { tick } from 'svelte';
	import { fade } from 'svelte/transition';
	import { editsOf } from '#lib/library/edit.ts';
	import { tagGroups } from '#lib/library/tags.ts';
	import type { EditSession } from './tagEditor.svelte.ts';

	interface Props {
		session: EditSession;
		/** Drops the draft and leaves. */
		leave: () => void;
		/** Writes the draft. */
		write: () => void;
	}

	let { session, leave, write }: Props = $props();

	const summary = $derived(session.summary);
	const report = $derived(session.report);
	const plural = (count: number, one: string): string =>
		`${String(count)} ${one}${count === 1 ? '' : 's'}`;
	/** Changes that touch no file are shuffle rules: they save, they aren't written. */
	const rulesOnly = $derived(summary.files === 0 && summary.changes > 0);
	const names = new Map(
		tagGroups.flatMap(({ fields }) =>
			fields.map(({ field, label, name }) => [field as string, (name ?? label).toLowerCase()])
		)
	);

	/** Where the changes are: the one song and field, the one song, or how many files. */
	const where = $derived.by(() => {
		if (rulesOnly) {
			return 'to your shuffle';
		}
		const only = summary.only === null ? undefined : session.track(summary.only);
		if (only === undefined) {
			return `across ${plural(summary.files, 'file')}`;
		}
		const title = only.tags.fields.title?.at(0) ?? only.tags.file;
		if (summary.changes > 1) {
			return title;
		}
		const [edit] = editsOf([only], session.draft, session.ticked);
		const [field] = [...Object.keys(edit?.set ?? {}), ...(edit?.clear ?? [])];
		const what = field === undefined ? 'tags' : (names.get(field) ?? field);
		return `${title}’s ${field === 'lyrics' ? 'lyrics' : what}`;
	});

	const busy = $derived(rulesOnly ? 'Saving…' : 'Writing…');

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
		<span class="say">
			<span class="count" aria-live="polite">
				{#if !session.ready}
					{report === null ? 'Reading the files…' : "Can't edit these songs"}
				{:else if summary.changes === 0}
					No changes yet
				{:else}
					{plural(summary.changes, 'change')} · {where}
				{/if}
			</span>
			<span class="keys" aria-hidden="true">BACK OR ESC LEAVES IT</span>
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
			{#if session.saving}
				{busy}
			{:else if rulesOnly}
				Save
			{:else}
				<span class="long">Write to files</span><span class="short">Write</span>
			{/if}
		</button>
	</div>
</div>

<style>
	/* Centred at the foot of the window, clear of the player island. */
	.dock {
		position: fixed;
		left: 50%;
		bottom: calc(var(--dock-bottom, 24px) + env(safe-area-inset-bottom));
		z-index: 3;
		display: flex;
		flex-direction: column;
		gap: 8px;
		width: min(600px, calc(100vw - 32px));
		translate: -50% 0;
		animation: arrive var(--morph) var(--curve-morph) both;
	}

	@keyframes arrive {
		from {
			opacity: 0;
			translate: -50% 16px;
		}
	}

	.bar {
		display: flex;
		align-items: center;
		gap: 12px;
		min-height: 60px;
		padding: 7px 8px 7px 24px;
		border-radius: 30px;
		background: var(--glass-solid);
		border: 1px solid var(--edge-card);
		box-shadow: var(--glass-shadow);
		backdrop-filter: var(--glass-blur);
	}

	.say {
		flex: 1;
		display: flex;
		flex-direction: column;
		min-width: 0;
	}

	.count {
		overflow: hidden;
		font-size: 14px;
		font-weight: 600;
		text-overflow: ellipsis;
		white-space: nowrap;
	}

	.keys {
		font-family: var(--font-mono);
		font-size: 10px;
		letter-spacing: 0.06em;
		color: var(--text-meta);
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
		border: 1px solid var(--edge-field);
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

	.short {
		display: none;
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

	/* A phone: the whole width, and Write says just that. */
	@media (max-width: 640px) {
		.dock {
			left: 16px;
			right: 16px;
			width: auto;
			translate: none;
			animation-name: rise;
		}

		@keyframes rise {
			from {
				opacity: 0;
				translate: 0 16px;
			}
		}

		.bar {
			gap: 8px;
			padding: 8px 8px 8px 18px;
		}

		.count {
			font-size: 13px;
		}

		.keys {
			display: none;
		}

		.leave {
			padding: 0 14px;
		}

		.write {
			padding: 0 16px;
		}

		.long {
			display: none;
		}

		.short {
			display: inline;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.dock {
			animation-name: none;
		}
	}
</style>
