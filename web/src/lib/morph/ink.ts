import { paletteFor } from '#lib/cover/palette.ts';
import { library } from '#lib/library/library.svelte.ts';
import type { AlbumEntry } from '#lib/library/source.ts';
import { restOf, wobble } from '#lib/pages/deck.ts';
import { drawTitle } from './engines/flier.ts';
import { reducedMotion } from './journey.ts';
import type { MorphEntry } from './registry.ts';

/** Chosen once, from the URL the app opened on, and held for every later navigation. */
export const inked =
	typeof location !== 'undefined' && new URLSearchParams(location.search).has('ink');

/** A plate's whole life, from the hit to gone. */
const plateMs = 100;
/** Where in that life it lets go of its offset, and where it's home and can fade. */
const snapsAt = 0.35;
const homeAt = 0.8;
const closestPixels = 2;
const farthestPixels = 3;

/** How to take a plate down, now: what's on the press must be gone before a page is pictured. */
const onPress = new Set<() => void>();

/** How far off register this album prints, always the same way: every album has its own slip. */
export function offRegister(id: string): { x: number; y: number } {
	const angle = Math.PI * wobble(id, 'ink-angle');
	const share = (1 + wobble(id, 'ink-reach')) / 2;
	const reach = closestPixels + (farthestPixels - closestPixels) * share;
	return { x: reach * Math.cos(angle), y: reach * Math.sin(angle) };
}

/**
 * Runs a plate through its one hit: off register, snapped home, gone. It's the compositor's from
 * here, so it can't hold anything up, and the element's own style keeps it unseen if a frame
 * outlives the animation.
 */
function strike(plate: HTMLElement, id: string, rest: string, lifted?: () => void): void {
	const { x, y } = offRegister(id);
	const off = `translate(${x.toFixed(2)}px, ${y.toFixed(2)}px) ${rest}`;
	const home = `translate(0, 0) ${rest}`;
	plate.setAttribute('aria-hidden', 'true');
	plate.dataset['inkPlate'] = '';
	plate.style.opacity = '0';
	plate.style.pointerEvents = 'none';
	const animation = plate.animate(
		[
			{ transform: off, opacity: 1 },
			{ transform: off, opacity: 1, offset: snapsAt, easing: 'cubic-bezier(0.2, 0, 0, 1)' },
			{ transform: home, opacity: 1, offset: homeAt },
			{ transform: home, opacity: 0 }
		],
		{ duration: plateMs }
	);
	const lift = (): void => {
		onPress.delete(lift);
		animation.cancel();
		plate.remove();
		lifted?.();
	};
	animation.onfinish = lift;
	onPress.add(lift);
}

/** Takes every plate off the press at once, for a page about to be pictured as it stands. */
export function wipePlates(): void {
	for (const lift of onPress) {
		lift();
	}
}

/** Finds the album a landed title belongs to. An artist's name lands too, but it's no headline. */
function albumOfTitle(id: string): AlbumEntry | undefined {
	if (id === 'song') {
		return library.playing?.album;
	}
	return id.startsWith('title:') ? library.entry(id.slice('title:'.length)) : undefined;
}

/** Hits a title that has just landed with a duplicate of itself, behind it, in the album's ink. */
export function printTitle({ id, element }: MorphEntry): void {
	const album = albumOfTitle(id);
	if (album === undefined || !element.isConnected || reducedMotion()) {
		return;
	}
	const plate = drawTitle(element);
	plate.style.color = paletteFor(album).accent;
	element.before(plate);
	// `offsetLeft` rounds, and half a pixel of real misregistration would stay after the snap.
	const { left, top } = plate.getBoundingClientRect();
	const home = element.getBoundingClientRect();
	plate.style.left = `${String(home.left - left)}px`;
	plate.style.top = `${String(home.top - top)}px`;
	// Positioned, it paints in order: over the plate that now comes before it.
	element.style.position = 'relative';
	strike(plate, album.id, '', () => {
		element.style.removeProperty('position');
	});
}

/** Hits a card that has just met the pile with a plate of its own shape, behind the cover. */
export function printCard(slot: HTMLElement, card: AlbumEntry): void {
	if (reducedMotion()) {
		return;
	}
	const plate = document.createElement('span');
	plate.style.cssText = `position:absolute;inset:0;border-radius:var(--radius-20);background:${paletteFor(card).accent}`;
	slot.prepend(plate);
	strike(plate, card.id, restOf(card.id));
}
