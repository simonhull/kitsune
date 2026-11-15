<!--
	The smart mark: a "SMART" stamp on a smart playlist's mosaic, lower left, a touch off register:
	mono in the fox with a fox line, turned −3°, and a teal misprint behind it. It's the only thing
	that tells a smart card from a regular one.
-->
<script lang="ts">
	import Icon from '#lib/shell/Icon.svelte';

	interface Props {
		/** Card, the page's 200px mosaic, or a phone's 112px one. */
		size?: 'card' | 'page' | 'phone';
	}

	let { size = 'card' }: Props = $props();
</script>

<span class={['stamp', size]}
	>{#if size === 'card'}<Icon name="rules" size={12} />{/if}SMART</span
>

<style>
	.stamp {
		position: absolute;
		left: calc(12px * var(--step));
		bottom: calc(12px * var(--step));
		display: flex;
		align-items: center;
		gap: 6px;
		height: calc(28px * var(--step));
		padding: 0 calc(10px * var(--step));
		box-sizing: border-box;
		border-radius: 8px;
		border: 1.5px solid var(--fox-line);
		background: color-mix(in srgb, var(--ground) 88%, transparent);
		color: var(--fox-text);
		font-family: var(--font-mono);
		font-size: calc(11px * var(--step));
		font-weight: 700;
		letter-spacing: 0.12em;
		transform: rotate(-3deg);
		box-shadow: 2px 2px 0 color-mix(in srgb, var(--misprint) 55%, transparent);
		pointer-events: none;
	}

	.page {
		height: calc(26px * var(--step));
		padding: 0 calc(9px * var(--step));
		font-size: calc(10px * var(--step));
	}

	.phone {
		left: 7px;
		bottom: 7px;
		height: 20px;
		padding: 0 6px;
		border-radius: 6px;
		font-size: 9px;
	}
</style>
