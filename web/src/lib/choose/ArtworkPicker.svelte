<!--
	Choose artwork: the ⋯ menu reshaped in place. One strip of kinds; every candidate with its source,
	size and credit, the chosen one ringed off register, a pin stamped; a tile to upload one of your
	own; Automatic (the pin's way out) and Not this artist… at its foot. Pinning shows on the page at
	once: the stage behind develops the new picture in place.
-->
<script lang="ts">
	import Icon from '#lib/shell/Icon.svelte';
	import CandidateTile from './CandidateTile.svelte';
	import { chooser, type ChooserOpen } from './chooser.svelte.ts';
	import { artistImageKinds, type ArtistImageKind } from './types.ts';
	import {
		askingWords,
		autoNote,
		identityLine,
		kindLabels,
		nothingWords,
		sourceNotes,
		uploadCaption
	} from './words.ts';

	interface Props {
		open: ChooserOpen;
		phone: boolean;
		/** Not this artist… was pressed: the picker's foot, where the confirm goes. */
		onconfirm: (bottom: number) => void;
	}

	let { open, phone, onconfirm }: Props = $props();

	const kind = $derived(chooser.kind);
	const load = $derived(chooser.sets.get(kind) ?? { kind: 'loading' as const });
	const set = $derived(load.kind === 'ready' ? load.value : undefined);
	const pinned = $derived(set?.pinned ?? false);
	const notes = $derived(set === undefined ? [] : sourceNotes(set));
	const busy = $derived(chooser.busy);

	let root: HTMLElement | undefined = $state();
	let file: HTMLInputElement | undefined = $state();

	/** The strip's keys: ← and → move between the kinds, as a tab list's do. */
	function stripKey(event: KeyboardEvent, at: number): void {
		if (event.key !== 'ArrowLeft' && event.key !== 'ArrowRight') {
			return;
		}
		event.preventDefault();
		const step = event.key === 'ArrowLeft' ? -1 : 1;
		const next = artistImageKinds.at((at + step) % artistImageKinds.length) ?? 'thumb';
		chooser.show(next);
		root?.querySelector<HTMLElement>(`[data-kind="${next}"]`)?.focus();
	}

	function choose(next: ArtistImageKind): void {
		chooser.show(next);
	}
</script>

<div class="picker" class:phone bind:this={root} data-testid="artwork-picker">
	<div class="head">
		<div class="titles">
			<h2>Choose artwork</h2>
			<p class="id">{identityLine(open.name, open.mbid, phone)}</p>
		</div>
		<button
			type="button"
			class="close"
			aria-label="Close the picker"
			data-menu-item
			onclick={() => {
				chooser.close(true);
			}}><Icon name="close" size={18} /></button
		>
	</div>

	<div class="kinds" role="tablist" aria-label="Kind of picture">
		{#each artistImageKinds as each, at (each)}
			<button
				type="button"
				role="tab"
				class="kind"
				data-kind={each}
				data-menu-item
				aria-selected={each === kind}
				tabindex={each === kind ? 0 : -1}
				onclick={() => {
					choose(each);
				}}
				onkeydown={(event) => {
					stripKey(event, at);
				}}>{kindLabels[each]}</button
			>
		{/each}
	</div>

	{#if notes.length > 0}
		<div class="notes" role="status">
			{#each notes as note (note)}<p>{note}</p>{/each}
		</div>
	{/if}

	<div
		class="grid"
		role="tabpanel"
		aria-label={kindLabels[kind]}
		aria-busy={load.kind === 'loading'}
	>
		{#if load.kind === 'loading'}
			{#each [0, 1, 2] as n (n)}
				<div class={['waiting', kind]} aria-hidden="true">
					<span class="ghost"></span>
					<span class="ghost-line"></span>
				</div>
			{/each}
			<p class="asking" role="status">{askingWords}</p>
		{:else if load.kind === 'failed'}
			<p class="asking failed" role="status">{load.words}</p>
		{:else if set !== undefined}
			{#each set.candidates as candidate (candidate.id)}
				<CandidateTile
					{candidate}
					{kind}
					{phone}
					busy={busy?.what === 'pin' && busy.id === candidate.id}
					waiting={busy !== null}
					onpin={() => void chooser.pin(candidate)}
				/>
			{/each}
		{/if}
		{#if load.kind !== 'loading'}
			<div class={['upload', kind]}>
				<button
					type="button"
					class="drop"
					data-menu-item
					disabled={busy !== null}
					onclick={() => file?.click()}
				>
					<Icon name="upload" size={22} />
					<span>{busy?.what === 'upload' ? 'Uploading…' : 'Upload an image…'}</span>
				</button>
				<span class="caption">{uploadCaption}</span>
				<input
					bind:this={file}
					type="file"
					accept="image/jpeg,image/png,image/webp"
					aria-label="Upload an image"
					hidden
					onchange={(event) => {
						const chosen = event.currentTarget.files?.[0];
						event.currentTarget.value = '';
						if (chosen !== undefined) {
							void chooser.upload(chosen);
						}
					}}
				/>
			</div>
		{/if}
		{#if set !== undefined && set.candidates.length === 0}
			<p class="nothing">{nothingWords}</p>
		{/if}
	</div>

	<div class="foot">
		<button
			type="button"
			class="auto"
			class:on={!pinned}
			data-menu-item
			aria-pressed={!pinned}
			disabled={busy !== null || !pinned}
			onclick={() => void chooser.automatic()}
			>{busy?.what === 'automatic' ? 'Unpinning…' : pinned ? 'Automatic' : 'Automatic ✓'}</button
		>
		{#if !phone}<span class="auto-note">{autoNote(pinned)}</span>{/if}
		{#if open.mbid !== undefined && set?.held !== true}
			<button
				type="button"
				class="not"
				data-menu-item
				disabled={busy !== null}
				onclick={() => {
					onconfirm(root?.getBoundingClientRect().bottom ?? innerHeight);
				}}>Not this artist…</button
			>
		{/if}
	</div>
</div>

<style>
	.picker {
		/* Tall enough for two rows, never past the screen: the grid scrolls, the foot stays. */
		height: min(612px, calc(100dvh / var(--step) - 220px));
		min-height: 360px;
		display: flex;
		flex-direction: column;
		gap: 18px;
		padding: 12px 12px 8px;
	}

	.head {
		display: flex;
		align-items: flex-start;
		justify-content: space-between;
		gap: 16px;
	}

	.titles {
		min-width: 0;
	}

	h2 {
		font-size: 26px;
		font-weight: 800;
		letter-spacing: -0.03em;
		line-height: 1.1;
	}

	.id {
		margin-top: 8px;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font: 500 11px var(--font-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
	}

	.close {
		flex: none;
		width: 44px;
		height: 44px;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: 50%;
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--edge-strong);
		color: var(--text);
	}

	.close:hover {
		background: var(--hover);
	}

	/* The kinds: one strip, its fill on the kind shown. */
	.kinds {
		align-self: flex-start;
		display: flex;
		gap: 4px;
		padding: 4px;
		border-radius: var(--radius-pill);
		box-shadow: inset 0 0 0 1px var(--edge);
	}

	.kind {
		height: 34px;
		padding: 0 16px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: var(--text-secondary);
		font: 600 13.5px var(--font-sans);
		transition:
			background-color var(--morph-fast) ease,
			color var(--morph-fast) ease;
	}

	.kind[aria-selected='true'] {
		background: var(--fill);
		color: var(--text);
		font-weight: 700;
	}

	.kind:hover:not([aria-selected='true']) {
		color: var(--text);
	}

	.notes p {
		font: 500 10.5px var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--text-label);
	}

	.grid {
		flex: 1;
		min-height: 0;
		display: grid;
		grid-template-columns: repeat(3, minmax(0, 1fr));
		align-content: start;
		gap: 16px;
		/* Room for the ring outside each tile, and the misprint behind it. */
		margin: -8px;
		padding: 8px;
		overflow-y: auto;
		overscroll-behavior: contain;
	}

	.waiting {
		display: flex;
		flex-direction: column;
		gap: 8px;
	}

	.ghost {
		aspect-ratio: 1;
		border-radius: var(--radius-16);
		background: var(--card-raised);
		box-shadow: inset 0 0 0 1px var(--edge-card);
	}

	.background .ghost {
		aspect-ratio: 16 / 9;
	}

	.banner .ghost {
		aspect-ratio: 1000 / 185;
	}

	.logo .ghost {
		aspect-ratio: 2.4;
	}

	.ghost-line {
		width: 60%;
		height: 10px;
		border-radius: 3px;
		background: var(--hover);
	}

	.asking,
	.nothing {
		grid-column: 1 / -1;
		font: 500 11px var(--font-mono);
		letter-spacing: 0.08em;
		color: var(--text-meta);
		text-transform: uppercase;
	}

	.asking.failed {
		color: var(--danger);
	}

	.upload {
		display: flex;
		flex-direction: column;
		gap: 4px;
		min-width: 0;
	}

	.drop {
		width: 100%;
		aspect-ratio: 1;
		display: flex;
		flex-direction: column;
		align-items: center;
		justify-content: center;
		gap: 10px;
		border: 0;
		border-radius: var(--radius-16);
		background: transparent;
		box-shadow: inset 0 0 0 1.5px var(--edge-strong);
		color: var(--text-secondary);
		font: 600 13px var(--font-sans);
		cursor: pointer;
	}

	.background .drop {
		aspect-ratio: 16 / 9;
	}

	.banner .drop {
		aspect-ratio: 1000 / 185;
		flex-direction: row;
	}

	.logo .drop {
		aspect-ratio: 2.4;
	}

	.drop:hover:not(:disabled) {
		background: var(--hover);
		color: var(--text);
	}

	.drop:disabled {
		cursor: progress;
	}

	.caption {
		margin-top: 5px;
		font: 500 10.5px var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--text-label);
	}

	.foot {
		display: flex;
		align-items: center;
		gap: 14px;
	}

	.auto,
	.not {
		flex: none;
		height: 42px;
		padding: 0 22px;
		border: 0;
		border-radius: var(--radius-pill);
		font: 600 13.5px var(--font-sans);
		cursor: pointer;
	}

	.auto {
		min-width: 132px;
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--fox-line);
		color: var(--fox-text);
	}

	.auto.on {
		background: var(--fill);
		box-shadow: inset 0 0 0 1px var(--edge-strong);
		color: var(--text);
		cursor: default;
	}

	.auto:hover:not(:disabled) {
		background: color-mix(in srgb, var(--fox) 12%, transparent);
	}

	.auto:disabled:not(.on) {
		cursor: progress;
	}

	.auto-note {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font: 500 10.5px var(--font-mono);
		letter-spacing: 0.06em;
		color: var(--text-label);
	}

	.not {
		margin-left: auto;
		min-width: 170px;
		background: transparent;
		box-shadow: inset 0 0 0 1px var(--danger-edge);
		color: var(--danger);
	}

	.not:hover:not(:disabled) {
		background: var(--danger-wash);
	}

	.not:disabled {
		opacity: 0.5;
		cursor: progress;
	}

	/* A phone: the menu at its width less its gutters, two across, held clear of the island. */
	.phone {
		height: min(632px, calc(100dvh - 210px));
		gap: 14px;
		padding: 8px 6px 4px;
	}

	.phone h2 {
		font-size: 22px;
	}

	.phone .kind {
		padding: 0 12px;
		font-size: 12.5px;
	}

	.phone .grid {
		grid-template-columns: repeat(2, minmax(0, 1fr));
		gap: 12px;
	}

	.phone .auto,
	.phone .not {
		min-width: 0;
		padding: 0 18px;
	}
</style>
