<!--
	The toast an album's ReplayGain run leaves above the player island once its files are written
	(board D): how many, of which album, and Undo for exactly that run, with a fox line draining over
	its 8 seconds. An undo's own end, and a refusal, say so here too.
-->
<script lang="ts">
	import { replayGain, toastMs } from '#lib/replaygain/gain.svelte.ts';
	import { floorElements, floors } from './floor.svelte.ts';
	import Icon from './Icon.svelte';

	const toast = $derived(replayGain.toast);

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
			class:failed={toast.failed}
			role="status"
			data-testid="gain-toast"
			style:bottom={`calc(${String(lift)}px + env(safe-area-inset-bottom))`}
			style:--drain={`${String(toastMs)}ms`}
		>
			<span class="words"
				>{toast.words}{#if toast.album !== ''}&nbsp;<i>{toast.album}</i>{/if}</span
			>
			{#if toast.undo !== undefined}
				<button
					type="button"
					class="undo"
					onclick={() => {
						replayGain.undoToast();
					}}>Undo</button
				>
			{:else}
				<button
					type="button"
					class="close"
					aria-label="Dismiss"
					onclick={() => {
						replayGain.dismiss();
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
		max-width: min(640px, calc(100vw - 32px));
		box-sizing: border-box;
		display: flex;
		align-items: center;
		gap: 14px;
		padding: 10px 10px 10px 20px;
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

	button {
		flex: none;
		height: 36px;
		border: 0;
		border-radius: var(--radius-pill);
		font: 700 14px var(--font-sans);
		cursor: pointer;
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
