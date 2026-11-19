<!--
	The toast a move to Trash leaves above the player island: what moved, Undo for exactly that, and
	a fox line draining over its 8 seconds. A second move replaces it, and the first stays in Trash.
-->
<script lang="ts">
	import { toastMs, trashBin } from '#lib/pages/trashBin.svelte.ts';
	import { floorElements, floors } from './floor.svelte.ts';
	import Icon from './Icon.svelte';
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { SurfaceSlot } from '#lib/morph/surface.ts';

	const toast = $derived(trashBin.toast);

	// Born at the floor at its own width; Undo makes it what it did, in place; replaced, it gives way
	// under the next, which is born in front of it.
	const surface = new SurfaceSlot({ floor: true, quick: true });
	const face = $derived(toast === null ? '' : JSON.stringify(toast));
	reshapeOn(
		surface,
		() => toast?.id,
		() => face
	);

	/** How far up it sits: above the highest thing at the foot of the screen. */
	let lift = $state(24);

	$effect(() => {
		void floors.moved;
		void floorElements.size;
		void toast?.id;
		let top = window.innerHeight;
		for (const element of floorElements) {
			const box = element.getBoundingClientRect();
			if (box.height > 0) {
				top = Math.min(top, box.top);
			}
		}
		lift = window.innerHeight - top + 20;
	});
</script>

{#if toast !== null}
	{#key toast.id}
		<div
			class="toast"
			{@attach surface.attach}
			class:failed={toast.state === 'failed'}
			role="status"
			style:bottom={`calc((${String(lift)}px + env(safe-area-inset-bottom)) / var(--step))`}
			style:--drain={`${String(toastMs)}ms`}
		>
			<span class="glyph"><Icon name="trash" size={16} /></span>
			<span class="words"
				>{#if toast.words.name !== null}<b>{toast.words.name}</b>{/if}{toast.words.rest}</span
			>
			{#if toast.state === 'moved'}
				<button
					type="button"
					class="undo"
					onclick={() => {
						trashBin.undo();
					}}>Undo</button
				>
			{:else}
				<button
					type="button"
					class="close"
					aria-label="Dismiss"
					onclick={() => {
						trashBin.dismiss();
					}}><Icon name="close" size={14} /></button
				>
			{/if}
			{#key face}<span class="drain" aria-hidden="true"></span>{/key}
		</div>
	{/key}
{/if}

<style>
	.toast {
		position: fixed;
		z-index: 4;
		left: 50%;
		width: max-content;
		min-width: min(460px, calc(100vw - 32px));
		max-width: min(640px, calc(100vw - 32px));
		box-sizing: border-box;
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 10px 10px 10px 18px;
		border-radius: var(--radius-pill);
		overflow: hidden;
		background: var(--card-raised);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
		transform: translateX(-50%);
		/* A step larger at 4K, with the page: its lift is divided back out. */
		zoom: var(--step);
	}

	.glyph {
		display: grid;
		color: var(--text-meta);
	}

	.failed .glyph {
		color: var(--danger);
	}

	.words {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: 14px;
	}

	b {
		font-weight: 700;
	}

	button {
		flex: none;
		height: 36px;
		border: 0;
		border-radius: var(--radius-pill);
		font: 700 14px var(--font-sans);
	}

	.undo {
		padding: 0 16px;
		background: color-mix(in srgb, var(--fox) 16%, transparent);
		color: var(--fox-text);
	}

	.undo:hover {
		background: color-mix(in srgb, var(--fox) 24%, transparent);
	}

	.close {
		width: 36px;
		display: grid;
		place-items: center;
		background: var(--hover);
		color: var(--text-secondary);
	}

	.drain {
		position: absolute;
		left: 0;
		right: 0;
		bottom: 0;
		height: 2px;
		transform-origin: left;
		background: var(--fox);
		animation: drain var(--drain) linear forwards;
	}

	.failed .drain {
		background: var(--danger);
	}

	@keyframes drain {
		to {
			transform: scaleX(0);
		}
	}

	@media (prefers-reduced-motion: reduce) {
		.drain {
			animation: none;
		}
	}
</style>
