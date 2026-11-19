import type { Rect } from '#lib/morph/bud.ts';
import { columnsFrom } from './columns.ts';

/**
 * Where Add to playlist opens from a menu's item: beside it, born at its chevron, or on a phone in
 * the menu itself, which reshapes into the list.
 */
export type PickerPlace = { mode: 'submenu'; x: number; y: number; from: Rect } | { mode: 'phone' };

/** How wide the submenu is. */
export const pickerWidth = 300;

/** The chevron's width: the part of the item a submenu opens past, and is born at. */
const chevron = 32;

/**
 * Beside the item, past its menu's edge, on the right when it fits and else on the left. A phone's
 * menu has no room beside it: there the menu becomes the list. A submenu is born at the item's
 * chevron, so it grows out beside its menu, never over it.
 */
export function pickerBeside(
	item: Pick<DOMRect, 'left' | 'right' | 'top' | 'height'>,
	screenWidth: number
): PickerPlace {
	if (screenWidth < columnsFrom) {
		return { mode: 'phone' };
	}
	const right = item.right + 16;
	const fits = right + pickerWidth + 12 <= screenWidth;
	return {
		mode: 'submenu',
		x: fits ? right : item.left - 16 - pickerWidth,
		y: item.top - 8,
		from: {
			left: fits ? item.right - chevron : item.left,
			top: item.top,
			width: chevron,
			height: item.height,
			radius: 12
		}
	};
}
