<script lang="ts">
	import { onMount, tick, type Snippet } from 'svelte';

	interface Props {
		/** Where it opens: the pointer, or the corner of what opened it. */
		x: number;
		y: number;
		label: string;
		width: number;
		onclose: (refocus: boolean) => void;
		children: Snippet;
	}

	let { x, y, label, width, onclose, children }: Props = $props();

	/** Clear of the screen's edges by this much. */
	const margin = 12;

	let panel: HTMLElement | undefined = $state();
	let left = $state(0);
	let top = $state(0);
	let placed = $state(false);

	/** Opens where it was asked to, pulled back inside the screen. */
	function place(): void {
		if (panel === undefined) {
			return;
		}
		const { height } = panel.getBoundingClientRect();
		left = Math.max(margin, Math.min(x, innerWidth - width - margin));
		top = Math.max(margin, Math.min(y, innerHeight - height - margin));
		placed = true;
	}

	/** The menu's items, for the arrows: buttons, boxes and anything else that takes focus. */
	function items(): HTMLElement[] {
		return panel === undefined
			? []
			: [...panel.querySelectorAll<HTMLElement>('[data-menu-item]')].filter(
					(item) => !item.hasAttribute('disabled')
				);
	}

	function onkeydown(event: KeyboardEvent): void {
		if (event.key === 'Escape') {
			event.preventDefault();
			event.stopPropagation();
			onclose(true);
			return;
		}
		if (event.key === 'Tab') {
			// A menu holds the focus until it closes, as a menu does.
			event.preventDefault();
		}
		if (
			event.altKey ||
			(event.key !== 'ArrowDown' && event.key !== 'ArrowUp' && event.key !== 'Tab')
		) {
			return;
		}
		event.preventDefault();
		const all = items();
		const at = all.findIndex(
			(item) => item === document.activeElement || item.contains(document.activeElement)
		);
		const back = event.key === 'ArrowUp' || (event.key === 'Tab' && event.shiftKey);
		const next = all.at((at + (back ? -1 : 1) + all.length) % all.length);
		next?.focus();
	}

	onMount(() => {
		place();
		void tick().then(() => items().at(0)?.focus());
		function outside(event: PointerEvent): void {
			if (panel !== undefined && event.target instanceof Node && !panel.contains(event.target)) {
				onclose(false);
			}
		}
		const away = (): void => {
			onclose(false);
		};
		addEventListener('pointerdown', outside, true);
		addEventListener('resize', away);
		return () => {
			removeEventListener('pointerdown', outside, true);
			removeEventListener('resize', away);
		};
	});
</script>

<div
	class="floating"
	class:placed
	role="menu"
	aria-label={label}
	tabindex="-1"
	bind:this={panel}
	style:left={`${String(left)}px`}
	style:top={`${String(top)}px`}
	style:width={`${String(width)}px`}
	{onkeydown}
	oncontextmenu={(event) => {
		event.preventDefault();
	}}
>
	{@render children()}
</div>

<style>
	.floating {
		position: fixed;
		/* Over the player island, which it may open across on a short screen. */
		z-index: 6;
		max-height: calc(100dvh - 24px);
		overflow-y: auto;
		box-sizing: border-box;
		padding: 8px;
		display: flex;
		flex-direction: column;
		border-radius: var(--radius-20);
		background: var(--card);
		border: 1px solid rgb(255 255 255 / 10%);
		box-shadow: 0 24px 64px rgb(0 0 0 / 60%);
		visibility: hidden;
		opacity: 0;
		transform: translateY(-4px) scale(0.98);
		transform-origin: top left;
	}

	.floating.placed {
		visibility: visible;
		opacity: 1;
		transform: none;
		transition:
			opacity 140ms var(--curve-morph),
			transform 180ms var(--curve-settle);
	}

	.floating:focus {
		outline: none;
	}

	@media (prefers-reduced-motion: reduce) {
		.floating.placed {
			transition: opacity 120ms linear;
		}
	}
</style>
