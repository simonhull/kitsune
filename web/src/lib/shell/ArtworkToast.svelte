<!--
	The toast Choose artwork leaves above the player island: a pin, an unpin, an upload, Look again
	now, Not this artist, or why one failed, with a fox line draining over its 8 seconds. It and the
	other toasts are never up at once: the newer takes the other's place.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import { artworkToastMs, chooser } from '#lib/choose/chooser.svelte.ts';
	import { playlistActions } from '#lib/pages/playlistActions.svelte.ts';
	import { trashBin } from '#lib/pages/trashBin.svelte.ts';
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { SurfaceSlot } from '#lib/morph/surface.ts';
	import { floorElements, floors } from './floor.svelte.ts';
	import Icon from './Icon.svelte';

	const toast = $derived(chooser.toast);

	// Born at the floor at its own width; replaced, it gives way under the next, born in front of it.
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

	// One toast at a time: whichever came last stays.
	$effect(() => {
		if (chooser.toast?.id !== undefined) {
			untrack(() => {
				trashBin.dismiss();
				playlistActions.dismiss();
			});
		}
	});
	$effect(() => {
		if (trashBin.toast?.id !== undefined || playlistActions.toast?.id !== undefined) {
			untrack(() => {
				chooser.dismiss();
			});
		}
	});
</script>

{#if toast !== null}
	{#key toast.id}
		<div
			class="toast"
			{@attach surface.attach}
			class:failed={toast.failed}
			role="status"
			data-testid="artwork-toast"
			style:bottom={`calc((${String(lift)}px + env(safe-area-inset-bottom)) / var(--step))`}
			style:--drain={`${String(artworkToastMs)}ms`}
		>
			<span class="words"
				>{#each toast.words as part, i (i)}{#if part.bold === true}<b>{part.text}</b
						>{:else}{part.text}{/if}{/each}</span
			>
			<button
				type="button"
				class="close"
				aria-label="Dismiss"
				onclick={() => {
					chooser.dismiss();
				}}><Icon name="close" size={14} /></button
			>
			<span class="drain" aria-hidden="true"></span>
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

	.words {
		flex: 1;
		min-width: 0;
		overflow: hidden;
		white-space: nowrap;
		text-overflow: ellipsis;
		font-size: 14px;
	}

	.failed .words {
		color: var(--danger);
	}

	b {
		font-weight: 700;
	}

	.close {
		flex: none;
		width: 36px;
		height: 36px;
		display: grid;
		place-items: center;
		border: 0;
		border-radius: var(--radius-pill);
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
