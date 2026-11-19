<script lang="ts">
	import { onMount, tick, untrack, type Snippet } from 'svelte';
	import { floorElements } from '#lib/shell/floor.svelte.ts';
	import type { Rect } from '#lib/morph/bud.ts';
	import { Surface } from '#lib/morph/surface.ts';

	interface Props {
		/** Where it opens: the pointer, or the corner of what opened it. */
		x: number;
		y: number;
		label: string;
		/** Its width at the size step 1: a 4K screen takes the page's step with it. */
		width: number;
		onclose: (refocus: boolean) => void;
		children: Snippet;
		/**
		 * It opens from an item of another menu, beside it: a press inside it doesn't close that menu,
		 * and ← takes it back.
		 */
		submenu?: boolean;
		/** `y` is where its bottom goes, as a popover above a bar has it. */
		above?: boolean;
		/**
		 * Which face it shows. A new face (a menu item into its confirm, File info, a phone's list) is
		 * the same box reshaped in place, never a second surface.
		 */
		face?: string;
		/**
		 * The whole width of a phone, clear of the island: what a sheet used to show, a confirm or a
		 * list, now the menu itself in place.
		 */
		full?: boolean;
		/** Where it is born, when that isn't the control just pressed: a submenu's chevron. */
		from?: Rect | undefined;
		/** Names it, so one opened again while it gives way takes over from where it had got to. */
		name?: string | undefined;
		/** How its box is drawn: a menu's, or a confirm's fox edge and misprint. */
		look?: 'menu' | 'confirm';
		/** What it is to assistive technology: a menu, or a confirm's dialog in its place. */
		role?: 'menu' | 'dialog';
	}

	let {
		x,
		y,
		label,
		width,
		onclose,
		children,
		submenu = false,
		above = false,
		face = '',
		full = false,
		from,
		name,
		look = 'menu',
		role = 'menu'
	}: Props = $props();

	/** Clear of the screen's edges by this much, and on a phone of the island by `floorGap`. */
	const margin = 12;
	const gutter = 16;
	const floorGap = 12;
	/** Where a screen is a phone's. */
	const phoneWidth = 720;

	const surface = untrack(() => new Surface(name === undefined ? {} : { key: name }));

	let panel: HTMLElement | undefined = $state();
	let left = $state(0);
	let top = $state(0);
	let shown = $state(0);
	let ceiling = $state<number | null>(null);
	let placed = $state(false);

	/** The page's size step: from 2560 a surface is a step larger with the page around it. */
	function step(): number {
		return (
			Number.parseFloat(getComputedStyle(document.documentElement).getPropertyValue('--step')) || 1
		);
	}

	/** The highest thing at the foot of the screen, the island or the selection bar. */
	function floorTop(): number {
		let lowest = innerHeight;
		for (const element of floorElements) {
			const box = element.getBoundingClientRect();
			if (box.height > 0) {
				lowest = Math.min(lowest, box.top);
			}
		}
		return lowest;
	}

	/** Opens where it was asked to, pulled back inside the screen. */
	function place(): void {
		if (panel === undefined) {
			return;
		}
		if (full) {
			// A phone's whole width, standing where the menu stood, clear of the island.
			const bottom = floorTop() - floorGap;
			shown = innerWidth - gutter * 2;
			ceiling = Math.max(120, bottom - margin);
			panel.style.width = `${String(shown)}px`;
			panel.style.maxHeight = `${String(ceiling)}px`;
			const { height } = panel.getBoundingClientRect();
			left = gutter;
			top = Math.max(margin, Math.min(y, bottom - height));
			placed = true;
			return;
		}
		ceiling = null;
		panel.style.removeProperty('max-height');
		shown = width * step();
		panel.style.width = `${String(shown)}px`;
		const { height } = panel.getBoundingClientRect();
		// On a phone it stays clear of the island, in the thumb's reach with the player live.
		const bottom = innerWidth <= phoneWidth ? floorTop() - floorGap : innerHeight - margin;
		left = Math.max(margin, Math.min(x, innerWidth - shown - margin));
		top = Math.max(margin, Math.min(above ? y - height : y, bottom - height));
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
		if (
			event.key === 'Escape' ||
			(submenu &&
				event.key === 'ArrowLeft' &&
				!(event.target instanceof HTMLInputElement && event.target.value !== ''))
		) {
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

	// A new face: the words it shows now are held where they stand before they change, and once the
	// new face is placed the same box reshapes to it.
	let faced = untrack(() => `${face}|${String(width)}|${String(full)}|${look}`);
	$effect.pre(() => {
		const next = `${face}|${String(width)}|${String(full)}|${look}`;
		if (next === untrack(() => faced) || !untrack(() => placed)) {
			return;
		}
		faced = next;
		untrack(() => {
			surface.hold();
		});
		void tick().then(async () => {
			place();
			await tick();
			surface.reshape();
			if (!(panel?.contains(document.activeElement) ?? false)) {
				items().at(0)?.focus();
			}
		});
	});

	onMount(() => {
		place();
		void tick().then(() => {
			if (panel !== undefined) {
				surface.open(panel, from ?? null);
			}
			items().at(0)?.focus();
		});
		function outside(event: PointerEvent): void {
			const inSubmenu =
				event.target instanceof Element && event.target.closest('[data-submenu]') !== null;
			if (
				panel !== undefined &&
				event.target instanceof Node &&
				!panel.contains(event.target) &&
				!inSubmenu
			) {
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
			surface.leave();
		};
	});
</script>

<div
	class="floating"
	class:placed
	class:full
	class:confirm={look === 'confirm'}
	data-surface
	data-submenu={submenu ? '' : undefined}
	{role}
	aria-label={label}
	tabindex="-1"
	bind:this={panel}
	style:left={`${String(left)}px`}
	style:top={`${String(top)}px`}
	style:width={`${String(shown)}px`}
	style:max-height={ceiling === null ? undefined : `${String(ceiling)}px`}
	{onkeydown}
	oncontextmenu={(event) => {
		event.preventDefault();
	}}
>
	<div class="faces">{@render children()}</div>
</div>

<style>
	.floating {
		position: fixed;
		/* Over the player island, which it may open across on a short screen. */
		z-index: 6;
		max-height: calc(100dvh - 24px);
		overflow-y: auto;
		box-sizing: border-box;
		padding: calc(8px * var(--step));
		border-radius: calc(var(--radius-20) * var(--step));
		background: var(--card);
		border: 1px solid var(--edge-raised);
		box-shadow: var(--shadow-float);
		visibility: hidden;
	}

	.floating.placed {
		visibility: visible;
	}

	/* What it holds takes the page's size step, every length in it at once. */
	.faces {
		display: flex;
		flex-direction: column;
		zoom: var(--step);
	}

	/* A confirm: the fox edge and its misprint, the shadows the menu's box reshapes into. */
	.floating.confirm {
		padding: calc(20px * var(--step)) calc(22px * var(--step));
		border: 1.5px solid var(--fox-line);
		box-shadow:
			3px 3px 0 color-mix(in srgb, var(--accent, var(--fox)) 50%, transparent),
			var(--shadow-float);
	}

	.floating:focus {
		outline: none;
	}
</style>
