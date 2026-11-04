<script lang="ts">
	import { currentEngine, engineNames, setEngine } from './engine.ts';
	import { hud } from './hud.svelte.ts';

	let active = $state(currentEngine().name);
</script>

<!-- Dev only: the two engines are being compared by feel, and this is the switch. -->
<div class="hud">
	{#each engineNames as name (name)}
		<button
			type="button"
			aria-pressed={active === name}
			onclick={() => {
				setEngine(name);
				active = name;
			}}>{name}</button
		>
	{/each}
	{#if hud.startMs !== null}
		<span>{hud.startMs.toFixed(0)} ms</span>
	{/if}
</div>

<style>
	.hud {
		position: fixed;
		left: 12px;
		bottom: 12px;
		z-index: 6;
		display: flex;
		align-items: center;
		gap: 2px;
		padding: 3px;
		border-radius: var(--radius-pill);
		background: var(--glass-fill);
		border: 1px solid var(--edge);
		font-family: var(--font-mono);
		font-size: 11px;
		color: var(--text-meta);
	}

	button {
		min-height: 28px;
		padding: 0 10px;
		border: 0;
		border-radius: var(--radius-pill);
		background: transparent;
		color: inherit;
	}

	button[aria-pressed='true'] {
		background: var(--fox);
		color: var(--fox-ink);
	}

	span {
		padding: 0 8px;
	}
</style>
