import { columnsFrom } from './columns.ts';

/** Where Add to playlist opens from a menu's item: beside it, or on a phone as a sheet. */
export interface PickerPlace {
	x: number;
	y: number;
	mode: 'submenu' | 'sheet';
}

/** How wide the submenu is. */
export const pickerWidth = 300;

/**
 * Beside the item, past its menu's edge, on the right when it fits and else on the left. A phone's
 * menu has no room beside it: there the list is a sheet.
 */
export function pickerBeside(
	item: Pick<DOMRect, 'left' | 'right' | 'top'>,
	screenWidth: number
): PickerPlace {
	if (screenWidth < columnsFrom) {
		return { x: 0, y: 0, mode: 'sheet' };
	}
	const right = item.right + 16;
	return {
		x: right + pickerWidth + 12 <= screenWidth ? right : item.left - 16 - pickerWidth,
		y: item.top - 8,
		mode: 'submenu'
	};
}
