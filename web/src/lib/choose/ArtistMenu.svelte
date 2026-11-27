<!--
	The artist hero's ⋯, for administrators: its menu (Choose artwork…, Look again now, Not this
	artist…), the picker Choose artwork… reshapes it into, and Not this artist's confirm. One surface
	whose face changes in place (M4), never a second one or a sheet: on a phone the picker and the
	confirm are the menu itself at the phone's width, clear of the island.
-->
<script lang="ts">
	import { MediaQuery } from 'svelte/reactivity';
	import Floating from '#lib/pages/Floating.svelte';
	import ArtworkPicker from './ArtworkPicker.svelte';
	import { chooser } from './chooser.svelte.ts';
	import NotThisArtist from './NotThisArtist.svelte';

	const open = $derived(chooser.current);
	const face = $derived(chooser.face);
	const phone = new MediaQuery('(max-width: 720px)', false);

	/** Each face's width at the size step 1. */
	const widths = { menu: 300, picker: 880, confirm: 520 } as const;
	const width = $derived(widths[face]);

	/** Where the confirm's foot goes when the picker opened it: the picker's own foot. */
	let pickerBottom = $state<number | null>(null);

	/** The page's size step: a 4K screen's surfaces are a step larger. */
	function step(): number {
		return (
			Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--step')) || 1
		);
	}

	/**
	 * Each face keeps to the ⋯'s side: the menu under it (over it, on a phone), the picker below the
	 * nav, the confirm where the picker's foot was, the button that asked for it.
	 */
	const place = $derived.by(() => {
		if (open === null) {
			return { x: 0, y: 0, above: false };
		}
		const { anchor } = open;
		const k = step();
		const x = anchor.right - width * k;
		switch (face) {
			case 'menu':
				return phone.current
					? { x, y: anchor.top - 6, above: true }
					: { x, y: anchor.bottom + 8, above: false };
			case 'picker':
				return { x, y: phone.current ? 100 : 104 * k, above: false };
			case 'confirm':
				return pickerBottom === null
					? { x, y: anchor.bottom + 8, above: false }
					: { x, y: pickerBottom, above: true };
		}
	});
</script>

{#if open !== null}
	<Floating
		x={place.x}
		y={place.y}
		above={place.above}
		label={face === 'picker'
			? 'Choose artwork'
			: face === 'confirm'
				? 'Not this artist'
				: `More for ${open.name}`}
		{width}
		{face}
		full={face !== 'menu' && phone.current}
		look={face === 'confirm' ? 'danger' : 'menu'}
		role={face === 'menu' ? 'menu' : 'dialog'}
		name="artist-menu"
		onclose={(refocus) => {
			if (chooser.busy !== null) {
				return;
			}
			// Esc on the confirm goes back to where it came from; a press elsewhere closes it all.
			if (refocus && face === 'confirm') {
				chooser.keep();
				return;
			}
			chooser.close(refocus);
		}}
	>
		{#if face === 'picker'}
			<ArtworkPicker
				{open}
				phone={phone.current}
				onconfirm={(bottom) => {
					pickerBottom = bottom;
					chooser.confirm();
				}}
			/>
		{:else if face === 'confirm'}
			<NotThisArtist {open} phone={phone.current} />
		{:else}
			<p class="note">ARTWORK · ADMINS</p>
			<button
				type="button"
				role="menuitem"
				class="item"
				data-menu-item
				onclick={() => {
					chooser.choose();
				}}>Choose artwork…</button
			>
			{#if open.mbid !== undefined}
				<button
					type="button"
					role="menuitem"
					class="item"
					data-menu-item
					onclick={() => void chooser.lookAgain()}>Look again now</button
				>
				<button
					type="button"
					role="menuitem"
					class="item danger"
					data-menu-item
					onclick={() => {
						pickerBottom = null;
						chooser.confirm();
					}}>Not this artist…</button
				>
			{/if}
		{/if}
	</Floating>
{/if}

<style>
	.note {
		padding: 6px 12px 4px;
		font-family: var(--font-mono);
		font-size: 10.5px;
		letter-spacing: 0.08em;
		color: var(--text-label);
	}

	.item {
		height: 44px;
		padding: 0 12px;
		display: flex;
		align-items: center;
		border: 0;
		border-radius: var(--radius-12);
		background: transparent;
		color: var(--text);
		font: 600 15px var(--font-sans);
		text-align: left;
	}

	.item:hover,
	.item:focus-visible {
		background: var(--hover);
	}

	.danger {
		color: var(--danger);
	}

	.danger:hover,
	.danger:focus-visible {
		background: var(--danger-wash);
	}
</style>
