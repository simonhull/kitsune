<script lang="ts">
	import { library } from '#lib/library/library.svelte.ts';
	import type { FolderKind, FolderListing } from '#lib/library/source.ts';
	import { checkLine, crumbsOf, kindNote } from './serverForm.ts';

	/** The folder picker: a browser of what the server can see, directories only. */
	interface Props {
		open: boolean;
		onadd: (path: string) => void;
		onclose: () => void;
	}

	let { open, onadd, onclose }: Props = $props();

	let dialog = $state<HTMLDialogElement | undefined>();
	let listing = $state.raw<FolderListing | null>(null);
	let reading = $state(false);
	let trouble = $state('');
	/** The last path asked for: an answer for an older one is dropped. */
	let asked = '';

	const addable = $derived(listing !== null && listing.kind === 'folder' && listing.path !== '/');

	async function go(path: string): Promise<void> {
		asked = path;
		reading = true;
		trouble = '';
		try {
			const next = await library.folders(path);
			if (asked === path) {
				listing = next;
			}
		} catch {
			if (asked === path) {
				trouble = 'That folder can’t be read by the server.';
			}
		} finally {
			if (asked === path) {
				reading = false;
			}
		}
	}

	$effect(() => {
		if (dialog === undefined) {
			return;
		}
		if (open && !dialog.open) {
			dialog.showModal();
			void go('/');
		} else if (!open && dialog.open) {
			dialog.close();
		}
	});

	function enterable(kind: FolderKind): boolean {
		return kind !== 'data' && kind !== 'config';
	}
</script>

<dialog
	bind:this={dialog}
	aria-labelledby="picker-title"
	{onclose}
	onclick={(event) => {
		if (event.target === dialog) {
			onclose();
		}
	}}
>
	<div class="picker">
		<div>
			<p class="mono eyebrow">WHAT THE SERVER CAN SEE</p>
			<h2 id="picker-title">Add a folder</h2>
		</div>

		<nav aria-label="Path" class="crumbs">
			{#each crumbsOf(listing?.path ?? '/') as crumb, index (crumb.path)}
				{#if index > 0}<span class="sep" aria-hidden="true">›</span>{/if}
				<button
					type="button"
					class="crumb mono"
					class:here={crumb.path === listing?.path}
					aria-current={crumb.path === listing?.path ? 'location' : undefined}
					onclick={() => void go(crumb.path)}>{crumb.name}</button
				>
			{/each}
		</nav>

		<div class="list" aria-busy={reading}>
			{#if listing !== null}
				{#each listing.folders as folder (folder.name)}
					{@const note = kindNote(folder.kind)}
					<button
						type="button"
						class="row"
						class:off={folder.kind !== 'folder'}
						disabled={!enterable(folder.kind)}
						onclick={() => void go(folder.path)}
					>
						<svg width="20" height="20" viewBox="0 0 24 24" aria-hidden="true">
							<path
								d="M3 6.5A1.5 1.5 0 0 1 4.5 5h4.6l2 2.2h8.4A1.5 1.5 0 0 1 21 8.7v9.8a1.5 1.5 0 0 1-1.5 1.5h-15A1.5 1.5 0 0 1 3 18.5z"
							></path>
						</svg>
						<span class="name mono">{folder.name}</span>
						{#if note !== ''}<span class="note">{note}</span>{/if}
						{#if enterable(folder.kind)}<span class="go" aria-hidden="true">›</span>{/if}
					</button>
				{:else}
					<p class="quiet">No folders in here.</p>
				{/each}
			{:else if !reading}
				<p class="quiet">{trouble}</p>
			{/if}
		</div>

		{#if listing !== null}
			<div class="check" role="status">
				{#if addable}
					<svg width="20" height="20" viewBox="0 0 24 24" aria-hidden="true">
						<path class="tick" d="M5 12.5l4.5 4.5L19 7.5"></path>
					</svg>
				{/if}
				<span>{trouble !== '' ? trouble : checkLine(listing)}</span>
			</div>
		{/if}

		<p class="aside">
			Under Docker these are the container’s paths: only what compose.yaml mounts shows up. Folders
			only, no file names.
		</p>

		<div class="buttons">
			<button type="button" class="cancel" onclick={onclose}>Cancel</button>
			<button
				type="button"
				class="add"
				disabled={!addable || reading}
				onclick={() => {
					if (listing !== null) onadd(listing.path);
				}}
			>
				{addable && listing !== null && listing.path.length <= 28
					? `Add ${listing.path}`
					: addable
						? 'Add this folder'
						: 'Add'}
			</button>
		</div>
	</div>
</dialog>

<style>
	dialog {
		box-sizing: border-box;
		width: min(600px, calc(100vw - 32px));
		max-height: calc(100dvh - 48px);
		margin: auto;
		padding: 0;
		border: 1px solid var(--edge-raised);
		border-radius: var(--radius-32);
		background: var(--card);
		color: var(--text);
		box-shadow: var(--shadow-float);
		overflow: auto;
	}

	dialog[open] {
		animation: rise var(--morph) var(--curve-settle) both;
	}

	dialog::backdrop {
		background: color-mix(in srgb, var(--ground) 72%, transparent);
		backdrop-filter: blur(6px);
	}

	.picker {
		padding: 36px;
		display: flex;
		flex-direction: column;
		gap: 20px;
	}

	.mono {
		font-family: var(--font-mono);
	}

	.eyebrow {
		font-size: var(--type-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	h2 {
		margin-top: 10px;
		font-size: var(--type-section);
		font-weight: 800;
		letter-spacing: var(--tracking-section);
	}

	.crumbs {
		display: flex;
		flex-wrap: wrap;
		align-items: center;
		gap: 6px;
	}

	.sep {
		color: var(--text-label);
	}

	.crumb {
		min-height: 32px;
		padding: 6px 10px;
		border: 0;
		border-radius: var(--radius-pill);
		background: var(--fill);
		color: var(--text);
		font-size: 14px;
		cursor: pointer;
	}

	.crumb.here {
		background: color-mix(in srgb, var(--fox) 16%, transparent);
	}

	.list {
		display: flex;
		flex-direction: column;
		gap: 4px;
		max-height: 360px;
		min-height: 120px;
		overflow-y: auto;
		transition: opacity var(--morph-fast) var(--curve-morph);
	}

	.list[aria-busy='true'] {
		opacity: 0.55;
	}

	.row {
		flex: none;
		display: flex;
		align-items: center;
		gap: 14px;
		height: 56px;
		padding: 0 18px;
		border: 0;
		border-radius: var(--radius-16);
		background: transparent;
		color: var(--text);
		font: inherit;
		font-size: 14px;
		text-align: left;
		cursor: pointer;
	}

	.row:hover:not(:disabled) {
		background: var(--hover);
	}

	.row:focus-visible {
		outline: 2px solid var(--fox-line);
		outline-offset: -2px;
	}

	.row path {
		fill: none;
		stroke: currentcolor;
		stroke-width: 1.8;
	}

	.row.off {
		color: var(--text-label);
	}

	.row:disabled {
		cursor: default;
	}

	.name {
		flex: 1;
		min-width: 0;
		overflow-wrap: anywhere;
	}

	.note {
		font-size: 12px;
		color: var(--text-meta);
	}

	.go {
		color: var(--text-label);
	}

	.quiet {
		padding: 16px 18px;
		font-size: 14px;
		color: var(--text-meta);
	}

	.check {
		display: flex;
		align-items: center;
		gap: 12px;
		padding: 16px 18px;
		border-radius: var(--radius-20);
		background: var(--input);
		font-size: 14px;
		line-height: 1.5;
		color: var(--text-secondary);
	}

	.check svg {
		flex: none;
	}

	.tick {
		fill: none;
		stroke: var(--fox-text);
		stroke-width: 2.2;
		stroke-linecap: round;
		stroke-linejoin: round;
	}

	.aside {
		font-size: 13px;
		line-height: 1.5;
		color: var(--text-meta);
	}

	.buttons {
		display: flex;
		justify-content: flex-end;
		gap: 10px;
	}

	.buttons button {
		height: 52px;
		border: 0;
		border-radius: var(--radius-pill);
		font: inherit;
		font-size: 15px;
		cursor: pointer;
	}

	.cancel {
		padding: 0 22px;
		background: transparent;
		color: var(--text-secondary);
		font-weight: 600;
	}

	.add {
		max-width: 100%;
		padding: 0 26px;
		overflow: hidden;
		text-overflow: ellipsis;
		white-space: nowrap;
		background: var(--fox);
		color: var(--fox-ink);
		font-weight: 700;
	}

	.add:disabled {
		opacity: 0.45;
		cursor: default;
	}

	@keyframes rise {
		from {
			opacity: 0;
			transform: translateY(16px) scale(0.98);
		}
	}

	@media (max-width: 720px) {
		.picker {
			padding: 24px 20px;
		}

		h2 {
			font-size: 28px;
		}
	}

	@media (prefers-reduced-motion: reduce) {
		dialog[open] {
			animation: none;
		}
	}
</style>
