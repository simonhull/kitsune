<!--
	The toast an add to a playlist, or a take-out, leaves above the player island: what it did, Add it
	again when songs were skipped, Undo for exactly that, and a fox line draining over its 8 seconds.
	It and Trash's are never up at once: the newer takes the other's place.
-->
<script lang="ts">
	import { untrack } from 'svelte';
	import { playlistActions, playlistToastMs } from '#lib/pages/playlistActions.svelte.ts';
	import { trashBin } from '#lib/pages/trashBin.svelte.ts';
	import { floorElements, floors } from './floor.svelte.ts';
	import Icon from './Icon.svelte';

	const toast = $derived(playlistActions.toast);

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
		if (playlistActions.toast?.id !== undefined) {
			untrack(() => {
				trashBin.dismiss();
			});
		}
	});
	$effect(() => {
		if (trashBin.toast?.id !== undefined) {
			untrack(() => {
				playlistActions.dismiss();
			});
		}
	});
</script>

{#if toast !== null}
	{#key toast.id}
		<div
			class="toast"
			class:failed={toast.state === 'failed'}
			role="status"
			style:bottom={`calc(${String(lift)}px + env(safe-area-inset-bottom))`}
			style:--drain={`${String(playlistToastMs)}ms`}
		>
			<span class="words"
				>{#each toast.words as part, i (i)}{#if part.bold === true}<b>{part.text}</b
						>{:else}{part.text}{/if}{/each}</span
			>
			{#if toast.state === 'done'}
				{#if toast.again !== null}
					<button
						type="button"
						class="again"
						onclick={() => {
							playlistActions.again();
						}}>{toast.again}</button
					>
				{/if}
				<button
					type="button"
					class="undo"
					onclick={() => {
						playlistActions.undo();
					}}>Undo</button
				>
			{:else}
				<button
					type="button"
					class="close"
					aria-label="Dismiss"
					onclick={() => {
						playlistActions.dismiss();
					}}><Icon name="close" size={14} /></button
				>
			{/if}
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
		animation: toast-in 260ms var(--curve-settle);
	}

	@keyframes toast-in {
		from {
			opacity: 0;
			transform: translate(-50%, 16px);
		}
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

	button {
		flex: none;
		height: 36px;
		border: 0;
		border-radius: var(--radius-pill);
		font: 700 14px var(--font-sans);
	}

	.again {
		padding: 0 14px;
		background: transparent;
		color: var(--text);
		font-weight: 600;
	}

	.again:hover {
		background: var(--hover);
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
		.toast {
			animation-duration: 120ms;
		}

		.drain {
			animation: none;
		}
	}
</style>
