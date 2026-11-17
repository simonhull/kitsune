/**
 * The way to Duplicates (plan 069) from Organize: a conflict card's "Compare in Duplicates ›" opens
 * the group the server says its songs are in, or the list when it names none.
 */

import { goto } from '$app/navigation';
import { resolve } from '$app/paths';

/** Opens Duplicates, on the group with this key when given. */
export function openDuplicates(group?: string): void {
	void goto(
		group === undefined
			? resolve('/server/duplicates')
			: resolve('/server/duplicates/[group]', { group })
	);
}
