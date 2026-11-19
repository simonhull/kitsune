<!--
	The toast an album's or a selection's Organize leaves when nothing would move: the songs are
	where the template puts them already, and See the template opens Organize.
-->
<script lang="ts">
	import { resolve } from '$app/paths';
	import { organizeEntry, toastMs } from '#lib/organize/entry.svelte.ts';
	import { floorElements, floors } from './floor.svelte.ts';
	import Icon from './Icon.svelte';
	import { reshapeOn } from '#lib/morph/reshapeOn.svelte.ts';
	import { SurfaceSlot } from '#lib/morph/surface.ts';

	const toast = $derived(organizeEntry.toast);

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
			role="status"
			style:bottom={`calc((${String(lift)}px + env(safe-area-inset-bottom)) / var(--step))`}
			style:--drain={`${String(toastMs)}ms`}
		>
			<span class="dot" aria-hidden="true"></span>
			<span class="words">{toast.words}</span>
			{#if toast.template}
				<a
					class="go"
					href={resolve('/server/organize')}
					onclick={() => {
						organizeEntry.dismiss();
					}}>See the template ›</a
				>
			{/if}
			<button
				type="button"
				class="close"
				aria-label="Dismiss"
				onclick={() => {
					organizeEntry.dismiss();
				}}><Icon name="close" size={14} /></button
			>
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
		max-width: min(640px, calc(100vw - 32px));
		box-sizing: border-box;
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 10px 10px 10px 18px;
		border-radius: var(--radius-16);
		overflow: hidden;
		background: var(--card-raised);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
		transform: translateX(-50%);
		/* A step larger at 4K, with the page: its lift is divided back out. */
		zoom: var(--step);
	}

	.dot {
		flex: none;
		width: 8px;
		height: 8px;
		border-radius: 50%;
		background: var(--fox);
	}

	.words {
		min-width: 0;
		font-size: 14px;
	}

	.go {
		flex: none;
		font-size: 14px;
		font-weight: 700;
		color: var(--fox-text);
		text-decoration: none;
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
		cursor: pointer;
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
