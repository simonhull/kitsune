<script lang="ts">
	import { hud } from './hud.svelte.ts';
</script>

<!-- Dev only: the motion budget says a morph starts within two frames of the input. -->
{#if hud.startMs !== null}
	<div class="hud">{hud.startMs.toFixed(0)} ms</div>
{/if}

<style>
	.hud {
		position: fixed;
		left: 12px;
		bottom: 12px;
		z-index: 6;
		padding: 6px 10px;
		border-radius: var(--radius-pill);
		background: var(--glass-fill);
		border: 1px solid var(--edge);
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
	}
</style>
