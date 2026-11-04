import type { MorphSlot } from './registry.ts';

/** The part of a registry entry that resolution needs: generic so tests need no DOM. */
export interface Candidate {
	id: string;
	slot: MorphSlot;
}

/**
 * Where each id's source sat among its siblings in its slot, kept for the way back. `null`
 * means that id had no source there: a cover with no title beside it, say.
 */
export type Origins = Map<string, { slot: MorphSlot; ordinal: number | null }>;

export interface PickOptions<C extends Candidate> {
	/** The ids this navigation shares. */
	ids: readonly string[];
	/** The slots to try, best first. */
	slots: readonly MorphSlot[];
	/** Reports whether a candidate is inside what was clicked. The clicked element wins. */
	clicked?: (candidate: C) => boolean;
	/** Where the journey started, when this is the way back. */
	origins?: Origins;
	/** Reports whether a candidate is on screen. */
	visible?: (candidate: C) => boolean;
	/** Rules candidates out altogether: a hero scrolled off screen can't travel. */
	eligible?: (candidate: C) => boolean;
}

function pickInSlot<C extends Candidate>(
	matches: C[],
	id: string,
	slot: MorphSlot,
	{ clicked, origins, visible }: PickOptions<C>
): C | undefined {
	const hit = clicked === undefined ? undefined : matches.find(clicked);
	if (hit !== undefined) {
		return hit;
	}
	const origin = origins?.get(id);
	if (origin?.slot === slot) {
		return origin.ordinal === null ? undefined : (matches.at(origin.ordinal) ?? matches.at(0));
	}
	return (visible === undefined ? undefined : matches.find(visible)) ?? matches.at(0);
}

/**
 * Resolves exactly one candidate per id: one album can appear several times on a page, and a
 * morph needs one source and one target. Ids with no candidate are absent from the result.
 */
export function pick<C extends Candidate>(
	candidates: C[],
	options: PickOptions<C>
): Map<string, C> {
	const picked = new Map<string, C>();
	for (const id of options.ids) {
		for (const slot of options.slots) {
			const matches = candidates.filter(
				(c) => c.id === id && c.slot === slot && (options.eligible?.(c) ?? true)
			);
			if (matches.length === 0) {
				continue;
			}
			const choice = pickInSlot(matches, id, slot, options);
			if (choice !== undefined) {
				picked.set(id, choice);
			}
			break;
		}
	}
	return picked;
}

/**
 * Records where each picked source sat, so the way back can land on the same element. An id with
 * no source is put down against `usual`, the slot it would have left from.
 */
export function recordOrigins<C extends Candidate>(
	candidates: C[],
	ids: readonly string[],
	usual: MorphSlot,
	sources: Map<string, C>,
	origins: Origins
): void {
	for (const id of ids) {
		const source = sources.get(id);
		const slot = source?.slot ?? usual;
		const siblings = candidates.filter((c) => c.id === id && c.slot === slot);
		const ordinal = source === undefined ? -1 : siblings.indexOf(source);
		origins.set(id, { slot, ordinal: ordinal < 0 ? null : ordinal });
	}
}
