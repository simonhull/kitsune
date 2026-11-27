import type { ImageCandidate } from './types.ts';

/**
 * A list read again in the order the person already sees: a pin moves the ring, never the tiles
 * under their pointer. Ones new to the list go after, in the server's order.
 */
export function keepOrder(
	shown: readonly ImageCandidate[],
	next: readonly ImageCandidate[]
): ImageCandidate[] {
	const at = new Map(shown.map((c, index) => [c.id, index]));
	const place = (c: ImageCandidate): number => at.get(c.id) ?? shown.length;
	return next
		.map((c, index) => ({ c, index }))
		.toSorted((a, b) => place(a.c) - place(b.c) || a.index - b.index)
		.map(({ c }) => c);
}
